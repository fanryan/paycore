package merchant

import (
	"context"
	"time"
)

type MerchantService struct {
	merchants MerchantRepository
	now       func() time.Time
}

type CreateMerchantInput struct {
	ID                 string
	Name               string
	SettlementCurrency string
}

func NewMerchantService(merchants MerchantRepository) *MerchantService {
	return &MerchantService{
		merchants: merchants,
		now:       time.Now,
	}
}

func (s *MerchantService) CreateMerchant(ctx context.Context, input CreateMerchantInput) (Merchant, error) {
	merchant, err := NewMerchant(input.ID, input.Name, input.SettlementCurrency, s.now())
	if err != nil {
		return Merchant{}, err
	}

	return s.merchants.CreateMerchant(ctx, merchant)
}

func (s *MerchantService) GetMerchant(ctx context.Context, merchantID string) (Merchant, error) {
	return s.merchants.GetMerchant(ctx, merchantID)
}

func (s *MerchantService) ListMerchants(ctx context.Context) ([]Merchant, error) {
	return s.merchants.ListMerchants(ctx)
}
