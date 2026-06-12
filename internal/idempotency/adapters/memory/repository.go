package memory

import (
	"context"
	"sync"

	"github.com/fanryan/paycore/internal/idempotency"
)

type Store struct {
	mu      sync.RWMutex
	records map[string]idempotency.Record
}

func NewStore() *Store {
	return &Store{
		records: make(map[string]idempotency.Record),
	}
}

func (s *Store) CreateRecord(ctx context.Context, record idempotency.Record) (idempotency.Record, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.records[record.Key]; exists {
		return idempotency.Record{}, idempotency.ErrDuplicateKey
	}

	s.records[record.Key] = cloneRecord(record)

	return cloneRecord(record), nil
}

func (s *Store) GetRecord(ctx context.Context, key string) (idempotency.Record, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Record{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists {
		return idempotency.Record{}, idempotency.ErrRecordNotFound
	}

	return cloneRecord(record), nil
}

func (s *Store) UpdateRecord(ctx context.Context, record idempotency.Record) (idempotency.Record, error) {
	if err := ctx.Err(); err != nil {
		return idempotency.Record{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.records[record.Key]; !exists {
		return idempotency.Record{}, idempotency.ErrRecordNotFound
	}

	s.records[record.Key] = cloneRecord(record)

	return cloneRecord(record), nil
}

func cloneRecord(record idempotency.Record) idempotency.Record {
	record.ResponseBody = append([]byte(nil), record.ResponseBody...)
	return record
}
