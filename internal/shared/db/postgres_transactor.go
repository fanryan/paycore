package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresTransactor struct {
	pool *pgxpool.Pool
}

func NewPostgresTransactor(pool *pgxpool.Pool) *PostgresTransactor {
	return &PostgresTransactor{
		pool: pool,
	}
}

func (t *PostgresTransactor) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := TxFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(ctx)
			panic(recovered)
		}
	}()

	txCtx := injectTx(ctx, tx)

	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
