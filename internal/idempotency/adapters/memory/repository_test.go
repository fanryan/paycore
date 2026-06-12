package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	"github.com/fanryan/paycore/internal/idempotency/adapters/memory"
)

func TestRepositoryCreatesAndGetsRecord(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	record := testRecord(t, "key-1")

	created, err := store.CreateRecord(ctx, record)
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	got, err := store.GetRecord(ctx, created.Key)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if got.Key != record.Key {
		t.Fatalf("expected key %q, got %q", record.Key, got.Key)
	}

	if got.RequestHash != record.RequestHash {
		t.Fatalf("expected request hash %q, got %q", record.RequestHash, got.RequestHash)
	}
}

func TestRepositoryRejectsDuplicateKey(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	record := testRecord(t, "key-1")

	if _, err := store.CreateRecord(ctx, record); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	_, err := store.CreateRecord(ctx, record)
	if !errors.Is(err, idempotency.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got %v", err)
	}
}

func TestRepositoryReturnsNotFoundForMissingRecord(t *testing.T) {
	store := memory.NewStore()

	_, err := store.GetRecord(context.Background(), "missing")
	if !errors.Is(err, idempotency.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestRepositoryUpdatesRecord(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	record := testRecord(t, "key-1")

	created, err := store.CreateRecord(ctx, record)
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	completed, err := created.Complete(201, []byte(`{"payment_id":"pay_1"}`), testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	updated, err := store.UpdateRecord(ctx, completed)
	if err != nil {
		t.Fatalf("expected update to succeed, got error: %v", err)
	}

	if updated.Status != idempotency.StatusCompleted {
		t.Fatalf("expected status COMPLETED, got %q", updated.Status)
	}

	got, err := store.GetRecord(ctx, updated.Key)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if string(got.ResponseBody) != `{"payment_id":"pay_1"}` {
		t.Fatalf("expected stored response body, got %s", got.ResponseBody)
	}
}

func TestRepositoryRejectsUpdateForMissingRecord(t *testing.T) {
	store := memory.NewStore()

	_, err := store.UpdateRecord(context.Background(), testRecord(t, "missing"))
	if !errors.Is(err, idempotency.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestRepositoryClonesResponseBody(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	record := testRecord(t, "key-1")
	completed, err := record.Complete(201, []byte(`{"ok":true}`), testNow().Add(time.Minute))
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	created, err := store.CreateRecord(ctx, completed)
	if err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	created.ResponseBody[0] = '['

	got, err := store.GetRecord(ctx, completed.Key)
	if err != nil {
		t.Fatalf("expected get to succeed, got error: %v", err)
	}

	if string(got.ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected response body to be cloned, got %s", got.ResponseBody)
	}

	got.ResponseBody[0] = '['

	gotAgain, err := store.GetRecord(ctx, completed.Key)
	if err != nil {
		t.Fatalf("expected second get to succeed, got error: %v", err)
	}

	if string(gotAgain.ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected returned response body to be cloned, got %s", gotAgain.ResponseBody)
	}
}

func TestRepositoryHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store := memory.NewStore()
	record := testRecord(t, "key-1")

	if _, err := store.CreateRecord(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on create, got %v", err)
	}

	if _, err := store.GetRecord(ctx, record.Key); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on get, got %v", err)
	}

	if _, err := store.UpdateRecord(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled on update, got %v", err)
	}
}

func testRecord(t *testing.T, key string) idempotency.Record {
	t.Helper()

	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         key,
		RequestHash: "hash-" + key,
		Now:         testNow(),
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("expected record create to succeed, got error: %v", err)
	}

	return record
}

func testNow() time.Time {
	return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
}
