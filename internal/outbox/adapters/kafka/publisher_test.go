package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	segmentio "github.com/segmentio/kafka-go"
)

func TestBrokersFromStringTrimsAndDropsEmptyValues(t *testing.T) {
	brokers := BrokersFromString(" localhost:9092, kafka:9092 ,, ")

	if len(brokers) != 2 {
		t.Fatalf("expected 2 brokers, got %d", len(brokers))
	}

	if brokers[0] != "localhost:9092" {
		t.Fatalf("expected first broker localhost:9092, got %q", brokers[0])
	}

	if brokers[1] != "kafka:9092" {
		t.Fatalf("expected second broker kafka:9092, got %q", brokers[1])
	}
}

func TestNewPublisherRequiresBroker(t *testing.T) {
	_, err := NewPublisher(PublisherConfig{
		Brokers: []string{" ", ""},
		Topic:   "paycore.outbox.events",
	})

	if !errors.Is(err, ErrBrokerRequired) {
		t.Fatalf("expected ErrBrokerRequired, got %v", err)
	}
}

func TestNewPublisherUsesDefaultTopic(t *testing.T) {
	publisher, err := NewPublisher(PublisherConfig{
		Brokers: []string{"localhost:9092"},
	})
	if err != nil {
		t.Fatalf("expected publisher, got error %v", err)
	}
	defer publisher.Close()
}

func TestPublisherPublishesOutboxEventToKafka(t *testing.T) {
	brokerValue := os.Getenv("PAYCORE_KAFKA_BROKERS")
	if brokerValue == "" {
		t.Skip("PAYCORE_KAFKA_BROKERS is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	brokers := BrokersFromString(brokerValue)
	topic := fmt.Sprintf("paycore.outbox.events.test.%d", time.Now().UnixNano())
	createTopic(t, ctx, brokers[0], topic)

	publisher, err := NewPublisher(PublisherConfig{
		Brokers: brokers,
		Topic:   topic,
	})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer publisher.Close()

	payload := map[string]any{
		"payment_id": "payment-1",
		"status":     "AUTHORIZED",
	}

	event, err := outbox.NewEvent(outbox.NewEventInput{
		AggregateType: "payment",
		AggregateID:   "payment-1",
		EventType:     "payment.authorized",
		Payload:       payload,
		Now:           time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("new event: %v", err)
	}

	if err := publisher.Publish(ctx, event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

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

	if string(message.Key) != event.AggregateID {
		t.Fatalf("expected message key %q, got %q", event.AggregateID, string(message.Key))
	}

	if string(message.Value) != string(event.Payload) {
		t.Fatalf("expected payload %s, got %s", string(event.Payload), string(message.Value))
	}

	var decoded map[string]any
	if err := json.Unmarshal(message.Value, &decoded); err != nil {
		t.Fatalf("decode message payload: %v", err)
	}

	if decoded["payment_id"] != "payment-1" {
		t.Fatalf("expected payment_id payment-1, got %v", decoded["payment_id"])
	}

	assertHeader(t, message.Headers, "event_id", event.ID)
	assertHeader(t, message.Headers, "event_type", event.EventType)
	assertHeader(t, message.Headers, "aggregate_type", event.AggregateType)
	assertHeader(t, message.Headers, "aggregate_id", event.AggregateID)
}

func createTopic(t *testing.T, ctx context.Context, broker string, topic string) {
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

func assertHeader(t *testing.T, headers []segmentio.Header, key string, expected string) {
	t.Helper()

	for _, header := range headers {
		if header.Key == key {
			if string(header.Value) != expected {
				t.Fatalf("expected header %s=%q, got %q", key, expected, string(header.Value))
			}

			return
		}
	}

	t.Fatalf("expected header %s", key)
}
