package payer

import (
	"testing"
	"time"
)

func TestNewPayerCreatesPayerWithAvailableBalance(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.FixedZone("SGT", 8*60*60))

	payer, err := NewPayer("payer-1", 10_000, " usd ", now)
	if err != nil {
		t.Fatalf("expected payer to be created, got error: %v", err)
	}

	if payer.ID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", payer.ID)
	}

	if payer.AvailableBalanceMinor != 10_000 {
		t.Fatalf("expected available balance 10000, got %d", payer.AvailableBalanceMinor)
	}

	if payer.HeldBalanceMinor != 0 {
		t.Fatalf("expected held balance 0, got %d", payer.HeldBalanceMinor)
	}

	if payer.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", payer.Currency)
	}

	if payer.Version != 0 {
		t.Fatalf("expected version 0, got %d", payer.Version)
	}

	if payer.CreatedAt.Location() != time.UTC {
		t.Fatal("expected created at to be UTC")
	}

	if payer.UpdatedAt.Location() != time.UTC {
		t.Fatal("expected updated at to be UTC")
	}
}

func TestNewPayerValidatesRequiredFields(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                  string
		id                    string
		availableBalanceMinor int64
		currency              string
	}{
		{name: "missing id", id: "", availableBalanceMinor: 10_000, currency: "USD"},
		{name: "negative available balance", id: "payer-1", availableBalanceMinor: -1, currency: "USD"},
		{name: "invalid currency", id: "payer-1", availableBalanceMinor: 10_000, currency: "USDT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPayer(tt.id, tt.availableBalanceMinor, tt.currency, now)

			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPayerCanAuthorize(t *testing.T) {
	payer := Payer{
		AvailableBalanceMinor: 10_000,
		Currency:              "USD",
	}

	tests := []struct {
		name        string
		amountMinor int64
		currency    string
		want        bool
	}{
		{name: "sufficient balance", amountMinor: 5_000, currency: "USD", want: true},
		{name: "exact balance", amountMinor: 10_000, currency: "usd", want: true},
		{name: "insufficient balance", amountMinor: 10_001, currency: "USD", want: false},
		{name: "zero amount", amountMinor: 0, currency: "USD", want: false},
		{name: "negative amount", amountMinor: -1, currency: "USD", want: false},
		{name: "currency mismatch", amountMinor: 5_000, currency: "SGD", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := payer.CanAuthorize(tt.amountMinor, tt.currency)

			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
