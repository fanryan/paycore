CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    available_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    published_at TIMESTAMPTZ,
    last_error TEXT,

    CONSTRAINT outbox_events_status_check
        CHECK (status IN ('PENDING', 'IN_PROGRESS', 'PUBLISHED', 'FAILED')),

    CONSTRAINT outbox_events_attempts_non_negative_check
        CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending_available
    ON outbox_events (available_at, created_at, id)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events (aggregate_type, aggregate_id, created_at);