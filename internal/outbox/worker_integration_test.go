package outbox_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	outboxkafka "github.com/fanryan/paycore/internal/outbox/adapters/kafka"
	outboxpostgres "github.com/fanryan/paycore/internal/outbox/adapters/postgres"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5/pgxpool"
	segmentio "github.com/segmentio/kafka-go"
)

func TestWorkerPublishesPostgresOutboxEventToKafka(t *testing.T) {
	databaseURL := os.Getenv("PAYCORE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PAYCORE_DATABASE_URL is not set")
	}

	brokerValue := os.Getenv("PAYCORE_KAFKA_BROKERS")
	if brokerValue == "" {
		t.Skip("PAYCORE_KAFKA_BROKERS is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create postgres pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	prefix := fmt.Sprintf("outbox-worker-kafka-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupIntegrationOutboxEvents(t, databaseURL, prefix)
	})

	brokers := outboxkafka.BrokersFromString(brokerValue)
	topic := fmt.Sprintf("paycore.outbox.worker.test.%d", time.Now().UnixNano())
	createKafkaTopic(t, ctx, brokers[0], topic)

	publisher, err := outboxkafka.NewPublisher(outboxkafka.PublisherConfig{
		Brokers: brokers,
		Topic:   topic,
	})
	if err != nil {
		t.Fatalf("new kafka publisher: %v", err)
	}
	defer publisher.Close()

	store := outboxpostgres.NewStore(pool)
	event, err := outbox.NewEvent(outbox.NewEventInput{
		AggregateType: "payment",
		AggregateID:   prefix + "-payment-1",
		EventType:     "payment.authorized",
		Payload: map[string]any{
			"payment_id": prefix + "-payment-1",
			"status":     "AUTHORIZED",
		},
		Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("new outbox event: %v", err)
	}

	if _, err := store.CreateEvent(ctx, event); err != nil {
		t.Fatalf("create outbox event: %v", err)
	}

	worker, err := outbox.NewWorker(outbox.WorkerConfig{
		Repository: store,
		Publisher:  publisher,
		Transactor: db.NewPostgresTransactor(pool),
		WorkerID:   "integration-worker",
		BatchSize:  10,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}

	if result.Claimed != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("expected claimed=1 published=1 failed=0, got %+v", result)
	}

	assertIntegrationOutboxEventPublished(t, pool, event.ID)
	message := readKafkaMessage(t, ctx, brokers, topic)

	if string(message.Key) != event.AggregateID {
		t.Fatalf("expected kafka key %q, got %q", event.AggregateID, string(message.Key))
	}

	var decoded map[string]any
	if err := json.Unmarshal(message.Value, &decoded); err != nil {
		t.Fatalf("decode kafka value: %v", err)
	}

	if decoded["payment_id"] != event.AggregateID {
		t.Fatalf("expected payment_id %q, got %v", event.AggregateID, decoded["payment_id"])
	}

	assertKafkaHeader(t, message.Headers, "event_id", event.ID)
	assertKafkaHeader(t, message.Headers, "event_type", event.EventType)
	assertKafkaHeader(t, message.Headers, "aggregate_type", event.AggregateType)
	assertKafkaHeader(t, message.Headers, "aggregate_id", event.AggregateID)
}

func createKafkaTopic(t *testing.T, ctx context.Context, broker string, topic string) {
	t.Helper()

	conn, err := segmentio.DialContext(ctx, "tcp", broker)
	if err != nil {
		t.Fatalf("dial kafka broker: %v", err)
	}
	defer conn.Close()

	err = conn.CreateTopics(segmentio.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil {
		t.Fatalf("create kafka topic: %v", err)
	}
}

func readKafkaMessage(t *testing.T, ctx context.Context, brokers []string, topic string) segmentio.Message {
	t.Helper()

	reader := segmentio.NewReader(segmentio.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		Partition:   0,
		StartOffset: segmentio.FirstOffset,
		MinBytes:    1,
		MaxBytes:    1e6,
		MaxWait:     time.Second,
	})
	defer reader.Close()

	message, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read kafka message: %v", err)
	}

	return message
}

func assertKafkaHeader(t *testing.T, headers []segmentio.Header, key string, expected string) {
	t.Helper()

	for _, header := range headers {
		if header.Key == key {
			if string(header.Value) != expected {
				t.Fatalf("expected kafka header %s=%q, got %q", key, expected, string(header.Value))
			}

			return
		}
	}

	t.Fatalf("expected kafka header %s", key)
}

func assertIntegrationOutboxEventPublished(t *testing.T, pool *pgxpool.Pool, eventID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var status string
	var publishedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, published_at
		FROM outbox_events
		WHERE id = $1
	`, eventID).Scan(&status, &publishedAt); err != nil {
		t.Fatalf("query outbox event: %v", err)
	}

	if status != string(outbox.StatusPublished) {
		t.Fatalf("expected outbox event published, got %q", status)
	}

	if publishedAt == nil {
		t.Fatal("expected published_at")
	}
}

func cleanupIntegrationOutboxEvents(t *testing.T, databaseURL string, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create cleanup pool: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, "DELETE FROM outbox_events WHERE aggregate_id LIKE $1", prefix+"%"); err != nil {
		t.Fatalf("cleanup outbox events: %v", err)
	}
}
