package postgres

import (
	"context"
	"errors"

	"github.com/fanryan/paycore/internal/payer"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *Store) CreatePayer(ctx context.Context, payerRecord payer.Payer) (payer.Payer, error) {
	const query = `
		INSERT INTO payers (
			id,
			available_balance_minor,
			held_balance_minor,
			currency,
			version,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
			id,
			available_balance_minor,
			held_balance_minor,
			currency,
			version,
			created_at,
			updated_at
	`

	created, err := scanPayer(s.executor(ctx).QueryRow(ctx, query,
		payerRecord.ID,
		payerRecord.AvailableBalanceMinor,
		payerRecord.HeldBalanceMinor,
		payerRecord.Currency,
		payerRecord.Version,
		payerRecord.CreatedAt,
		payerRecord.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return payer.Payer{}, payer.ErrDuplicatePayer
		}

		return payer.Payer{}, err
	}

	return created, nil
}

func (s *Store) GetPayer(ctx context.Context, payerID string) (payer.Payer, error) {
	const query = `
		SELECT
			id,
			available_balance_minor,
			held_balance_minor,
			currency,
			version,
			created_at,
			updated_at
		FROM payers
		WHERE id = $1
	`

	payerRecord, err := scanPayer(s.executor(ctx).QueryRow(ctx, query, payerID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payer.Payer{}, payer.ErrPayerNotFound
		}

		return payer.Payer{}, err
	}

	return payerRecord, nil
}

func (s *Store) ListPayers(ctx context.Context) ([]payer.Payer, error) {
	const query = `
		SELECT
			id,
			available_balance_minor,
			held_balance_minor,
			currency,
			version,
			created_at,
			updated_at
		FROM payers
		ORDER BY created_at ASC, id ASC
	`

	rows, err := s.executor(ctx).Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payers := make([]payer.Payer, 0)
	for rows.Next() {
		payerRecord, err := scanPayer(rows)
		if err != nil {
			return nil, err
		}

		payers = append(payers, payerRecord)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payers, nil
}

func (s *Store) UpdatePayer(ctx context.Context, payerRecord payer.Payer) (payer.Payer, error) {
	const query = `
		UPDATE payers
		SET
			available_balance_minor = $2,
			held_balance_minor = $3,
			currency = $4,
			version = $5,
			updated_at = $6
		WHERE id = $1
		AND version = $7
	`

	previousVersion := payerRecord.Version - 1

	commandTag, err := s.executor(ctx).Exec(ctx, query,
		payerRecord.ID,
		payerRecord.AvailableBalanceMinor,
		payerRecord.HeldBalanceMinor,
		payerRecord.Currency,
		payerRecord.Version,
		payerRecord.UpdatedAt,
		previousVersion,
	)
	if err != nil {
		return payer.Payer{}, err
	}

	if commandTag.RowsAffected() == 0 {
		exists, err := s.payerExists(ctx, payerRecord.ID)
		if err != nil {
			return payer.Payer{}, err
		}

		if !exists {
			return payer.Payer{}, payer.ErrPayerNotFound
		}

		return payer.Payer{}, payer.ErrPayerVersionConflict
	}

	return s.GetPayer(ctx, payerRecord.ID)
}

func (s *Store) payerExists(ctx context.Context, payerID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM payers
			WHERE id = $1
		)
	`

	var exists bool
	if err := s.executor(ctx).QueryRow(ctx, query, payerID).Scan(&exists); err != nil {
		return false, err
	}

	return exists, nil
}

type executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Store) executor(ctx context.Context) executor {
	if tx, ok := db.TxFromContext(ctx); ok {
		return tx
	}

	return s.pool
}

type payerScanner interface {
	Scan(dest ...any) error
}

func scanPayer(scanner payerScanner) (payer.Payer, error) {
	var payerRecord payer.Payer

	if err := scanner.Scan(
		&payerRecord.ID,
		&payerRecord.AvailableBalanceMinor,
		&payerRecord.HeldBalanceMinor,
		&payerRecord.Currency,
		&payerRecord.Version,
		&payerRecord.CreatedAt,
		&payerRecord.UpdatedAt,
	); err != nil {
		return payer.Payer{}, err
	}

	return payerRecord, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
}
