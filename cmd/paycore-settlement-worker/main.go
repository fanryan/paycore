package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"time"

	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	paymentpostgres "github.com/fanryan/paycore/internal/payment/adapters/postgres"
	"github.com/fanryan/paycore/internal/settlement"
	settlementpostgres "github.com/fanryan/paycore/internal/settlement/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultWorkerID      = "paycore-settlement-worker"
	defaultWindowMinutes = 60
	defaultClaimLimit    = 100
	defaultLockMinutes   = 5
)

type workerConfig struct {
	databaseURL   string
	workerID      string
	windowMinutes int
	claimLimit    int
	lockTTL       time.Duration
}

type settlementWindow struct {
	start time.Time
	end   time.Time
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := loadWorkerConfig(config.Load())

	if cfg.databaseURL == "" {
		logger.Error("PAYCORE_DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		logger.Error("failed to create postgres pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping postgres", "error", err)
		os.Exit(1)
	}

	service, err := settlement.NewService(settlement.ServiceConfig{
		Repository: settlementpostgres.NewStore(pool),
		Payments:   paymentpostgres.NewStore(pool),
		Outbox:     outboxpostgres.NewStore(pool),
		Transactor: db.NewPostgresTransactor(pool),
		WorkerID:   cfg.workerID,
		ClaimLimit: cfg.claimLimit,
		LockTTL:    cfg.lockTTL,
	})
	if err != nil {
		logger.Error("failed to create settlement service", "error", err)
		os.Exit(1)
	}

	window, err := previousFullWindow(time.Now().UTC(), cfg.windowMinutes)
	if err != nil {
		logger.Error("failed to calculate settlement window", "error", err)
		os.Exit(1)
	}

	logger.Info(
		"paycore settlement worker starting",
		"worker_id", cfg.workerID,
		"window_start", window.start.Format(time.RFC3339),
		"window_end", window.end.Format(time.RFC3339),
		"claim_limit", cfg.claimLimit,
	)

	result, err := service.CreateBatch(ctx, settlement.CreateBatchInput{
		WindowStart: window.start,
		WindowEnd:   window.end,
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("settlement batch failed", "error", err)
		os.Exit(1)
	}
	if err != nil {
		logger.Info("paycore settlement worker stopped")
		return
	}

	logger.Info(
		"settlement batch completed",
		"batch_id", result.Batch.ID,
		"line_items", len(result.LineItems),
		"settled_payments", len(result.Payments),
		"window_start", result.Batch.WindowStart.Format(time.RFC3339),
		"window_end", result.Batch.WindowEnd.Format(time.RFC3339),
	)
}

func loadWorkerConfig(base config.Config) workerConfig {
	return workerConfig{
		databaseURL:   base.DatabaseURL,
		workerID:      getenv("PAYCORE_SETTLEMENT_WORKER_ID", defaultWorkerID),
		windowMinutes: intenv("PAYCORE_SETTLEMENT_WINDOW_MINUTES", defaultWindowMinutes),
		claimLimit:    intenv("PAYCORE_SETTLEMENT_CLAIM_LIMIT", defaultClaimLimit),
		lockTTL:       time.Duration(intenv("PAYCORE_SETTLEMENT_LOCK_MINUTES", defaultLockMinutes)) * time.Minute,
	}
}

func previousFullWindow(now time.Time, windowMinutes int) (settlementWindow, error) {
	if windowMinutes <= 0 {
		return settlementWindow{}, fmt.Errorf("settlement window minutes must be positive")
	}

	window := time.Duration(windowMinutes) * time.Minute
	end := now.UTC().Truncate(window)
	start := end.Add(-window)

	return settlementWindow{
		start: start,
		end:   end,
	}, nil
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func intenv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}
