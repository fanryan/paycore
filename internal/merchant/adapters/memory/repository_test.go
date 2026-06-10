package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
)

var _ merchant.MerchantRepository = (*Store)(nil)

func TestStoreCreatesAndGetsMerchant(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	merchantRecord := testMerchant(t, "merchant-1")

	created, err := store.CreateMerchant(ctx, merchantRecord)
	if err != nil {
		t.Fatalf("expected merchant create to succeed, got error: %v", err)
	}

	got, err := store.GetMerchant(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected merchant get to succeed, got error: %v", err)
	}

	if got.ID != merchantRecord.ID {
		t.Fatalf("expected merchant id %q, got %q", merchantRecord.ID, got.ID)
	}
}

func TestStoreRejectsDuplicateMerchant(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	merchantRecord := testMerchant(t, "merchant-1")

	if _, err := store.CreateMerchant(ctx, merchantRecord); err != nil {
		t.Fatalf("expected first merchant create to succeed, got error: %v", err)
	}

	_, err := store.CreateMerchant(ctx, merchantRecord)
	if !errors.Is(err, merchant.ErrDuplicateMerchant) {
		t.Fatalf("expected ErrDuplicateMerchant, got %v", err)
	}
}

func TestStoreReturnsMerchantNotFound(t *testing.T) {
	_, err := NewStore().GetMerchant(context.Background(), "missing")

	if !errors.Is(err, merchant.ErrMerchantNotFound) {
		t.Fatalf("expected ErrMerchantNotFound, got %v", err)
	}
}

func TestStoreListsMerchants(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.CreateMerchant(ctx, testMerchant(t, "merchant-1"))
	if err != nil {
		t.Fatalf("expected first merchant create to succeed, got error: %v", err)
	}

	_, err = store.CreateMerchant(ctx, testMerchant(t, "merchant-2"))
	if err != nil {
		t.Fatalf("expected second merchant create to succeed, got error: %v", err)
	}

	merchants, err := store.ListMerchants(ctx)
	if err != nil {
		t.Fatalf("expected list merchants to succeed, got error: %v", err)
	}

	if len(merchants) != 2 {
		t.Fatalf("expected 2 merchants, got %d", len(merchants))
	}
}

func TestStoreReturnsContextErrorForMerchantRepository(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewStore()

	_, err := store.CreateMerchant(ctx, testMerchant(t, "merchant-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from create merchant, got %v", err)
	}

	_, err = store.GetMerchant(ctx, "merchant-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from get merchant, got %v", err)
	}

	_, err = store.ListMerchants(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from list merchants, got %v", err)
	}
}

func testMerchant(t *testing.T, id string) merchant.Merchant {
	t.Helper()

	merchantRecord, err := merchant.NewMerchant(id, "Demo Merchant", "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create test merchant: %v", err)
	}

	return merchantRecord
}

func testNow() time.Time {
	return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
}
