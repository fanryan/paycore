package outbox

import (
	"context"
	"errors"
)

var (
	ErrEventNotFound  = errors.New("outbox event not found")
	ErrDuplicateEvent = errors.New("outbox event already exists")
)

type Repository interface {
	CreateEvent(ctx context.Context, event Event) (Event, error)
}

type NoopRepository struct{}

func (NoopRepository) CreateEvent(ctx context.Context, event Event) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}

	return event, nil
}
