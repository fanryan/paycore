package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/payment"
)

var _ payment.Repository = (*Store)(nil)

func TestStoreCreatesAndGetsPayment(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	paymentRecord := testPayment(t, "payment-1", "hold-1")

	created, err := store.CreatePayment(ctx, paymentRecord)
	if err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	got, err := store.GetPayment(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected payment get to succeed, got error: %v", err)
	}

	if got.ID != paymentRecord.ID {
		t.Fatalf("expected payment id %q, got %q", paymentRecord.ID, got.ID)
	}
}

func TestStoreRejectsDuplicatePayment(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	paymentRecord := testPayment(t, "payment-1", "hold-1")

	if _, err := store.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected first payment create to succeed, got error: %v", err)
	}

	_, err := store.CreatePayment(ctx, paymentRecord)
	if !errors.Is(err, payment.ErrDuplicatePayment) {
		t.Fatalf("expected ErrDuplicatePayment, got %v", err)
	}
}

func TestStoreUpdatesPayment(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	paymentRecord := testPayment(t, "payment-1", "hold-1")

	if _, err := store.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	captured, err := paymentRecord.Capture(testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected payment capture to succeed, got error: %v", err)
	}

	updated, err := store.UpdatePayment(ctx, captured)
	if err != nil {
		t.Fatalf("expected payment update to succeed, got error: %v", err)
	}

	if updated.Status != payment.StatusCaptured {
		t.Fatalf("expected status CAPTURED, got %q", updated.Status)
	}
}

func TestStoreReturnsPaymentNotFound(t *testing.T) {
	store := NewStore()

	_, err := store.GetPayment(context.Background(), "missing")
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}

	_, err = store.UpdatePayment(context.Background(), testPayment(t, "missing", "hold-1"))
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound from update, got %v", err)
	}
}

func TestStoreCreatesAndGetsHold(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	hold := testHold(t, "hold-1", "payment-1")

	created, err := store.CreateHold(ctx, hold)
	if err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	got, err := store.GetHold(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected hold get to succeed, got error: %v", err)
	}

	if got.ID != hold.ID {
		t.Fatalf("expected hold id %q, got %q", hold.ID, got.ID)
	}
}

func TestStoreGetsHoldByPaymentID(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	hold := testHold(t, "hold-1", "payment-1")

	if _, err := store.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	got, err := store.GetHoldByPaymentID(ctx, "payment-1")
	if err != nil {
		t.Fatalf("expected hold get by payment id to succeed, got error: %v", err)
	}

	if got.ID != hold.ID {
		t.Fatalf("expected hold id %q, got %q", hold.ID, got.ID)
	}
}

func TestStoreRejectsDuplicateHold(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	hold := testHold(t, "hold-1", "payment-1")

	if _, err := store.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected first hold create to succeed, got error: %v", err)
	}

	_, err := store.CreateHold(ctx, hold)
	if !errors.Is(err, payment.ErrDuplicateHold) {
		t.Fatalf("expected ErrDuplicateHold, got %v", err)
	}

	holdWithDuplicatePaymentID := testHold(t, "hold-2", "payment-1")
	_, err = store.CreateHold(ctx, holdWithDuplicatePaymentID)
	if !errors.Is(err, payment.ErrDuplicateHold) {
		t.Fatalf("expected ErrDuplicateHold for duplicate payment id, got %v", err)
	}
}

func TestStoreUpdatesHold(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	hold := testHold(t, "hold-1", "payment-1")

	if _, err := store.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	captured, err := hold.Capture(testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected hold capture to succeed, got error: %v", err)
	}

	updated, err := store.UpdateHold(ctx, captured)
	if err != nil {
		t.Fatalf("expected hold update to succeed, got error: %v", err)
	}

	if updated.Status != payment.HoldStatusCaptured {
		t.Fatalf("expected status CAPTURED, got %q", updated.Status)
	}
}

func TestStoreReturnsHoldNotFound(t *testing.T) {
	store := NewStore()

	_, err := store.GetHold(context.Background(), "missing")
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound, got %v", err)
	}

	_, err = store.GetHoldByPaymentID(context.Background(), "missing-payment")
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound by payment id, got %v", err)
	}

	_, err = store.UpdateHold(context.Background(), testHold(t, "missing", "payment-1"))
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound from update, got %v", err)
	}
}

func TestStoreReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := NewStore()

	_, err := store.CreatePayment(ctx, testPayment(t, "payment-1", "hold-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from create payment, got %v", err)
	}

	_, err = store.GetPayment(ctx, "payment-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from get payment, got %v", err)
	}

	_, err = store.UpdatePayment(ctx, testPayment(t, "payment-1", "hold-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from update payment, got %v", err)
	}

	_, err = store.CreateHold(ctx, testHold(t, "hold-1", "payment-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from create hold, got %v", err)
	}

	_, err = store.GetHold(ctx, "hold-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from get hold, got %v", err)
	}

	_, err = store.GetHoldByPaymentID(ctx, "payment-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from get hold by payment id, got %v", err)
	}

	_, err = store.UpdateHold(ctx, testHold(t, "hold-1", "payment-1"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled from update hold, got %v", err)
	}
}

func testPayment(t *testing.T, paymentID string, holdID string) payment.Payment {
	t.Helper()

	paymentRecord, err := payment.NewAuthorizedPayment(payment.NewAuthorizedPaymentInput{
		ID:                  paymentID,
		MerchantID:          "merchant-1",
		PayerID:             "payer-1",
		AmountMinor:         10_000,
		Currency:            "USD",
		AuthorizationHoldID: holdID,
		Now:                 testNow(),
		ExpiresAt:           testNow().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create test payment: %v", err)
	}

	return paymentRecord
}

func testHold(t *testing.T, holdID string, paymentID string) payment.Hold {
	t.Helper()

	hold, err := payment.NewHold(payment.NewHoldInput{
		ID:          holdID,
		PaymentID:   paymentID,
		PayerID:     "payer-1",
		AmountMinor: 10_000,
		Currency:    "USD",
		Now:         testNow(),
	})
	if err != nil {
		t.Fatalf("failed to create test hold: %v", err)
	}

	return hold
}

func testNow() time.Time {
	return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
}
