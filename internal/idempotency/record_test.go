package idempotency_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
)

func TestNewRecordCreatesInProgressRecord(t *testing.T) {
	now := testNow()

	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         " key-1 ",
		RequestHash: " hash-1 ",
		Now:         now,
		TTL:         time.Hour,
	})
	if err != nil {
		t.Fatalf("expected record create to succeed, got error: %v", err)
	}

	if record.Key != "key-1" {
		t.Fatalf("expected trimmed key key-1, got %q", record.Key)
	}

	if record.RequestHash != "hash-1" {
		t.Fatalf("expected trimmed request hash hash-1, got %q", record.RequestHash)
	}

	if record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected status IN_PROGRESS, got %q", record.Status)
	}

	if !record.CreatedAt.Equal(now) {
		t.Fatalf("expected created at %s, got %s", now, record.CreatedAt)
	}

	if !record.UpdatedAt.Equal(now) {
		t.Fatalf("expected updated at %s, got %s", now, record.UpdatedAt)
	}

	if !record.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expected expires at %s, got %s", now.Add(time.Hour), record.ExpiresAt)
	}
}

func TestNewRecordDefaultsTTL(t *testing.T) {
	now := testNow()

	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         "key-1",
		RequestHash: "hash-1",
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected record create to succeed, got error: %v", err)
	}

	if !record.ExpiresAt.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("expected default ttl of 24 hours, got expires at %s", record.ExpiresAt)
	}
}

func TestNewRecordRejectsMissingKey(t *testing.T) {
	_, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         " ",
		RequestHash: "hash-1",
		Now:         testNow(),
	})
	if !errors.Is(err, idempotency.ErrInvalidKey) {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestNewRecordRejectsMissingRequestHash(t *testing.T) {
	_, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         "key-1",
		RequestHash: " ",
		Now:         testNow(),
	})
	if !errors.Is(err, idempotency.ErrInvalidRequestHash) {
		t.Fatalf("expected ErrInvalidRequestHash, got %v", err)
	}
}

func TestRecordCompleteStoresResponse(t *testing.T) {
	now := testNow()
	record, err := idempotency.NewRecord(idempotency.NewRecordInput{
		Key:         "key-1",
		RequestHash: "hash-1",
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected record create to succeed, got error: %v", err)
	}

	responseBody := []byte(`{"ok":true}`)
	completed, err := record.Complete(201, responseBody, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected complete to succeed, got error: %v", err)
	}

	responseBody[0] = '['

	if completed.Status != idempotency.StatusCompleted {
		t.Fatalf("expected status COMPLETED, got %q", completed.Status)
	}

	if completed.ResponseCode != 201 {
		t.Fatalf("expected response code 201, got %d", completed.ResponseCode)
	}

	if string(completed.ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected copied response body, got %s", completed.ResponseBody)
	}

	if !completed.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("expected updated at %s, got %s", now.Add(time.Minute), completed.UpdatedAt)
	}
}

func TestRecordCompleteRejectsNonInProgressRecord(t *testing.T) {
	record := idempotency.Record{
		Key:         "key-1",
		RequestHash: "hash-1",
		Status:      idempotency.StatusCompleted,
	}

	_, err := record.Complete(200, []byte(`{}`), testNow())
	if !errors.Is(err, idempotency.ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestRecordIsExpired(t *testing.T) {
	record := idempotency.Record{
		ExpiresAt: testNow().Add(time.Hour),
	}

	if record.IsExpired(testNow().Add(30 * time.Minute)) {
		t.Fatal("expected record to not be expired before expiry")
	}

	if !record.IsExpired(testNow().Add(2 * time.Hour)) {
		t.Fatal("expected record to be expired after expiry")
	}
}

func TestHashRequestBodyIsStable(t *testing.T) {
	first := idempotency.HashRequestBody([]byte(`{"amount":4000}`))
	second := idempotency.HashRequestBody([]byte(`{"amount":4000}`))
	third := idempotency.HashRequestBody([]byte(`{"amount":5000}`))

	if first == "" {
		t.Fatal("expected hash to be populated")
	}

	if first != second {
		t.Fatalf("expected same body to hash equally, got %q and %q", first, second)
	}

	if first == third {
		t.Fatal("expected different body to hash differently")
	}
}

func testNow() time.Time {
	return time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
}
