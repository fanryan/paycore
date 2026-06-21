package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/payment"
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

func (s *Store) CreatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	const query = `
		INSERT INTO payments (
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			settled_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			settled_at,
			created_at,
			updated_at
	`

	created, err := scanPayment(s.queryRow(ctx, query,
		paymentRecord.ID,
		paymentRecord.MerchantID,
		paymentRecord.PayerID,
		paymentRecord.AmountMinor,
		paymentRecord.Currency,
		string(paymentRecord.Status),
		paymentRecord.AuthorizationHoldID,
		paymentRecord.AuthorizedAt,
		paymentRecord.ExpiresAt,
		paymentRecord.CapturedAt,
		paymentRecord.SettledAt,
		paymentRecord.CreatedAt,
		paymentRecord.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return payment.Payment{}, payment.ErrDuplicatePayment
		}

		return payment.Payment{}, err
	}

	return created, nil
}

func (s *Store) GetPayment(ctx context.Context, paymentID string) (payment.Payment, error) {
	const query = `
		SELECT
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			settled_at,
			created_at,
			updated_at
		FROM payments
		WHERE id = $1
	`

	paymentRecord, err := scanPayment(s.queryRow(ctx, query, paymentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.Payment{}, payment.ErrPaymentNotFound
		}

		return payment.Payment{}, err
	}

	return paymentRecord, nil
}

func (s *Store) ListExpiredAuthorizedPayments(ctx context.Context, now time.Time, limit int) ([]payment.Payment, error) {
	if limit <= 0 {
		limit = 100
	}

	const query = `
		SELECT
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			settled_at,
			created_at,
			updated_at
		FROM payments
		WHERE status = 'AUTHORIZED'
		AND expires_at <= $1
		ORDER BY expires_at ASC, id ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := s.executor(ctx).Query(ctx, query, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	payments := make([]payment.Payment, 0)
	for rows.Next() {
		paymentRecord, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}

		payments = append(payments, paymentRecord)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return payments, nil
}

func (s *Store) UpdatePayment(ctx context.Context, paymentRecord payment.Payment) (payment.Payment, error) {
	const query = `
		UPDATE payments
		SET
			merchant_id = $2,
			payer_id = $3,
			amount_minor = $4,
			currency = $5,
			status = $6,
			authorization_hold_id = $7,
			authorized_at = $8,
			expires_at = $9,
			captured_at = $10,
			settled_at = $11,
			updated_at = $12
		WHERE id = $1
		RETURNING
			id,
			merchant_id,
			payer_id,
			amount_minor,
			currency,
			status,
			authorization_hold_id,
			authorized_at,
			expires_at,
			captured_at,
			settled_at,
			created_at,
			updated_at
	`

	updated, err := scanPayment(s.queryRow(ctx, query,
		paymentRecord.ID,
		paymentRecord.MerchantID,
		paymentRecord.PayerID,
		paymentRecord.AmountMinor,
		paymentRecord.Currency,
		string(paymentRecord.Status),
		paymentRecord.AuthorizationHoldID,
		paymentRecord.AuthorizedAt,
		paymentRecord.ExpiresAt,
		paymentRecord.CapturedAt,
		paymentRecord.SettledAt,
		paymentRecord.UpdatedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.Payment{}, payment.ErrPaymentNotFound
		}

		return payment.Payment{}, err
	}

	return updated, nil
}

func (s *Store) CreateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	const query = `
		INSERT INTO payment_holds (
			id,
			payment_id,
			payer_id,
			amount_minor,
			currency,
			status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING
			id,
			payment_id,
			payer_id,
			amount_minor,
			currency,
			status,
			created_at,
			updated_at
	`

	created, err := scanHold(s.queryRow(ctx, query,
		hold.ID,
		hold.PaymentID,
		hold.PayerID,
		hold.AmountMinor,
		hold.Currency,
		string(hold.Status),
		hold.CreatedAt,
		hold.UpdatedAt,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return payment.Hold{}, payment.ErrDuplicateHold
		}

		return payment.Hold{}, err
	}

	return created, nil
}

func (s *Store) GetHold(ctx context.Context, holdID string) (payment.Hold, error) {
	const query = `
		SELECT
			id,
			payment_id,
			payer_id,
			amount_minor,
			currency,
			status,
			created_at,
			updated_at
		FROM payment_holds
		WHERE id = $1
	`

	hold, err := scanHold(s.queryRow(ctx, query, holdID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.Hold{}, payment.ErrHoldNotFound
		}

		return payment.Hold{}, err
	}

	return hold, nil
}

func (s *Store) GetHoldByPaymentID(ctx context.Context, paymentID string) (payment.Hold, error) {
	const query = `
		SELECT
			id,
			payment_id,
			payer_id,
			amount_minor,
			currency,
			status,
			created_at,
			updated_at
		FROM payment_holds
		WHERE payment_id = $1
	`

	hold, err := scanHold(s.queryRow(ctx, query, paymentID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.Hold{}, payment.ErrHoldNotFound
		}

		return payment.Hold{}, err
	}

	return hold, nil
}

func (s *Store) UpdateHold(ctx context.Context, hold payment.Hold) (payment.Hold, error) {
	const query = `
		UPDATE payment_holds
		SET
			payment_id = $2,
			payer_id = $3,
			amount_minor = $4,
			currency = $5,
			status = $6,
			updated_at = $7
		WHERE id = $1
		RETURNING
			id,
			payment_id,
			payer_id,
			amount_minor,
			currency,
			status,
			created_at,
			updated_at
	`

	updated, err := scanHold(s.queryRow(ctx, query,
		hold.ID,
		hold.PaymentID,
		hold.PayerID,
		hold.AmountMinor,
		hold.Currency,
		string(hold.Status),
		hold.UpdatedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payment.Hold{}, payment.ErrHoldNotFound
		}

		return payment.Hold{}, err
	}

	return updated, nil
}

func (s *Store) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.executor(ctx).QueryRow(ctx, sql, args...)
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

type paymentScanner interface {
	Scan(dest ...any) error
}

func scanPayment(scanner paymentScanner) (payment.Payment, error) {
	var paymentRecord payment.Payment
	var status string
	var capturedAt pgtype.Timestamptz
	var settledAt pgtype.Timestamptz

	if err := scanner.Scan(
		&paymentRecord.ID,
		&paymentRecord.MerchantID,
		&paymentRecord.PayerID,
		&paymentRecord.AmountMinor,
		&paymentRecord.Currency,
		&status,
		&paymentRecord.AuthorizationHoldID,
		&paymentRecord.AuthorizedAt,
		&paymentRecord.ExpiresAt,
		&capturedAt,
		&settledAt,
		&paymentRecord.CreatedAt,
		&paymentRecord.UpdatedAt,
	); err != nil {
		return payment.Payment{}, err
	}

	paymentRecord.Status = payment.Status(status)

	if capturedAt.Valid {
		value := capturedAt.Time
		paymentRecord.CapturedAt = &value
	}

	if settledAt.Valid {
		value := settledAt.Time
		paymentRecord.SettledAt = &value
	}

	return paymentRecord, nil
}

type holdScanner interface {
	Scan(dest ...any) error
}

func scanHold(scanner holdScanner) (payment.Hold, error) {
	var hold payment.Hold
	var status string

	if err := scanner.Scan(
		&hold.ID,
		&hold.PaymentID,
		&hold.PayerID,
		&hold.AmountMinor,
		&hold.Currency,
		&status,
		&hold.CreatedAt,
		&hold.UpdatedAt,
	); err != nil {
		return payment.Hold{}, err
	}

	hold.Status = payment.HoldStatus(status)

	return hold, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
}
