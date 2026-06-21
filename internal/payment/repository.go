package payment

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPaymentNotFound  = errors.New("payment not found")
	ErrDuplicatePayment = errors.New("payment already exists")
	ErrHoldNotFound     = errors.New("payment hold not found")
	ErrDuplicateHold    = errors.New("payment hold already exists")
)

type Repository interface {
	CreatePayment(ctx context.Context, payment Payment) (Payment, error)
	GetPayment(ctx context.Context, paymentID string) (Payment, error)
	ListExpiredAuthorizedPayments(ctx context.Context, now time.Time, limit int) ([]Payment, error)
	UpdatePayment(ctx context.Context, payment Payment) (Payment, error)

	CreateHold(ctx context.Context, hold Hold) (Hold, error)
	GetHold(ctx context.Context, holdID string) (Hold, error)
	GetHoldByPaymentID(ctx context.Context, paymentID string) (Hold, error)
	UpdateHold(ctx context.Context, hold Hold) (Hold, error)
}
