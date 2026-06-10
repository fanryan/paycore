package payment

import (
	"errors"
	"strings"
	"time"

	currencycode "github.com/fanryan/paycore/internal/shared/currency"
)

type HoldStatus string

const (
	HoldStatusHeld     HoldStatus = "HELD"
	HoldStatusCaptured HoldStatus = "CAPTURED"
	HoldStatusReleased HoldStatus = "RELEASED"
)

type Hold struct {
	ID          string
	PaymentID   string
	PayerID     string
	AmountMinor int64
	Currency    string
	Status      HoldStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type NewHoldInput struct {
	ID          string
	PaymentID   string
	PayerID     string
	AmountMinor int64
	Currency    string
	Now         time.Time
}

func NewHold(input NewHoldInput) (Hold, error) {
	id := strings.TrimSpace(input.ID)
	paymentID := strings.TrimSpace(input.PaymentID)
	payerID := strings.TrimSpace(input.PayerID)
	currency := currencycode.NormalizeCurrency(input.Currency)

	if id == "" {
		return Hold{}, errors.New("hold id is required")
	}

	if paymentID == "" {
		return Hold{}, errors.New("payment id is required")
	}

	if payerID == "" {
		return Hold{}, errors.New("payer id is required")
	}

	if input.AmountMinor <= 0 {
		return Hold{}, errors.New("hold amount must be positive")
	}

	if !currencycode.IsValidCurrency(currency) {
		return Hold{}, errors.New("currency must be a 3-letter ISO currency code")
	}

	now := input.Now.UTC()

	return Hold{
		ID:          id,
		PaymentID:   paymentID,
		PayerID:     payerID,
		AmountMinor: input.AmountMinor,
		Currency:    currency,
		Status:      HoldStatusHeld,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (h Hold) Capture(now time.Time) (Hold, error) {
	if h.Status != HoldStatusHeld {
		return Hold{}, errors.New("only held holds can be captured")
	}

	now = now.UTC()

	h.Status = HoldStatusCaptured
	h.UpdatedAt = now

	return h, nil
}

func (h Hold) Release(now time.Time) (Hold, error) {
	if h.Status != HoldStatusHeld {
		return Hold{}, errors.New("only held holds can be released")
	}

	now = now.UTC()

	h.Status = HoldStatusReleased
	h.UpdatedAt = now

	return h, nil
}
