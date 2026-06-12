CREATE TABLE merchants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    settlement_currency CHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT merchants_status_check
        CHECK (status IN ('ACTIVE', 'SUSPENDED', 'CLOSED')),
    CONSTRAINT merchants_settlement_currency_check
        CHECK (settlement_currency = upper(settlement_currency))
);

CREATE INDEX merchants_status_idx ON merchants (status);
