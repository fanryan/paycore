package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	merchantmemory "github.com/fanryan/paycore/internal/merchant/adapters/memory"
	"github.com/fanryan/paycore/internal/payer"
	payermemory "github.com/fanryan/paycore/internal/payer/adapters/memory"
	"github.com/fanryan/paycore/internal/payment"
	paymentmemory "github.com/fanryan/paycore/internal/payment/adapters/memory"
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

type fixture struct {
	merchants *merchantmemory.Store
	payers    *payermemory.Store
	payments  *paymentmemory.Store
	service   *payment.Service
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	ctx := context.Background()
	merchants := merchantmemory.NewStore()
	payers := payermemory.NewStore()
	payments := paymentmemory.NewStore()

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
		service:   payment.NewService(merchants, payers, payments),
	}
}

func testNow() time.Time {
	return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
}
