package merchant

import (
	"context"
	"errors"
)

var (
	ErrMerchantNotFound  = errors.New("merchant not found")
	ErrDuplicateMerchant = errors.New("merchant already exists")
)

type MerchantRepository interface {
	CreateMerchant(ctx context.Context, merchant Merchant) (Merchant, error)
	GetMerchant(ctx context.Context, merchantID string) (Merchant, error)
	ListMerchants(ctx context.Context) ([]Merchant, error)
}
