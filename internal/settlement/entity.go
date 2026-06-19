package settlement

import (
	"errors"
	"strings"
	"time"

	currencycode "github.com/fanryan/paycore/internal/shared/currency"
)

type BatchStatus string

const (
	BatchStatusCreated    BatchStatus = "CREATED"
	BatchStatusProcessing BatchStatus = "PROCESSING"
	BatchStatusCompleted  BatchStatus = "COMPLETED"
	BatchStatusFailed     BatchStatus = "FAILED"
)

var (
	ErrInvalidSettlementWindow = errors.New("settlement window end must be after window start")
	ErrInvalidBatchStatus      = errors.New("invalid settlement batch status")
	ErrInvalidLineItemAmount   = errors.New("settlement line item amount must be positive")
)

type Batch struct {
	ID          string
	Status      BatchStatus
	WindowStart time.Time
	WindowEnd   time.Time
	ClaimedBy   *string
	LockedUntil *time.Time
	CompletedAt *time.Time
	LastError   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type NewBatchInput struct {
	ID          string
	WindowStart time.Time
	WindowEnd   time.Time
	Now         time.Time
}

func NewBatch(input NewBatchInput) (Batch, error) {
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return Batch{}, errors.New("settlement batch id is required")
	}

	windowStart := input.WindowStart.UTC()
	windowEnd := input.WindowEnd.UTC()
	if !windowEnd.After(windowStart) {
		return Batch{}, ErrInvalidSettlementWindow
	}

	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Batch{
		ID:          id,
		Status:      BatchStatusCreated,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (b Batch) StartProcessing(workerID string, lockedUntil time.Time, now time.Time) (Batch, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return Batch{}, errors.New("settlement worker id is required")
	}

	if b.Status != BatchStatusCreated && b.Status != BatchStatusFailed && !b.IsStale(now) {
		return Batch{}, ErrInvalidBatchStatus
	}

	now = now.UTC()
	lockedUntil = lockedUntil.UTC()
	if !lockedUntil.After(now) {
		return Batch{}, errors.New("settlement lock expiry must be after now")
	}

	b.Status = BatchStatusProcessing
	b.ClaimedBy = &workerID
	b.LockedUntil = &lockedUntil
	b.LastError = nil
	b.UpdatedAt = now

	return b, nil
}

func (b Batch) Complete(now time.Time) (Batch, error) {
	if b.Status != BatchStatusProcessing {
		return Batch{}, ErrInvalidBatchStatus
	}

	now = now.UTC()

	b.Status = BatchStatusCompleted
	b.CompletedAt = &now
	b.LockedUntil = nil
	b.LastError = nil
	b.UpdatedAt = now

	return b, nil
}

func (b Batch) Fail(errorMessage string, now time.Time) (Batch, error) {
	if b.Status != BatchStatusProcessing {
		return Batch{}, ErrInvalidBatchStatus
	}

	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		return Batch{}, errors.New("settlement error message is required")
	}

	now = now.UTC()

	b.Status = BatchStatusFailed
	b.LockedUntil = nil
	b.LastError = &errorMessage
	b.UpdatedAt = now

	return b, nil
}

func (b Batch) IsStale(now time.Time) bool {
	if b.Status != BatchStatusProcessing || b.LockedUntil == nil {
		return false
	}

	return !b.LockedUntil.After(now.UTC())
}

type LineItem struct {
	ID              string
	BatchID         string
	MerchantID      string
	PaymentID       string
	AmountMinor     int64
	FeeAmountMinor  int64
	NetAmountMinor  int64
	Currency        string
	PaymentCaptured time.Time
	CreatedAt       time.Time
}

type NewLineItemInput struct {
	ID              string
	BatchID         string
	MerchantID      string
	PaymentID       string
	AmountMinor     int64
	FeeAmountMinor  int64
	Currency        string
	PaymentCaptured time.Time
	Now             time.Time
}

func NewLineItem(input NewLineItemInput) (LineItem, error) {
	id := strings.TrimSpace(input.ID)
	batchID := strings.TrimSpace(input.BatchID)
	merchantID := strings.TrimSpace(input.MerchantID)
	paymentID := strings.TrimSpace(input.PaymentID)
	currency := currencycode.NormalizeCurrency(input.Currency)

	if id == "" {
		return LineItem{}, errors.New("settlement line item id is required")
	}

	if batchID == "" {
		return LineItem{}, errors.New("settlement batch id is required")
	}

	if merchantID == "" {
		return LineItem{}, errors.New("merchant id is required")
	}

	if paymentID == "" {
		return LineItem{}, errors.New("payment id is required")
	}

	if input.AmountMinor <= 0 {
		return LineItem{}, ErrInvalidLineItemAmount
	}

	if input.FeeAmountMinor < 0 {
		return LineItem{}, errors.New("settlement fee amount cannot be negative")
	}

	if input.FeeAmountMinor > input.AmountMinor {
		return LineItem{}, errors.New("settlement fee amount cannot exceed amount")
	}

	if !currencycode.IsValidCurrency(currency) {
		return LineItem{}, errors.New("currency must be a 3-letter ISO currency code")
	}

	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return LineItem{
		ID:              id,
		BatchID:         batchID,
		MerchantID:      merchantID,
		PaymentID:       paymentID,
		AmountMinor:     input.AmountMinor,
		FeeAmountMinor:  input.FeeAmountMinor,
		NetAmountMinor:  input.AmountMinor - input.FeeAmountMinor,
		Currency:        currency,
		PaymentCaptured: input.PaymentCaptured.UTC(),
		CreatedAt:       now,
	}, nil
}
