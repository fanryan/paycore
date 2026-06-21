package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/fanryan/paycore/internal/ratelimit"
	"github.com/fanryan/paycore/internal/shared/httpjson"
	"github.com/fanryan/paycore/internal/shared/metrics"
	"github.com/go-chi/chi/v5"
)

type RouterConfig struct {
	ServiceName     string
	Version         string
	StartedAt       time.Time
	Logger          *slog.Logger
	MerchantHandler http.Handler
	PayerHandler    http.Handler
	PaymentHandler  http.Handler
	MetricsHandler  http.Handler
	Metrics         *metrics.Metrics
	RateLimiter     ratelimit.Limiter
}

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

func NewRouter(config RouterConfig) http.Handler {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	router := chi.NewRouter()
	if config.Metrics != nil {
		router.Use(metricsMiddleware(config.Metrics))
	}

	router.Get("/healthz", healthHandler)
	router.Get("/readyz", readyHandler)
	router.Get("/version", versionHandler(config))

	if config.MetricsHandler != nil {
		router.Handle("/metrics", config.MetricsHandler)
	}

	if config.MerchantHandler != nil {
		router.Handle("/merchants", config.MerchantHandler)
	}

	if config.PayerHandler != nil {
		router.Handle("/payers", config.PayerHandler)
	}

	if config.PaymentHandler != nil {
		router.Route("/payments", func(r chi.Router) {
			if config.RateLimiter != nil {
				var rateLimitMetrics rateLimitMetricsRecorder
				if config.Metrics != nil {
					rateLimitMetrics = config.Metrics
				}

				r.Use(rateLimitMiddleware(config.RateLimiter, rateLimitMetrics))
			}

			r.Post("/authorize", config.PaymentHandler.ServeHTTP)
			r.Post("/{payment_id}/capture", config.PaymentHandler.ServeHTTP)
		})
	}

	router.NotFound(notFoundHandler)

	return requestIDMiddleware(
		recoveryMiddleware(config.Logger)(
			loggingMiddleware(config.Logger)(
				bodyLimitMiddleware(defaultMaxBodyBytes)(
					router,
				),
			),
		),
	)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "ROUTE_NOT_FOUND", "Route not found")
}

func writeError(w http.ResponseWriter, r *http.Request, status int, errorCode string, message string) {
	httpjson.Write(w, status, ErrorResponse{
		ErrorCode: errorCode,
		Message:   message,
		RequestID: requestIDFromContext(r.Context()),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
