package idempotency

import (
	"context"
	"errors"
)

var (
	ErrRecordNotFound        = errors.New("idempotency record not found")
	ErrDuplicateKey          = errors.New("idempotency key already exists")
	ErrRequestHashMismatch   = errors.New("idempotency request hash does not match")
	ErrExpiredIdempotencyKey = errors.New("idempotency key has expired")
)

type Repository interface {
	CreateRecord(ctx context.Context, record Record) (Record, error)
	GetRecord(ctx context.Context, key string) (Record, error)
	UpdateRecord(ctx context.Context, record Record) (Record, error)
}
