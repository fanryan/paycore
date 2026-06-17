package kafka

import (
	"errors"
	"testing"
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
