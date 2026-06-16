package postgres_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	idempotencypostgres "github.com/fanryan/paycore/internal/idempotency/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesGetsAndUpdatesRecord(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := idempotencypostgres.NewStore(pool)
	key := testKey()
	t.Cleanup(func() {
		cleanupRecords(t, pool, key)
	})

	record := testRecord(t, key)

	created, err := store.CreateRecord(ctx, record)
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	if created.Status != idempotency.StatusInProgress {
		t.Fatalf("expected status IN_PROGRESS, got %q", created.Status)
	}

	completed, err := created.Complete(201, []byte(`{"ok":true}`), testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	updated, err := store.UpdateRecord(ctx, completed)
	if err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}

	if updated.Status != idempotency.StatusCompleted {
		t.Fatalf("expected status COMPLETED, got %q", updated.Status)
	}

	got, err := store.GetRecord(ctx, key)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if string(got.ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected response body, got %s", got.ResponseBody)
	}
}

func TestRepositoryRejectsDuplicateKey(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := idempotencypostgres.NewStore(pool)
	key := testKey()
	t.Cleanup(func() {
		cleanupRecords(t, pool, key)
	})

	record := testRecord(t, key)

	if _, err := store.CreateRecord(ctx, record); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err := store.CreateRecord(ctx, record)
	if !errors.Is(err, idempotency.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", err)
	}
}

func TestRepositoryReturnsNotFoundForMissingRecord(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := idempotencypostgres.NewStore(pool)

	_, err := store.GetRecord(ctx, "missing-key")
	if !errors.Is(err, idempotency.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}

	_, err = store.UpdateRecord(ctx, testRecord(t, "missing-key"))
	if !errors.Is(err, idempotency.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound on update, got %v", err)
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping postgres: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func cleanupRecords(t *testing.T, pool *pgxpool.Pool, key string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DELETE FROM idempotency_records WHERE key = $1", key); err != nil {
		t.Fatalf("failed to cleanup idempotency records: %v", err)
	}
}

func testRecord(t *testing.T, key string) idempotency.Record {
	t.Helper()

	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         key,
		RequestHash: "hash-" + key,
		Now:         testNow(),
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create idempotency record: %v", err)
	}

	return record
}

func testKey() string {
	return "test-key-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
