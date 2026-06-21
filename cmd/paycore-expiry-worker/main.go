package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	merchantpostgres "github.com/fanryan/paycore/internal/merchant/adapters/postgres"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	payerpostgres "github.com/fanryan/paycore/internal/payer/adapters/postgres"
	"github.com/fanryan/paycore/internal/payment"
	paymentpostgres "github.com/fanryan/paycore/internal/payment/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultExpiryLimit = 100

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	limit := intenv("PAYCORE_EXPIRY_LIMIT", defaultExpiryLimit)

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

	service := payment.NewServiceWithTransactorAndOutbox(
		merchantpostgres.NewStore(pool),
		payerpostgres.NewStore(pool),
		paymentpostgres.NewStore(pool),
		db.NewPostgresTransactor(pool),
		outboxpostgres.NewStore(pool),
	)

	logger.Info("paycore expiry worker starting", "limit", limit)

	result, err := service.ExpireAuthorizedPayments(ctx, payment.ExpireAuthorizedPaymentsInput{
		Limit: limit,
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("payment expiry failed", "error", err)
		os.Exit(1)
	}
	if err != nil {
		logger.Info("paycore expiry worker stopped")
		return
	}

	logger.Info(
		"payment expiry completed",
		"expired_payments", len(result.Payments),
		"released_holds", len(result.Holds),
		"updated_payers", len(result.Payers),
	)
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
