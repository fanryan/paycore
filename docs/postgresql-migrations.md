# PostgreSQL Migrations

This document explains the current PayCore PostgreSQL migration foundation as it exists today. It is written for resume and interview preparation, so it focuses on the schema decisions currently captured in SQL, what is intentionally not wired yet, and what is planned next.

## 1. Current Migration Scope

### Implemented

The repository currently includes plain SQL migrations for:

- Merchant table creation in `migrations/000001_create_merchants.sql`.
- Payer table creation in `migrations/000002_create_payers.sql`.
- Payment and hold table creation in `migrations/000003_create_payments.sql`.
- Idempotency record table creation in `migrations/000004_create_idempotency_records.sql`.

The migrations define:

- Primary keys matching current domain ids.
- Merchant status constraints.
- Uppercase 3-letter currency constraints.
- Non-negative payer balances.
- Non-negative payer optimistic-locking version.
- Payment status constraints.
- Payment hold status constraints.
- Idempotency status constraints.
- Timestamp columns for creation and update time.

### Not Implemented Yet

These are planned but not currently implemented:

- Migration runner.
- Automatic migration execution in app startup.
- Settlement migrations.
- Outbox migrations.
- PostgreSQL repository adapters.
- Integration tests against local PostgreSQL.

## 2. Migration Files

Current files:

```text
migrations/
  000001_create_merchants.sql
  000002_create_payers.sql
  000003_create_payments.sql
  000004_create_idempotency_records.sql
```

The files are intentionally plain SQL for now. A migration tool such as `golang-migrate`, `goose`, or a small custom runner can be selected after the initial schema shape is stable.

## 3. Merchant Schema

The `merchants` table stores:

- `id`
- `name`
- `status`
- `settlement_currency`
- `created_at`
- `updated_at`

Current constraints:

- `id` is the primary key.
- `status` must be `ACTIVE`, `SUSPENDED`, or `CLOSED`.
- `settlement_currency` must be uppercase.

The current schema mirrors `internal/merchant/entity.go`.

## 4. Payer Schema

The `payers` table stores:

- `id`
- `available_balance_minor`
- `held_balance_minor`
- `currency`
- `version`
- `created_at`
- `updated_at`

Current constraints:

- `id` is the primary key.
- balances must be non-negative.
- `version` must be non-negative.
- `currency` must be uppercase.

The `version` column exists for upcoming optimistic concurrency checks in the durable payer repository.

## 5. Payment And Hold Schema

The `payments` table stores:

- `id`
- `merchant_id`
- `payer_id`
- `amount_minor`
- `currency`
- `status`
- `authorization_hold_id`
- authorization, expiry, capture, and settlement timestamps
- creation and update timestamps

Current constraints:

- `id` is the primary key.
- `merchant_id` references `merchants`.
- `payer_id` references `payers`.
- amount must be positive.
- `currency` must be uppercase.
- `status` must match the current payment status constants.
- `expires_at` must be after `authorized_at`.

The `payment_holds` table stores:

- `id`
- `payment_id`
- `payer_id`
- `amount_minor`
- `currency`
- `status`
- creation and update timestamps

Current constraints:

- `id` is the primary key.
- `payment_id` is unique and references `payments`.
- `payer_id` references `payers`.
- amount must be positive.
- `currency` must be uppercase.
- `status` must match the current hold status constants.

`payments.authorization_hold_id` is kept as text for now to avoid circular foreign keys between payments and holds. The repository transaction will enforce the relationship when durable adapters are added.

## 6. Idempotency Schema

The `idempotency_records` table stores:

- `key`
- `request_hash`
- `status`
- `response_code`
- `response_body`
- `created_at`
- `updated_at`
- `expires_at`

Current constraints:

- `key` is the primary key.
- `status` must be `IN_PROGRESS`, `COMPLETED`, or `FAILED`.
- response code must be non-negative.
- `expires_at` must be after `created_at`.

The response body is stored as `BYTEA` so the repository can replay the exact HTTP response payload.

## 7. Current Runtime Relationship

The PayCore API does not yet run these migrations or connect repositories to PostgreSQL.

Current runtime repositories remain in memory:

```text
merchant memory repository
payer memory repository
payment memory repository
idempotency memory repository
```

The migrations exist to prepare for durable repository adapters.

## 8. Manual Usage

With local PostgreSQL running:

```bash
docker compose up -d postgres
```

The SQL can be applied manually for now:

```bash
docker exec -i paycore-postgres psql -U paycore -d paycore < migrations/000001_create_merchants.sql
docker exec -i paycore-postgres psql -U paycore -d paycore < migrations/000002_create_payers.sql
docker exec -i paycore-postgres psql -U paycore -d paycore < migrations/000003_create_payments.sql
docker exec -i paycore-postgres psql -U paycore -d paycore < migrations/000004_create_idempotency_records.sql
```

This manual flow is temporary until a migration runner is selected.

## Checklist

- [x] Add merchant table migration.
- [x] Add payer table migration.
- [x] Add payment and hold migrations.
- [x] Add idempotency record migration.
- [ ] Add migration runner.
- [ ] Add settlement migration.
- [ ] Add outbox migration.
- [ ] Add PostgreSQL repository adapters.
