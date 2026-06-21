package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	outboxmemory "github.com/fanryan/paycore/internal/outbox/adapters/memory"
	"github.com/fanryan/paycore/internal/shared/db"
)

func TestWorkerPublishesClaimedEvents(t *testing.T) {
	ctx := context.Background()
	store := outboxmemory.NewStore()
	publisher := &fakePublisher{}
	worker := newTestWorker(t, store, publisher)

	event := createEvent(t, store, "payment-1", "payment.authorized")

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("expected process batch to succeed, got error: %v", err)
	}

	if result.Claimed != 1 {
		t.Fatalf("expected claimed 1, got %d", result.Claimed)
	}

	if result.Published != 1 {
		t.Fatalf("expected published 1, got %d", result.Published)
	}

	if result.Failed != 0 {
		t.Fatalf("expected failed 0, got %d", result.Failed)
	}

	if len(publisher.published) != 1 {
		t.Fatalf("expected publisher to receive 1 event, got %d", len(publisher.published))
	}

	if publisher.published[0].ID != event.ID {
		t.Fatalf("expected published event %q, got %q", event.ID, publisher.published[0].ID)
	}

	stored := findEvent(t, store, event.ID)
	if stored.Status != outbox.StatusPublished {
		t.Fatalf("expected stored event published, got %q", stored.Status)
	}

	if stored.PublishedAt == nil {
		t.Fatal("expected published_at")
	}
}

func TestWorkerRecordsOutboxMetricsForPublishedEvents(t *testing.T) {
	ctx := context.Background()
	store := outboxmemory.NewStore()
	publisher := &fakePublisher{}
	metrics := &fakeMetricsRecorder{}
	worker := newTestWorkerWithMetrics(t, store, publisher, metrics)

	createEvent(t, store, "payment-1", "payment.authorized")

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("expected process batch to succeed, got error: %v", err)
	}

	if result.Published != 1 {
		t.Fatalf("expected published 1, got %d", result.Published)
	}

	if len(metrics.batches) != 1 {
		t.Fatalf("expected 1 metric batch, got %d", len(metrics.batches))
	}

	batch := metrics.batches[0]
	if batch.publisher != "test-publisher" {
		t.Fatalf("expected publisher test-publisher, got %q", batch.publisher)
	}

	if batch.claimed != 1 || batch.published != 1 || batch.failed != 0 {
		t.Fatalf("unexpected metric batch: %+v", batch)
	}
}

func TestWorkerMarksFailedPublishForRetry(t *testing.T) {
	ctx := context.Background()
	store := outboxmemory.NewStore()
	publisher := &fakePublisher{
		err: errors.New("publisher unavailable"),
	}
	worker := newTestWorker(t, store, publisher)

	event := createEvent(t, store, "payment-1", "payment.authorized")

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("expected process batch to succeed, got error: %v", err)
	}

	if result.Claimed != 1 {
		t.Fatalf("expected claimed 1, got %d", result.Claimed)
	}

	if result.Published != 0 {
		t.Fatalf("expected published 0, got %d", result.Published)
	}

	if result.Failed != 1 {
		t.Fatalf("expected failed 1, got %d", result.Failed)
	}

	stored := findEvent(t, store, event.ID)
	if stored.Status != outbox.StatusFailed {
		t.Fatalf("expected stored event failed, got %q", stored.Status)
	}

	if stored.LastError == nil || *stored.LastError != "publisher unavailable" {
		t.Fatalf("expected last error publisher unavailable, got %v", stored.LastError)
	}

	if !stored.AvailableAt.After(testNow()) {
		t.Fatalf("expected retry availability after now, got %s", stored.AvailableAt)
	}
}

func TestWorkerRecordsOutboxMetricsForFailedPublish(t *testing.T) {
	ctx := context.Background()
	store := outboxmemory.NewStore()
	publisher := &fakePublisher{
		err: errors.New("publisher unavailable"),
	}
	metrics := &fakeMetricsRecorder{}
	worker := newTestWorkerWithMetrics(t, store, publisher, metrics)

	createEvent(t, store, "payment-1", "payment.authorized")

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("expected process batch to succeed, got error: %v", err)
	}

	if result.Failed != 1 {
		t.Fatalf("expected failed 1, got %d", result.Failed)
	}

	if len(metrics.batches) != 1 {
		t.Fatalf("expected 1 metric batch, got %d", len(metrics.batches))
	}

	batch := metrics.batches[0]
	if batch.claimed != 1 || batch.published != 0 || batch.failed != 1 {
		t.Fatalf("unexpected metric batch: %+v", batch)
	}
}

func TestWorkerReturnsEmptyResultWhenNoEventsAreClaimed(t *testing.T) {
	ctx := context.Background()
	store := outboxmemory.NewStore()
	publisher := &fakePublisher{}
	worker := newTestWorker(t, store, publisher)

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("expected process batch to succeed, got error: %v", err)
	}

	if result.Claimed != 0 || result.Published != 0 || result.Failed != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}

	if len(publisher.published) != 0 {
		t.Fatalf("expected publisher to receive no events, got %d", len(publisher.published))
	}
}

func TestNewWorkerValidatesRequiredDependencies(t *testing.T) {
	_, err := outbox.NewWorker(outbox.WorkerConfig{
		Publisher:  &fakePublisher{},
		Transactor: db.NoopTransactor{},
	})
	if !errors.Is(err, outbox.ErrRepositoryRequired) {
		t.Fatalf("expected ErrRepositoryRequired, got %v", err)
	}

	_, err = outbox.NewWorker(outbox.WorkerConfig{
		Repository: outboxmemory.NewStore(),
		Transactor: db.NoopTransactor{},
	})
	if !errors.Is(err, outbox.ErrPublisherRequired) {
		t.Fatalf("expected ErrPublisherRequired, got %v", err)
	}

	_, err = outbox.NewWorker(outbox.WorkerConfig{
		Repository: outboxmemory.NewStore(),
		Publisher:  &fakePublisher{},
	})
	if !errors.Is(err, outbox.ErrTransactorRequired) {
		t.Fatalf("expected ErrTransactorRequired, got %v", err)
	}
}

func newTestWorker(t *testing.T, repository outbox.Repository, publisher outbox.Publisher) *outbox.Worker {
	t.Helper()

	return newTestWorkerWithMetrics(t, repository, publisher, nil)
}

func newTestWorkerWithMetrics(t *testing.T, repository outbox.Repository, publisher outbox.Publisher, metrics outbox.MetricsRecorder) *outbox.Worker {
	t.Helper()

	worker, err := outbox.NewWorker(outbox.WorkerConfig{
		Repository:    repository,
		Publisher:     publisher,
		Transactor:    db.NoopTransactor{},
		Metrics:       metrics,
		WorkerID:      "worker-1",
		BatchSize:     10,
		PublisherName: "test-publisher",
		Now:           testNow,
	})
	if err != nil {
		t.Fatalf("expected worker create to succeed, got error: %v", err)
	}

	return worker
}

func createEvent(t *testing.T, store *outboxmemory.Store, aggregateID string, eventType string) outbox.Event {
	t.Helper()

	event, err := outbox.NewEvent(outbox.NewEventInput{
		AggregateType: "payment",
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       map[string]any{"payment_id": aggregateID},
		Now:           testNow(),
	})
	if err != nil {
		t.Fatalf("expected event create to succeed, got error: %v", err)
	}

	created, err := store.CreateEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("expected store create to succeed, got error: %v", err)
	}

	return created
}

func findEvent(t *testing.T, store *outboxmemory.Store, eventID string) outbox.Event {
	t.Helper()

	events, err := store.ListEvents(context.Background())
	if err != nil {
		t.Fatalf("expected list events to succeed, got error: %v", err)
	}

	for _, event := range events {
		if event.ID == eventID {
			return event
		}
	}

	t.Fatalf("expected event %q", eventID)
	return outbox.Event{}
}

type fakePublisher struct {
	err       error
	published []outbox.Event
}

func (p *fakePublisher) Publish(ctx context.Context, event outbox.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if p.err != nil {
		return p.err
	}

	p.published = append(p.published, event)

	return nil
}

type fakeMetricsRecorder struct {
	batches []fakeOutboxBatchMetric
}

type fakeOutboxBatchMetric struct {
	publisher string
	claimed   int
	published int
	failed    int
}

func (r *fakeMetricsRecorder) ObserveOutboxBatch(publisher string, claimed int, published int, failed int) {
	r.batches = append(r.batches, fakeOutboxBatchMetric{
		publisher: publisher,
		claimed:   claimed,
		published: published,
		failed:    failed,
	})
}

func testNow() time.Time {
	return time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
}
