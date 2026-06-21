package postgres

import (
	"context"
	"errors"
	"time"

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

func (s *Store) ClaimPendingEvents(ctx context.Context, input outbox.ClaimPendingEventsInput) ([]outbox.Event, error) {
	if input.Limit <= 0 {
		return []outbox.Event{}, nil
	}

	tx, ok := db.TxFromContext(ctx)
	if !ok {
		return nil, errors.New("claim pending outbox events requires an active transaction")
	}

	const query = `
		WITH claimable AS (
			SELECT id
			FROM outbox_events
			WHERE status IN ('PENDING', 'FAILED')
			AND available_at <= $1
			ORDER BY available_at ASC, created_at ASC, id ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS event
		SET
			status = 'IN_PROGRESS',
			attempts = event.attempts + 1,
			locked_at = $1,
			locked_by = $3,
			last_error = NULL,
			updated_at = $1
		FROM claimable
		WHERE event.id = claimable.id
		RETURNING
			event.id,
			event.aggregate_type,
			event.aggregate_id,
			event.event_type,
			event.payload,
			event.status,
			event.attempts,
			event.available_at,
			event.created_at,
			event.updated_at,
			event.locked_at,
			event.locked_by,
			event.published_at,
			event.last_error
	`

	rows, err := tx.Query(ctx, query, input.Now.UTC(), input.Limit, input.WorkerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]outbox.Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (s *Store) MarkEventPublished(ctx context.Context, eventID string, now time.Time) (outbox.Event, error) {
	const query = `
		UPDATE outbox_events
		SET
			status = 'PUBLISHED',
			published_at = $2,
			locked_at = NULL,
			locked_by = NULL,
			last_error = NULL,
			updated_at = $2
		WHERE id = $1
		AND status = 'IN_PROGRESS'
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

	published, err := scanEvent(s.queryRow(ctx, query, eventID, now.UTC()))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outbox.Event{}, outbox.ErrEventNotFound
		}

		return outbox.Event{}, err
	}

	return published, nil
}

func (s *Store) MarkEventFailed(ctx context.Context, input outbox.MarkEventFailedInput) (outbox.Event, error) {
	const query = `
		UPDATE outbox_events
		SET
			status = 'FAILED',
			available_at = $2,
			locked_at = NULL,
			locked_by = NULL,
			published_at = NULL,
			last_error = $3,
			updated_at = $4
		WHERE id = $1
		AND status = 'IN_PROGRESS'
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

	failed, err := scanEvent(s.queryRow(
		ctx,
		query,
		input.EventID,
		input.NextAvailable.UTC(),
		input.ErrorMessage,
		input.Now.UTC(),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return outbox.Event{}, outbox.ErrEventNotFound
		}

		return outbox.Event{}, err
	}

	return failed, nil
}

func (s *Store) Stats(ctx context.Context, input outbox.StatsInput) (outbox.Stats, error) {
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	const query = `
		SELECT
			COUNT(*),
			MIN(available_at)
		FROM outbox_events
		WHERE status IN ('PENDING', 'FAILED')
		AND available_at <= $1
	`

	var pendingEvents int
	var oldestAvailable pgtype.Timestamptz
	if err := s.queryRow(ctx, query, now).Scan(&pendingEvents, &oldestAvailable); err != nil {
		return outbox.Stats{}, err
	}

	stats := outbox.Stats{
		PendingEvents: pendingEvents,
	}
	if oldestAvailable.Valid {
		stats.PublishLag = now.Sub(oldestAvailable.Time)
		if stats.PublishLag < 0 {
			stats.PublishLag = 0
		}
	}

	return stats, nil
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
