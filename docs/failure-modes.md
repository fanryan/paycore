# Failure Modes

This document explains the current PayCore failure-mode handling as it exists today. It is written for resume and interview preparation, so it focuses on how the system behaves under realistic infrastructure and correctness failures, and what tradeoffs were made.

## 1. Current Feature Scope

### Implemented

- Request panic recovery in `internal/http/middleware.go`.
- Request ID propagation through HTTP responses and logs.
- Structured JSON errors with stable `error_code` values.
- PostgreSQL as durable source of truth for payments, holds, payer balances, idempotency records, settlements, and outbox events.
- Service-level transaction orchestration with `internal/shared/db.Transactor`.
- Redis rate limiter fail-closed behavior for payment mutation routes.
- Redis idempotency cache fallback to durable PostgreSQL records.
- Optimistic concurrency detection for payer balance updates.
- Transactional outbox writes for payment and settlement lifecycle events.
- Claim-based outbox publishing with retry state.
- Settlement stale-batch recovery.
- Authorization expiry worker for releasing expired holds.
- Prometheus metrics for request latency, payment results, rate limiting, idempotency cache, outbox lag, settlement processing, and payer version conflicts.

### Out Of Scope And Future Hardening

These items are outside the current portfolio milestone:

- Authentication and authorization.
- Durable idempotency completion inside the same transaction as payment mutation.
- Automated retry loop around payer version conflicts.
- Continuous scheduler for expiry and settlement workers.
- Grafana dashboards and alert rules.
- Dead-letter replay tooling.

## 2. Runtime Flow

### App Startup

The main API process starts from:

```bash
go run ./cmd/paycore-api
```

Worker commands start separately:

```bash
go run ./cmd/paycore-outbox-worker
go run ./cmd/paycore-expiry-worker
go run ./cmd/paycore-settlement-worker
```

### Failure Boundary

```text
HTTP request
  |
  v
request middleware
  |
  +--> request ID
  +--> panic recovery
  +--> body size limit
  +--> rate limiting on payment mutations
  |
  v
feature handler
  |
  v
service transaction boundary
  |
  +--> PostgreSQL state mutation
  +--> outbox event write
  |
  v
durable commit
```

## 3. Main Failure Paths

### Redis Rate Limiter Unavailable

If Redis rate limiting is enabled and Redis cannot be reached, payment mutation routes fail closed:

```text
HTTP 503 RATE_LIMITER_UNAVAILABLE
```

This protects payment mutation state during admission-control uncertainty.

### Redis Idempotency Cache Unavailable

Redis idempotency cache failures do not decide correctness. PayCore falls back to durable PostgreSQL idempotency records.

Expected behavior:

```text
cache hit      -> replay from Redis
cache miss     -> replay from PostgreSQL record
cache error    -> replay from PostgreSQL record
```

### Duplicate Idempotency Key

Same key and same request hash:

```text
replay original completed response
```

Same key and different request hash:

```text
HTTP 409 IDEMPOTENCY_KEY_CONFLICT
```

Expired key reuse:

```text
HTTP 409 IDEMPOTENCY_KEY_EXPIRED
```

### Payer Balance Contention

Payer balance updates use a version column. If another request updates the same payer first, the repository returns:

```text
payer.ErrPayerVersionConflict
```

Payment handlers map this to:

```text
HTTP 409 PAYER_VERSION_CONFLICT
```

This prevents lost updates under concurrent authorization or capture attempts.

### Kafka Unavailable

Payment and settlement services do not publish directly to Kafka. They write durable outbox events in the same PostgreSQL transaction as business state changes.

If Kafka is unavailable:

```text
business transaction still commits
outbox event remains pending or failed
outbox worker retries later
```

### Outbox Worker Crash

Outbox events are claimed before publishing. If a worker crashes after claiming but before successful publishing, events remain durable and can be retried by later worker runs.

### Settlement Worker Crash

Settlement batches use processing state and lock expiry. A later worker can recover stale `PROCESSING` batches and continue settlement work.

### Expired Authorization

Capture rejects expired authorizations:

```text
HTTP 422 AUTHORIZATION_EXPIRED
```

The expiry worker later releases the held balance, marks the hold `RELEASED`, marks the payment `EXPIRED`, and writes a `payment.expired` outbox event.

## 4. Known Crash Window

The most important remaining correctness hardening area is idempotency completion.

Current behavior:

1. Payment mutation and outbox event commit inside the payment service transaction.
2. The HTTP handler receives the service result.
3. The handler completes the idempotency record with the response body.

Known gap:

```text
payment committed
outbox event committed
process crashes before idempotency completion
```

Impact:

- The payment state remains durable and correct.
- The outbox event remains durable.
- A retry with the same idempotency key may not replay the original response because the idempotency record may still be incomplete.

Next hardening step:

Move durable idempotency completion into the same transaction boundary as payment mutation and outbox creation. Keep Redis cache population best-effort after commit.

## Validation And Errors

Stable error codes are part of the API contract. Examples:

```text
IDEMPOTENCY_KEY_REQUIRED
IDEMPOTENCY_KEY_CONFLICT
IDEMPOTENCY_KEY_EXPIRED
PAYER_VERSION_CONFLICT
RATE_LIMIT_EXCEEDED
RATE_LIMITER_UNAVAILABLE
PAYMENT_NOT_CAPTURABLE
AUTHORIZATION_EXPIRED
```

## Persistence

Correctness-critical state is durable in PostgreSQL:

- payer balances
- payment lifecycle state
- payment holds
- idempotency records
- settlement batches and line items
- outbox events

Redis is not required for correctness. Kafka is not the source of truth.

## Tests

Run:

```bash
go test ./...
```

Load-test failure paths are covered by:

```bash
bash loadtest/run_all.sh
```

## File Guide

- `internal/http/middleware.go` owns request recovery, request IDs, body limits, and rate-limit middleware.
- `internal/idempotency/service.go` owns durable idempotency replay and conflict behavior.
- `internal/payment/service.go` owns authorization, capture, expiry, and outbox transaction orchestration.
- `internal/outbox/worker.go` owns publish retry behavior.
- `internal/settlement/service.go` owns settlement processing and stale-batch recovery.
- `docs/architecture-tradeoffs.md` explains the architecture decisions behind these failure-mode choices.

