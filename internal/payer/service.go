package payer

import (
	"context"
	"time"
)

type PayerService struct {
	payers PayerRepository
	now    func() time.Time
}

type CreatePayerInput struct {
	ID                    string
	AvailableBalanceMinor int64
	Currency              string
}

func NewPayerService(payers PayerRepository) *PayerService {
	return &PayerService{
		payers: payers,
		now:    time.Now,
	}
}

func (s *PayerService) CreatePayer(ctx context.Context, input CreatePayerInput) (Payer, error) {
	payer, err := NewPayer(input.ID, input.AvailableBalanceMinor, input.Currency, s.now())
	if err != nil {
		return Payer{}, err
	}

	return s.payers.CreatePayer(ctx, payer)
}

func (s *PayerService) GetPayer(ctx context.Context, payerID string) (Payer, error) {
	return s.payers.GetPayer(ctx, payerID)
}

func (s *PayerService) ListPayers(ctx context.Context) ([]Payer, error) {
	return s.payers.ListPayers(ctx)
}

func (s *PayerService) UpdatePayer(ctx context.Context, payer Payer) (Payer, error) {
	return s.payers.UpdatePayer(ctx, payer)
}
