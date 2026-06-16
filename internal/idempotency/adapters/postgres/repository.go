package postgres

import (
	"context"
	"errors"

	"github.com/fanryan/paycore/internal/idempotency"
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

func (s *Store) CreateRecord(ctx context.Context, record idempotency.Record) (idempotency.Record, error) {
	const query = `
		INSERT INTO idempotency_records (
			key,
			request_hash,
			status,
			response_code,
			response_body,
			created_at,
			updated_at,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING
			key,
			request_hash,
			status,
			response_code,
			response_body,
			created_at,
			updated_at,
			expires_at
	`

	created, err := scanRecord(s.pool.QueryRow(ctx, query,
		record.Key,
		record.RequestHash,
		string(record.Status),
		record.ResponseCode,
		record.ResponseBody,
		record.CreatedAt,
		record.UpdatedAt,
		record.ExpiresAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return idempotency.Record{}, idempotency.ErrDuplicateKey
		}

		return idempotency.Record{}, err
	}

	return created, nil
}

func (s *Store) GetRecord(ctx context.Context, key string) (idempotency.Record, error) {
	const query = `
		SELECT
			key,
			request_hash,
			status,
			response_code,
			response_body,
			created_at,
			updated_at,
			expires_at
		FROM idempotency_records
		WHERE key = $1
	`

	record, err := scanRecord(s.pool.QueryRow(ctx, query, key))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return idempotency.Record{}, idempotency.ErrRecordNotFound
		}

		return idempotency.Record{}, err
	}

	return record, nil
}

func (s *Store) UpdateRecord(ctx context.Context, record idempotency.Record) (idempotency.Record, error) {
	const query = `
		UPDATE idempotency_records
		SET
			request_hash = $2,
			status = $3,
			response_code = $4,
			response_body = $5,
			updated_at = $6,
			expires_at = $7
		WHERE key = $1
		RETURNING
			key,
			request_hash,
			status,
			response_code,
			response_body,
			created_at,
			updated_at,
			expires_at
	`

	updated, err := scanRecord(s.pool.QueryRow(ctx, query,
		record.Key,
		record.RequestHash,
		string(record.Status),
		record.ResponseCode,
		record.ResponseBody,
		record.UpdatedAt,
		record.ExpiresAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return idempotency.Record{}, idempotency.ErrRecordNotFound
		}

		return idempotency.Record{}, err
	}

	return updated, nil
}

type recordScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner recordScanner) (idempotency.Record, error) {
	var record idempotency.Record
	var status string

	if err := scanner.Scan(
		&record.Key,
		&record.RequestHash,
		&status,
		&record.ResponseCode,
		&record.ResponseBody,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.ExpiresAt,
	); err != nil {
		return idempotency.Record{}, err
	}

	record.Status = idempotency.Status(status)
	record.ResponseBody = append([]byte(nil), record.ResponseBody...)

	return record, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
}
