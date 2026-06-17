package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryCreatesOutboxEvent(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	event := testEvent(t, prefix+"-payment-1", "payment.authorized")

	created, err := store.CreateEvent(ctx, event)
	if err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	if created.ID != event.ID {
		t.Fatalf("expected event id %q, got %q", event.ID, created.ID)
	}

	if created.AggregateType != "payment" {
		t.Fatalf("expected aggregate type payment, got %q", created.AggregateType)
	}

	if created.AggregateID != prefix+"-payment-1" {
		t.Fatalf("expected aggregate id %q, got %q", prefix+"-payment-1", created.AggregateID)
	}

	if created.EventType != "payment.authorized" {
		t.Fatalf("expected event type payment.authorized, got %q", created.EventType)
	}

	if created.Status != outbox.StatusPending {
		t.Fatalf("expected pending status, got %q", created.Status)
	}

	if !bytes.Equal(created.Payload, event.Payload) {
		t.Fatalf("expected payload %s, got %s", string(event.Payload), string(created.Payload))
	}

	if created.Attempts != 0 {
		t.Fatalf("expected attempts 0, got %d", created.Attempts)
	}
}

func TestRepositoryRejectsDuplicateOutboxEvent(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	event := testEvent(t, prefix+"-payment-1", "payment.authorized")

	if _, err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	_, err := store.CreateEvent(ctx, event)
	if !errors.Is(err, outbox.ErrDuplicateEvent) {
		t.Fatalf("expected ErrDuplicateEvent, got %v", err)
	}
}

func TestRepositoryCreateEventUsesContextTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	expectedErr := errors.New("force rollback")
	event := testEvent(t, prefix+"-payment-1", "payment.authorized")

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := store.CreateEvent(ctx, event); err != nil {
			return err
		}

		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if outboxEventExists(t, pool, event.ID) {
		t.Fatal("expected outbox event to be rolled back")
	}
}

func TestRepositoryClaimsPendingEventsInTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	now := testNow()
	later := testEventAt(t, prefix+"-payment-later", "payment.authorized", now.Add(time.Minute), now)
	first := testEventAt(t, prefix+"-payment-first", "payment.authorized", now.Add(-time.Minute), now.Add(time.Minute))
	second := testEventAt(t, prefix+"-payment-second", "payment.authorized", now.Add(-time.Minute), now.Add(2*time.Minute))

	createEvents(t, store, later, second, first)

	var claimed []outbox.Event
	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		claimed, err = store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
			WorkerID: "worker-1",
			Limit:    2,
			Now:      now,
		})

		return err
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed events, got %d", len(claimed))
	}

	if claimed[0].AggregateID != prefix+"-payment-first" {
		t.Fatalf("expected first event %q, got %q", prefix+"-payment-first", claimed[0].AggregateID)
	}

	if claimed[1].AggregateID != prefix+"-payment-second" {
		t.Fatalf("expected second event %q, got %q", prefix+"-payment-second", claimed[1].AggregateID)
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

func TestRepositoryClaimPendingEventsRequiresTransaction(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)

	_, err := store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
		WorkerID: "worker-1",
		Limit:    1,
		Now:      testNow(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRepositoryClaimPendingEventsRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	event := testEvent(t, prefix+"-payment-1", "payment.authorized")
	if _, err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	expectedErr := errors.New("force rollback")

	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		claimed, err := store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
			WorkerID: "worker-1",
			Limit:    1,
			Now:      testNow(),
		})
		if err != nil {
			return err
		}

		if len(claimed) != 1 {
			t.Fatalf("expected 1 claimed event, got %d", len(claimed))
		}

		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	status, attempts := getOutboxEventStatusAndAttempts(t, pool, event.ID)
	if status != outbox.StatusPending {
		t.Fatalf("expected pending status after rollback, got %q", status)
	}

	if attempts != 0 {
		t.Fatalf("expected attempts 0 after rollback, got %d", attempts)
	}
}

func TestRepositoryMarksEventPublished(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	claimed := createClaimedEvent(t, store, transactor, prefix)

	published, err := store.MarkEventPublished(ctx, claimed.ID, testNow().Add(time.Minute))
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

func TestRepositoryMarksEventFailed(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	transactor := db.NewPostgresTransactor(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	claimed := createClaimedEvent(t, store, transactor, prefix)
	nextAvailable := testNow().Add(5 * time.Minute)

	failed, err := store.MarkEventFailed(ctx, outbox.MarkEventFailedInput{
		EventID:       claimed.ID,
		ErrorMessage:  "publish failed",
		NextAvailable: nextAvailable,
		Now:           testNow().Add(time.Minute),
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
}

func TestRepositoryReturnsNotFoundWhenMarkingMissingOrUnclaimedEvent(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	store := outboxpostgres.NewStore(pool)
	prefix := testPrefix()
	t.Cleanup(func() {
		cleanupOutboxEvents(t, pool, prefix)
	})

	_, err := store.MarkEventPublished(ctx, "missing", testNow())
	if !errors.Is(err, outbox.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound, got %v", err)
	}

	event := testEvent(t, prefix+"-payment-1", "payment.authorized")
	if _, err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	_, err = store.MarkEventPublished(ctx, event.ID, testNow())
	if !errors.Is(err, outbox.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound for unclaimed event, got %v", err)
	}

	_, err = store.MarkEventFailed(ctx, outbox.MarkEventFailedInput{
		EventID:       event.ID,
		ErrorMessage:  "publish failed",
		NextAvailable: testNow(),
		Now:           testNow(),
	})
	if !errors.Is(err, outbox.ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound for unclaimed event, got %v", err)
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping postgres: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func cleanupOutboxEvents(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE $1", prefix+"%"); err != nil {
		t.Fatalf("failed to cleanup outbox events: %v", err)
	}
}

func outboxEventExists(t *testing.T, pool *pgxpool.Pool, eventID string) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM outbox_events WHERE id = $1)", eventID).Scan(&exists); err != nil {
		t.Fatalf("failed to check outbox event existence: %v", err)
	}

	return exists
}

func getOutboxEventStatusAndAttempts(t *testing.T, pool *pgxpool.Pool, eventID string) (outbox.Status, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var status string
	var attempts int
	if err := pool.QueryRow(ctx, "SELECT status, attempts FROM outbox_events WHERE id = $1", eventID).Scan(&status, &attempts); err != nil {
		t.Fatalf("failed to get outbox event status and attempts: %v", err)
	}

	return outbox.Status(status), attempts
}

func createEvents(t *testing.T, store *outboxpostgres.Store, events ...outbox.Event) {
	t.Helper()

	for _, event := range events {
		if _, err := store.CreateEvent(context.Background(), event); err != nil {
			t.Fatalf("expected event create to succeed, got error: %v", err)
		}
	}
}

func createClaimedEvent(t *testing.T, store *outboxpostgres.Store, transactor *db.PostgresTransactor, prefix string) outbox.Event {
	t.Helper()

	ctx := context.Background()
	event := testEvent(t, prefix+"-payment-1", "payment.authorized")
	if _, err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	var claimed []outbox.Event
	err := transactor.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		claimed, err = store.ClaimPendingEvents(ctx, outbox.ClaimPendingEventsInput{
			WorkerID: "worker-1",
			Limit:    1,
			Now:      testNow(),
		})

		return err
	})
	if err != nil {
		t.Fatalf("expected claim to succeed, got error: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed event, got %d", len(claimed))
	}

	return claimed[0]
}

func testEvent(t *testing.T, aggregateID string, eventType string) outbox.Event {
	t.Helper()

	return testEventAt(t, aggregateID, eventType, testNow(), testNow())
}

func testEventAt(t *testing.T, aggregateID string, eventType string, availableAt time.Time, createdAt time.Time) outbox.Event {
	t.Helper()

	event, err := outbox.NewEvent(outbox.NewEventInput{
		AggregateType: "payment",
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload: map[string]any{
			"payment_id": aggregateID,
			"amount":     4000,
			"currency":   "USD",
		},
		Now: createdAt,
	})
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}

	event.AvailableAt = availableAt.UTC()

	return event
}

func testPrefix() string {
	return "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
