package kafka

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	segmentio "github.com/segmentio/kafka-go"
)

const defaultTopic = "paycore.outbox.events"

var (
	ErrBrokerRequired = errors.New("at least one kafka broker is required")
	ErrTopicRequired  = errors.New("kafka topic is required")
)

type Publisher struct {
	writer *segmentio.Writer
}

type PublisherConfig struct {
	Brokers []string
	Topic   string
}

func NewPublisher(config PublisherConfig) (*Publisher, error) {
	brokers := cleanBrokers(config.Brokers)
	if len(brokers) == 0 {
		return nil, ErrBrokerRequired
	}

	topic := strings.TrimSpace(config.Topic)
	if topic == "" {
		topic = defaultTopic
	}
	if topic == "" {
		return nil, ErrTopicRequired
	}

	return &Publisher{
		writer: &segmentio.Writer{
			Addr:         segmentio.TCP(brokers...),
			Topic:        topic,
			Balancer:     &segmentio.Hash{},
			BatchTimeout: 10 * time.Millisecond,
			RequiredAcks: segmentio.RequireAll,
		},
	}, nil
}

func (p *Publisher) Publish(ctx context.Context, event outbox.Event) error {
	return p.writer.WriteMessages(ctx, segmentio.Message{
		Key:   []byte(event.AggregateID),
		Value: event.Payload,
		Headers: []segmentio.Header{
			{Key: "event_id", Value: []byte(event.ID)},
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "aggregate_type", Value: []byte(event.AggregateType)},
			{Key: "aggregate_id", Value: []byte(event.AggregateID)},
		},
		Time: event.CreatedAt,
	})
}

func (p *Publisher) Close() error {
	return p.writer.Close()
}

func BrokersFromString(value string) []string {
	return cleanBrokers(strings.Split(value, ","))
}

func cleanBrokers(values []string) []string {
	brokers := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			brokers = append(brokers, value)
		}
	}

	return brokers
}
