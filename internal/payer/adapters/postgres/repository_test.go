package postgres_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/payer"
	payerpostgres "github.com/fanryan/paycore/internal/payer/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesGetsListsAndUpdatesPayers(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := payerpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPayers(t, pool, prefix)
	})

	first := testPayer(t, prefix+"-payer-1", 10_000, testNow())
	second := testPayer(t, prefix+"-payer-2", 20_000, testNow().Add(time.Minute))

	createdFirst, err := store.CreatePayer(ctx, first)
	if err != nil {
		t.Fatalf("expected first payer create to succeed, got error: %v", err)
	}

	if createdFirst.AvailableBalanceMinor != 10_000 {
		t.Fatalf("expected available balance 10000, got %d", createdFirst.AvailableBalanceMinor)
	}

	if _, err := store.CreatePayer(ctx, second); err != nil {
		t.Fatalf("expected second payer create to succeed, got error: %v", err)
	}

	reserved, err := createdFirst.Reserve(4_000, "USD", testNow().Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	updated, err := store.UpdatePayer(ctx, reserved)
	if err != nil {
		t.Fatalf("expected payer update to succeed, got error: %v", err)
	}

	if updated.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance 6000, got %d", updated.AvailableBalanceMinor)
	}

	if updated.HeldBalanceMinor != 4_000 {
		t.Fatalf("expected held balance 4000, got %d", updated.HeldBalanceMinor)
	}

	got, err := store.GetPayer(ctx, first.ID)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if got.Version != 1 {
		t.Fatalf("expected version 1, got %d", got.Version)
	}

	payers, err := store.ListPayers(ctx)
	if err != nil {
		t.Fatalf("expected list to succeed, got error: %v", err)
	}

	if !containsPayer(payers, first.ID) {
		t.Fatalf("expected list to contain %q", first.ID)
	}

	if !containsPayer(payers, second.ID) {
		t.Fatalf("expected list to contain %q", second.ID)
	}
}

func TestRepositoryRejectsDuplicatePayer(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := payerpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPayers(t, pool, prefix)
	})

	payerRecord := testPayer(t, prefix+"-payer-1", 10_000, testNow())

	if _, err := store.CreatePayer(ctx, payerRecord); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err := store.CreatePayer(ctx, payerRecord)
	if !errors.Is(err, payer.ErrDuplicatePayer) {
		t.Fatalf("expected ErrDuplicatePayer, got %v", err)
	}
}

func TestRepositoryRejectsStalePayerVersion(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := payerpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPayers(t, pool, prefix)
	})

	created, err := store.CreatePayer(ctx, testPayer(t, prefix+"-payer-1", 10_000, testNow()))
	if err != nil {
		t.Fatalf("expected payer create to succeed, got error: %v", err)
	}

	firstUpdate, err := created.Reserve(4_000, "USD", testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected first reserve to succeed, got error: %v", err)
	}

	if _, err := store.UpdatePayer(ctx, firstUpdate); err != nil {
		t.Fatalf("expected first payer update to succeed, got error: %v", err)
	}

	staleUpdate, err := created.Reserve(3_000, "USD", testNow().Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expected stale reserve to succeed, got error: %v", err)
	}

	_, err = store.UpdatePayer(ctx, staleUpdate)
	if !errors.Is(err, payer.ErrPayerVersionConflict) {
		t.Fatalf("expected ErrPayerVersionConflict, got %v", err)
	}

	got, err := store.GetPayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected payer get to succeed, got error: %v", err)
	}

	if got.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance to remain 6000, got %d", got.AvailableBalanceMinor)
	}
}

func TestRepositoryReturnsNotFoundForMissingPayer(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := payerpostgres.NewStore(pool)

	_, err := store.GetPayer(ctx, "missing-payer")
	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound, got %v", err)
	}

	_, err = store.UpdatePayer(ctx, testPayer(t, "missing-payer", 10_000, testNow()))
	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound on update, got %v", err)
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

func cleanupPayers(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DELETE FROM payers WHERE id LIKE $1", prefix+"%"); err != nil {
		t.Fatalf("failed to cleanup payers: %v", err)
	}
}

func testPayer(t *testing.T, id string, availableBalanceMinor int64, now time.Time) payer.Payer {
	t.Helper()

	payerRecord, err := payer.NewPayer(id, availableBalanceMinor, "USD", now)
	if err != nil {
		t.Fatalf("failed to create payer: %v", err)
	}

	return payerRecord
}

func containsPayer(payers []payer.Payer, payerID string) bool {
	for _, payerRecord := range payers {
		if payerRecord.ID == payerID {
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
