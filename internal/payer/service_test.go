package payer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/payer/adapters/memory"
)

func TestPayerServiceCreatesPayer(t *testing.T) {
	service := payer.NewPayerService(memory.NewStore())

	payerRecord, err := service.CreatePayer(context.Background(), payer.CreatePayerInput{
		ID:                    "payer-1",
		AvailableBalanceMinor: 10_000,
		Currency:              "usd",
	})
	if err != nil {
		t.Fatalf("expected payer create to succeed, got error: %v", err)
	}

	if payerRecord.ID != "payer-1" {
		t.Fatalf("expected payer id payer-1, got %q", payerRecord.ID)
	}

	if payerRecord.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", payerRecord.Currency)
	}
}

func TestPayerServiceRejectsInvalidPayer(t *testing.T) {
	service := payer.NewPayerService(memory.NewStore())

	_, err := service.CreatePayer(context.Background(), payer.CreatePayerInput{
		ID:                    "payer-1",
		AvailableBalanceMinor: -1,
		Currency:              "USD",
	})

	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPayerServiceGetsAndListsPayers(t *testing.T) {
	ctx := context.Background()
	service := payer.NewPayerService(memory.NewStore())

	created, err := service.CreatePayer(ctx, payer.CreatePayerInput{
		ID:                    "payer-1",
		AvailableBalanceMinor: 10_000,
		Currency:              "USD",
	})
	if err != nil {
		t.Fatalf("expected payer create to succeed, got error: %v", err)
	}

	got, err := service.GetPayer(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected payer get to succeed, got error: %v", err)
	}

	if got.ID != created.ID {
		t.Fatalf("expected payer id %q, got %q", created.ID, got.ID)
	}

	payers, err := service.ListPayers(ctx)
	if err != nil {
		t.Fatalf("expected payer list to succeed, got error: %v", err)
	}

	if len(payers) != 1 {
		t.Fatalf("expected 1 payer, got %d", len(payers))
	}
}

func TestPayerServiceReturnsRepositoryErrors(t *testing.T) {
	service := payer.NewPayerService(memory.NewStore())

	_, err := service.GetPayer(context.Background(), "missing")
	if !errors.Is(err, payer.ErrPayerNotFound) {
		t.Fatalf("expected ErrPayerNotFound, got %v", err)
	}
}
