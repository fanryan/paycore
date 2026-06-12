CREATE TABLE payments (
    id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL REFERENCES merchants (id),
    payer_id TEXT NOT NULL REFERENCES payers (id),
    amount_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL,
    authorization_hold_id TEXT NOT NULL,
    authorized_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    captured_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT payments_amount_positive_check
        CHECK (amount_minor > 0),
    CONSTRAINT payments_currency_check
        CHECK (currency = upper(currency)),
    CONSTRAINT payments_status_check
        CHECK (status IN ('PENDING', 'AUTHORIZED', 'CAPTURED', 'SETTLED', 'EXPIRED', 'FAILED')),
    CONSTRAINT payments_expiry_after_authorized_check
        CHECK (expires_at > authorized_at),
    CONSTRAINT payments_captured_at_check
        CHECK (
            (status = 'CAPTURED' AND captured_at IS NOT NULL)
            OR status <> 'CAPTURED'
        ),
    CONSTRAINT payments_settled_at_check
        CHECK (
            (status = 'SETTLED' AND settled_at IS NOT NULL)
            OR status <> 'SETTLED'
        )
);

CREATE INDEX payments_merchant_id_idx ON payments (merchant_id);
CREATE INDEX payments_payer_id_idx ON payments (payer_id);
CREATE INDEX payments_status_idx ON payments (status);
CREATE INDEX payments_authorized_at_idx ON payments (authorized_at);
CREATE INDEX payments_expires_at_idx ON payments (expires_at);

CREATE TABLE payment_holds (
    id TEXT PRIMARY KEY,
    payment_id TEXT NOT NULL UNIQUE REFERENCES payments (id),
    payer_id TEXT NOT NULL REFERENCES payers (id),
    amount_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT payment_holds_amount_positive_check
        CHECK (amount_minor > 0),
    CONSTRAINT payment_holds_currency_check
        CHECK (currency = upper(currency)),
    CONSTRAINT payment_holds_status_check
        CHECK (status IN ('HELD', 'CAPTURED', 'RELEASED'))
);

CREATE INDEX payment_holds_payer_id_idx ON payment_holds (payer_id);
CREATE INDEX payment_holds_status_idx ON payment_holds (status);
