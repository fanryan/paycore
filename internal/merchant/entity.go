package merchant

import (
	"errors"
	"strings"
	"time"

	currencycode "github.com/fanryan/paycore/internal/shared/currency"
)

type MerchantStatus string

const (
	MerchantStatusActive    MerchantStatus = "ACTIVE"
	MerchantStatusSuspended MerchantStatus = "SUSPENDED"
	MerchantStatusClosed    MerchantStatus = "CLOSED"
)

type Merchant struct {
	ID                 string
	Name               string
	Status             MerchantStatus
	SettlementCurrency string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewMerchant(id string, name string, settlementCurrency string, now time.Time) (Merchant, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	settlementCurrency = currencycode.NormalizeCurrency(settlementCurrency)

	if id == "" {
		return Merchant{}, errors.New("merchant id is required")
	}

	if name == "" {
		return Merchant{}, errors.New("merchant name is required")
	}

	if !currencycode.IsValidCurrency(settlementCurrency) {
		return Merchant{}, errors.New("settlement currency must be a 3-letter ISO currency code")
	}

	now = now.UTC()

	return Merchant{
		ID:                 id,
		Name:               name,
		Status:             MerchantStatusActive,
		SettlementCurrency: settlementCurrency,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func (m Merchant) CanCreatePayments() bool {
	return m.Status == MerchantStatusActive
}
