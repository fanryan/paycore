package merchant

import (
	"testing"
	"time"
)

func TestNewMerchantCreatesActiveMerchant(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.FixedZone("SGT", 8*60*60))

	merchant, err := NewMerchant("merchant-1", " Demo Merchant ", " usd ", now)
	if err != nil {
		t.Fatalf("expected merchant to be created, got error: %v", err)
	}

	if merchant.ID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", merchant.ID)
	}

	if merchant.Name != "Demo Merchant" {
		t.Fatalf("expected merchant name to be trimmed, got %q", merchant.Name)
	}

	if merchant.Status != MerchantStatusActive {
		t.Fatalf("expected merchant status ACTIVE, got %q", merchant.Status)
	}

	if merchant.SettlementCurrency != "USD" {
		t.Fatalf("expected settlement currency USD, got %q", merchant.SettlementCurrency)
	}

	if merchant.CreatedAt.Location() != time.UTC {
		t.Fatal("expected created at to be UTC")
	}

	if merchant.UpdatedAt.Location() != time.UTC {
		t.Fatal("expected updated at to be UTC")
	}
}

func TestNewMerchantValidatesRequiredFields(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name               string
		id                 string
		merchantName       string
		settlementCurrency string
	}{
		{name: "missing id", id: "", merchantName: "Demo Merchant", settlementCurrency: "USD"},
		{name: "missing name", id: "merchant-1", merchantName: " ", settlementCurrency: "USD"},
		{name: "invalid currency", id: "merchant-1", merchantName: "Demo Merchant", settlementCurrency: "USDT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMerchant(tt.id, tt.merchantName, tt.settlementCurrency, now)

			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMerchantCanCreatePaymentsOnlyWhenActive(t *testing.T) {
	merchant := Merchant{Status: MerchantStatusActive}
	if !merchant.CanCreatePayments() {
		t.Fatal("expected active merchant to create payments")
	}

	merchant.Status = MerchantStatusSuspended
	if merchant.CanCreatePayments() {
		t.Fatal("expected suspended merchant to be blocked")
	}

	merchant.Status = MerchantStatusClosed
	if merchant.CanCreatePayments() {
		t.Fatal("expected closed merchant to be blocked")
	}
}
