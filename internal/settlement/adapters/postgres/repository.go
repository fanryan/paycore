package postgres

import (
	"context"
	"errors"

	"github.com/fanryan/paycore/internal/settlement"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolationCode = "23505"

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
	}
}

func (s *Store) CreateBatch(ctx context.Context, batch settlement.Batch) (settlement.Batch, error) {
	const query = `
		INSERT INTO settlement_batches (
			id,
			status,
			window_start,
			window_end,
			claimed_by,
			locked_until,
			completed_at,
			last_error,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING
			id,
			status,
			window_start,
			window_end,
			claimed_by,
			locked_until,
			completed_at,
			last_error,
			created_at,
			updated_at
	`

	created, err := scanBatch(s.queryRow(ctx, query,
		batch.ID,
		string(batch.Status),
		batch.WindowStart,
		batch.WindowEnd,
		batch.ClaimedBy,
		batch.LockedUntil,
		batch.CompletedAt,
		batch.LastError,
		batch.CreatedAt,
		batch.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return settlement.Batch{}, settlement.ErrDuplicateBatch
		}

		return settlement.Batch{}, err
	}

	return created, nil
}

func (s *Store) GetBatch(ctx context.Context, batchID string) (settlement.Batch, error) {
	const query = `
		SELECT
			id,
			status,
			window_start,
			window_end,
			claimed_by,
			locked_until,
			completed_at,
			last_error,
			created_at,
			updated_at
		FROM settlement_batches
		WHERE id = $1
	`

	batch, err := scanBatch(s.queryRow(ctx, query, batchID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return settlement.Batch{}, settlement.ErrBatchNotFound
		}

		return settlement.Batch{}, err
	}

	return batch, nil
}

func (s *Store) UpdateBatch(ctx context.Context, batch settlement.Batch) (settlement.Batch, error) {
	const query = `
		UPDATE settlement_batches
		SET
			status = $2,
			window_start = $3,
			window_end = $4,
			claimed_by = $5,
			locked_until = $6,
			completed_at = $7,
			last_error = $8,
			updated_at = $9
		WHERE id = $1
		RETURNING
			id,
			status,
			window_start,
			window_end,
			claimed_by,
			locked_until,
			completed_at,
			last_error,
			created_at,
			updated_at
	`

	updated, err := scanBatch(s.queryRow(ctx, query,
		batch.ID,
		string(batch.Status),
		batch.WindowStart,
		batch.WindowEnd,
		batch.ClaimedBy,
		batch.LockedUntil,
		batch.CompletedAt,
		batch.LastError,
		batch.UpdatedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return settlement.Batch{}, settlement.ErrBatchNotFound
		}

		return settlement.Batch{}, err
	}

	return updated, nil
}

func (s *Store) CreateLineItem(ctx context.Context, item settlement.LineItem) (settlement.LineItem, error) {
	const query = `
		INSERT INTO settlement_line_items (
			id,
			settlement_batch_id,
			merchant_id,
			payment_id,
			amount_minor,
			fee_amount_minor,
			net_amount_minor,
			currency,
			payment_captured_at,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING
			id,
			settlement_batch_id,
			merchant_id,
			payment_id,
			amount_minor,
			fee_amount_minor,
			net_amount_minor,
			currency,
			payment_captured_at,
			created_at
	`

	created, err := scanLineItem(s.queryRow(ctx, query,
		item.ID,
		item.BatchID,
		item.MerchantID,
		item.PaymentID,
		item.AmountMinor,
		item.FeeAmountMinor,
		item.NetAmountMinor,
		item.Currency,
		item.PaymentCaptured,
		item.CreatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return settlement.LineItem{}, settlement.ErrDuplicateLineItem
		}

		return settlement.LineItem{}, err
	}

	return created, nil
}

func (s *Store) ListLineItems(ctx context.Context, batchID string) ([]settlement.LineItem, error) {
	const query = `
		SELECT
			id,
			settlement_batch_id,
			merchant_id,
			payment_id,
			amount_minor,
			fee_amount_minor,
			net_amount_minor,
			currency,
			payment_captured_at,
			created_at
		FROM settlement_line_items
		WHERE settlement_batch_id = $1
		ORDER BY created_at ASC, id ASC
	`

	rows, err := s.query(ctx, query, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]settlement.LineItem, 0)
	for rows.Next() {
		item, err := scanLineItem(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *Store) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := db.TxFromContext(ctx); ok {
		return tx.QueryRow(ctx, sql, args...)
	}

	return s.pool.QueryRow(ctx, sql, args...)
}

func (s *Store) query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx, ok := db.TxFromContext(ctx); ok {
		return tx.Query(ctx, sql, args...)
	}

	return s.pool.Query(ctx, sql, args...)
}

type batchScanner interface {
	Scan(dest ...any) error
}

func scanBatch(scanner batchScanner) (settlement.Batch, error) {
	var batch settlement.Batch
	var status string
	var lockedUntil pgtype.Timestamptz
	var completedAt pgtype.Timestamptz

	if err := scanner.Scan(
		&batch.ID,
		&status,
		&batch.WindowStart,
		&batch.WindowEnd,
		&batch.ClaimedBy,
		&lockedUntil,
		&completedAt,
		&batch.LastError,
		&batch.CreatedAt,
		&batch.UpdatedAt,
	); err != nil {
		return settlement.Batch{}, err
	}

	batch.Status = settlement.BatchStatus(status)

	if lockedUntil.Valid {
		value := lockedUntil.Time
		batch.LockedUntil = &value
	}

	if completedAt.Valid {
		value := completedAt.Time
		batch.CompletedAt = &value
	}

	return batch, nil
}

type lineItemScanner interface {
	Scan(dest ...any) error
}

func scanLineItem(scanner lineItemScanner) (settlement.LineItem, error) {
	var item settlement.LineItem

	if err := scanner.Scan(
		&item.ID,
		&item.BatchID,
		&item.MerchantID,
		&item.PaymentID,
		&item.AmountMinor,
		&item.FeeAmountMinor,
		&item.NetAmountMinor,
		&item.Currency,
		&item.PaymentCaptured,
		&item.CreatedAt,
	); err != nil {
		return settlement.LineItem{}, err
	}

	return item, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
}
