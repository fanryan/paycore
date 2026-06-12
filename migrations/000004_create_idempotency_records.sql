CREATE TABLE idempotency_records (
    key TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    status TEXT NOT NULL,
    response_code INTEGER NOT NULL DEFAULT 0,
    response_body BYTEA NOT NULL DEFAULT ''::bytea,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT idempotency_records_status_check
        CHECK (status IN ('IN_PROGRESS', 'COMPLETED', 'FAILED')),
    CONSTRAINT idempotency_records_response_code_check
        CHECK (response_code >= 0),
    CONSTRAINT idempotency_records_expiry_after_created_check
        CHECK (expires_at > created_at)
);

CREATE INDEX idempotency_records_status_idx ON idempotency_records (status);
CREATE INDEX idempotency_records_expires_at_idx ON idempotency_records (expires_at);
