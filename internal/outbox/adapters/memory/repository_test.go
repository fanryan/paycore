package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
)

func TestStoreClaimsPendingEventsInAvailabilityOrder(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	now := testNow()

	later := testEvent(t, "payment-later", "payment.authorized", now.Add(time.Minute), now.Add(time.Minute))
	first := testEvent(t, "payment-first", "payment.authorized", now.Add(-time.Minute), now.Add(time.Minute))
	second := testEvent(t, "payment-second", "payment.authorized", now.Add(-time.Minute), now.Add(2*time.Minute))

	createEvents(t, store, later, second, first)

	claimed, err := store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
		WorkerID: "worker-1",
		Limit:    2,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed events, got %d", len(claimed))
	}

	if claimed[0].AggregateID != "payment-first" {
		t.Fatalf("expected first event payment-first, got %q", claimed[0].AggregateID)
	}

	if claimed[1].AggregateID != "payment-second" {
		t.Fatalf("expected second event payment-second, got %q", claimed[1].AggregateID)
	}

	for _, event := range claimed {
		if event.Status != outbox.StatusInProgress {
			t.Fatalf("expected in-progress status, got %q", event.Status)
		}

		if event.Attempts != 1 {
			t.Fatalf("expected attempts 1, got %d", event.Attempts)
		}

		if event.LockedBy == nil || *event.LockedBy != "worker-1" {
			t.Fatalf("expected locked_by worker-1, got %v", event.LockedBy)
		}
	}
}

func TestStoreDoesNotClaimFutureOrInProgressEvents(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	now := testNow()

	future := testEvent(t, "payment-future", "payment.authorized", now.Add(time.Minute), now)
	ready := testEvent(t, "payment-ready", "payment.authorized", now, now)

	inProgress, err := testEvent(t, "payment-progress", "payment.authorized", now, now).Claim("worker-1", now)
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	createEvents(t, store, future, ready, inProgress)

	claimed, err := store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
		WorkerID: "worker-2",
		Limit:    10,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed event, got %d", len(claimed))
	}

	if claimed[0].AggregateID != "payment-ready" {
		t.Fatalf("expected ready event, got %q", claimed[0].AggregateID)
	}
}

func TestStoreClaimsFailedEventsWhenAvailable(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	now := testNow()

	claimed, err := testEvent(t, "payment-1", "payment.authorized", now, now).Claim("worker-1", now)
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	failed, err := claimed.MarkFailed("temporary failure", now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected mark failed to succeed, got error: %v", err)
	}

	if _, err := store.CreateEvent(ctx, failed); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	reclaimed, err := store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
		WorkerID: "worker-2",
		Limit:    10,
		Now:      now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(reclaimed) != 1 {
		t.Fatalf("expected 1 reclaimed event, got %d", len(reclaimed))
	}

	if reclaimed[0].Attempts != 2 {
		t.Fatalf("expected attempts 2, got %d", reclaimed[0].Attempts)
	}

	if reclaimed[0].LastError != nil {
		t.Fatalf("expected last error to be cleared, got %v", reclaimed[0].LastError)
	}
}

func TestStoreMarksEventPublished(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	now := testNow()

	claimed := createClaimedEvent(t, store, now)

	published, err := store.MarkEventPublished(ctx, claimed.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected mark published to succeed, got error: %v", err)
	}

	if published.Status != outbox.StatusPublished {
		t.Fatalf("expected published status, got %q", published.Status)
	}

	if published.PublishedAt == nil {
		t.Fatal("expected published_at")
	}

	if published.LockedAt != nil {
		t.Fatalf("expected locked_at nil, got %v", published.LockedAt)
	}

	if published.LockedBy != nil {
		t.Fatalf("expected locked_by nil, got %v", published.LockedBy)
	}
}

func TestStoreMarksEventFailed(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	now := testNow()

	claimed := createClaimedEvent(t, store, now)
	nextAvailable := now.Add(5 * time.Minute)

	failed, err := store.MarkEventFailed(ctx, outbox.MarkEventFailedInput{
		EventID:       claimed.ID,
		ErrorMessage:  "publish failed",
		NextAvailable: nextAvailable,
		Now:           now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("expected mark failed to succeed, got error: %v", err)
	}

	if failed.Status != outbox.StatusFailed {
		t.Fatalf("expected failed status, got %q", failed.Status)
	}

	if !failed.AvailableAt.Equal(nextAvailable) {
		t.Fatalf("expected next available %s, got %s", nextAvailable, failed.AvailableAt)
	}

	if failed.LastError == nil || *failed.LastError != "publish failed" {
		t.Fatalf("expected last error publish failed, got %v", failed.LastError)
	}

	if failed.LockedAt != nil {
		t.Fatalf("expected locked_at nil, got %v", failed.LockedAt)
	}

	if failed.LockedBy != nil {
		t.Fatalf("expected locked_by nil, got %v", failed.LockedBy)
	}
}

func TestStoreReturnsNotFoundWhenMarkingMissingEvent(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.MarkEventPublished(ctx, "missing", testNow())
	if !errors.Is(err, outbox.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}

	_, err = store.MarkEventFailed(ctx, outbox.MarkEventFailedInput{
		EventID:       "missing",
		ErrorMessage:  "publish failed",
		NextAvailable: testNow(),
		Now:           testNow(),
	})
	if !errors.Is(err, outbox.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}
}

func createEvents(t *testing.T, store *Store, events ...outbox.Event) {
	t.Helper()

	for _, event := range events {
		if _, err := store.CreateEvent(context.Background(), event); err != nil {
			t.Fatalf("expected create to succeed, got error: %v", err)
		}
	}
}

func createClaimedEvent(t *testing.T, store *Store, now time.Time) outbox.Event {
	t.Helper()

	event := testEvent(t, "payment-1", "payment.authorized", now, now)
	if _, err := store.CreateEvent(context.Background(), event); err != nil {
		t.Fatalf("expected create to succeed, got error: %v", err)
	}

	claimed, err := store.ClaimPendingEvents(context.Background(), outbox.ClaimPendingEventsInput{
		WorkerID: "worker-1",
		Limit:    1,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed event, got %d", len(claimed))
	}

	return claimed[0]
}

func testEvent(t *testing.T, aggregateID string, eventType string, availableAt time.Time, createdAt time.Time) outbox.Event {
	t.Helper()

	event, err := outbox.NewEvent(outbox.NewEventInput{
		AggregateType: "payment",
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       map[string]any{"payment_id": aggregateID},
		Now:           createdAt,
	})
	if err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	event.AvailableAt = availableAt.UTC()

	return event
}

func testNow() time.Time {
	return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
}
