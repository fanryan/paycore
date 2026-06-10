package memory

import (
	"context"
	"sync"

	"github.com/fanryan/paycore/internal/merchant"
)

type Store struct {
	mu        sync.RWMutex
	merchants map[string]merchant.Merchant
}

func NewStore() *Store {
	return &Store{
		merchants: make(map[string]merchant.Merchant),
	}
}

func (s *Store) CreateMerchant(ctx context.Context, merchantRecord merchant.Merchant) (merchant.Merchant, error) {
	if err := ctx.Err(); err != nil {
		return merchant.Merchant{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.merchants[merchantRecord.ID]; exists {
		return merchant.Merchant{}, merchant.ErrDuplicateMerchant
	}

	s.merchants[merchantRecord.ID] = merchantRecord

	return merchantRecord, nil
}

func (s *Store) GetMerchant(ctx context.Context, merchantID string) (merchant.Merchant, error) {
	if err := ctx.Err(); err != nil {
		return merchant.Merchant{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	merchantRecord, exists := s.merchants[merchantID]
	if !exists {
		return merchant.Merchant{}, merchant.ErrMerchantNotFound
	}

	return merchantRecord, nil
}

func (s *Store) ListMerchants(ctx context.Context) ([]merchant.Merchant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	merchants := make([]merchant.Merchant, 0, len(s.merchants))
	for _, merchantRecord := range s.merchants {
		merchants = append(merchants, merchantRecord)
	}

	return merchants, nil
}
