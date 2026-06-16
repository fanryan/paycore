package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/fanryan/paycore/internal/shared/id"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusPublished  Status = "PUBLISHED"
	StatusFailed     Status = "FAILED"
)

type Event struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Status        Status
	Attempts      int
	AvailableAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LockedAt      *time.Time
	LockedBy      *string
	PublishedAt   *time.Time
	LastError     *string
}

type NewEventInput struct {
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       any
	Now           time.Time
}

func NewEvent(input NewEventInput) (Event, error) {
	aggregateType := strings.TrimSpace(input.AggregateType)
	aggregateID := strings.TrimSpace(input.AggregateID)
	eventType := strings.TrimSpace(input.EventType)

	if aggregateType == "" {
		return Event{}, errors.New("aggregate type is required")
	}

	if aggregateID == "" {
		return Event{}, errors.New("aggregate id is required")
	}

	if eventType == "" {
		return Event{}, errors.New("event type is required")
	}

	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return Event{}, err
	}

	eventID, err := id.New("evt")
	if err != nil {
		return Event{}, err
	}

	now := input.Now.UTC()

	return Event{
		ID:            eventID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       payload,
		Status:        StatusPending,
		Attempts:      0,
		AvailableAt:   now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}
