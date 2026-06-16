package postgres

import (
	"context"
	"errors"

	"github.com/fanryan/paycore/internal/outbox"
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

func (s *Store) CreateEvent(ctx context.Context, event outbox.Event) (outbox.Event, error) {
	const query = `
		INSERT INTO outbox_events (
			id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload,
			status,
			attempts,
			available_at,
			created_at,
			updated_at,
			locked_at,
			locked_by,
			published_at,
			last_error
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING
			id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload,
			status,
			attempts,
			available_at,
			created_at,
			updated_at,
			locked_at,
			locked_by,
			published_at,
			last_error
	`

	created, err := scanEvent(s.queryRow(ctx, query,
		event.ID,
		event.AggregateType,
		event.AggregateID,
		event.EventType,
		event.Payload,
		string(event.Status),
		event.Attempts,
		event.AvailableAt,
		event.CreatedAt,
		event.UpdatedAt,
		event.LockedAt,
		event.LockedBy,
		event.PublishedAt,
		event.LastError,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return outbox.Event{}, outbox.ErrDuplicateEvent
		}

		return outbox.Event{}, err
	}

	return created, nil
}

func (s *Store) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx, ok := db.TxFromContext(ctx); ok {
		return tx.QueryRow(ctx, sql, args...)
	}

	return s.pool.QueryRow(ctx, sql, args...)
}

type eventScanner interface {
	Scan(dest ...any) error
}

func scanEvent(scanner eventScanner) (outbox.Event, error) {
	var event outbox.Event
	var status string
	var lockedAt pgtype.Timestamptz
	var lockedBy *string
	var publishedAt pgtype.Timestamptz
	var lastError *string

	if err := scanner.Scan(
		&event.ID,
		&event.AggregateType,
		&event.AggregateID,
		&event.EventType,
		&event.Payload,
		&status,
		&event.Attempts,
		&event.AvailableAt,
		&event.CreatedAt,
		&event.UpdatedAt,
		&lockedAt,
		&lockedBy,
		&publishedAt,
		&lastError,
	); err != nil {
		return outbox.Event{}, err
	}

	event.Status = outbox.Status(status)
	event.Payload = append([]byte(nil), event.Payload...)

	if lockedAt.Valid {
		value := lockedAt.Time
		event.LockedAt = &value
	}

	event.LockedBy = lockedBy

	if publishedAt.Valid {
		value := publishedAt.Time
		event.PublishedAt = &value
	}

	event.LastError = lastError

	return event, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == uniqueViolationCode
}
