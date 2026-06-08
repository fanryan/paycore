package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fanryan/paycore/internal/httpapi"
)

const (
	serviceName = "paycore-api"
	version     = "0.1.0"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := getenv("PAYCORE_HTTP_ADDR", ":8080")

	server := &http.Server{
		Addr: addr,
		Handler: httpapi.NewRouter(httpapi.RouterConfig{
			ServiceName: serviceName,
			Version:     version,
			StartedAt:   time.Now().UTC(),
			Logger:      logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("paycore api starting", "addr", addr)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("paycore api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("paycore api shutting down")

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("paycore api shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("paycore api stopped")
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
