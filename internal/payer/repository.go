package payer

import (
	"context"
	"errors"
)

var (
	ErrPayerNotFound        = errors.New("payer not found")
	ErrDuplicatePayer       = errors.New("payer already exists")
	ErrPayerVersionConflict = errors.New("payer version conflict")
)

type PayerRepository interface {
	CreatePayer(ctx context.Context, payer Payer) (Payer, error)
	GetPayer(ctx context.Context, payerID string) (Payer, error)
	ListPayers(ctx context.Context) ([]Payer, error)
	UpdatePayer(ctx context.Context, payer Payer) (Payer, error)
}
