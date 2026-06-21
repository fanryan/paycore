package outbox

import (
	"context"
	"errors"
	"time"
)

var (
	ErrEventNotFound  = errors.New("outbox event not found")
	ErrDuplicateEvent = errors.New("outbox event already exists")
)

type ClaimPendingEventsInput struct {
	WorkerID string
	Limit    int
	Now      time.Time
}

type MarkEventFailedInput struct {
	EventID       string
	ErrorMessage  string
	NextAvailable time.Time
	Now           time.Time
}

type StatsInput struct {
	Now time.Time
}

type Stats struct {
	PendingEvents int
	PublishLag    time.Duration
}

type Repository interface {
	CreateEvent(ctx context.Context, event Event) (Event, error)
	ClaimPendingEvents(ctx context.Context, input ClaimPendingEventsInput) ([]Event, error)
	MarkEventPublished(ctx context.Context, eventID string, now time.Time) (Event, error)
	MarkEventFailed(ctx context.Context, input MarkEventFailedInput) (Event, error)
	Stats(ctx context.Context, input StatsInput) (Stats, error)
}

type NoopRepository struct{}

func (NoopRepository) CreateEvent(ctx context.Context, event Event) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}

	return event, nil
}

func (NoopRepository) ClaimPendingEvents(ctx context.Context, input ClaimPendingEventsInput) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return nil, nil
}

func (NoopRepository) MarkEventPublished(ctx context.Context, eventID string, now time.Time) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}

	return Event{}, ErrEventNotFound
}

func (NoopRepository) MarkEventFailed(ctx context.Context, input MarkEventFailedInput) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}

	return Event{}, ErrEventNotFound
}

func (NoopRepository) Stats(ctx context.Context, input StatsInput) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}

	return Stats{}, nil
}
