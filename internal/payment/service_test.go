package payment_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/outbox"
	outboxmemory "github.com/fanryan/paycore/internal/outbox/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
	"github.com/fanryan/paycore/internal/shared/db"
)

func TestServiceAuthorizesPayment(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)

	result, err := fixture.service.AuthorizePayment(ctx, payment.AuthorizePaymentInput{
		MerchantID:  "merchant-1",
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "usd",
	})
	if err != nil {
		t.Fatalf("expected authorization to succeed, got error: %v", err)
	}

	if result.Payment.Status != payment.StatusAuthorized {
		t.Fatalf("expected payment status AUTHORIZED, got %q", result.Payment.Status)
	}

	if result.Payment.MerchantID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", result.Payment.MerchantID)
	}

	if result.Payment.PayerID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", result.Payment.PayerID)
	}

	if result.Payment.AmountMinor != 4_000 {
		t.Fatalf("expected amount 4000, got %d", result.Payment.AmountMinor)
	}

	if result.Payment.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", result.Payment.Currency)
	}

	if result.Hold.Status != payment.HoldStatusHeld {
		t.Fatalf("expected hold status HELD, got %q", result.Hold.Status)
	}

	if result.Hold.PaymentID != result.Payment.ID {
		t.Fatalf("expected hold payment id %q, got %q", result.Payment.ID, result.Hold.PaymentID)
	}

	if result.Payment.AuthorizationHoldID != result.Hold.ID {
		t.Fatalf("expected payment hold id %q, got %q", result.Hold.ID, result.Payment.AuthorizationHoldID)
	}

	if result.Payer.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance 6000, got %d", result.Payer.AvailableBalanceMinor)
	}

	if result.Payer.HeldBalanceMinor != 4_000 {
		t.Fatalf("expected held balance 4000, got %d", result.Payer.HeldBalanceMinor)
	}

	storedPayment, err := fixture.payments.GetPayment(ctx, result.Payment.ID)
	if err != nil {
		t.Fatalf("expected stored payment, got error: %v", err)
	}

	if storedPayment.ID != result.Payment.ID {
		t.Fatalf("expected stored payment id %q, got %q", result.Payment.ID, storedPayment.ID)
	}

	storedHold, err := fixture.payments.GetHoldByPaymentID(ctx, result.Payment.ID)
	if err != nil {
		t.Fatalf("expected stored hold, got error: %v", err)
	}

	if storedHold.ID != result.Hold.ID {
		t.Fatalf("expected stored hold id %q, got %q", result.Hold.ID, storedHold.ID)
	}

	storedPayer, err := fixture.payers.GetPayer(ctx, "payer-1")
	if err != nil {
		t.Fatalf("expected stored payer, got error: %v", err)
	}

	if storedPayer.HeldBalanceMinor != 4_000 {
		t.Fatalf("expected stored held balance 4000, got %d", storedPayer.HeldBalanceMinor)
	}

	event := assertOutboxEvent(t, fixture, "payment.authorized", result.Payment.ID)

	var payload map[string]any
	decodePayload(t, event, &payload)

	if payload["payment_id"] != result.Payment.ID {
		t.Fatalf("expected outbox payment_id %q, got %v", result.Payment.ID, payload["payment_id"])
	}

	if payload["hold_id"] != result.Hold.ID {
		t.Fatalf("expected outbox hold_id %q, got %v", result.Hold.ID, payload["hold_id"])
	}
}

func TestServiceRejectsInactiveMerchant(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)

	inactiveMerchant, err := merchant.NewMerchant("merchant-2", "Suspended Merchant", "USD", testNow())
	if err != nil {
		t.Fatalf("expected merchant create to succeed, got error: %v", err)
	}
	inactiveMerchant.Status = merchant.MerchantStatusSuspended

	if _, err := fixture.merchants.CreateMerchant(ctx, inactiveMerchant); err != nil {
		t.Fatalf("expected merchant save to succeed, got error: %v", err)
	}

	_, err = fixture.service.AuthorizePayment(ctx, payment.AuthorizePaymentInput{
		MerchantID:  "merchant-2",
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "USD",
	})
	if !errors.Is(err, payment.ErrMerchantCannotCreatePayments) {
		t.Fatalf("expected ErrMerchantCannotCreatePayments, got %v", err)
	}
}

func TestServiceRejectsMissingMerchant(t *testing.T) {
	fixture := newFixture(t)

	_, err := fixture.service.AuthorizePayment(context.Background(), payment.AuthorizePaymentInput{
		MerchantID:  "missing",
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "USD",
	})
	if !errors.Is(err, merchant.ErrMerchantNotFound) {
		t.Fatalf("expected ErrMerchantNotFound, got %v", err)
	}
}

func TestServiceRejectsMissingPayer(t *testing.T) {
	fixture := newFixture(t)

	_, err := fixture.service.AuthorizePayment(context.Background(), payment.AuthorizePaymentInput{
		MerchantID:  "merchant-1",
		PayerID:     "missing",
		AmountMinor: 4_000,
		Currency:    "USD",
	})
	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound, got %v", err)
	}
}

func TestServiceRejectsCurrencyMismatch(t *testing.T) {
	fixture := newFixture(t)

	_, err := fixture.service.AuthorizePayment(context.Background(), payment.AuthorizePaymentInput{
		MerchantID:  "merchant-1",
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "SGD",
	})
	if !errors.Is(err, payment.ErrPayerCurrencyMismatch) {
		t.Fatalf("expected ErrPayerCurrencyMismatch, got %v", err)
	}
}

func TestServiceRejectsInsufficientAvailableBalance(t *testing.T) {
	fixture := newFixture(t)

	_, err := fixture.service.AuthorizePayment(context.Background(), payment.AuthorizePaymentInput{
		MerchantID:  "merchant-1",
		PayerID:     "payer-1",
		AmountMinor: 10_001,
		Currency:    "USD",
	})
	if !errors.Is(err, payment.ErrInsufficientAvailableBalance) {
		t.Fatalf("expected ErrInsufficientAvailableBalance, got %v", err)
	}
}

func TestServiceCapturesPayment(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)

	authorized := authorizePayment(t, fixture)

	result, err := fixture.service.CapturePayment(ctx, payment.CapturePaymentInput{
		PaymentID: authorized.Payment.ID,
	})
	if err != nil {
		t.Fatalf("expected capture to succeed, got error: %v", err)
	}

	if result.Payment.Status != payment.StatusCaptured {
		t.Fatalf("expected payment status CAPTURED, got %q", result.Payment.Status)
	}

	if result.Payment.CapturedAt == nil {
		t.Fatal("expected captured at to be set")
	}

	if result.Hold.Status != payment.HoldStatusCaptured {
		t.Fatalf("expected hold status CAPTURED, got %q", result.Hold.Status)
	}

	if result.Payer.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance 6000, got %d", result.Payer.AvailableBalanceMinor)
	}

	if result.Payer.HeldBalanceMinor != 0 {
		t.Fatalf("expected held balance 0, got %d", result.Payer.HeldBalanceMinor)
	}

	storedPayment, err := fixture.payments.GetPayment(ctx, authorized.Payment.ID)
	if err != nil {
		t.Fatalf("expected stored payment, got error: %v", err)
	}

	if storedPayment.Status != payment.StatusCaptured {
		t.Fatalf("expected stored payment status CAPTURED, got %q", storedPayment.Status)
	}

	storedHold, err := fixture.payments.GetHoldByPaymentID(ctx, authorized.Payment.ID)
	if err != nil {
		t.Fatalf("expected stored hold, got error: %v", err)
	}

	if storedHold.Status != payment.HoldStatusCaptured {
		t.Fatalf("expected stored hold status CAPTURED, got %q", storedHold.Status)
	}

	storedPayer, err := fixture.payers.GetPayer(ctx, "payer-1")
	if err != nil {
		t.Fatalf("expected stored payer, got error: %v", err)
	}

	if storedPayer.HeldBalanceMinor != 0 {
		t.Fatalf("expected stored held balance 0, got %d", storedPayer.HeldBalanceMinor)
	}

	event := assertOutboxEvent(t, fixture, "payment.captured", result.Payment.ID)

	var payload map[string]any
	decodePayload(t, event, &payload)

	if payload["payment_id"] != result.Payment.ID {
		t.Fatalf("expected outbox payment_id %q, got %v", result.Payment.ID, payload["payment_id"])
	}

	if payload["hold_id"] != result.Hold.ID {
		t.Fatalf("expected outbox hold_id %q, got %v", result.Hold.ID, payload["hold_id"])
	}
}

func TestServiceRejectsMissingPaymentOnCapture(t *testing.T) {
	fixture := newFixture(t)

	_, err := fixture.service.CapturePayment(context.Background(), payment.CapturePaymentInput{
		PaymentID: "missing",
	})
	if !errors.Is(err, payment.ErrPaymentNotFound) {
		t.Fatalf("expected ErrPaymentNotFound, got %v", err)
	}
}

func TestServiceRejectsMissingHoldOnCapture(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)
	paymentRecord := testAuthorizedPayment(t, "payment-without-hold", "hold-missing")

	if _, err := fixture.payments.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	_, err := fixture.service.CapturePayment(ctx, payment.CapturePaymentInput{
		PaymentID: paymentRecord.ID,
	})
	if !errors.Is(err, payment.ErrHoldNotFound) {
		t.Fatalf("expected ErrHoldNotFound, got %v", err)
	}
}

func TestServiceRejectsMissingPayerOnCapture(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)

	paymentRecord := testAuthorizedPayment(t, "payment-1", "hold-1")
	paymentRecord.PayerID = "missing"
	hold := testHold(t, "hold-1", paymentRecord.ID)

	if _, err := fixture.payments.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	if _, err := fixture.payments.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	_, err := fixture.service.CapturePayment(ctx, payment.CapturePaymentInput{
		PaymentID: paymentRecord.ID,
	})
	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound, got %v", err)
	}
}

func TestServiceRejectsNonCapturablePayment(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)
	authorized := authorizePayment(t, fixture)

	captured, err := authorized.Payment.Capture(testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected capture to succeed, got error: %v", err)
	}

	if _, err := fixture.payments.UpdatePayment(ctx, captured); err != nil {
		t.Fatalf("expected payment update to succeed, got error: %v", err)
	}

	_, err = fixture.service.CapturePayment(ctx, payment.CapturePaymentInput{
		PaymentID: captured.ID,
	})
	if !errors.Is(err, payment.ErrPaymentNotCapturable) {
		t.Fatalf("expected ErrPaymentNotCapturable, got %v", err)
	}
}

func TestServiceRejectsExpiredAuthorizationOnCapture(t *testing.T) {
	ctx := context.Background()
	fixture := newFixture(t)
	paymentRecord := testAuthorizedPayment(t, "payment-1", "hold-1")
	paymentRecord.ExpiresAt = testNow().Add(-time.Minute)
	hold := testHold(t, "hold-1", paymentRecord.ID)
	payerRecord, err := fixture.payers.GetPayer(ctx, "payer-1")
	if err != nil {
		t.Fatalf("expected payer get to succeed, got error: %v", err)
	}

	reservedPayer, err := payerRecord.Reserve(paymentRecord.AmountMinor, paymentRecord.Currency, testNow())
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	if _, err := fixture.payers.UpdatePayer(ctx, reservedPayer); err != nil {
		t.Fatalf("expected payer update to succeed, got error: %v", err)
	}

	if _, err := fixture.payments.CreatePayment(ctx, paymentRecord); err != nil {
		t.Fatalf("expected payment create to succeed, got error: %v", err)
	}

	if _, err := fixture.payments.CreateHold(ctx, hold); err != nil {
		t.Fatalf("expected hold create to succeed, got error: %v", err)
	}

	_, err = fixture.service.CapturePayment(ctx, payment.CapturePaymentInput{
		PaymentID: paymentRecord.ID,
	})
	if !errors.Is(err, payment.ErrAuthorizationExpired) {
		t.Fatalf("expected ErrAuthorizationExpired, got %v", err)
	}
}

type fixture struct {
	merchants *merchantmemory.Store
	payers    *payermemory.Store
	payments  *paymentmemory.Store
	outbox    *outboxmemory.Store
	service   *payment.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	ctx := context.Background()
	merchants := merchantmemory.NewStore()
	payers := payermemory.NewStore()
	payments := paymentmemory.NewStore()
	outboxStore := outboxmemory.NewStore()

	merchantRecord, err := merchant.NewMerchant("merchant-1", "Demo Merchant", "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create merchant: %v", err)
	}

	if _, err := merchants.CreateMerchant(ctx, merchantRecord); err != nil {
		t.Fatalf("failed to save merchant: %v", err)
	}

	payerRecord, err := payer.NewPayer("payer-1", 10_000, "USD", testNow())
	if err != nil {
		t.Fatalf("failed to create payer: %v", err)
	}

	if _, err := payers.CreatePayer(ctx, payerRecord); err != nil {
		t.Fatalf("failed to save payer: %v", err)
	}

	return fixture{
		merchants: merchants,
		payers:    payers,
		payments:  payments,
		outbox:    outboxStore,
		service: payment.NewServiceWithTransactorAndOutbox(
			merchants,
			payers,
			payments,
			db.NoopTransactor{},
			outboxStore,
		),
	}
}

func authorizePayment(t *testing.T, fixture fixture) payment.AuthorizePaymentResult {
	t.Helper()

	result, err := fixture.service.AuthorizePayment(context.Background(), payment.AuthorizePaymentInput{
		MerchantID:  "merchant-1",
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "USD",
	})
	if err != nil {
		t.Fatalf("expected authorization to succeed, got error: %v", err)
	}

	return result
}

func testAuthorizedPayment(t *testing.T, paymentID string, holdID string) payment.Payment {
	t.Helper()

	now := time.Now().UTC()

	paymentRecord, err := payment.NewAuthorizedPayment(payment.NewAuthorizedPaymentInput{
		ID:                  paymentID,
		MerchantID:          "merchant-1",
		PayerID:             "payer-1",
		AmountMinor:         4_000,
		Currency:            "USD",
		AuthorizationHoldID: holdID,
		Now:                 now,
		ExpiresAt:           now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create authorized payment: %v", err)
	}

	return paymentRecord
}

func testHold(t *testing.T, holdID string, paymentID string) payment.Hold {
	t.Helper()

	hold, err := payment.NewHold(payment.NewHoldInput{
		ID:          holdID,
		PaymentID:   paymentID,
		PayerID:     "payer-1",
		AmountMinor: 4_000,
		Currency:    "USD",
		Now:         testNow(),
	})
	if err != nil {
		t.Fatalf("failed to create hold: %v", err)
	}

	return hold
}

func assertOutboxEvent(t *testing.T, fixture fixture, eventType string, aggregateID string) outbox.Event {
	t.Helper()

	events, err := fixture.outbox.ListEvents(context.Background())
	if err != nil {
		t.Fatalf("expected outbox list to succeed, got error: %v", err)
	}

	for _, event := range events {
		if event.EventType == eventType && event.AggregateID == aggregateID {
			return event
		}
	}

	t.Fatalf("expected outbox event %q for aggregate %q", eventType, aggregateID)
	return outbox.Event{}
}

func decodePayload(t *testing.T, event outbox.Event, target any) {
	t.Helper()

	if err := json.Unmarshal(event.Payload, target); err != nil {
		t.Fatalf("expected outbox payload to decode, got error: %v", err)
	}
}

func testNow() time.Time {
	return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
}
