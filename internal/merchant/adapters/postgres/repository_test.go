package postgres_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	merchantpostgres "github.com/fanryan/paycore/internal/merchant/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesGetsAndListsMerchants(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := merchantpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupMerchants(t, pool, prefix)
	})

	first := testMerchant(t, prefix+"-merchant-1", "First Merchant", testNow())
	second := testMerchant(t, prefix+"-merchant-2", "Second Merchant", testNow().Add(time.Minute))

	createdFirst, err := store.CreateMerchant(ctx, first)
	if err != nil {
		t.Fatalf("expected first merchant create to succeed, got error: %v", err)
	}

	if createdFirst.ID != first.ID {
		t.Fatalf("expected created merchant id %q, got %q", first.ID, createdFirst.ID)
	}

	if createdFirst.Status != merchant.MerchantStatusActive {
		t.Fatalf("expected status ACTIVE, got %q", createdFirst.Status)
	}

	if _, err := store.CreateMerchant(ctx, second); err != nil {
		t.Fatalf("expected second merchant create to succeed, got error: %v", err)
	}

	got, err := store.GetMerchant(ctx, first.ID)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if got.Name != first.Name {
		t.Fatalf("expected name %q, got %q", first.Name, got.Name)
	}

	merchants, err := store.ListMerchants(ctx)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if !containsMerchant(merchants, first.ID) {
		t.Fatalf("expected list to contain %q", first.ID)
	}

	if !containsMerchant(merchants, second.ID) {
		t.Fatalf("expected list to contain %q", second.ID)
	}
}

func TestRepositoryRejectsDuplicateMerchant(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := merchantpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupMerchants(t, pool, prefix)
	})

	merchantRecord := testMerchant(t, prefix+"-merchant-1", "Demo Merchant", testNow())

	if _, err := store.CreateMerchant(ctx, merchantRecord); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err := store.CreateMerchant(ctx, merchantRecord)
	if !errors.Is(err, merchant.ErrDuplicateMerchant) {
		t.Fatalf("expected ErrDuplicateMerchant, got %v", err)
	}
}

func TestRepositoryReturnsNotFoundForMissingMerchant(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := merchantpostgres.NewStore(pool)

	_, err := store.GetMerchant(ctx, "missing-merchant")
	if !errors.Is(err, merchant.ErrMerchantNotFound) {
		t.Fatalf("expected ErrMerchantNotFound, got %v", err)
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

func cleanupMerchants(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DELETE FROM merchants WHERE id LIKE $1", prefix+"%"); err != nil {
		t.Fatalf("failed to cleanup merchants: %v", err)
	}
}

func testMerchant(t *testing.T, id string, name string, now time.Time) merchant.Merchant {
	t.Helper()

	merchantRecord, err := merchant.NewMerchant(id, name, "USD", now)
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	return merchantRecord
}

func containsMerchant(merchants []merchant.Merchant, merchantID string) bool {
	for _, merchantRecord := range merchants {
		if merchantRecord.ID == merchantID {
			return true
		}
	}

	return false
}

func testPrefix() string {
	return "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
