package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	workerID     = "paycore-outbox-worker"
	pollInterval = 2 * time.Second
	batchSize    = 100
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		logger.Error("PAYCORE_DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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

	worker, err := outbox.NewWorker(outbox.WorkerConfig{
		Repository: outboxpostgres.NewStore(pool),
		Publisher:  loggingPublisher{logger: logger},
		Transactor: db.NewPostgresTransactor(pool),
		WorkerID:   workerID,
		BatchSize:  batchSize,
	})
	if err != nil {
		logger.Error("failed to create outbox worker", "error", err)
		os.Exit(1)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	logger.Info(
		"paycore outbox worker starting",
		"env", cfg.Env,
		"worker_id", workerID,
		"batch_size", batchSize,
		"poll_interval", pollInterval.String(),
	)

	for {
		result, err := worker.ProcessBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("outbox batch failed", "error", err)
		}

		if result.Claimed > 0 || result.Published > 0 || result.Failed > 0 {
			logger.Info(
				"outbox batch processed",
				"claimed", result.Claimed,
				"published", result.Published,
				"failed", result.Failed,
			)
		}

		select {
		case <-ctx.Done():
			logger.Info("paycore outbox worker stopped")
			return
		case <-ticker.C:
		}
	}
}

type loggingPublisher struct {
	logger *slog.Logger
}

func (p loggingPublisher) Publish(ctx context.Context, event outbox.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	p.logger.Info(
		"outbox event published by logging publisher",
		"event_id", event.ID,
		"aggregate_type", event.AggregateType,
		"aggregate_id", event.AggregateID,
		"event_type", event.EventType,
		"attempts", event.Attempts,
	)

	return nil
}
