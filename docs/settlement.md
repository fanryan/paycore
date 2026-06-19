# Settlement

This document explains the current PayCore settlement foundation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is still planned, and how PayCore prevents double settlement.

## 1. Current Feature Scope

### Implemented

- Settlement batch entity in `internal/settlement/entity.go`.
- Settlement line item entity in `internal/settlement/entity.go`.
- Settlement repository interface in `internal/settlement/repository.go`.
- Settlement service in `internal/settlement/service.go`.
- PostgreSQL settlement repository adapter in `internal/settlement/adapters/postgres/repository.go`.
- Settlement schema migration in `migrations/000006_create_settlements.sql`.
- Settlement batch statuses:
  - `CREATED`
  - `PROCESSING`
  - `COMPLETED`
  - `FAILED`
- Batch processing lock fields:
  - `claimed_by`
  - `locked_until`
- Batch lifecycle methods:
  - create batch
  - start processing
  - complete
  - fail
  - stale-lock detection
- Line item construction with gross amount, fee amount, and net amount.
- Migration-level double-settlement guard:
  - `payments.settlement_batch_id`
  - unique `settlement_line_items.payment_id`
- PostgreSQL adapter support for:
  - create/get/update batch
  - claim captured payments
  - create line item
  - list line items
  - transaction context propagation
- Service-level batch orchestration:
  - create batch
  - start processing
  - claim captured payments
  - create line items
  - mark claimed payments `SETTLED`
  - create `payment.settled` outbox events
  - complete batch
- Domain tests for batch lifecycle, stale locks, line item validation, and net amount calculation.
- Service tests for batch creation and empty-batch completion.
- PostgreSQL adapter integration tests.

### Not Implemented Yet

- In-memory settlement repository adapter.
- Looping settlement scheduler.
- `POST /settlement-batches`.
- `GET /settlement-batches/{batch_id}`.
- Redis settlement coordination lock.
- Stale batch recovery worker.
- Settlement metrics.

### Public Endpoints

None currently.

### Protected Endpoints Or Protected By Default

No settlement HTTP endpoints exist yet. When added, settlement endpoints should be protected by operator/admin auth.

## 2. Runtime Flow

### App Startup

Settlement is not wired into `cmd/paycore-api/main.go` yet.

Settlement can be run as a one-shot worker command:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-settlement-worker
```

The worker processes the previous completed time window and exits. For example, at `11:37` with a 60-minute window, it settles `10:00` to `11:00`.

### Feature Package Boundary

```text
internal/settlement
  |
  +--> entity.go
  +--> repository.go
  +--> service.go
  +--> entity_test.go
  |
  +--> adapters/postgres/repository.go

migrations/
  |
  +--> 000006_create_settlements.sql
```

The settlement package owns settlement batch and line item domain rules. Future repository adapters should live under `internal/settlement/adapters/...`.

## 3. Main Settlement Flow

### Service Input

The current service input represents a settlement time window:

```go
settlement.CreateBatchInput{
    WindowStart: windowStart,
    WindowEnd:   windowEnd,
}
```

### Step-by-Step

1. Caller triggers settlement for a time window.
2. Service creates a settlement batch in `CREATED`.
3. Service marks the batch `PROCESSING` with `claimed_by` and `locked_until`.
4. Repository claims eligible captured payments for the batch.
5. Service creates settlement line items.
6. Service loads each claimed payment.
7. Service marks each payment `SETTLED`.
8. Service writes a `payment.settled` outbox event for each settled payment.
9. Service marks settlement batch `COMPLETED`.

### Diagram

```text
Settlement Trigger
  |
  v
Settlement Service
  |
  +--> create settlement batch
  +--> claim captured payments
  +--> create line items
  +--> mark payments SETTLED
  +--> write payment.settled outbox events
  +--> complete batch
```

### Failure Path

Current domain failure paths include:

```text
ErrInvalidSettlementWindow
ErrInvalidBatchStatus
ErrInvalidLineItemAmount
```

Planned repository failure paths include:

```text
ErrDuplicateBatch
ErrDuplicateLineItem
ErrPaymentAlreadySettled
ErrBatchNotFound
ErrLineItemNotFound
```

## 4. Crash Recovery Flow

### Current Foundation

Settlement batches support processing locks:

```text
claimed_by
locked_until
```

`Batch.IsStale(now)` returns true when:

- batch status is `PROCESSING`
- `locked_until` is not nil
- `locked_until <= now`

### Planned Recovery

If a worker crashes after claiming a batch:

1. Batch remains `PROCESSING`.
2. `locked_until` eventually expires.
3. Another worker can detect the stale batch.
4. The worker resumes processing the same batch.
5. Claimed payments stay associated with the same batch.

## Validation And Errors

Batch validation:

- batch id is required
- settlement window end must be after window start
- worker id is required when starting processing
- processing lock expiry must be after now
- only processing batches can complete or fail

Line item validation:

- line item id is required
- batch id is required
- merchant id is required
- payment id is required
- amount must be positive
- fee cannot be negative
- fee cannot exceed amount
- currency must be a valid 3-letter uppercase-normalized ISO currency code

## Persistence

`settlement_batches` stores:

- batch id
- status
- window start and end
- processing lock fields
- completion timestamp
- last error
- created and updated timestamps

`settlement_line_items` stores:

- line item id
- settlement batch id
- merchant id
- payment id
- amount, fee, and net amount in minor units
- currency
- payment captured timestamp
- created timestamp

`payments.settlement_batch_id` links a payment to the batch that claimed it.

Double-settlement protection:

- `settlement_line_items.payment_id` is unique.
- `payments.settlement_batch_id` links a payment to at most one batch.
- `payments_captured_unsettled_idx` supports scanning captured payments that have not been claimed.

## Tests

Current tests cover:

- batch creation
- invalid settlement windows
- start processing
- complete
- fail
- stale lock detection
- line item net amount calculation
- line item amount validation
- service creates completed batch with line items
- service completes empty batch when no payments are claimed
- service marks claimed payments settled
- service writes `payment.settled` outbox events
- settlement worker window calculation
- settlement worker config parsing
- Postgres service integration for settled payment and outbox event
- Postgres batch create/get/update
- Postgres captured-payment claim flow
- Postgres line item create/list
- duplicate batch mapping
- duplicate line item mapping
- transaction rollback through context propagation

Run:

```bash
go test ./internal/settlement
```

Run settlement worker command tests:

```bash
go test ./cmd/paycore-settlement-worker
```

Run PostgreSQL settlement service integration tests:

```bash
docker compose up -d postgres
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go test ./internal/settlement
```

Run PostgreSQL adapter tests:

```bash
docker compose up -d postgres
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go test ./internal/settlement/adapters/postgres
```

## File Guide

`internal/settlement/entity.go`

Defines settlement batch and line item entities, status constants, lifecycle methods, and validation.

`internal/settlement/repository.go`

Defines the repository interface and repository-level errors for future adapters.

`internal/settlement/service.go`

Creates settlement batches, starts processing, claims captured payments, creates line items, marks payments `SETTLED`, writes `payment.settled` outbox events, and completes the batch inside a transaction.

`internal/settlement/adapters/postgres/repository.go`

Persists settlement batches and line items in PostgreSQL and participates in context-propagated transactions.

`cmd/paycore-settlement-worker/main.go`

Runs one settlement batch for the previous completed time window and exits.

`migrations/000006_create_settlements.sql`

Creates settlement batch and line item tables, adds `payments.settlement_batch_id`, and adds indexes/constraints for settlement processing.

## Checklist

- [x] Add settlement batch entity.
- [x] Add settlement line item entity.
- [x] Add settlement repository interface.
- [x] Add settlement migration.
- [x] Add domain tests.
- [x] Add PostgreSQL settlement repository adapter.
- [x] Add settlement service.
- [x] Add captured-payment claim query.
- [x] Add payment `SETTLED` transition wiring.
- [x] Add `payment.settled` outbox event creation.
- [x] Add settlement worker command.
- [ ] Add settlement HTTP endpoints.
- [ ] Add stale batch recovery tests.
- [ ] Add settlement metrics.
