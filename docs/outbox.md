# Outbox

This document explains the current PayCore transactional outbox foundation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, and what is still planned.

## 1. Current Feature Scope

### Implemented

- Outbox event entity in `internal/outbox/event.go`.
- Outbox repository interface in `internal/outbox/repository.go`.
- In-memory outbox adapter in `internal/outbox/adapters/memory/repository.go`.
- PostgreSQL outbox adapter in `internal/outbox/adapters/postgres/repository.go`.
- Outbox migration in `migrations/000005_create_outbox_events.sql`.
- Payment authorization creates a `payment.authorized` event.
- Payment capture creates a `payment.captured` event.
- Postgres mode writes outbox events inside the payment service transaction.
- API Postgres smoke test verifies both payment lifecycle and outbox event rows.

### Not Implemented Yet

- Outbox publisher worker.
- Event claiming with `FOR UPDATE SKIP LOCKED`.
- Retry backoff.
- Kafka publishing.
- Dead-letter handling.
- LedgerFlow consumer integration.

### Public Endpoints

None. The outbox is internal infrastructure.

### Protected Endpoints

None currently.

## 2. Runtime Flow

### App Startup

```bash
go run ./cmd/paycore-api
```

```text
go run ./cmd/paycore-api
  |
  v
main()
  |
  +--> loads shared config
  +--> creates repositories
  +--> memory mode wires in-memory outbox repository
  +--> postgres mode wires PostgreSQL outbox repository
  +--> payment service receives outbox repository and transactor
  +--> starts net/http server
```

### Feature Package Boundary

```text
internal/outbox
  |
  +--> event.go
  +--> repository.go
  |
  +--> adapters/memory/repository.go
  +--> adapters/postgres/repository.go
```

The outbox package owns durable event shape and persistence. Payment service owns when payment lifecycle events are created.

## 3. Main Feature Flow

### Service Input

Outbox events are created from payment service state after payment authorization or capture succeeds.

### Step-by-Step

1. `payment.Service` enters `Transactor.WithinTx`.
2. Service mutates payer, payment, and hold state.
3. Service builds an outbox event with aggregate type `payment`.
4. Service stores the outbox event through `outbox.Repository`.
5. In Postgres mode, the repository uses the active transaction from context.
6. If any write fails, the transaction rolls back payment state and the outbox event together.
7. If all writes succeed, the transaction commits.

### Diagram

```text
Payment Service
  |
  v
Transactor.WithinTx
  |
  +--> update payer balance
  +--> create/update payment
  +--> create/update hold
  +--> create outbox event
  |
  v
PostgreSQL commit
```

### Failure Path

Current failures include:

```text
outbox.ErrDuplicateEvent
context cancellation
PostgreSQL write errors
```

Any outbox creation error aborts the payment service transaction.

## Validation And Errors

`outbox.NewEvent` validates:

- aggregate type is required
- aggregate id is required
- event type is required
- payload must marshal as JSON

Repository errors:

```text
ErrDuplicateEvent
ErrEventNotFound
```

`ErrEventNotFound` is reserved for upcoming claim/read flows.

## Persistence

The `outbox_events` table stores:

- event id
- aggregate type and id
- event type
- JSONB payload
- status
- attempts
- availability timestamp
- lock metadata
- publish timestamp
- last error
- created and updated timestamps

Current statuses:

```text
PENDING
IN_PROGRESS
PUBLISHED
FAILED
```

Current indexes:

- pending available events index for future publisher scans
- aggregate lookup index for debugging and audit views

## Tests

Current tests cover:

- event construction
- required-field validation
- JSON payload validation
- Postgres event creation
- duplicate event mapping
- transaction rollback through context propagation
- payment authorization outbox event creation
- payment capture outbox event creation
- API Postgres smoke coverage for outbox rows

Run:

```bash
go test ./...
```

## File Guide

`internal/outbox/event.go`

Defines event status constants, event fields, and `NewEvent`.

`internal/outbox/repository.go`

Defines `Repository`, `NoopRepository`, and repository errors.

`internal/outbox/adapters/memory/repository.go`

Provides the memory repository used by local memory mode and service tests.

`internal/outbox/adapters/postgres/repository.go`

Persists outbox events and participates in context-propagated Postgres transactions.

`migrations/000005_create_outbox_events.sql`

Creates the durable outbox table and indexes.

## Checklist

- [x] Add outbox event entity.
- [x] Add outbox repository interface.
- [x] Add memory outbox adapter.
- [x] Add PostgreSQL outbox adapter.
- [x] Add outbox migration.
- [x] Emit `payment.authorized`.
- [x] Emit `payment.captured`.
- [ ] Add claim/retry repository methods.
- [ ] Add outbox publisher worker.
- [ ] Publish events to Kafka.
- [ ] Add LedgerFlow integration notes.
