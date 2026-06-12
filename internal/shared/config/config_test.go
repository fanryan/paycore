package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("PAYCORE_ENV", "")
	t.Setenv("PAYCORE_HTTP_ADDR", "")
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "")
	t.Setenv("PAYCORE_DATABASE_URL", "")
	t.Setenv("PAYCORE_REDIS_ADDR", "")

	cfg := Load()

	if cfg.Env != "local" {
		t.Fatalf("expected env local, got %q", cfg.Env)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("expected addr :8080, got %q", cfg.HTTPAddr)
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
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("PAYCORE_ENV", "test")
	t.Setenv("PAYCORE_HTTP_ADDR", ":9090")
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "7")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "12")
	t.Setenv("PAYCORE_DATABASE_URL", "postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable")
	t.Setenv("PAYCORE_REDIS_ADDR", "redis:6379")

	cfg := Load()

	if cfg.Env != "test" {
		t.Fatalf("expected env test, got %q", cfg.Env)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("expected addr :9090, got %q", cfg.HTTPAddr)
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
}

func TestLoadFallsBackForInvalidDurations(t *testing.T) {
	t.Setenv("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", "not-a-number")
	t.Setenv("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", "-1")

	cfg := Load()

	if cfg.HTTPReadHeaderTimeout != 5*time.Second {
		t.Fatalf("expected read header timeout fallback 5s, got %s", cfg.HTTPReadHeaderTimeout)
	}

	if cfg.HTTPShutdownTimeout != 10*time.Second {
		t.Fatalf("expected shutdown timeout fallback 10s, got %s", cfg.HTTPShutdownTimeout)
	}
}
