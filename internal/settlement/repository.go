package settlement

import (
	"context"
	"errors"
	"time"
)

var (
	ErrBatchNotFound         = errors.New("settlement batch not found")
	ErrLineItemNotFound      = errors.New("settlement line item not found")
	ErrDuplicateLineItem     = errors.New("settlement line item already exists for payment")
	ErrDuplicateBatch        = errors.New("settlement batch already exists")
	ErrPaymentAlreadySettled = errors.New("payment already belongs to a settlement batch")
)

type Repository interface {
	CreateBatch(ctx context.Context, batch Batch) (Batch, error)
	GetBatch(ctx context.Context, batchID string) (Batch, error)
	UpdateBatch(ctx context.Context, batch Batch) (Batch, error)
	ClaimCapturedPayments(ctx context.Context, input ClaimCapturedPaymentsInput) ([]ClaimedPayment, error)
	CreateLineItem(ctx context.Context, item LineItem) (LineItem, error)
	ListLineItems(ctx context.Context, batchID string) ([]LineItem, error)
}

type ClaimCapturedPaymentsInput struct {
	BatchID     string
	WindowStart time.Time
	WindowEnd   time.Time
	Limit       int
	Now         time.Time
}

type ClaimedPayment struct {
	PaymentID       string
	MerchantID      string
	AmountMinor     int64
	Currency        string
	CapturedAt      time.Time
	SettlementBatch string
}
