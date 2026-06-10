package payment

import (
	"testing"
	"time"
)

func TestNewHoldCreatesHeldHold(t *testing.T) {
	now := testNow()

	hold, err := NewHold(NewHoldInput{
		ID:          "hold-1",
		PaymentID:   "payment-1",
		PayerID:     "payer-1",
		AmountMinor: 10_000,
		Currency:    "usd",
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected hold to be created, got error: %v", err)
	}

	if hold.ID != "hold-1" {
		t.Fatalf("expected hold id hold-1, got %q", hold.ID)
	}

	if hold.Status != HoldStatusHeld {
		t.Fatalf("expected status HELD, got %q", hold.Status)
	}

	if hold.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", hold.Currency)
	}

	if hold.CreatedAt.Location() != time.UTC {
		t.Fatal("expected created at to be UTC")
	}
}

func TestNewHoldValidatesInput(t *testing.T) {
	now := testNow()

	tests := []struct {
		name  string
		input NewHoldInput
	}{
		{name: "missing hold id", input: validHoldInput(now, func(input *NewHoldInput) {
			input.ID = ""
		})},
		{name: "missing payment id", input: validHoldInput(now, func(input *NewHoldInput) {
			input.PaymentID = ""
		})},
		{name: "missing payer id", input: validHoldInput(now, func(input *NewHoldInput) {
			input.PayerID = ""
		})},
		{name: "zero amount", input: validHoldInput(now, func(input *NewHoldInput) {
			input.AmountMinor = 0
		})},
		{name: "invalid currency", input: validHoldInput(now, func(input *NewHoldInput) {
			input.Currency = "USDT"
		})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHold(tt.input)

			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestHoldCaptureTransitionsHeldToCaptured(t *testing.T) {
	now := testNow()
	hold := mustHold(t, now)

	captured, err := hold.Capture(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected hold capture to succeed, got error: %v", err)
	}

	if captured.Status != HoldStatusCaptured {
		t.Fatalf("expected status CAPTURED, got %q", captured.Status)
	}
}

func TestHoldReleaseTransitionsHeldToReleased(t *testing.T) {
	now := testNow()
	hold := mustHold(t, now)

	released, err := hold.Release(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected hold release to succeed, got error: %v", err)
	}

	if released.Status != HoldStatusReleased {
		t.Fatalf("expected status RELEASED, got %q", released.Status)
	}
}

func TestHoldRejectsInvalidTransitions(t *testing.T) {
	now := testNow()
	hold := mustHold(t, now)

	captured, err := hold.Capture(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected hold capture to succeed, got error: %v", err)
	}

	if _, err := captured.Release(now.Add(6 * time.Minute)); err == nil {
		t.Fatal("expected release of captured hold to fail")
	}

	released, err := hold.Release(now.Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("expected hold release to succeed, got error: %v", err)
	}

	if _, err := released.Capture(now.Add(6 * time.Minute)); err == nil {
		t.Fatal("expected capture of released hold to fail")
	}
}

func validHoldInput(now time.Time, mutate func(*NewHoldInput)) NewHoldInput {
	input := NewHoldInput{
		ID:          "hold-1",
		PaymentID:   "payment-1",
		PayerID:     "payer-1",
		AmountMinor: 10_000,
		Currency:    "USD",
		Now:         now,
	}

	mutate(&input)

	return input
}

func mustHold(t *testing.T, now time.Time) Hold {
	t.Helper()

	hold, err := NewHold(validHoldInput(now, func(input *NewHoldInput) {}))
	if err != nil {
		t.Fatalf("failed to create test hold: %v", err)
	}

	return hold
}
