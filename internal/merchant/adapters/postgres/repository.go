package postgres

import (
	"context"
	"errors"

	"github.com/fanryan/paycore/internal/merchant"
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

func (s *Store) CreateMerchant(ctx context.Context, merchantRecord merchant.Merchant) (merchant.Merchant, error) {
	const query = `
		INSERT INTO merchants (
			id,
			name,
			status,
			settlement_currency,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id,
			name,
			status,
			settlement_currency,
			created_at,
			updated_at
	`

	created, err := scanMerchant(s.executor(ctx).QueryRow(ctx, query,
		merchantRecord.ID,
		merchantRecord.Name,
		string(merchantRecord.Status),
		merchantRecord.SettlementCurrency,
		merchantRecord.CreatedAt,
		merchantRecord.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return merchant.Merchant{}, merchant.ErrDuplicateMerchant
		}

		return merchant.Merchant{}, err
	}

	return created, nil
}

func (s *Store) GetMerchant(ctx context.Context, merchantID string) (merchant.Merchant, error) {
	const query = `
		SELECT
			id,
			name,
			status,
			settlement_currency,
			created_at,
			updated_at
		FROM merchants
		WHERE id = $1
	`

	merchantRecord, err := scanMerchant(s.executor(ctx).QueryRow(ctx, query, merchantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return merchant.Merchant{}, merchant.ErrMerchantNotFound
		}

		return merchant.Merchant{}, err
	}

	return merchantRecord, nil
}

func (s *Store) ListMerchants(ctx context.Context) ([]merchant.Merchant, error) {
	const query = `
		SELECT
			id,
			name,
			status,
			settlement_currency,
			created_at,
			updated_at
		FROM merchants
		ORDER BY created_at ASC, id ASC
	`

	rows, err := s.executor(ctx).Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merchants := make([]merchant.Merchant, 0)
	for rows.Next() {
		merchantRecord, err := scanMerchant(rows)
		if err != nil {
			return nil, err
		}

		merchants = append(merchants, merchantRecord)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return merchants, nil
}

type merchantScanner interface {
	Scan(dest ...any) error
}

func scanMerchant(scanner merchantScanner) (merchant.Merchant, error) {
	var merchantRecord merchant.Merchant
	var status string

	if err := scanner.Scan(
		&merchantRecord.ID,
		&merchantRecord.Name,
		&status,
		&merchantRecord.SettlementCurrency,
		&merchantRecord.CreatedAt,
		&merchantRecord.UpdatedAt,
	); err != nil {
		return merchant.Merchant{}, err
	}

	merchantRecord.Status = merchant.MerchantStatus(status)

	return merchantRecord, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
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
