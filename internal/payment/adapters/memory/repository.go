package memory

import (
	"context"
	"sync"

	"github.com/fanryan/paycore/internal/payment"
)

type Store struct {
	mu                sync.RWMutex
	payments          map[string]payment.Payment
	holds             map[string]payment.Hold
	holdIDByPaymentID map[string]string
}

func NewStore() *Store {
	return &Store{
		payments:          make(map[string]payment.Payment),
		holds:             make(map[string]payment.Hold),
		holdIDByPaymentID: make(map[string]string),
	}
}

func (s *Store) CreatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	if err := ctx.Err(); err != nil {
		return payment.Payment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.payments[paymentRecord.ID]; exists {
		return payment.Payment{}, payment.ErrDuplicatePayment
	}

	s.payments[paymentRecord.ID] = paymentRecord

	return paymentRecord, nil
}

func (s *Store) GetPayment(ctx context.Context, paymentID string) (payment.Payment, error) {
	if err := ctx.Err(); err != nil {
		return payment.Payment{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	paymentRecord, exists := s.payments[paymentID]
	if !exists {
		return payment.Payment{}, payment.ErrPaymentNotFound
	}

	return paymentRecord, nil
}

func (s *Store) UpdatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	if err := ctx.Err(); err != nil {
		return payment.Payment{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.payments[paymentRecord.ID]; !exists {
		return payment.Payment{}, payment.ErrPaymentNotFound
	}

	s.payments[paymentRecord.ID] = paymentRecord

	return paymentRecord, nil
}

func (s *Store) CreateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	if err := ctx.Err(); err != nil {
		return payment.Hold{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.holds[hold.ID]; exists {
		return payment.Hold{}, payment.ErrDuplicateHold
	}

	if _, exists := s.holdIDByPaymentID[hold.PaymentID]; exists {
		return payment.Hold{}, payment.ErrDuplicateHold
	}

	s.holds[hold.ID] = hold
	s.holdIDByPaymentID[hold.PaymentID] = hold.ID

	return hold, nil
}

func (s *Store) GetHold(ctx context.Context, holdID string) (payment.Hold, error) {
	if err := ctx.Err(); err != nil {
		return payment.Hold{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	hold, exists := s.holds[holdID]
	if !exists {
		return payment.Hold{}, payment.ErrHoldNotFound
	}

	return hold, nil
}

func (s *Store) GetHoldByPaymentID(ctx context.Context, paymentID string) (payment.Hold, error) {
	if err := ctx.Err(); err != nil {
		return payment.Hold{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	holdID, exists := s.holdIDByPaymentID[paymentID]
	if !exists {
		return payment.Hold{}, payment.ErrHoldNotFound
	}

	hold, exists := s.holds[holdID]
	if !exists {
		return payment.Hold{}, payment.ErrHoldNotFound
	}

	return hold, nil
}

func (s *Store) UpdateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	if err := ctx.Err(); err != nil {
		return payment.Hold{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.holds[hold.ID]; !exists {
		return payment.Hold{}, payment.ErrHoldNotFound
	}

	s.holds[hold.ID] = hold
	s.holdIDByPaymentID[hold.PaymentID] = hold.ID

	return hold, nil
}
