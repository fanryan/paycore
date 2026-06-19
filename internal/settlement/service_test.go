package settlement_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	"github.com/fanryan/paycore/internal/payment"
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
	payments := newFakePaymentRepository(t, "pay-1", "pay-2")
	outboxRepository := &fakeOutboxRepository{}
	service := newSettlementService(t, repository, payments, outboxRepository)

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

	if len(result.Payments) != 2 {
		t.Fatalf("expected 2 settled payments, got %d", len(result.Payments))
	}

	if result.Payments[0].Status != payment.StatusSettled {
		t.Fatalf("expected first payment SETTLED, got %q", result.Payments[0].Status)
	}

	if result.LineItems[0].PaymentID != "pay-1" {
		t.Fatalf("expected first payment pay-1, got %q", result.LineItems[0].PaymentID)
	}

	if len(outboxRepository.created) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(outboxRepository.created))
	}

	if outboxRepository.created[0].EventType != "payment.settled" {
		t.Fatalf("expected payment.settled event, got %q", outboxRepository.created[0].EventType)
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
	service := newSettlementService(t, repository, newFakePaymentRepository(t), &fakeOutboxRepository{})

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
		Transactor: db.NoopTransactor{},
	})
	if !errors.Is(err, settlement.ErrPaymentRepositoryRequired) {
		t.Fatalf("expected ErrPaymentRepositoryRequired, got %v", err)
	}

	_, err = settlement.NewService(settlement.ServiceConfig{
		Repository: &fakeRepository{},
		Payments:   newFakePaymentRepository(t),
	})
	if !errors.Is(err, settlement.ErrTransactorRequired) {
		t.Fatalf("expected ErrTransactorRequired, got %v", err)
	}
}

func TestServiceReturnsValidationErrorForInvalidWindow(t *testing.T) {
	service := newSettlementService(t, &fakeRepository{}, newFakePaymentRepository(t), &fakeOutboxRepository{})

	_, err := service.CreateBatch(context.Background(), settlement.CreateBatchInput{
		WindowStart: testNow(),
		WindowEnd:   testNow(),
	})
	if !errors.Is(err, settlement.ErrInvalidSettlementWindow) {
		t.Fatalf("expected ErrInvalidSettlementWindow, got %v", err)
	}
}

func newSettlementService(t *testing.T, repository settlement.Repository, payments payment.Repository, outboxRepository outbox.Repository) *settlement.Service {
	t.Helper()

	service, err := settlement.NewService(settlement.ServiceConfig{
		Repository: repository,
		Payments:   payments,
		Outbox:     outboxRepository,
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

func newFakePaymentRepository(t *testing.T, paymentIDs ...string) *fakePaymentRepository {
	t.Helper()

	repository := &fakePaymentRepository{
		payments: map[string]payment.Payment{},
	}

	for _, paymentID := range paymentIDs {
		repository.payments[paymentID] = capturedPayment(t, paymentID)
	}

	return repository
}

func capturedPayment(t *testing.T, paymentID string) payment.Payment {
	t.Helper()

	now := testNow().Add(-time.Hour)
	paymentRecord, err := payment.NewAuthorizedPayment(payment.NewAuthorizedPaymentInput{
		ID:                  paymentID,
		MerchantID:          "merchant-1",
		PayerID:             "payer-1",
		AmountMinor:         4000,
		Currency:            "USD",
		AuthorizationHoldID: paymentID + "-hold",
		Now:                 now,
		ExpiresAt:           now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("new authorized payment: %v", err)
	}

	captured, err := paymentRecord.Capture(now.Add(30 * time.Minute))
	if err != nil {
		t.Fatalf("capture payment: %v", err)
	}

	return captured
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

type fakePaymentRepository struct {
	payments map[string]payment.Payment
}

func (r *fakePaymentRepository) CreatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	return payment.Payment{}, errors.New("not implemented")
}

func (r *fakePaymentRepository) GetPayment(ctx context.Context, paymentID string) (payment.Payment, error) {
	if err := ctx.Err(); err != nil {
		return payment.Payment{}, err
	}

	paymentRecord, ok := r.payments[paymentID]
	if !ok {
		return payment.Payment{}, payment.ErrPaymentNotFound
	}

	return paymentRecord, nil
}

func (r *fakePaymentRepository) UpdatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	if err := ctx.Err(); err != nil {
		return payment.Payment{}, err
	}

	r.payments[paymentRecord.ID] = paymentRecord
	return paymentRecord, nil
}

func (r *fakePaymentRepository) CreateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	return payment.Hold{}, errors.New("not implemented")
}

func (r *fakePaymentRepository) GetHold(ctx context.Context, holdID string) (payment.Hold, error) {
	return payment.Hold{}, errors.New("not implemented")
}

func (r *fakePaymentRepository) GetHoldByPaymentID(ctx context.Context, paymentID string) (payment.Hold, error) {
	return payment.Hold{}, errors.New("not implemented")
}

func (r *fakePaymentRepository) UpdateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	return payment.Hold{}, errors.New("not implemented")
}

type fakeOutboxRepository struct {
	created []outbox.Event
}

func (r *fakeOutboxRepository) CreateEvent(ctx context.Context, event outbox.Event) (outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return outbox.Event{}, err
	}

	r.created = append(r.created, event)
	return event, nil
}

func (r *fakeOutboxRepository) ClaimPendingEvents(ctx context.Context, input outbox.ClaimPendingEventsInput) ([]outbox.Event, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeOutboxRepository) MarkEventPublished(ctx context.Context, eventID string, now time.Time) (outbox.Event, error) {
	return outbox.Event{}, errors.New("not implemented")
}

func (r *fakeOutboxRepository) MarkEventFailed(ctx context.Context, input outbox.MarkEventFailedInput) (outbox.Event, error) {
	return outbox.Event{}, errors.New("not implemented")
}
