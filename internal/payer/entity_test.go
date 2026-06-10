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

func TestPayerReserveMovesAvailableBalanceToHeldBalance(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)

	updated, err := payer.Reserve(4_000, "usd", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	if updated.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance 6000, got %d", updated.AvailableBalanceMinor)
	}

	if updated.HeldBalanceMinor != 4_000 {
		t.Fatalf("expected held balance 4000, got %d", updated.HeldBalanceMinor)
	}

	if updated.Version != 1 {
		t.Fatalf("expected version 1, got %d", updated.Version)
	}
}

func TestPayerReserveRejectsInvalidRequests(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)

	tests := []struct {
		name        string
		amountMinor int64
		currency    string
	}{
		{name: "zero amount", amountMinor: 0, currency: "USD"},
		{name: "currency mismatch", amountMinor: 1_000, currency: "SGD"},
		{name: "insufficient available balance", amountMinor: 10_001, currency: "USD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := payer.Reserve(tt.amountMinor, tt.currency, now.Add(time.Minute))

			if err == nil {
				t.Fatal("expected reserve error")
			}
		})
	}
}

func TestPayerReleaseMovesHeldBalanceBackToAvailableBalance(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)

	reserved, err := payer.Reserve(4_000, "USD", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	released, err := reserved.Release(1_500, "usd", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expected release to succeed, got error: %v", err)
	}

	if released.AvailableBalanceMinor != 7_500 {
		t.Fatalf("expected available balance 7500, got %d", released.AvailableBalanceMinor)
	}

	if released.HeldBalanceMinor != 2_500 {
		t.Fatalf("expected held balance 2500, got %d", released.HeldBalanceMinor)
	}

	if released.Version != 2 {
		t.Fatalf("expected version 2, got %d", released.Version)
	}
}

func TestPayerReleaseRejectsInvalidRequests(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)
	reserved, err := payer.Reserve(4_000, "USD", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	tests := []struct {
		name        string
		amountMinor int64
		currency    string
	}{
		{name: "zero amount", amountMinor: 0, currency: "USD"},
		{name: "currency mismatch", amountMinor: 1_000, currency: "SGD"},
		{name: "insufficient held balance", amountMinor: 4_001, currency: "USD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reserved.Release(tt.amountMinor, tt.currency, now.Add(2*time.Minute))

			if err == nil {
				t.Fatal("expected release error")
			}
		})
	}
}

func TestPayerCaptureHeldDecreasesHeldBalance(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)

	reserved, err := payer.Reserve(4_000, "USD", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	captured, err := reserved.CaptureHeld(4_000, "usd", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expected capture held to succeed, got error: %v", err)
	}

	if captured.AvailableBalanceMinor != 6_000 {
		t.Fatalf("expected available balance 6000, got %d", captured.AvailableBalanceMinor)
	}

	if captured.HeldBalanceMinor != 0 {
		t.Fatalf("expected held balance 0, got %d", captured.HeldBalanceMinor)
	}

	if captured.Version != 2 {
		t.Fatalf("expected version 2, got %d", captured.Version)
	}
}

func TestPayerCaptureHeldRejectsInvalidRequests(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	payer := mustPayer(t, "payer-1", 10_000, "USD", now)
	reserved, err := payer.Reserve(4_000, "USD", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected reserve to succeed, got error: %v", err)
	}

	tests := []struct {
		name        string
		amountMinor int64
		currency    string
	}{
		{name: "zero amount", amountMinor: 0, currency: "USD"},
		{name: "currency mismatch", amountMinor: 1_000, currency: "SGD"},
		{name: "insufficient held balance", amountMinor: 4_001, currency: "USD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := reserved.CaptureHeld(tt.amountMinor, tt.currency, now.Add(2*time.Minute))

			if err == nil {
				t.Fatal("expected capture held error")
			}
		})
	}
}

func mustPayer(t *testing.T, id string, availableBalanceMinor int64, currency string, now time.Time) Payer {
	t.Helper()

	payer, err := NewPayer(id, availableBalanceMinor, currency, now)
	if err != nil {
		t.Fatalf("failed to create payer: %v", err)
	}

	return payer
}
