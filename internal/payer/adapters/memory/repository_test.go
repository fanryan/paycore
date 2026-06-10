package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/payer"
)

var _ payer.PayerRepository = (*Store)(nil)

func TestStoreCreatesAndGetsPayer(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	payerRecord := testPayer(t, "payer-1")

	created, err := store.CreatePayer(ctx, payerRecord)
	if err != nil {
		t.Fatalf("expected payer create to succeed, got error: %v", err)
	}

	got, err := store.GetPayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected payer get to succeed, got error: %v", err)
	}

	if got.ID != payerRecord.ID {
		t.Fatalf("expected payer id %q, got %q", payerRecord.ID, got.ID)
	}
}

func TestStoreRejectsDuplicatePayer(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	payerRecord := testPayer(t, "payer-1")

	if _, err := store.CreatePayer(ctx, payerRecord); err != nil {
		t.Fatalf("expected first payer create to succeed, got error: %v", err)
	}

	_, err := store.CreatePayer(ctx, payerRecord)
	if !errors.Is(err, payer.ErrDuplicatePayer) {
		t.Fatalf("expected ErrDuplicatePayer, got %v", err)
	}
}

func TestStoreReturnsPayerNotFound(t *testing.T) {
	_, err := NewStore().GetPayer(context.Background(), "missing")

	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound, got %v", err)
	}
}

func TestStoreListsPayers(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.CreatePayer(ctx, testPayer(t, "payer-1"))
	if err != nil {
		t.Fatalf("expected first payer create to succeed, got error: %v", err)
	}

	_, err = store.CreatePayer(ctx, testPayer(t, "payer-2"))
	if err != nil {
		t.Fatalf("expected second payer create to succeed, got error: %v", err)
	}

	payers, err := store.ListPayers(ctx)
	if err != nil {
		t.Fatalf("expected list payers to succeed, got error: %v", err)
	}

	if len(payers) != 2 {
		t.Fatalf("expected 2 payers, got %d", len(payers))
	}
}

func TestStoreReturnsContextErrorForPayerRepository(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewStore()

	_, err := store.CreatePayer(ctx, testPayer(t, "payer-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from create payer, got %v", err)
	}

	_, err = store.GetPayer(ctx, "payer-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from get payer, got %v", err)
	}

	_, err = store.ListPayers(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from list payers, got %v", err)
	}
}

func testPayer(t *testing.T, id string) payer.Payer {
	t.Helper()

	payerRecord, err := payer.NewPayer(id, 10_000, "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create test payer: %v", err)
	}

	return payerRecord
}

func testNow() time.Time {
	return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
}
