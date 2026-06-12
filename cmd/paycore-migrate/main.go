package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationsDir = "migrations"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		logger.Error("PAYCORE_DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to create postgres pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping postgres", "error", err)
		os.Exit(1)
	}

	if err := ensureSchemaMigrations(ctx, pool); err != nil {
		logger.Error("failed to ensure schema_migrations table", "error", err)
		os.Exit(1)
	}

	migrations, err := listMigrations(migrationsDir)
	if err != nil {
		logger.Error("failed to list migrations", "error", err)
		os.Exit(1)
	}

	for _, migration := range migrations {
		applied, err := migrationApplied(ctx, pool, migration.version)
		if err != nil {
			logger.Error("failed to check migration status", "migration", migration.version, "error", err)
			os.Exit(1)
		}

		if applied {
			logger.Info("migration already applied", "migration", migration.version)
			continue
		}

		if err := applyMigration(ctx, pool, migration); err != nil {
			logger.Error("failed to apply migration", "migration", migration.version, "error", err)
			os.Exit(1)
		}

		logger.Info("migration applied", "migration", migration.version)
	}

	logger.Info("migrations complete")
}

type migrationFile struct {
	version string
	path    string
	sql     string
}

func ensureSchemaMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func listMigrations(dir string) ([]migrationFile, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)

	migrations := make([]migrationFile, 0, len(paths))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, migrationFile{
			version: filepath.Base(path),
			path:    path,
			sql:     string(body),
		})
	}

	return migrations, nil
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool

	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM schema_migrations
			WHERE version = $1
		)
	`, version).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, migration migrationFile) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, migration.sql); err != nil {
		return fmt.Errorf("execute migration sql: %w", err)
	}

	commandTag, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (version)
		VALUES ($1)
	`, migration.version)
	if err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	if commandTag.RowsAffected() != 1 {
		return errors.New("schema_migrations insert affected unexpected row count")
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}
