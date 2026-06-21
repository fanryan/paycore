package postgres_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/payment"
	paymentpostgres "github.com/fanryan/paycore/internal/payment/adapters/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesGetsAndUpdatesPaymentsAndHolds(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := paymentpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPaymentFixture(t, pool, prefix)
	})

	seedPaymentParents(t, pool, prefix)

	paymentRecord := testPayment(t, prefix+"-payment-1", prefix+"-hold-1", prefix)
	createdPayment, err := store.CreatePayment(ctx, paymentRecord)
	if err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	if createdPayment.Status != payment.StatusAuthorized {
		t.Fatalf("expected payment status AUTHORIZED, got %q", createdPayment.Status)
	}

	hold := testHold(t, prefix+"-hold-1", createdPayment.ID, prefix)
	createdHold, err := store.CreateHold(ctx, hold)
	if err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	if createdHold.Status != payment.HoldStatusHeld {
		t.Fatalf("expected hold status HELD, got %q", createdHold.Status)
	}

	gotPayment, err := store.GetPayment(ctx, createdPayment.ID)
	if err != nil {
		t.Fatalf("expected payment get to succeed, got error: %v", err)
	}

	if gotPayment.AuthorizationHoldID != hold.ID {
		t.Fatalf("expected authorization hold id %q, got %q", hold.ID, gotPayment.AuthorizationHoldID)
	}

	gotHold, err := store.GetHold(ctx, hold.ID)
	if err != nil {
		t.Fatalf("expected hold get to succeed, got error: %v", err)
	}

	if gotHold.PaymentID != createdPayment.ID {
		t.Fatalf("expected hold payment id %q, got %q", createdPayment.ID, gotHold.PaymentID)
	}

	gotHoldByPaymentID, err := store.GetHoldByPaymentID(ctx, createdPayment.ID)
	if err != nil {
		t.Fatalf("expected hold get by payment id to succeed, got error: %v", err)
	}

	if gotHoldByPaymentID.ID != hold.ID {
		t.Fatalf("expected hold id %q, got %q", hold.ID, gotHoldByPaymentID.ID)
	}

	capturedPayment, err := gotPayment.Capture(testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected payment capture to succeed, got error: %v", err)
	}

	updatedPayment, err := store.UpdatePayment(ctx, capturedPayment)
	if err != nil {
		t.Fatalf("expected payment update to succeed, got error: %v", err)
	}

	if updatedPayment.Status != payment.StatusCaptured {
		t.Fatalf("expected payment status CAPTURED, got %q", updatedPayment.Status)
	}

	if updatedPayment.CapturedAt == nil {
		t.Fatal("expected captured_at to be populated")
	}

	capturedHold, err := gotHold.Capture(testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected hold capture to succeed, got error: %v", err)
	}

	updatedHold, err := store.UpdateHold(ctx, capturedHold)
	if err != nil {
		t.Fatalf("expected hold update to succeed, got error: %v", err)
	}

	if updatedHold.Status != payment.HoldStatusCaptured {
		t.Fatalf("expected hold status CAPTURED, got %q", updatedHold.Status)
	}
}

func TestRepositoryRejectsDuplicatePaymentAndHold(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := paymentpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPaymentFixture(t, pool, prefix)
	})

	seedPaymentParents(t, pool, prefix)

	paymentRecord := testPayment(t, prefix+"-payment-1", prefix+"-hold-1", prefix)
	if _, err := store.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	_, err := store.CreatePayment(ctx, paymentRecord)
	if !errors.Is(err, payment.ErrDuplicatePayment) {
		t.Fatalf("expected ErrDuplicatePayment, got %v", err)
	}

	hold := testHold(t, prefix+"-hold-1", paymentRecord.ID, prefix)
	if _, err := store.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	_, err = store.CreateHold(ctx, hold)
	if !errors.Is(err, payment.ErrDuplicateHold) {
		t.Fatalf("expected ErrDuplicateHold, got %v", err)
	}
}

func TestRepositoryListsExpiredAuthorizedPayments(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := paymentpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupPaymentFixture(t, pool, prefix)
	})

	seedPaymentParents(t, pool, prefix)

	first := testPayment(t, prefix+"-payment-1", prefix+"-hold-1", prefix)
	first.ExpiresAt = testNow().Add(-2 * time.Minute)
	second := testPayment(t, prefix+"-payment-2", prefix+"-hold-2", prefix)
	second.ExpiresAt = testNow().Add(-time.Minute)
	notExpired := testPayment(t, prefix+"-payment-3", prefix+"-hold-3", prefix)
	notExpired.ExpiresAt = testNow().Add(time.Minute)
	captured := testPayment(t, prefix+"-payment-4", prefix+"-hold-4", prefix)
	capturedPayment, err := captured.Capture(testNow().Add(-30 * time.Second))
	if err != nil {
		t.Fatalf("expected capture to succeed, got error: %v", err)
	}
	capturedPayment.ExpiresAt = testNow().Add(-time.Minute)

	for _, paymentRecord := range []payment.Payment{second, notExpired, capturedPayment, first} {
		if _, err := store.CreatePayment(ctx, paymentRecord); err != nil {
			t.Fatalf("expected payment create to succeed, got error: %v", err)
		}
	}

	expired, err := store.ListExpiredAuthorizedPayments(ctx, testNow(), 1)
	if err != nil {
		t.Fatalf("expected expired payment list to succeed, got error: %v", err)
	}

	if len(expired) != 1 {
		t.Fatalf("expected 1 expired payment, got %d", len(expired))
	}

	if expired[0].ID != first.ID {
		t.Fatalf("expected earliest expired payment %q, got %q", first.ID, expired[0].ID)
	}
}

func TestRepositoryReturnsNotFoundForMissingPaymentAndHold(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := paymentpostgres.NewStore(pool)

	_, err := store.GetPayment(ctx, "missing-payment")
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}

	_, err = store.UpdatePayment(ctx, testPayment(t, "missing-payment", "missing-hold", "missing"))
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound on update, got %v", err)
	}

	_, err = store.GetHold(ctx, "missing-hold")
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound, got %v", err)
	}

	_, err = store.GetHoldByPaymentID(ctx, "missing-payment")
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound by payment id, got %v", err)
	}

	_, err = store.UpdateHold(ctx, testHold(t, "missing-hold", "missing-payment", "missing"))
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound on update, got %v", err)
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

func seedPaymentParents(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	merchantRecord, err := merchant.NewMerchant(prefix+"-merchant-1", "Demo Merchant", "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	payerRecord, err := payer.NewPayer(prefix+"-payer-1", 10_000, "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create payer: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO merchants (
			id,
			name,
			status,
			settlement_currency,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, merchantRecord.ID, merchantRecord.Name, string(merchantRecord.Status), merchantRecord.SettlementCurrency, merchantRecord.CreatedAt, merchantRecord.UpdatedAt); err != nil {
		t.Fatalf("failed to insert merchant fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO payers (
			id,
			available_balance_minor,
			held_balance_minor,
			currency,
			version,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, payerRecord.ID, payerRecord.AvailableBalanceMinor, payerRecord.HeldBalanceMinor, payerRecord.Currency, payerRecord.Version, payerRecord.CreatedAt, payerRecord.UpdatedAt); err != nil {
		t.Fatalf("failed to insert payer fixture: %v", err)
	}
}

func cleanupPaymentFixture(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	statements := []string{
		"DELETE FROM payment_holds WHERE id LIKE $1 OR payment_id LIKE $1",
		"DELETE FROM payments WHERE id LIKE $1",
		"DELETE FROM payers WHERE id LIKE $1",
		"DELETE FROM merchants WHERE id LIKE $1",
	}

	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, prefix+"%"); err != nil {
			t.Fatalf("failed to cleanup payment fixture: %v", err)
		}
	}
}

func testPayment(t *testing.T, paymentID string, holdID string, prefix string) payment.Payment {
	t.Helper()

	paymentRecord, err := payment.NewAuthorizedPayment(payment.NewAuthorizedPaymentInput{
		ID:                  paymentID,
		MerchantID:          prefix + "-merchant-1",
		PayerID:             prefix + "-payer-1",
		AmountMinor:         4_000,
		Currency:            "USD",
		AuthorizationHoldID: holdID,
		Now:                 testNow(),
		ExpiresAt:           testNow().Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create payment: %v", err)
	}

	return paymentRecord
}

func testHold(t *testing.T, holdID string, paymentID string, prefix string) payment.Hold {
	t.Helper()

	hold, err := payment.NewHold(payment.NewHoldInput{
		ID:          holdID,
		PaymentID:   paymentID,
		PayerID:     prefix + "-payer-1",
		AmountMinor: 4_000,
		Currency:    "USD",
		Now:         testNow(),
	})
	if err != nil {
		t.Fatalf("failed to create hold: %v", err)
	}

	return hold
}

func testPrefix() string {
	return "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
