package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type RouterConfig struct {
	ServiceName string
	Version     string
	StartedAt   time.Time
	Logger      *slog.Logger
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
	mux.HandleFunc("/", notFoundHandler)

	return requestIDMiddleware(
		loggingMiddleware(config.Logger)(
			mux,
		),
	)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func versionHandler(config RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service":    config.ServiceName,
			"version":    config.Version,
			"started_at": config.StartedAt.UTC().Format(time.RFC3339),
		})
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, "ROUTE_NOT_FOUND", "Route not found")
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, errorCode string, message string) {
	writeJSON(w, status, ErrorResponse{
		ErrorCode: errorCode,
		Message:   message,
		RequestID: requestIDFromContext(r.Context()),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
