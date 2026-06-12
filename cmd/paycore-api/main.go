package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	httpapi "github.com/fanryan/paycore/internal/http"
	"github.com/fanryan/paycore/internal/idempotency"
	idempotencymemory "github.com/fanryan/paycore/internal/idempotency/adapters/memory"
	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
	"github.com/fanryan/paycore/internal/shared/config"
)

const (
	serviceName = "paycore-api"
	version     = "0.1.0"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	merchantRepository := merchantmemory.NewStore()
	merchantService := merchant.NewMerchantService(merchantRepository)
	merchantHandler := merchant.NewHandler(merchantService)

	payerRepository := payermemory.NewStore()
	payerService := payer.NewPayerService(payerRepository)
	payerHandler := payer.NewHandler(payerService)

	paymentRepository := paymentmemory.NewStore()
	paymentService := payment.NewService(merchantRepository, payerRepository, paymentRepository)

	idempotencyRepository := idempotencymemory.NewStore()
	idempotencyService := idempotency.NewService(idempotencyRepository, 24*time.Hour)

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
			"read_header_timeout", cfg.HTTPReadHeaderTimeout.String(),
			"shutdown_timeout", cfg.HTTPShutdownTimeout.String(),
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
