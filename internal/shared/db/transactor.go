package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type txContextKey struct{}

func injectTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}

type NoopTransactor struct{}

func (NoopTransactor) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
