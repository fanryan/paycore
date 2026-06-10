package payment

import (
	"errors"
	"strings"
	"time"

	currencycode "github.com/fanryan/paycore/internal/shared/currency"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusAuthorized Status = "AUTHORIZED"
	StatusCaptured   Status = "CAPTURED"
	StatusSettled    Status = "SETTLED"
	StatusExpired    Status = "EXPIRED"
	StatusFailed     Status = "FAILED"
)

type Payment struct {
	ID                  string
	MerchantID          string
	PayerID             string
	AmountMinor         int64
	Currency            string
	Status              Status
	AuthorizationHoldID string
	AuthorizedAt        time.Time
	ExpiresAt           time.Time
	CapturedAt          *time.Time
	SettledAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func NewAuthorizedPayment(input NewAuthorizedPaymentInput) (Payment, error) {
	id := strings.TrimSpace(input.ID)
	merchantID := strings.TrimSpace(input.MerchantID)
	payerID := strings.TrimSpace(input.PayerID)
	holdID := strings.TrimSpace(input.AuthorizationHoldID)
	currency := currencycode.NormalizeCurrency(input.Currency)

	if id == "" {
		return Payment{}, errors.New("payment id is required")
	}

	if merchantID == "" {
		return Payment{}, errors.New("merchant id is required")
	}

	if payerID == "" {
		return Payment{}, errors.New("payer id is required")
	}

	if holdID == "" {
		return Payment{}, errors.New("authorization hold id is required")
	}

	if input.AmountMinor <= 0 {
		return Payment{}, errors.New("amount must be positive")
	}

	if !currencycode.IsValidCurrency(currency) {
		return Payment{}, errors.New("currency must be a 3-letter ISO currency code")
	}

	now := input.Now.UTC()
	expiresAt := input.ExpiresAt.UTC()

	if !expiresAt.After(now) {
		return Payment{}, errors.New("authorization expiry must be after authorization time")
	}

	return Payment{
		ID:                  id,
		MerchantID:          merchantID,
		PayerID:             payerID,
		AmountMinor:         input.AmountMinor,
		Currency:            currency,
		Status:              StatusAuthorized,
		AuthorizationHoldID: holdID,
		AuthorizedAt:        now,
		ExpiresAt:           expiresAt,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

type NewAuthorizedPaymentInput struct {
	ID                  string
	MerchantID          string
	PayerID             string
	AmountMinor         int64
	Currency            string
	AuthorizationHoldID string
	Now                 time.Time
	ExpiresAt           time.Time
}

func (p Payment) CanCapture(now time.Time) bool {
	if p.Status != StatusAuthorized {
		return false
	}

	return now.UTC().Before(p.ExpiresAt) || now.UTC().Equal(p.ExpiresAt)
}

func (p Payment) Capture(now time.Time) (Payment, error) {
	if p.Status != StatusAuthorized {
		return Payment{}, errors.New("only authorized payments can be captured")
	}

	now = now.UTC()

	if now.After(p.ExpiresAt) {
		return Payment{}, errors.New("authorization has expired")
	}

	p.Status = StatusCaptured
	p.CapturedAt = &now
	p.UpdatedAt = now

	return p, nil
}

func (p Payment) Expire(now time.Time) (Payment, error) {
	if p.Status != StatusAuthorized {
		return Payment{}, errors.New("only authorized payments can expire")
	}

	now = now.UTC()

	p.Status = StatusExpired
	p.UpdatedAt = now

	return p, nil
}

func (p Payment) Settle(now time.Time) (Payment, error) {
	if p.Status != StatusCaptured {
		return Payment{}, errors.New("only captured payments can be settled")
	}

	now = now.UTC()

	p.Status = StatusSettled
	p.SettledAt = &now
	p.UpdatedAt = now

	return p, nil
}
