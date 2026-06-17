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
- Memory and PostgreSQL repositories can claim pending/failed events for publishing.
- PostgreSQL claiming uses `FOR UPDATE SKIP LOCKED`.
- Claimed events can be marked `PUBLISHED`.
- Claimed events can be marked `FAILED` with a future retry availability time.
- Outbox worker skeleton in `internal/outbox/worker.go`.
- Outbox worker command in `cmd/paycore-outbox-worker/main.go`.
- Publisher interface in `internal/outbox/publisher.go`.
- Logging publisher for local worker runs.
- Kafka publisher adapter in `internal/outbox/adapters/kafka/publisher.go`.
- Worker publisher backend selection through `PAYCORE_OUTBOX_PUBLISHER=logging|kafka`.
- API Postgres smoke test verifies both payment lifecycle and outbox event rows.

### Not Implemented Yet

- Retry backoff policy.
- Kafka publisher integration test.
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

The worker command starts separately:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-outbox-worker
```

Run migrations before starting the worker so `outbox_events` exists.

### Feature Package Boundary

```text
internal/outbox
  |
  +--> event.go
  +--> repository.go
  +--> publisher.go
  +--> worker.go
  |
  +--> adapters/memory/repository.go
  +--> adapters/postgres/repository.go
  +--> adapters/kafka/publisher.go
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

## 4. Claim And Retry Flow

### Service Input

The worker claims publishable events in batches:

```go
outbox.ClaimPendingEventsInput{
    WorkerID: "worker-1",
    Limit:    100,
    Now:      now,
}
```

### Step-by-Step

1. The publisher worker opens a transaction.
2. The worker calls `ClaimPendingEvents`.
3. PostgreSQL selects claimable rows with `FOR UPDATE SKIP LOCKED`.
4. Claimable rows are `PENDING` or retryable `FAILED` rows with `available_at <= now`.
5. Claimed rows move to `IN_PROGRESS`.
6. Attempts increment.
7. Lock metadata is set.
8. The worker commits the claim transaction.
9. After publish succeeds, the worker calls `MarkEventPublished`.
10. If publish fails, the worker calls `MarkEventFailed` with a future `available_at`.

### Diagram

```text
Publisher Worker
  |
  v
Transactor.WithinTx
  |
  +--> ClaimPendingEvents
          |
          +--> SELECT ... FOR UPDATE SKIP LOCKED
          +--> status = IN_PROGRESS
          +--> attempts++
          +--> locked_by = worker id
  |
  v
commit claim
  |
  +--> publish later
  |
  +--> MarkEventPublished OR MarkEventFailed
```

### Failure Path

If the claim transaction rolls back, claimed rows return to their previous state because the status/attempt/lock updates were never committed.

## 5. Worker Flow

### Service Input

The worker owns publisher orchestration and depends on a small publisher interface:

```go
type Publisher interface {
    Publish(ctx context.Context, event Event) error
}
```

### Step-by-Step

1. `Worker.ProcessBatch` claims events inside `Transactor.WithinTx`.
2. For each claimed event, the worker calls `Publisher.Publish`.
3. If publish succeeds, the worker marks the event `PUBLISHED`.
4. If publish fails, the worker marks the event `FAILED` and schedules retry after a short delay.
5. The result reports claimed, published, and failed counts.

### Diagram

```text
Outbox Worker
  |
  +--> claim batch in transaction
  |
  +--> Publisher.Publish(event)
  |       |
  |       +--> success -> MarkEventPublished
  |       +--> failure -> MarkEventFailed
  |
  v
ProcessBatchResult
```

### Failure Path

If claiming fails, `ProcessBatch` returns the claim error and publishes nothing.

If publishing fails for one event, that event is marked failed and the worker continues with the rest of the batch.

If marking published or failed fails, `ProcessBatch` returns that repository error.

### Publisher Backends

The worker currently supports:

- `logging`: local default publisher that logs event metadata.
- `kafka`: Kafka publisher adapter that writes event payloads to `PAYCORE_KAFKA_OUTBOX_TOPIC`.

`PAYCORE_KAFKA_OUTBOX_TOPIC` defaults to `paycore.outbox.events`.

Kafka messages use:

- message key: payment aggregate id
- message value: event JSON payload
- headers: event id, event type, aggregate type, aggregate id

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
ErrRepositoryRequired
ErrPublisherRequired
ErrTransactorRequired
```

`ErrEventNotFound` is returned when a publisher tries to mark a missing event or an event that is not currently `IN_PROGRESS`.

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

- pending available events index for publisher scans
- aggregate lookup index for debugging and audit views

## Tests

Current tests cover:

- event construction
- required-field validation
- JSON payload validation
- Postgres event creation
- duplicate event mapping
- transaction rollback through context propagation
- memory outbox claim ordering
- memory outbox publish/fail transitions
- PostgreSQL outbox claim ordering
- PostgreSQL outbox claim rollback
- PostgreSQL outbox publish/fail transitions
- worker successful publish flow
- worker failed publish retry flow
- worker empty batch behavior
- worker dependency validation
- Kafka broker parsing
- Kafka publisher constructor validation
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

Defines `Repository`, `NoopRepository`, claim inputs, mark-failed inputs, and repository errors.

`internal/outbox/publisher.go`

Defines the publisher interface used by the worker.

`internal/outbox/worker.go`

Claims events, calls the publisher interface, and marks events published or failed.

`cmd/paycore-outbox-worker/main.go`

Runs the outbox worker loop with PostgreSQL repositories and a configurable publisher backend.

`internal/outbox/adapters/kafka/publisher.go`

Publishes outbox event payloads to Kafka with stable metadata headers.

`internal/outbox/adapters/memory/repository.go`

Provides the memory repository used by local memory mode and service tests.

`internal/outbox/adapters/postgres/repository.go`

Persists outbox events, claims publisher work with `FOR UPDATE SKIP LOCKED`, marks events published/failed, and participates in context-propagated Postgres transactions.

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
- [x] Add claim/retry repository methods.
- [x] Add outbox publisher worker skeleton.
- [x] Add runtime worker command.
- [x] Publish events to Kafka.
- [ ] Add Kafka publisher integration test.
- [ ] Add LedgerFlow integration notes.
