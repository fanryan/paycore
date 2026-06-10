package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/fanryan/paycore/internal/shared/httpjson"
)

type RouterConfig struct {
	ServiceName     string
	Version         string
	StartedAt       time.Time
	Logger          *slog.Logger
	MerchantHandler http.Handler
	PayerHandler    http.Handler
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

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", readyHandler)
	mux.HandleFunc("GET /version", versionHandler(config))

	if config.MerchantHandler != nil {
		mux.Handle("/merchants", config.MerchantHandler)
	}

	if config.PayerHandler != nil {
		mux.Handle("/payers", config.PayerHandler)
	}

	mux.HandleFunc("/", notFoundHandler)

	return requestIDMiddleware(
		loggingMiddleware(config.Logger)(
			mux,
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
