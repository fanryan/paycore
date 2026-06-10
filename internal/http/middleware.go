package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}

	n, err := r.ResponseWriter.Write(body)
	r.bytes += n

	return n, err
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set("X-Request-ID", requestID)

		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()

			recorder := &responseRecorder{
				ResponseWriter: w,
			}

			next.ServeHTTP(recorder, r)

			statusCode := recorder.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}

			logger.Info(
				"http request completed",
				"request_id", requestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", statusCode,
				"bytes", recorder.bytes,
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

func requestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey).(string)
	if !ok || requestID == "" {
		return ""
	}

	return requestID
}

func newRequestID() string {
	var bytes [16]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}

	return hex.EncodeToString(bytes[:])
}
