package memory

import (
	"context"
	"sort"
	"sync"
	"time"

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

func (s *Store) ClaimPendingEvents(ctx context.Context, input outbox.ClaimPendingEventsInput) ([]outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if input.Limit <= 0 {
		return []outbox.Event{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	claimable := make([]outbox.Event, 0)
	for _, event := range s.events {
		if event.AvailableAt.After(input.Now) {
			continue
		}

		if event.Status != outbox.StatusPending && event.Status != outbox.StatusFailed {
			continue
		}

		claimable = append(claimable, event)
	}

	sort.Slice(claimable, func(i, j int) bool {
		if !claimable[i].AvailableAt.Equal(claimable[j].AvailableAt) {
			return claimable[i].AvailableAt.Before(claimable[j].AvailableAt)
		}

		if !claimable[i].CreatedAt.Equal(claimable[j].CreatedAt) {
			return claimable[i].CreatedAt.Before(claimable[j].CreatedAt)
		}

		return claimable[i].ID < claimable[j].ID
	})

	if len(claimable) > input.Limit {
		claimable = claimable[:input.Limit]
	}

	claimed := make([]outbox.Event, 0, len(claimable))
	for _, event := range claimable {
		claimedEvent, err := event.Claim(input.WorkerID, input.Now)
		if err != nil {
			return nil, err
		}

		s.events[claimedEvent.ID] = claimedEvent
		claimed = append(claimed, claimedEvent)
	}

	return claimed, nil
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, now time.Time) (outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return outbox.Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	event, exists := s.events[eventID]
	if !exists {
		return outbox.Event{}, outbox.ErrEventNotFound
	}

	published, err := event.MarkPublished(now)
	if err != nil {
		return outbox.Event{}, err
	}

	s.events[eventID] = published

	return published, nil
}

func (s *Store) MarkEventFailed(ctx context.Context, input outbox.MarkEventFailedInput) (outbox.Event, error) {
	if err := ctx.Err(); err != nil {
		return outbox.Event{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	event, exists := s.events[input.EventID]
	if !exists {
		return outbox.Event{}, outbox.ErrEventNotFound
	}

	failed, err := event.MarkFailed(input.ErrorMessage, input.NextAvailable, input.Now)
	if err != nil {
		return outbox.Event{}, err
	}

	s.events[input.EventID] = failed

	return failed, nil
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

func (s *Store) Stats(ctx context.Context, input outbox.StatsInput) (outbox.Stats, error) {
	if err := ctx.Err(); err != nil {
		return outbox.Stats{}, err
	}

	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var oldestAvailable *time.Time
	pending := 0
	for _, event := range s.events {
		if event.Status != outbox.StatusPending && event.Status != outbox.StatusFailed {
			continue
		}

		if event.AvailableAt.After(now) {
			continue
		}

		pending++
		availableAt := event.AvailableAt
		if oldestAvailable == nil || availableAt.Before(*oldestAvailable) {
			oldestAvailable = &availableAt
		}
	}

	stats := outbox.Stats{
		PendingEvents: pending,
	}
	if oldestAvailable != nil {
		stats.PublishLag = now.Sub(*oldestAvailable)
		if stats.PublishLag < 0 {
			stats.PublishLag = 0
		}
	}

	return stats, nil
}
