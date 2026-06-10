package payer

import (
	"errors"
	"strings"
	"time"

	currencycode "github.com/fanryan/paycore/internal/shared/currency"
)

type Payer struct {
	ID                    string
	AvailableBalanceMinor int64
	HeldBalanceMinor      int64
	Currency              string
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func NewPayer(id string, availableBalanceMinor int64, currency string, now time.Time) (Payer, error) {
	id = strings.TrimSpace(id)
	currency = currencycode.NormalizeCurrency(currency)

	if id == "" {
		return Payer{}, errors.New("payer id is required")
	}

	if availableBalanceMinor < 0 {
		return Payer{}, errors.New("available balance cannot be negative")
	}

	if !currencycode.IsValidCurrency(currency) {
		return Payer{}, errors.New("currency must be a 3-letter ISO currency code")
	}

	now = now.UTC()

	return Payer{
		ID:                    id,
		AvailableBalanceMinor: availableBalanceMinor,
		HeldBalanceMinor:      0,
		Currency:              currency,
		Version:               0,
		CreatedAt:             now,
		UpdatedAt:             now,
	}, nil
}

func (p Payer) CanAuthorize(amountMinor int64, currency string) bool {
	if amountMinor <= 0 {
		return false
	}

	if p.Currency != currencycode.NormalizeCurrency(currency) {
		return false
	}

	return p.AvailableBalanceMinor >= amountMinor
}

func (p Payer) Reserve(amountMinor int64, currency string, now time.Time) (Payer, error) {
	if amountMinor <= 0 {
		return Payer{}, errors.New("reserve amount must be positive")
	}

	if p.Currency != currencycode.NormalizeCurrency(currency) {
		return Payer{}, errors.New("reserve currency does not match payer currency")
	}

	if p.AvailableBalanceMinor < amountMinor {
		return Payer{}, errors.New("insufficient available balance")
	}

	now = now.UTC()

	p.AvailableBalanceMinor -= amountMinor
	p.HeldBalanceMinor += amountMinor
	p.Version++
	p.UpdatedAt = now

	return p, nil
}

func (p Payer) Release(amountMinor int64, currency string, now time.Time) (Payer, error) {
	if amountMinor <= 0 {
		return Payer{}, errors.New("release amount must be positive")
	}

	if p.Currency != currencycode.NormalizeCurrency(currency) {
		return Payer{}, errors.New("release currency does not match payer currency")
	}

	if p.HeldBalanceMinor < amountMinor {
		return Payer{}, errors.New("insufficient held balance")
	}

	now = now.UTC()

	p.AvailableBalanceMinor += amountMinor
	p.HeldBalanceMinor -= amountMinor
	p.Version++
	p.UpdatedAt = now

	return p, nil
}

func (p Payer) CaptureHeld(amountMinor int64, currency string, now time.Time) (Payer, error) {
	if amountMinor <= 0 {
		return Payer{}, errors.New("capture amount must be positive")
	}

	if p.Currency != currencycode.NormalizeCurrency(currency) {
		return Payer{}, errors.New("capture currency does not match payer currency")
	}

	if p.HeldBalanceMinor < amountMinor {
		return Payer{}, errors.New("insufficient held balance")
	}

	now = now.UTC()

	p.HeldBalanceMinor -= amountMinor
	p.Version++
	p.UpdatedAt = now

	return p, nil
}
