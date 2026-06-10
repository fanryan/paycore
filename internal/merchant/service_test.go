package merchant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/merchant/adapters/memory"
)

func TestMerchantServiceCreatesMerchant(t *testing.T) {
	service := merchant.NewMerchantService(memory.NewStore())

	merchantRecord, err := service.CreateMerchant(context.Background(), merchant.CreateMerchantInput{
		ID:                 "merchant-1",
		Name:               "Demo Merchant",
		SettlementCurrency: "usd",
	})
	if err != nil {
		t.Fatalf("expected merchant create to succeed, got error: %v", err)
	}

	if merchantRecord.ID != "merchant-1" {
		t.Fatalf("expected merchant id merchant-1, got %q", merchantRecord.ID)
	}

	if merchantRecord.SettlementCurrency != "USD" {
		t.Fatalf("expected settlement currency USD, got %q", merchantRecord.SettlementCurrency)
	}
}

func TestMerchantServiceRejectsInvalidMerchant(t *testing.T) {
	service := merchant.NewMerchantService(memory.NewStore())

	_, err := service.CreateMerchant(context.Background(), merchant.CreateMerchantInput{
		ID:                 "",
		Name:               "Demo Merchant",
		SettlementCurrency: "USD",
	})

	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMerchantServiceGetsAndListsMerchants(t *testing.T) {
	ctx := context.Background()
	service := merchant.NewMerchantService(memory.NewStore())

	created, err := service.CreateMerchant(ctx, merchant.CreateMerchantInput{
		ID:                 "merchant-1",
		Name:               "Demo Merchant",
		SettlementCurrency: "USD",
	})
	if err != nil {
		t.Fatalf("expected merchant create to succeed, got error: %v", err)
	}

	got, err := service.GetMerchant(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected merchant get to succeed, got error: %v", err)
	}

	if got.ID != created.ID {
		t.Fatalf("expected merchant id %q, got %q", created.ID, got.ID)
	}

	merchants, err := service.ListMerchants(ctx)
	if err != nil {
		t.Fatalf("expected merchant list to succeed, got error: %v", err)
	}

	if len(merchants) != 1 {
		t.Fatalf("expected 1 merchant, got %d", len(merchants))
	}
}

func TestMerchantServiceReturnsRepositoryErrors(t *testing.T) {
	service := merchant.NewMerchantService(memory.NewStore())

	_, err := service.GetMerchant(context.Background(), "missing")
	if !errors.Is(err, merchant.ErrMerchantNotFound) {
		t.Fatalf("expected ErrMerchantNotFound, got %v", err)
	}
}
