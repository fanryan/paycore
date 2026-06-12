package idempotency_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	"github.com/fanryan/paycore/internal/idempotency/adapters/memory"
)

func TestServiceStartsNewRequest(t *testing.T) {
	service := newService()

	result, err := service.StartRequest(context.Background(), idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("expected start to succeed, got error: %v", err)
	}

	if result.Replay {
		t.Fatal("expected new request to not be replay")
	}

	if result.Record.Key != "key-1" {
		t.Fatalf("expected key key-1, got %q", result.Record.Key)
	}

	if result.Record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected status IN_PROGRESS, got %q", result.Record.Status)
	}
}

func TestServiceReplaysCompletedRequest(t *testing.T) {
	service := newService()
	ctx := context.Background()

	if _, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	}); err != nil {
		t.Fatalf("expected start to succeed, got error: %v", err)
	}

	if _, err := service.CompleteRequest(ctx, idempotency.CompleteRequestInput{
		Key:          "key-1",
		ResponseCode: 201,
		ResponseBody: []byte(`{"payment_id":"pay_1"}`),
	}); err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	result, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("expected replay start to succeed, got error: %v", err)
	}

	if !result.Replay {
		t.Fatal("expected completed request to replay")
	}

	if result.ResponseCode != 201 {
		t.Fatalf("expected response code 201, got %d", result.ResponseCode)
	}

	if string(result.ResponseBody) != `{"payment_id":"pay_1"}` {
		t.Fatalf("expected replay response body, got %s", result.ResponseBody)
	}
}

func TestServiceRejectsDuplicateInProgressRequest(t *testing.T) {
	service := newService()
	ctx := context.Background()

	if _, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	}); err != nil {
		t.Fatalf("expected first start to succeed, got error: %v", err)
	}

	_, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	})
	if !errors.Is(err, idempotency.ErrRequestInProgress) {
		t.Fatalf("expected ErrRequestInProgress, got %v", err)
	}
}

func TestServiceRejectsRequestHashMismatch(t *testing.T) {
	service := newService()
	ctx := context.Background()

	if _, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	}); err != nil {
		t.Fatalf("expected first start to succeed, got error: %v", err)
	}

	_, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "different-hash",
	})
	if !errors.Is(err, idempotency.ErrRequestHashMismatch) {
		t.Fatalf("expected ErrRequestHashMismatch, got %v", err)
	}
}

func TestServiceRejectsExpiredIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	service := idempotency.NewService(store, time.Hour)

	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         "key-1",
		RequestHash: "hash-1",
		Now:         time.Now().UTC().Add(-2 * time.Hour),
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("expected record create to succeed, got error: %v", err)
	}

	if _, err := store.CreateRecord(ctx, record); err != nil {
		t.Fatalf("expected store create to succeed, got error: %v", err)
	}

	_, err = service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	})
	if !errors.Is(err, idempotency.ErrExpiredIdempotencyKey) {
		t.Fatalf("expected ErrExpiredIdempotencyKey, got %v", err)
	}
}

func TestServiceCompletesRequest(t *testing.T) {
	service := newService()
	ctx := context.Background()

	if _, err := service.StartRequest(ctx, idempotency.StartRequestInput{
		Key:         "key-1",
		RequestHash: "hash-1",
	}); err != nil {
		t.Fatalf("expected start to succeed, got error: %v", err)
	}

	completed, err := service.CompleteRequest(ctx, idempotency.CompleteRequestInput{
		Key:          "key-1",
		ResponseCode: 201,
		ResponseBody: []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	if completed.Status != idempotency.StatusCompleted {
		t.Fatalf("expected status COMPLETED, got %q", completed.Status)
	}

	if completed.ResponseCode != 201 {
		t.Fatalf("expected response code 201, got %d", completed.ResponseCode)
	}
}

func TestServiceRejectsCompleteForMissingRequest(t *testing.T) {
	service := newService()

	_, err := service.CompleteRequest(context.Background(), idempotency.CompleteRequestInput{
		Key:          "missing",
		ResponseCode: 201,
		ResponseBody: []byte(`{}`),
	})
	if !errors.Is(err, idempotency.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func newService() *idempotency.Service {
	return idempotency.NewService(memory.NewStore(), time.Hour)
}
