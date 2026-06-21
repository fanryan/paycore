# PostgreSQL Migrations

This document explains the current PayCore PostgreSQL migration foundation as it exists today. It is written for resume and interview preparation, so it focuses on the schema decisions captured in SQL and how the schema supports durable payment, idempotency, outbox, and settlement state.

## 1. Current Migration Scope

### Implemented

The repository currently includes plain SQL migrations for:

- Merchant table creation in `migrations/000001_create_merchants.sql`.
- Payer table creation in `migrations/000002_create_payers.sql`.
- Payment and hold table creation in `migrations/000003_create_payments.sql`.
- Idempotency record table creation in `migrations/000004_create_idempotency_records.sql`.
- Outbox event table creation in `migrations/000005_create_outbox_events.sql`.
- Settlement batch and line item table creation in `migrations/000006_create_settlements.sql`.
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
- Outbox event status constraints.
- Settlement batch status constraints.
- Settlement double-settlement constraints.
- Timestamp columns for creation and update time.

### Out Of Scope And Future Hardening

These items are outside the current local-first milestone:

- Automatic migration execution in app startup.
- Single transaction that also includes idempotency completion.

## 2. Migration Files

Current files:

```text
migrations/
  000001_create_merchants.sql
  000002_create_payers.sql
  000003_create_payments.sql
  000004_create_idempotency_records.sql
  000005_create_outbox_events.sql
  000006_create_settlements.sql
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

The `version` column supports optimistic concurrency checks in the durable payer repository.

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

## 7. Outbox Schema

The `outbox_events` table stores:

- `id`
- `aggregate_type`
- `aggregate_id`
- `event_type`
- `payload`
- `status`
- `attempts`
- `available_at`
- `created_at`
- `updated_at`
- lock, publish, and error metadata for future publisher workers

Current constraints:

- `id` is the primary key.
- `status` must be `PENDING`, `IN_PROGRESS`, `PUBLISHED`, or `FAILED`.
- `attempts` must be non-negative.

Current indexes:

- partial index on pending events by availability time for publisher claim scans.
- aggregate lookup index on aggregate type, aggregate id, and creation time.

Payment authorization currently writes a `payment.authorized` event. Payment capture writes a `payment.captured` event. In Postgres mode, those event inserts run inside the payment service transaction with payer, payment, and hold mutations.

## 8. Settlement Schema

The `settlement_batches` table stores:

- `id`
- `status`
- settlement window start and end
- processing lock fields
- completion timestamp
- last error
- created and updated timestamps

Current constraints:

- `id` is the primary key.
- `status` must be `CREATED`, `PROCESSING`, `COMPLETED`, or `FAILED`.
- `window_end` must be after `window_start`.
- processing batches must have `claimed_by` and `locked_until`.
- completed batches must have `completed_at`.

The `settlement_line_items` table stores:

- `id`
- `settlement_batch_id`
- `merchant_id`
- `payment_id`
- gross, fee, and net amounts in minor units
- currency
- payment captured timestamp
- created timestamp

Current constraints:

- `payment_id` is unique to prevent duplicate settlement line items.
- amount must be positive.
- fee amount must be non-negative.
- net amount must equal amount minus fee.
- currency must be uppercase.

The migration also adds `payments.settlement_batch_id` and indexes captured, unsettled payments for future settlement claim queries.

## 9. Migration Runner

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

## 10. Current Runtime Relationship

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

In Postgres mode, merchant, payer, payment, hold, idempotency, outbox, and settlement repositories use PostgreSQL.

## 11. Manual Usage

With local PostgreSQL running:

```bash
docker compose up -d postgres
```

Apply migrations with:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

Run the command repeatedly as needed. Already-applied migrations are skipped.
