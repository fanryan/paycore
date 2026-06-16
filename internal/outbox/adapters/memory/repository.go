package memory

import (
	"context"
	"sync"

	"github.com/fanryan/paycore/internal/outbox"
)

type Store struct {
	mu     sync.RWMutex
	events map[string]outbox.Event
}

func NewStore() *Store {
	return &Store{
		events: make(map[string]outbox.Event),
	}
}

func (s *Store) CreateEvent(ctx context.Context, event outbox.Event) (outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return outbox.Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.events[event.ID]; exists {
		return outbox.Event{}, outbox.ErrDuplicateEvent
	}

	s.events[event.ID] = event

	return event, nil
}

func (s *Store) ListEvents(ctx context.Context) ([]outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]outbox.Event, 0, len(s.events))
	for _, event := range s.events {
		events = append(events, event)
	}

	return events, nil
}
