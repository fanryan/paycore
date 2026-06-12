CREATE TABLE payers (
    id TEXT PRIMARY KEY,
    available_balance_minor BIGINT NOT NULL,
    held_balance_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT payers_available_balance_non_negative_check
        CHECK (available_balance_minor >= 0),
    CONSTRAINT payers_held_balance_non_negative_check
        CHECK (held_balance_minor >= 0),
    CONSTRAINT payers_version_non_negative_check
        CHECK (version >= 0),
    CONSTRAINT payers_currency_check
        CHECK (currency = upper(currency))
);
