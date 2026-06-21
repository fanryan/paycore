package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	httpapi "github.com/fanryan/paycore/internal/http"
	"github.com/fanryan/paycore/internal/idempotency"
	idempotencymemory "github.com/fanryan/paycore/internal/idempotency/adapters/memory"
	idempotencypostgres "github.com/fanryan/paycore/internal/idempotency/adapters/postgres"
	idempotencyredis "github.com/fanryan/paycore/internal/idempotency/adapters/redis"
	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	merchantpostgres "github.com/fanryan/paycore/internal/merchant/adapters/postgres"
	"github.com/fanryan/paycore/internal/outbox"
	outboxmemory "github.com/fanryan/paycore/internal/outbox/adapters/memory"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	payerpostgres "github.com/fanryan/paycore/internal/payer/adapters/postgres"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
	paymentpostgres "github.com/fanryan/paycore/internal/payment/adapters/postgres"
	"github.com/fanryan/paycore/internal/ratelimit"
	ratelimitredis "github.com/fanryan/paycore/internal/ratelimit/adapters/redis"
	"github.com/fanryan/paycore/internal/shared/config"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/fanryan/paycore/internal/shared/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

const (
	serviceName = "paycore-api"
	version     = "0.1.0"
)

type repositories struct {
	merchants   merchant.MerchantRepository
	payers      payer.PayerRepository
	payments    payment.Repository
	idempotency idempotency.Repository
	outbox      outbox.Repository
	transactor  db.Transactor
	close       func()
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	repositories, err := newRepositories(context.Background(), cfg)
	if err != nil {
		logger.Error("failed to initialize repositories", "error", err)
		os.Exit(1)
	}
	defer repositories.close()

	rateLimiter, closeRateLimiter, err := newRateLimiter(context.Background(), cfg)
	if err != nil {
		logger.Error("failed to initialize rate limiter", "error", err)
		os.Exit(1)
	}
	defer closeRateLimiter()

	idempotencyCache, closeIdempotencyCache, err := newIdempotencyCache(context.Background(), cfg)
	if err != nil {
		logger.Error("failed to initialize idempotency cache", "error", err)
		os.Exit(1)
	}
	defer closeIdempotencyCache()

	appMetrics := metrics.New()
	merchantService := merchant.NewMerchantService(repositories.merchants)
	merchantHandler := merchant.NewHandler(merchantService)

	payerService := payer.NewPayerService(repositories.payers)
	payerHandler := payer.NewHandler(payerService)

	paymentService := payment.NewServiceWithTransactorAndOutbox(
		repositories.merchants,
		repositories.payers,
		repositories.payments,
		repositories.transactor,
		repositories.outbox,
	)
	idempotencyService := idempotency.NewServiceWithCacheAndMetrics(repositories.idempotency, idempotencyCache, appMetrics, 24*time.Hour)
	paymentHandler := payment.NewHandlerWithIdempotency(paymentService, idempotencyService)

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.RouterConfig{
			ServiceName:     serviceName,
			Version:         version,
			StartedAt:       time.Now().UTC(),
			Logger:          logger,
			MerchantHandler: merchantHandler,
			PayerHandler:    payerHandler,
			PaymentHandler:  paymentHandler,
			MetricsHandler:  appMetrics.Handler(),
			Metrics:         appMetrics,
			RateLimiter:     rateLimiter,
		}),
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info(
			"paycore api starting",
			"env", cfg.Env,
			"addr", cfg.HTTPAddr,
			"repository_backend", cfg.RepositoryBackend,
			"read_header_timeout", cfg.HTTPReadHeaderTimeout.String(),
			"shutdown_timeout", cfg.HTTPShutdownTimeout.String(),
			"rate_limit_enabled", cfg.RateLimitEnabled,
			"idempotency_cache_enabled", cfg.IdempotencyCacheEnabled,
		)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("paycore api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer cancel()

	logger.Info("paycore api shutting down")

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("paycore api shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("paycore api stopped")
}

func newRepositories(ctx context.Context, cfg config.Config) (repositories, error) {
	switch cfg.RepositoryBackend {
	case "memory":
		return newMemoryRepositories(), nil
	case "postgres":
		return newPostgresRepositories(ctx, cfg.DatabaseURL)
	default:
		return repositories{}, fmt.Errorf("unsupported repository backend %q", cfg.RepositoryBackend)
	}
}

func newMemoryRepositories() repositories {
	return repositories{
		merchants:   merchantmemory.NewStore(),
		payers:      payermemory.NewStore(),
		payments:    paymentmemory.NewStore(),
		idempotency: idempotencymemory.NewStore(),
		outbox:      outboxmemory.NewStore(),
		transactor:  db.NoopTransactor{},
		close:       func() {},
	}
}

func newPostgresRepositories(ctx context.Context, databaseURL string) (repositories, error) {
	if databaseURL == "" {
		return repositories{}, errors.New("PAYCORE_DATABASE_URL is required when PAYCORE_REPOSITORY_BACKEND=postgres")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return repositories{}, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return repositories{}, fmt.Errorf("ping postgres: %w", err)
	}

	return repositories{
		merchants:   merchantpostgres.NewStore(pool),
		payers:      payerpostgres.NewStore(pool),
		payments:    paymentpostgres.NewStore(pool),
		idempotency: idempotencypostgres.NewStore(pool),
		outbox:      outboxpostgres.NewStore(pool),
		transactor:  db.NewPostgresTransactor(pool),
		close:       pool.Close,
	}, nil
}

func newRateLimiter(ctx context.Context, cfg config.Config) (ratelimit.Limiter, func(), error) {
	if !cfg.RateLimitEnabled {
		return nil, func() {}, nil
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.RedisAddr,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("ping redis: %w", err)
	}

	limiter, err := ratelimitredis.NewLimiter(ratelimitredis.Config{
		Client: client,
		Limit:  cfg.RateLimitRequests,
		Window: cfg.RateLimitWindow,
	})
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	return limiter, func() {
		_ = client.Close()
	}, nil
}

func newIdempotencyCache(ctx context.Context, cfg config.Config) (idempotency.Cache, func(), error) {
	if !cfg.IdempotencyCacheEnabled {
		return idempotency.NoopCache{}, func() {}, nil
	}

	client := goredis.NewClient(&goredis.Options{
		Addr: cfg.RedisAddr,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("ping redis: %w", err)
	}

	cache, err := idempotencyredis.NewCache(idempotencyredis.Config{
		Client: client,
		TTL:    cfg.IdempotencyCacheTTL,
	})
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}

	return cache, func() {
		_ = client.Close()
	}, nil
}
