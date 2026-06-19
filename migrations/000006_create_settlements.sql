CREATE TABLE settlement_batches (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    claimed_by TEXT,
    locked_until TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT settlement_batches_status_check
        CHECK (status IN ('CREATED', 'PROCESSING', 'COMPLETED', 'FAILED')),
    CONSTRAINT settlement_batches_window_check
        CHECK (window_end > window_start),
    CONSTRAINT settlement_batches_processing_lock_check
        CHECK (
            (status = 'PROCESSING' AND claimed_by IS NOT NULL AND locked_until IS NOT NULL)
            OR status <> 'PROCESSING'
        ),
    CONSTRAINT settlement_batches_completed_at_check
        CHECK (
            (status = 'COMPLETED' AND completed_at IS NOT NULL)
            OR status <> 'COMPLETED'
        )
);

ALTER TABLE payments
    ADD COLUMN settlement_batch_id TEXT REFERENCES settlement_batches (id);

CREATE INDEX settlement_batches_status_idx ON settlement_batches (status);
CREATE INDEX settlement_batches_window_idx ON settlement_batches (window_start, window_end);
CREATE INDEX settlement_batches_stale_locks_idx
    ON settlement_batches (locked_until)
    WHERE status = 'PROCESSING';

CREATE INDEX payments_settlement_batch_id_idx ON payments (settlement_batch_id);
CREATE INDEX payments_captured_unsettled_idx
    ON payments (captured_at)
    WHERE status = 'CAPTURED' AND settlement_batch_id IS NULL;

CREATE TABLE settlement_line_items (
    id TEXT PRIMARY KEY,
    settlement_batch_id TEXT NOT NULL REFERENCES settlement_batches (id),
    merchant_id TEXT NOT NULL REFERENCES merchants (id),
    payment_id TEXT NOT NULL UNIQUE REFERENCES payments (id),
    amount_minor BIGINT NOT NULL,
    fee_amount_minor BIGINT NOT NULL DEFAULT 0,
    net_amount_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,
    payment_captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT settlement_line_items_amount_positive_check
        CHECK (amount_minor > 0),
    CONSTRAINT settlement_line_items_fee_non_negative_check
        CHECK (fee_amount_minor >= 0),
    CONSTRAINT settlement_line_items_net_amount_check
        CHECK (net_amount_minor = amount_minor - fee_amount_minor AND net_amount_minor >= 0),
    CONSTRAINT settlement_line_items_currency_check
        CHECK (currency = upper(currency))
);

CREATE INDEX settlement_line_items_batch_idx ON settlement_line_items (settlement_batch_id);
CREATE INDEX settlement_line_items_merchant_idx ON settlement_line_items (merchant_id);
