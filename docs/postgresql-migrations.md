# PostgreSQL Migrations

This document explains the current PayCore PostgreSQL migration foundation as it exists today. It is written for resume and interview preparation, so it focuses on the schema decisions currently captured in SQL, what is intentionally not wired yet, and what is planned next.

## 1. Current Migration Scope

### Implemented

The repository currently includes plain SQL migrations for:

- Merchant table creation in `migrations/000001_create_merchants.sql`.
- Payer table creation in `migrations/000002_create_payers.sql`.
- Payment and hold table creation in `migrations/000003_create_payments.sql`.
- Idempotency record table creation in `migrations/000004_create_idempotency_records.sql`.
- Migration runner command in `cmd/paycore-migrate`.

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

- Automatic migration execution in app startup.
- Settlement migrations.
- Outbox migrations.
- Single transaction that also includes idempotency completion and future outbox writes.
- Redis-backed idempotency response cache.

## 2. Migration Files

Current files:

```text
migrations/
  000001_create_merchants.sql
  000002_create_payers.sql
  000003_create_payments.sql
  000004_create_idempotency_records.sql
```

The files are plain SQL and are applied by the local `paycore-migrate` command.

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

`payments.authorization_hold_id` is kept as text for now to avoid circular foreign keys between payments and holds. The service currently persists the payment before the hold so the hold can reference the existing payment row. In Postgres mode, payer balance updates, payment creation, and hold creation run inside one transaction through `internal/shared/db.Transactor`. Idempotency completion is still handled outside that transaction and should be folded in later.

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

## 7. Migration Runner

The migration runner lives at:

```text
cmd/paycore-migrate/main.go
```

It currently:

- reads `PAYCORE_DATABASE_URL`
- connects to PostgreSQL with `pgxpool`
- creates `schema_migrations` if needed
- reads `migrations/*.sql`
- applies migrations in filename order
- records applied migration filenames
- skips already-applied migrations on later runs

Run it with:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

## 8. Current Runtime Relationship

The PayCore API does not run migrations automatically. Migrations are applied manually through `cmd/paycore-migrate`.

The API can run with either memory repositories or PostgreSQL repositories. Memory remains the default:

```text
merchant memory repository
payer memory repository
payment memory repository
idempotency memory repository
```

PostgreSQL runtime mode is enabled with:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-api
```

In Postgres mode, merchant, payer, payment, hold, and idempotency repositories use PostgreSQL.

## 9. Manual Usage

With local PostgreSQL running:

```bash
docker compose up -d postgres
```

Apply migrations with:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

Run the command repeatedly as needed. Already-applied migrations are skipped.

## Checklist

- [x] Add merchant table migration.
- [x] Add payer table migration.
- [x] Add payment and hold migrations.
- [x] Add idempotency record migration.
- [x] Add migration runner.
- [ ] Add settlement migration.
- [ ] Add outbox migration.
- [x] Add PostgreSQL repository adapters.
- [x] Wire API runtime to PostgreSQL repository adapters.
- [x] Add Postgres-backed HTTP lifecycle smoke test.
- [x] Add transaction boundary around authorization and capture business mutations.
- [ ] Include durable idempotency completion in the transaction boundary.
