package memory

import (
	"context"
	"sync"

	"github.com/fanryan/paycore/internal/payer"
)

type Store struct {
	mu     sync.RWMutex
	payers map[string]payer.Payer
}

func NewStore() *Store {
	return &Store{
		payers: make(map[string]payer.Payer),
	}
}

func (s *Store) CreatePayer(ctx context.Context, payerRecord payer.Payer) (payer.Payer, error) {
	if err := ctx.Err(); err != nil {
		return payer.Payer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.payers[payerRecord.ID]; exists {
		return payer.Payer{}, payer.ErrDuplicatePayer
	}

	s.payers[payerRecord.ID] = payerRecord

	return payerRecord, nil
}

func (s *Store) GetPayer(ctx context.Context, payerID string) (payer.Payer, error) {
	if err := ctx.Err(); err != nil {
		return payer.Payer{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	payerRecord, exists := s.payers[payerID]
	if !exists {
		return payer.Payer{}, payer.ErrPayerNotFound
	}

	return payerRecord, nil
}

func (s *Store) ListPayers(ctx context.Context) ([]payer.Payer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	payers := make([]payer.Payer, 0, len(s.payers))
	for _, payerRecord := range s.payers {
		payers = append(payers, payerRecord)
	}

	return payers, nil
}

func (s *Store) UpdatePayer(ctx context.Context, payerRecord payer.Payer) (payer.Payer, error) {
	if err := ctx.Err(); err != nil {
		return payer.Payer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.payers[payerRecord.ID]; !exists {
		return payer.Payer{}, payer.ErrPayerNotFound
	}

	s.payers[payerRecord.ID] = payerRecord

	return payerRecord, nil
}
