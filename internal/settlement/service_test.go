package settlement_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/settlement"
	"github.com/fanryan/paycore/internal/shared/db"
)

func TestServiceCreatesCompletedBatchWithLineItems(t *testing.T) {
	repository := &fakeRepository{
		claimed: []settlement.ClaimedPayment{
			{
				PaymentID:   "pay-1",
				MerchantID:  "merchant-1",
				AmountMinor: 4000,
				Currency:    "USD",
				CapturedAt:  testNow().Add(-time.Minute),
			},
			{
				PaymentID:   "pay-2",
				MerchantID:  "merchant-1",
				AmountMinor: 6000,
				Currency:    "USD",
				CapturedAt:  testNow().Add(-2 * time.Minute),
			},
		},
	}
	service := newSettlementService(t, repository)

	result, err := service.CreateBatch(context.Background(), settlement.CreateBatchInput{
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow(),
	})
	if err != nil {
		t.Fatalf("expected create batch to succeed, got error: %v", err)
	}

	if result.Batch.Status != settlement.BatchStatusCompleted {
		t.Fatalf("expected completed batch, got %q", result.Batch.Status)
	}

	if len(result.LineItems) != 2 {
		t.Fatalf("expected 2 line items, got %d", len(result.LineItems))
	}

	if result.LineItems[0].PaymentID != "pay-1" {
		t.Fatalf("expected first payment pay-1, got %q", result.LineItems[0].PaymentID)
	}

	if repository.claimInput.BatchID != result.Batch.ID {
		t.Fatalf("expected claim batch id %q, got %q", result.Batch.ID, repository.claimInput.BatchID)
	}

	if repository.claimInput.Limit != 10 {
		t.Fatalf("expected claim limit 10, got %d", repository.claimInput.Limit)
	}
}

func TestServiceCompletesEmptyBatchWhenNoPaymentsAreClaimed(t *testing.T) {
	repository := &fakeRepository{}
	service := newSettlementService(t, repository)

	result, err := service.CreateBatch(context.Background(), settlement.CreateBatchInput{
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow(),
	})
	if err != nil {
		t.Fatalf("expected create batch to succeed, got error: %v", err)
	}

	if result.Batch.Status != settlement.BatchStatusCompleted {
		t.Fatalf("expected completed batch, got %q", result.Batch.Status)
	}

	if len(result.LineItems) != 0 {
		t.Fatalf("expected no line items, got %d", len(result.LineItems))
	}
}

func TestServiceReturnsErrorWhenDependenciesMissing(t *testing.T) {
	_, err := settlement.NewService(settlement.ServiceConfig{
		Transactor: db.NoopTransactor{},
	})
	if !errors.Is(err, settlement.ErrRepositoryRequired) {
		t.Fatalf("expected ErrRepositoryRequired, got %v", err)
	}

	_, err = settlement.NewService(settlement.ServiceConfig{
		Repository: &fakeRepository{},
	})
	if !errors.Is(err, settlement.ErrTransactorRequired) {
		t.Fatalf("expected ErrTransactorRequired, got %v", err)
	}
}

func TestServiceReturnsValidationErrorForInvalidWindow(t *testing.T) {
	service := newSettlementService(t, &fakeRepository{})

	_, err := service.CreateBatch(context.Background(), settlement.CreateBatchInput{
		WindowStart: testNow(),
		WindowEnd:   testNow(),
	})
	if !errors.Is(err, settlement.ErrInvalidSettlementWindow) {
		t.Fatalf("expected ErrInvalidSettlementWindow, got %v", err)
	}
}

func newSettlementService(t *testing.T, repository settlement.Repository) *settlement.Service {
	t.Helper()

	service, err := settlement.NewService(settlement.ServiceConfig{
		Repository: repository,
		Transactor: db.NoopTransactor{},
		WorkerID:   "worker-1",
		ClaimLimit: 10,
		LockTTL:    time.Minute,
		Now:        testNow,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

type fakeRepository struct {
	batches    map[string]settlement.Batch
	lineItems  []settlement.LineItem
	claimed    []settlement.ClaimedPayment
	claimInput settlement.ClaimCapturedPaymentsInput
}

func (r *fakeRepository) CreateBatch(ctx context.Context, batch settlement.Batch) (settlement.Batch, error) {
	if err := ctx.Err(); err != nil {
		return settlement.Batch{}, err
	}

	if r.batches == nil {
		r.batches = map[string]settlement.Batch{}
	}

	r.batches[batch.ID] = batch
	return batch, nil
}

func (r *fakeRepository) GetBatch(ctx context.Context, batchID string) (settlement.Batch, error) {
	if err := ctx.Err(); err != nil {
		return settlement.Batch{}, err
	}

	batch, ok := r.batches[batchID]
	if !ok {
		return settlement.Batch{}, settlement.ErrBatchNotFound
	}

	return batch, nil
}

func (r *fakeRepository) UpdateBatch(ctx context.Context, batch settlement.Batch) (settlement.Batch, error) {
	if err := ctx.Err(); err != nil {
		return settlement.Batch{}, err
	}

	if r.batches == nil {
		r.batches = map[string]settlement.Batch{}
	}

	r.batches[batch.ID] = batch
	return batch, nil
}

func (r *fakeRepository) ClaimCapturedPayments(ctx context.Context, input settlement.ClaimCapturedPaymentsInput) ([]settlement.ClaimedPayment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.claimInput = input

	claimed := make([]settlement.ClaimedPayment, len(r.claimed))
	copy(claimed, r.claimed)

	return claimed, nil
}

func (r *fakeRepository) CreateLineItem(ctx context.Context, item settlement.LineItem) (settlement.LineItem, error) {
	if err := ctx.Err(); err != nil {
		return settlement.LineItem{}, err
	}

	r.lineItems = append(r.lineItems, item)
	return item, nil
}

func (r *fakeRepository) ListLineItems(ctx context.Context, batchID string) ([]settlement.LineItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	items := make([]settlement.LineItem, 0)
	for _, item := range r.lineItems {
		if item.BatchID == batchID {
			items = append(items, item)
		}
	}

	return items, nil
}
