# Settlement

This document explains the current PayCore settlement foundation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is still planned, and how PayCore prevents double settlement.

## 1. Current Feature Scope

### Implemented

- Settlement batch entity in `internal/settlement/entity.go`.
- Settlement line item entity in `internal/settlement/entity.go`.
- Settlement repository interface in `internal/settlement/repository.go`.
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
- Domain tests for batch lifecycle, stale locks, line item validation, and net amount calculation.

### Not Implemented Yet

- Settlement service or batch engine.
- In-memory settlement repository adapter.
- PostgreSQL settlement repository adapter.
- Settlement worker command.
- `POST /settlement-batches`.
- `GET /settlement-batches/{batch_id}`.
- Captured-payment claim query.
- Payment `SETTLED` transition wiring inside the settlement flow.
- `payment.settled` outbox event creation.
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

Current settlement code is domain and schema only:

```text
go test ./internal/settlement
```

### Feature Package Boundary

```text
internal/settlement
  |
  +--> entity.go
  +--> repository.go
  +--> entity_test.go

migrations/
  |
  +--> 000006_create_settlements.sql
```

The settlement package owns settlement batch and line item domain rules. Future repository adapters should live under `internal/settlement/adapters/...`.

## 3. Main Settlement Flow

### Planned Service Input

No settlement service exists yet. The planned input will represent a settlement time window:

```go
settlement.CreateBatchInput{
    WindowStart: windowStart,
    WindowEnd:   windowEnd,
}
```

### Planned Step-by-Step

1. Worker or operator triggers settlement for a time window.
2. Service creates a settlement batch in `CREATED`.
3. Worker marks the batch `PROCESSING` with `claimed_by` and `locked_until`.
4. Repository claims eligible captured payments for the batch.
5. Service creates settlement line items.
6. Service marks claimed payments `SETTLED`.
7. Service writes `payment.settled` outbox events.
8. Service marks settlement batch `COMPLETED`.

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

Run:

```bash
go test ./internal/settlement
```

## File Guide

`internal/settlement/entity.go`

Defines settlement batch and line item entities, status constants, lifecycle methods, and validation.

`internal/settlement/repository.go`

Defines the repository interface and repository-level errors for future adapters.

`migrations/000006_create_settlements.sql`

Creates settlement batch and line item tables, adds `payments.settlement_batch_id`, and adds indexes/constraints for settlement processing.

## Checklist

- [x] Add settlement batch entity.
- [x] Add settlement line item entity.
- [x] Add settlement repository interface.
- [x] Add settlement migration.
- [x] Add domain tests.
- [ ] Add PostgreSQL settlement repository adapter.
- [ ] Add settlement service.
- [ ] Add captured-payment claim query.
- [ ] Add payment `SETTLED` transition wiring.
- [ ] Add `payment.settled` outbox event creation.
- [ ] Add settlement worker command.
- [ ] Add settlement HTTP endpoints.
- [ ] Add stale batch recovery tests.
- [ ] Add settlement metrics.
