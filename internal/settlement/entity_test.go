package settlement_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/settlement"
)

func TestNewBatchCreatesCreatedSettlementBatch(t *testing.T) {
	now := testNow()

	batch, err := settlement.NewBatch(settlement.NewBatchInput{
		ID:          "setbat-1",
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected batch create to succeed, got error: %v", err)
	}

	if batch.ID != "setbat-1" {
		t.Fatalf("expected batch id setbat-1, got %q", batch.ID)
	}

	if batch.Status != settlement.BatchStatusCreated {
		t.Fatalf("expected status CREATED, got %q", batch.Status)
	}

	if !batch.WindowStart.Equal(now.Add(-time.Hour)) {
		t.Fatalf("expected window start %s, got %s", now.Add(-time.Hour), batch.WindowStart)
	}

	if !batch.WindowEnd.Equal(now) {
		t.Fatalf("expected window end %s, got %s", now, batch.WindowEnd)
	}
}

func TestNewBatchRejectsInvalidWindow(t *testing.T) {
	now := testNow()

	_, err := settlement.NewBatch(settlement.NewBatchInput{
		ID:          "setbat-1",
		WindowStart: now,
		WindowEnd:   now,
		Now:         now,
	})
	if !errors.Is(err, settlement.ErrInvalidSettlementWindow) {
		t.Fatalf("expected ErrInvalidSettlementWindow, got %v", err)
	}
}

func TestBatchProcessingLifecycle(t *testing.T) {
	now := testNow()
	batch := newBatch(t)

	processing, err := batch.StartProcessing("worker-1", now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("expected start processing to succeed, got error: %v", err)
	}

	if processing.Status != settlement.BatchStatusProcessing {
		t.Fatalf("expected PROCESSING, got %q", processing.Status)
	}

	if processing.ClaimedBy == nil || *processing.ClaimedBy != "worker-1" {
		t.Fatalf("expected claimed by worker-1, got %v", processing.ClaimedBy)
	}

	completed, err := processing.Complete(now.Add(30 * time.Second))
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	if completed.Status != settlement.BatchStatusCompleted {
		t.Fatalf("expected COMPLETED, got %q", completed.Status)
	}

	if completed.CompletedAt == nil {
		t.Fatal("expected completed_at")
	}

	if completed.LockedUntil != nil {
		t.Fatalf("expected lock cleared, got %v", completed.LockedUntil)
	}
}

func TestBatchCanFailFromProcessing(t *testing.T) {
	now := testNow()
	batch := newBatch(t)

	processing, err := batch.StartProcessing("worker-1", now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("expected start processing to succeed, got error: %v", err)
	}

	failed, err := processing.Fail("database unavailable", now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("expected fail to succeed, got error: %v", err)
	}

	if failed.Status != settlement.BatchStatusFailed {
		t.Fatalf("expected FAILED, got %q", failed.Status)
	}

	if failed.LastError == nil || *failed.LastError != "database unavailable" {
		t.Fatalf("expected last error database unavailable, got %v", failed.LastError)
	}
}

func TestBatchReportsStaleProcessingLock(t *testing.T) {
	now := testNow()
	batch := newBatch(t)

	processing, err := batch.StartProcessing("worker-1", now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("expected start processing to succeed, got error: %v", err)
	}

	if processing.IsStale(now.Add(30 * time.Second)) {
		t.Fatal("expected active lock to not be stale")
	}

	if !processing.IsStale(now.Add(time.Minute)) {
		t.Fatal("expected lock at expiry time to be stale")
	}
}

func TestNewLineItemComputesNetAmount(t *testing.T) {
	now := testNow()

	item, err := settlement.NewLineItem(settlement.NewLineItemInput{
		ID:              "setitem-1",
		BatchID:         "setbat-1",
		MerchantID:      "merchant-1",
		PaymentID:       "pay-1",
		AmountMinor:     10000,
		FeeAmountMinor:  250,
		Currency:        "usd",
		PaymentCaptured: now.Add(-time.Minute),
		Now:             now,
	})
	if err != nil {
		t.Fatalf("expected line item create to succeed, got error: %v", err)
	}

	if item.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", item.Currency)
	}

	if item.NetAmountMinor != 9750 {
		t.Fatalf("expected net amount 9750, got %d", item.NetAmountMinor)
	}
}

func TestNewLineItemRejectsInvalidAmounts(t *testing.T) {
	now := testNow()

	_, err := settlement.NewLineItem(settlement.NewLineItemInput{
		ID:              "setitem-1",
		BatchID:         "setbat-1",
		MerchantID:      "merchant-1",
		PaymentID:       "pay-1",
		AmountMinor:     0,
		Currency:        "USD",
		PaymentCaptured: now,
		Now:             now,
	})
	if !errors.Is(err, settlement.ErrInvalidLineItemAmount) {
		t.Fatalf("expected ErrInvalidLineItemAmount, got %v", err)
	}

	_, err = settlement.NewLineItem(settlement.NewLineItemInput{
		ID:              "setitem-1",
		BatchID:         "setbat-1",
		MerchantID:      "merchant-1",
		PaymentID:       "pay-1",
		AmountMinor:     100,
		FeeAmountMinor:  101,
		Currency:        "USD",
		PaymentCaptured: now,
		Now:             now,
	})
	if err == nil {
		t.Fatal("expected error when fee exceeds amount")
	}
}

func newBatch(t *testing.T) settlement.Batch {
	t.Helper()

	now := testNow()
	batch, err := settlement.NewBatch(settlement.NewBatchInput{
		ID:          "setbat-1",
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected batch create to succeed, got error: %v", err)
	}

	return batch
}

func testNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
