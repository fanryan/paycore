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

var (
	ErrEventNotClaimable   = errors.New("outbox event is not claimable")
	ErrEventNotPublishable = errors.New("outbox event is not publishable")
	ErrEventNotFailable    = errors.New("outbox event is not failable")
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

func (e Event) Claim(workerID string, now time.Time) (Event, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return Event{}, errors.New("worker id is required")
	}

	if e.Status != StatusPending && e.Status != StatusFailed {
		return Event{}, ErrEventNotClaimable
	}

	now = now.UTC()

	e.Status = StatusInProgress
	e.Attempts++
	e.LockedAt = &now
	e.LockedBy = &workerID
	e.LastError = nil
	e.UpdatedAt = now

	return e, nil
}

func (e Event) MarkPublished(now time.Time) (Event, error) {
	if e.Status != StatusInProgress {
		return Event{}, ErrEventNotPublishable
	}

	now = now.UTC()

	e.Status = StatusPublished
	e.PublishedAt = &now
	e.LockedAt = nil
	e.LockedBy = nil
	e.LastError = nil
	e.UpdatedAt = now

	return e, nil
}

func (e Event) MarkFailed(errorMessage string, nextAvailableAt time.Time, now time.Time) (Event, error) {
	if e.Status != StatusInProgress {
		return Event{}, ErrEventNotFailable
	}

	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		return Event{}, errors.New("error message is required")
	}

	now = now.UTC()
	nextAvailableAt = nextAvailableAt.UTC()

	e.Status = StatusFailed
	e.AvailableAt = nextAvailableAt
	e.LockedAt = nil
	e.LockedBy = nil
	e.PublishedAt = nil
	e.LastError = &errorMessage
	e.UpdatedAt = now

	return e, nil
}
