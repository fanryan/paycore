package main

import (
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/shared/config"
)

func TestPreviousFullWindowUsesPreviousCompletedWindow(t *testing.T) {
	now := time.Date(2026, 6, 19, 11, 37, 45, 0, time.UTC)

	window, err := previousFullWindow(now, 60)
	if err != nil {
		t.Fatalf("expected window, got error: %v", err)
	}

	expectedStart := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 6, 19, 11, 0, 0, 0, time.UTC)

	if !window.start.Equal(expectedStart) {
		t.Fatalf("expected start %s, got %s", expectedStart, window.start)
	}

	if !window.end.Equal(expectedEnd) {
		t.Fatalf("expected end %s, got %s", expectedEnd, window.end)
	}
}

func TestPreviousFullWindowSupportsShorterWindows(t *testing.T) {
	now := time.Date(2026, 6, 19, 11, 37, 45, 0, time.UTC)

	window, err := previousFullWindow(now, 15)
	if err != nil {
		t.Fatalf("expected window, got error: %v", err)
	}

	expectedStart := time.Date(2026, 6, 19, 11, 15, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 6, 19, 11, 30, 0, 0, time.UTC)

	if !window.start.Equal(expectedStart) {
		t.Fatalf("expected start %s, got %s", expectedStart, window.start)
	}

	if !window.end.Equal(expectedEnd) {
		t.Fatalf("expected end %s, got %s", expectedEnd, window.end)
	}
}

func TestPreviousFullWindowRejectsInvalidWindow(t *testing.T) {
	_, err := previousFullWindow(time.Now().UTC(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadWorkerConfigUsesDefaults(t *testing.T) {
	t.Setenv("PAYCORE_SETTLEMENT_WORKER_ID", "")
	t.Setenv("PAYCORE_SETTLEMENT_WINDOW_MINUTES", "")
	t.Setenv("PAYCORE_SETTLEMENT_CLAIM_LIMIT", "")
	t.Setenv("PAYCORE_SETTLEMENT_LOCK_MINUTES", "")

	cfg := loadWorkerConfig(config.Config{
		DatabaseURL: "postgres://example",
	})

	if cfg.databaseURL != "postgres://example" {
		t.Fatalf("expected database url postgres://example, got %q", cfg.databaseURL)
	}

	if cfg.workerID != defaultWorkerID {
		t.Fatalf("expected worker id %q, got %q", defaultWorkerID, cfg.workerID)
	}

	if cfg.windowMinutes != defaultWindowMinutes {
		t.Fatalf("expected window minutes %d, got %d", defaultWindowMinutes, cfg.windowMinutes)
	}

	if cfg.claimLimit != defaultClaimLimit {
		t.Fatalf("expected claim limit %d, got %d", defaultClaimLimit, cfg.claimLimit)
	}

	if cfg.lockTTL != defaultLockMinutes*time.Minute {
		t.Fatalf("expected lock ttl %s, got %s", defaultLockMinutes*time.Minute, cfg.lockTTL)
	}
}

func TestLoadWorkerConfigUsesOverrides(t *testing.T) {
	t.Setenv("PAYCORE_SETTLEMENT_WORKER_ID", "worker-test")
	t.Setenv("PAYCORE_SETTLEMENT_WINDOW_MINUTES", "30")
	t.Setenv("PAYCORE_SETTLEMENT_CLAIM_LIMIT", "25")
	t.Setenv("PAYCORE_SETTLEMENT_LOCK_MINUTES", "2")

	cfg := loadWorkerConfig(config.Config{
		DatabaseURL: "postgres://example",
	})

	if cfg.workerID != "worker-test" {
		t.Fatalf("expected worker id worker-test, got %q", cfg.workerID)
	}

	if cfg.windowMinutes != 30 {
		t.Fatalf("expected window minutes 30, got %d", cfg.windowMinutes)
	}

	if cfg.claimLimit != 25 {
		t.Fatalf("expected claim limit 25, got %d", cfg.claimLimit)
	}

	if cfg.lockTTL != 2*time.Minute {
		t.Fatalf("expected lock ttl 2m, got %s", cfg.lockTTL)
	}
}
