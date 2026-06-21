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

func TestServiceRecordsSettlementBatchMetrics(t *testing.T) {
	repository := &fakeRepository{
		claimed: []settlement.ClaimedPayment{
			{
				PaymentID:   "pay-1",
				MerchantID:  "merchant-1",
				AmountMinor: 4000,
				Currency:    "USD",
				CapturedAt:  testNow().Add(-time.Minute),
			},
		},
	}
	metrics := &fakeMetricsRecorder{}
	service := newSettlementServiceWithMetrics(t, repository, newFakePaymentRepository(t, "pay-1"), &fakeOutboxRepository{}, metrics)

	_, err := service.CreateBatch(context.Background(), settlement.CreateBatchInput{
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow(),
	})
	if err != nil {
		t.Fatalf("expected create batch to succeed, got error: %v", err)
	}

	if len(metrics.batches) != 1 {
		t.Fatalf("expected 1 batch metric, got %d", len(metrics.batches))
	}

	if metrics.batches[0].status != string(settlement.BatchStatusCompleted) {
		t.Fatalf("expected COMPLETED metric, got %q", metrics.batches[0].status)
	}

	if metrics.batches[0].payments != 1 {
		t.Fatalf("expected 1 settled payment metric, got %d", metrics.batches[0].payments)
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

func TestServiceRecoversStaleProcessingBatch(t *testing.T) {
	staleBatch := staleProcessingBatch(t, "batch-stale-1")
	repository := &fakeRepository{
		batches: map[string]settlement.Batch{
			staleBatch.ID: staleBatch,
		},
		staleBatches: []settlement.Batch{staleBatch},
		claimedByBatch: map[string][]settlement.ClaimedPayment{
			staleBatch.ID: {
				{
					PaymentID:       "pay-1",
					MerchantID:      "merchant-1",
					AmountMinor:     4000,
					Currency:        "USD",
					CapturedAt:      testNow().Add(-time.Minute),
					SettlementBatch: staleBatch.ID,
				},
			},
		},
	}
	payments := newFakePaymentRepository(t, "pay-1")
	outboxRepository := &fakeOutboxRepository{}
	service := newSettlementService(t, repository, payments, outboxRepository)

	result, err := service.RecoverStaleBatches(context.Background(), settlement.RecoverStaleBatchesInput{
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("expected recovery to succeed, got error: %v", err)
	}

	if repository.staleInput.Limit != 5 {
		t.Fatalf("expected stale lookup limit 5, got %d", repository.staleInput.Limit)
	}

	if len(result.Batches) != 1 {
		t.Fatalf("expected 1 recovered batch, got %d", len(result.Batches))
	}

	if result.Batches[0].ID != staleBatch.ID {
		t.Fatalf("expected recovered batch %q, got %q", staleBatch.ID, result.Batches[0].ID)
	}

	if result.Batches[0].Status != settlement.BatchStatusCompleted {
		t.Fatalf("expected recovered batch COMPLETED, got %q", result.Batches[0].Status)
	}

	if len(result.LineItems) != 1 {
		t.Fatalf("expected 1 line item, got %d", len(result.LineItems))
	}

	if result.LineItems[0].BatchID != staleBatch.ID {
		t.Fatalf("expected line item batch %q, got %q", staleBatch.ID, result.LineItems[0].BatchID)
	}

	if len(result.Payments) != 1 {
		t.Fatalf("expected 1 settled payment, got %d", len(result.Payments))
	}

	if result.Payments[0].Status != payment.StatusSettled {
		t.Fatalf("expected recovered payment SETTLED, got %q", result.Payments[0].Status)
	}

	if len(outboxRepository.created) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(outboxRepository.created))
	}
}

func TestServiceRecordsStaleRecoveryMetrics(t *testing.T) {
	staleBatch := staleProcessingBatch(t, "batch-stale-1")
	repository := &fakeRepository{
		batches: map[string]settlement.Batch{
			staleBatch.ID: staleBatch,
		},
		staleBatches: []settlement.Batch{staleBatch},
		claimedByBatch: map[string][]settlement.ClaimedPayment{
			staleBatch.ID: {
				{
					PaymentID:       "pay-1",
					MerchantID:      "merchant-1",
					AmountMinor:     4000,
					Currency:        "USD",
					CapturedAt:      testNow().Add(-time.Minute),
					SettlementBatch: staleBatch.ID,
				},
			},
		},
	}
	metrics := &fakeMetricsRecorder{}
	service := newSettlementServiceWithMetrics(t, repository, newFakePaymentRepository(t, "pay-1"), &fakeOutboxRepository{}, metrics)

	_, err := service.RecoverStaleBatches(context.Background(), settlement.RecoverStaleBatchesInput{
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("expected recovery to succeed, got error: %v", err)
	}

	if metrics.recoveredBatches != 1 {
		t.Fatalf("expected 1 recovered batch metric, got %d", metrics.recoveredBatches)
	}

	if len(metrics.batches) != 1 {
		t.Fatalf("expected 1 batch metric, got %d", len(metrics.batches))
	}

	if metrics.batches[0].payments != 1 {
		t.Fatalf("expected 1 recovered payment metric, got %d", metrics.batches[0].payments)
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

	return newSettlementServiceWithMetrics(t, repository, payments, outboxRepository, nil)
}

func newSettlementServiceWithMetrics(t *testing.T, repository settlement.Repository, payments payment.Repository, outboxRepository outbox.Repository, metrics settlement.MetricsRecorder) *settlement.Service {
	t.Helper()

	service, err := settlement.NewService(settlement.ServiceConfig{
		Repository: repository,
		Payments:   payments,
		Outbox:     outboxRepository,
		Transactor: db.NoopTransactor{},
		Metrics:    metrics,
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

func staleProcessingBatch(t *testing.T, batchID string) settlement.Batch {
	t.Helper()

	batch, err := settlement.NewBatch(settlement.NewBatchInput{
		ID:          batchID,
		WindowStart: testNow().Add(-time.Hour),
		WindowEnd:   testNow(),
		Now:         testNow().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("new stale batch: %v", err)
	}

	processing, err := batch.StartProcessing("old-worker", testNow().Add(-time.Minute), testNow().Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("start stale batch processing: %v", err)
	}

	return processing
}

type fakeRepository struct {
	batches        map[string]settlement.Batch
	lineItems      []settlement.LineItem
	claimed        []settlement.ClaimedPayment
	claimedByBatch map[string][]settlement.ClaimedPayment
	staleBatches   []settlement.Batch
	staleInput     settlement.ListStaleProcessingBatchesInput
	claimInput     settlement.ClaimCapturedPaymentsInput
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

func (r *fakeRepository) ListStaleProcessingBatches(ctx context.Context, input settlement.ListStaleProcessingBatchesInput) ([]settlement.Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.staleInput = input

	batches := make([]settlement.Batch, len(r.staleBatches))
	copy(batches, r.staleBatches)

	return batches, nil
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

func (r *fakeRepository) ListClaimedPayments(ctx context.Context, batchID string) ([]settlement.ClaimedPayment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	claimed := make([]settlement.ClaimedPayment, len(r.claimedByBatch[batchID]))
	copy(claimed, r.claimedByBatch[batchID])

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

func (r *fakeOutboxRepository) Stats(ctx context.Context, input outbox.StatsInput) (outbox.Stats, error) {
	return outbox.Stats{}, errors.New("not implemented")
}

type fakeMetricsRecorder struct {
	batches          []fakeSettlementBatchMetric
	recoveredBatches int
}

type fakeSettlementBatchMetric struct {
	status   string
	payments int
	duration time.Duration
}

func (r *fakeMetricsRecorder) ObserveSettlementBatch(status string, payments int, duration time.Duration) {
	r.batches = append(r.batches, fakeSettlementBatchMetric{
		status:   status,
		payments: payments,
		duration: duration,
	})
}

func (r *fakeMetricsRecorder) ObserveSettlementRecoveredBatches(count int) {
	r.recoveredBatches += count
}
