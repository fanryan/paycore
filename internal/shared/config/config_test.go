package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("PAYCORE_ENV", "")
	t.Setenv("PAYCORE_HTTP_ADDR", "")
	t.Setenv("PAYCORE_METRICS_ADDR", "")
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "")
	t.Setenv("PAYCORE_DATABASE_URL", "")
	t.Setenv("PAYCORE_REDIS_ADDR", "")
	t.Setenv("PAYCORE_KAFKA_BROKERS", "")
	t.Setenv("PAYCORE_KAFKA_OUTBOX_TOPIC", "")
	t.Setenv("PAYCORE_OUTBOX_PUBLISHER", "")
	t.Setenv("PAYCORE_RATE_LIMIT_ENABLED", "")
	t.Setenv("PAYCORE_RATE_LIMIT_REQUESTS", "")
	t.Setenv("PAYCORE_RATE_LIMIT_WINDOW_SECONDS", "")
	t.Setenv("PAYCORE_IDEMPOTENCY_CACHE_ENABLED", "")
	t.Setenv("PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS", "")
	t.Setenv("PAYCORE_REPOSITORY_BACKEND", "")

	cfg := Load()

	if cfg.Env != "local" {
		t.Fatalf("expected env local, got %q", cfg.Env)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("expected addr :8080, got %q", cfg.HTTPAddr)
	}

	if cfg.MetricsAddr != ":9091" {
		t.Fatalf("expected metrics addr :9091, got %q", cfg.MetricsAddr)
	}

	if cfg.HTTPReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected read header timeout 5s, got %s", cfg.HTTPReadHeaderTimeout)
	}

	if cfg.HTTPShutdownTimeout != 10*time.Second {
		t.Fatalf("expected shutdown timeout 10s, got %s", cfg.HTTPShutdownTimeout)
	}

	if cfg.DatabaseURL != "" {
		t.Fatalf("expected empty database url, got %q", cfg.DatabaseURL)
	}

	if cfg.RedisAddr != "localhost:6379" {
		t.Fatalf("expected redis addr localhost:6379, got %q", cfg.RedisAddr)
	}

	if cfg.KafkaBrokers != "localhost:9092" {
		t.Fatalf("expected kafka brokers localhost:9092, got %q", cfg.KafkaBrokers)
	}

	if cfg.KafkaOutboxTopic != "paycore.outbox.events" {
		t.Fatalf("expected kafka outbox topic paycore.outbox.events, got %q", cfg.KafkaOutboxTopic)
	}

	if cfg.OutboxPublisher != "logging" {
		t.Fatalf("expected outbox publisher logging, got %q", cfg.OutboxPublisher)
	}

	if cfg.RateLimitEnabled {
		t.Fatal("expected rate limit disabled by default")
	}

	if cfg.RateLimitRequests != 60 {
		t.Fatalf("expected rate limit requests 60, got %d", cfg.RateLimitRequests)
	}

	if cfg.RateLimitWindow != time.Minute {
		t.Fatalf("expected rate limit window 1m, got %s", cfg.RateLimitWindow)
	}

	if cfg.IdempotencyCacheEnabled {
		t.Fatal("expected idempotency cache disabled by default")
	}

	if cfg.IdempotencyCacheTTL != 24*time.Hour {
		t.Fatalf("expected idempotency cache ttl 24h, got %s", cfg.IdempotencyCacheTTL)
	}

	if cfg.RepositoryBackend != "memory" {
		t.Fatalf("expected repository backend memory, got %q", cfg.RepositoryBackend)
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("PAYCORE_ENV", "test")
	t.Setenv("PAYCORE_HTTP_ADDR", ":9090")
	t.Setenv("PAYCORE_METRICS_ADDR", ":9191")
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "7")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "12")
	t.Setenv("PAYCORE_DATABASE_URL", "postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable")
	t.Setenv("PAYCORE_REDIS_ADDR", "redis:6379")
	t.Setenv("PAYCORE_KAFKA_BROKERS", "kafka:9092")
	t.Setenv("PAYCORE_KAFKA_OUTBOX_TOPIC", "paycore.payment.events")
	t.Setenv("PAYCORE_OUTBOX_PUBLISHER", "kafka")
	t.Setenv("PAYCORE_RATE_LIMIT_ENABLED", "true")
	t.Setenv("PAYCORE_RATE_LIMIT_REQUESTS", "42")
	t.Setenv("PAYCORE_RATE_LIMIT_WINDOW_SECONDS", "30")
	t.Setenv("PAYCORE_IDEMPOTENCY_CACHE_ENABLED", "true")
	t.Setenv("PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS", "3600")
	t.Setenv("PAYCORE_REPOSITORY_BACKEND", "postgres")

	cfg := Load()

	if cfg.Env != "test" {
		t.Fatalf("expected env test, got %q", cfg.Env)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("expected addr :9090, got %q", cfg.HTTPAddr)
	}

	if cfg.MetricsAddr != ":9191" {
		t.Fatalf("expected metrics addr :9191, got %q", cfg.MetricsAddr)
	}

	if cfg.HTTPReadHeaderTimeout != 7*time.Second {
		t.Fatalf("expected read header timeout 7s, got %s", cfg.HTTPReadHeaderTimeout)
	}

	if cfg.HTTPShutdownTimeout != 12*time.Second {
		t.Fatalf("expected shutdown timeout 12s, got %s", cfg.HTTPShutdownTimeout)
	}

	if cfg.DatabaseURL != "postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable" {
		t.Fatalf("expected database url override, got %q", cfg.DatabaseURL)
	}

	if cfg.RedisAddr != "redis:6379" {
		t.Fatalf("expected redis addr redis:6379, got %q", cfg.RedisAddr)
	}

	if cfg.KafkaBrokers != "kafka:9092" {
		t.Fatalf("expected kafka brokers kafka:9092, got %q", cfg.KafkaBrokers)
	}

	if cfg.KafkaOutboxTopic != "paycore.payment.events" {
		t.Fatalf("expected kafka outbox topic paycore.payment.events, got %q", cfg.KafkaOutboxTopic)
	}

	if cfg.OutboxPublisher != "kafka" {
		t.Fatalf("expected outbox publisher kafka, got %q", cfg.OutboxPublisher)
	}

	if !cfg.RateLimitEnabled {
		t.Fatal("expected rate limit enabled")
	}

	if cfg.RateLimitRequests != 42 {
		t.Fatalf("expected rate limit requests 42, got %d", cfg.RateLimitRequests)
	}

	if cfg.RateLimitWindow != 30*time.Second {
		t.Fatalf("expected rate limit window 30s, got %s", cfg.RateLimitWindow)
	}

	if !cfg.IdempotencyCacheEnabled {
		t.Fatal("expected idempotency cache enabled")
	}

	if cfg.IdempotencyCacheTTL != time.Hour {
		t.Fatalf("expected idempotency cache ttl 1h, got %s", cfg.IdempotencyCacheTTL)
	}

	if cfg.RepositoryBackend != "postgres" {
		t.Fatalf("expected repository backend postgres, got %q", cfg.RepositoryBackend)
	}
}

func TestLoadFallsBackForInvalidDurations(t *testing.T) {
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "not-a-number")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "-1")
	t.Setenv("PAYCORE_RATE_LIMIT_WINDOW_SECONDS", "0")
	t.Setenv("PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS", "-5")

	cfg := Load()

	if cfg.HTTPReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected read header timeout fallback 5s, got %s", cfg.HTTPReadHeaderTimeout)
	}

	if cfg.HTTPShutdownTimeout != 10*time.Second {
		t.Fatalf("expected shutdown timeout fallback 10s, got %s", cfg.HTTPShutdownTimeout)
	}

	if cfg.RateLimitWindow != time.Minute {
		t.Fatalf("expected rate limit window fallback 1m, got %s", cfg.RateLimitWindow)
	}

	if cfg.IdempotencyCacheTTL != 24*time.Hour {
		t.Fatalf("expected idempotency cache ttl fallback 24h, got %s", cfg.IdempotencyCacheTTL)
	}
}

func TestLoadFallsBackForInvalidBoolAndInt64Values(t *testing.T) {
	t.Setenv("PAYCORE_RATE_LIMIT_ENABLED", "not-a-bool")
	t.Setenv("PAYCORE_RATE_LIMIT_REQUESTS", "-1")

	cfg := Load()

	if cfg.RateLimitEnabled {
		t.Fatal("expected rate limit enabled fallback false")
	}

	if cfg.RateLimitRequests != 60 {
		t.Fatalf("expected rate limit requests fallback 60, got %d", cfg.RateLimitRequests)
	}
}
