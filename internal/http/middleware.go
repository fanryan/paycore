package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fanryan/paycore/internal/ratelimit"
	"github.com/fanryan/paycore/internal/shared/metrics"
	"github.com/go-chi/chi/v5"
)

type contextKey string

const (
	requestIDContextKey contextKey = "request_id"
	defaultMaxBodyBytes int64      = 1 << 20
)

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

func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error(
						"http request panic recovered",
						"request_id", requestIDFromContext(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"panic", recovered,
					)

					writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func bodyLimitMiddleware(maxBodyBytes int64) func(http.Handler) http.Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			next.ServeHTTP(w, r)
		})
	}
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

func metricsMiddleware(recorder *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			response := &responseRecorder{
				ResponseWriter: w,
			}

			next.ServeHTTP(response, r)

			statusCode := response.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}

			recorder.ObserveHTTPRequest(
				r.Method,
				routePattern(r),
				statusCode,
				time.Since(startedAt),
			)
		})
	}
}

type rateLimitMetricsRecorder interface {
	ObserveRateLimit(result string, duration time.Duration)
}

func rateLimitMiddleware(limiter ratelimit.Limiter, metricsRecorder rateLimitMetricsRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			result, err := limiter.Allow(r.Context(), rateLimitKey(r))
			if err != nil {
				if errors.Is(err, ratelimit.ErrLimiterUnavailable) {
					observeRateLimit(metricsRecorder, "redis_error", time.Since(startedAt))
					writeError(w, r, http.StatusServiceUnavailable, "RATE_LIMITER_UNAVAILABLE", "Rate limiter unavailable")
					return
				}

				observeRateLimit(metricsRecorder, "error", time.Since(startedAt))
				writeError(w, r, http.StatusInternalServerError, "RATE_LIMIT_ERROR", "Rate limit check failed")
				return
			}

			if result.Limit > 0 {
				w.Header().Set("RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
				w.Header().Set("RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
			}

			if !result.Allowed {
				if result.RetryAfter > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
				}

				observeRateLimit(metricsRecorder, "rejected", time.Since(startedAt))
				writeError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "Rate limit exceeded")
				return
			}

			observeRateLimit(metricsRecorder, "allowed", time.Since(startedAt))
			next.ServeHTTP(w, r)
		})
	}
}

func observeRateLimit(recorder rateLimitMetricsRecorder, result string, duration time.Duration) {
	if recorder == nil {
		return
	}

	recorder.ObserveRateLimit(result, duration)
}

func routePattern(r *http.Request) string {
	routeContext := chi.RouteContext(r.Context())
	if routeContext == nil {
		return "unknown"
	}

	pattern := routeContext.RoutePattern()
	if pattern == "" {
		return "unmatched"
	}

	return pattern
}

func requestIDFromContext(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey).(string)
	if !ok || requestID == "" {
		return ""
	}

	return requestID
}

func rateLimitKey(r *http.Request) string {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		candidate := strings.TrimSpace(parts[0])
		if candidate != "" {
			return candidate
		}
	}

	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}

	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if remoteAddr != "" {
		return remoteAddr
	}

	return "unknown"
}

func newRequestID() string {
	var bytes [16]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}

	return hex.EncodeToString(bytes[:])
}
