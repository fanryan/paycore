package db_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTransactorCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupTransactorRows(t, pool, prefix)
	})

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		tx, ok := db.TxFromContext(ctx)
		if !ok {
			t.Fatal("expected transaction in context")
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO transactor_test_records (id, value)
			VALUES ($1, $2)
		`, prefix+"-record-1", "committed")

		return err
	})
	if err != nil {
		t.Fatalf("expected transaction to commit, got error: %v", err)
	}

	value := getTransactorRecordValue(t, pool, prefix+"-record-1")
	if value != "committed" {
		t.Fatalf("expected committed value, got %q", value)
	}
}

func TestPostgresTransactorRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupTransactorRows(t, pool, prefix)
	})

	expectedErr := errors.New("force rollback")

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		tx, ok := db.TxFromContext(ctx)
		if !ok {
			t.Fatal("expected transaction in context")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO transactor_test_records (id, value)
			VALUES ($1, $2)
		`, prefix+"-record-1", "rolled-back"); err != nil {
			return err
		}

		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if transactorRecordExists(t, pool, prefix+"-record-1") {
		t.Fatal("expected record to be rolled back")
	}
}

func TestPostgresTransactorReusesExistingTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupTransactorRows(t, pool, prefix)
	})

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		outerTx, ok := db.TxFromContext(ctx)
		if !ok {
			t.Fatal("expected outer transaction in context")
		}

		return transactor.WithinTx(ctx, func(ctx context.Context) error {
			innerTx, ok := db.TxFromContext(ctx)
			if !ok {
				t.Fatal("expected inner transaction in context")
			}

			if innerTx != outerTx {
				t.Fatal("expected nested WithinTx call to reuse existing transaction")
			}

			_, err := innerTx.Exec(ctx, `
				INSERT INTO transactor_test_records (id, value)
				VALUES ($1, $2)
			`, prefix+"-record-1", "nested")

			return err
		})
	})
	if err != nil {
		t.Fatalf("expected nested transaction to commit, got error: %v", err)
	}

	value := getTransactorRecordValue(t, pool, prefix+"-record-1")
	if value != "nested" {
		t.Fatalf("expected nested value, got %q", value)
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping postgres: %v", err)
	}

	ensureTransactorTestTable(t, pool)
	t.Cleanup(pool.Close)

	return pool
}

func ensureTransactorTestTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS transactor_test_records (
			id TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to ensure transactor test table: %v", err)
	}
}

func cleanupTransactorRows(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DELETE FROM transactor_test_records WHERE id LIKE $1", prefix+"%"); err != nil {
		t.Fatalf("failed to cleanup transactor test rows: %v", err)
	}
}

func getTransactorRecordValue(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var value string
	if err := pool.QueryRow(ctx, "SELECT value FROM transactor_test_records WHERE id = $1", id).Scan(&value); err != nil {
		t.Fatalf("failed to get transactor test record: %v", err)
	}

	return value
}

func transactorRecordExists(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM transactor_test_records WHERE id = $1)", id).Scan(&exists); err != nil {
		t.Fatalf("failed to check transactor test record existence: %v", err)
	}

	return exists
}

func testPrefix() string {
	return "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}
