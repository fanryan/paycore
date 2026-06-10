package payment

import (
	"testing"
	"time"
)

func TestNewAuthorizedPaymentCreatesAuthorizedPayment(t *testing.T) {
	now := testNow()

	payment, err := NewAuthorizedPayment(NewAuthorizedPaymentInput{
		ID:                  "payment-1",
		MerchantID:          "merchant-1",
		PayerID:             "payer-1",
		AmountMinor:         10_000,
		Currency:            "usd",
		AuthorizationHoldID: "hold-1",
		Now:                 now,
		ExpiresAt:           now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatalf("expected payment to be created, got error: %v", err)
	}

	if payment.ID != "payment-1" {
		t.Fatalf("expected payment id payment-1, got %q", payment.ID)
	}

	if payment.Status != StatusAuthorized {
		t.Fatalf("expected status AUTHORIZED, got %q", payment.Status)
	}

	if payment.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", payment.Currency)
	}

	if payment.AmountMinor != 10_000 {
		t.Fatalf("expected amount 10000, got %d", payment.AmountMinor)
	}

	if payment.CapturedAt != nil {
		t.Fatal("expected captured at to be nil")
	}

	if payment.SettledAt != nil {
		t.Fatal("expected settled at to be nil")
	}

	if payment.CreatedAt.Location() != time.UTC {
		t.Fatal("expected created at to be UTC")
	}
}

func TestNewAuthorizedPaymentValidatesInput(t *testing.T) {
	now := testNow()

	tests := []struct {
		name  string
		input NewAuthorizedPaymentInput
	}{
		{name: "missing payment id", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.ID = ""
		})},
		{name: "missing merchant id", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.MerchantID = ""
		})},
		{name: "missing payer id", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.PayerID = ""
		})},
		{name: "missing hold id", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.AuthorizationHoldID = ""
		})},
		{name: "zero amount", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.AmountMinor = 0
		})},
		{name: "invalid currency", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.Currency = "USDT"
		})},
		{name: "expired authorization", input: validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {
			input.ExpiresAt = now
		})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAuthorizedPayment(tt.input)

			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPaymentCanCapture(t *testing.T) {
	now := testNow()
	payment := mustPayment(t, now)

	if !payment.CanCapture(now.Add(10 * time.Minute)) {
		t.Fatal("expected authorized payment before expiry to be capturable")
	}

	if !payment.CanCapture(payment.ExpiresAt) {
		t.Fatal("expected authorized payment at expiry instant to be capturable")
	}

	if payment.CanCapture(payment.ExpiresAt.Add(time.Nanosecond)) {
		t.Fatal("expected payment after expiry to not be capturable")
	}

	payment.Status = StatusCaptured
	if payment.CanCapture(now.Add(10 * time.Minute)) {
		t.Fatal("expected captured payment to not be capturable")
	}
}

func TestPaymentCaptureTransitionsAuthorizedToCaptured(t *testing.T) {
	now := testNow()
	payment := mustPayment(t, now)

	captured, err := payment.Capture(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected capture to succeed, got error: %v", err)
	}

	if captured.Status != StatusCaptured {
		t.Fatalf("expected status CAPTURED, got %q", captured.Status)
	}

	if captured.CapturedAt == nil {
		t.Fatal("expected captured at to be set")
	}
}

func TestPaymentCaptureRejectsInvalidStateAndExpiredAuthorization(t *testing.T) {
	now := testNow()
	payment := mustPayment(t, now)

	payment.Status = StatusFailed
	if _, err := payment.Capture(now.Add(time.Minute)); err == nil {
		t.Fatal("expected invalid state error")
	}

	payment = mustPayment(t, now)
	if _, err := payment.Capture(payment.ExpiresAt.Add(time.Nanosecond)); err == nil {
		t.Fatal("expected expired authorization error")
	}
}

func TestPaymentExpireTransitionsAuthorizedToExpired(t *testing.T) {
	now := testNow()
	payment := mustPayment(t, now)

	expired, err := payment.Expire(now.Add(20 * time.Minute))
	if err != nil {
		t.Fatalf("expected expiry to succeed, got error: %v", err)
	}

	if expired.Status != StatusExpired {
		t.Fatalf("expected status EXPIRED, got %q", expired.Status)
	}
}

func TestPaymentSettleTransitionsCapturedToSettled(t *testing.T) {
	now := testNow()
	payment := mustPayment(t, now)
	captured, err := payment.Capture(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected capture to succeed, got error: %v", err)
	}

	settled, err := captured.Settle(now.Add(30 * time.Minute))
	if err != nil {
		t.Fatalf("expected settlement to succeed, got error: %v", err)
	}

	if settled.Status != StatusSettled {
		t.Fatalf("expected status SETTLED, got %q", settled.Status)
	}

	if settled.SettledAt == nil {
		t.Fatal("expected settled at to be set")
	}
}

func TestPaymentSettleRejectsNonCapturedPayment(t *testing.T) {
	payment := mustPayment(t, testNow())

	if _, err := payment.Settle(testNow().Add(30 * time.Minute)); err == nil {
		t.Fatal("expected invalid state error")
	}
}

func validPaymentInput(now time.Time, mutate func(*NewAuthorizedPaymentInput)) NewAuthorizedPaymentInput {
	input := NewAuthorizedPaymentInput{
		ID:                  "payment-1",
		MerchantID:          "merchant-1",
		PayerID:             "payer-1",
		AmountMinor:         10_000,
		Currency:            "USD",
		AuthorizationHoldID: "hold-1",
		Now:                 now,
		ExpiresAt:           now.Add(15 * time.Minute),
	}

	mutate(&input)

	return input
}

func mustPayment(t *testing.T, now time.Time) Payment {
	t.Helper()

	payment, err := NewAuthorizedPayment(validPaymentInput(now, func(input *NewAuthorizedPaymentInput) {}))
	if err != nil {
		t.Fatalf("failed to create test payment: %v", err)
	}

	return payment
}

func testNow() time.Time {
	return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
}
