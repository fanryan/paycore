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

func testEvent(t *testing.T, aggregateID string, eventType string) outbox.Event {
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
		Now: testNow(),
	})
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}

	return event
}

func testPrefix() string {
	return "test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
