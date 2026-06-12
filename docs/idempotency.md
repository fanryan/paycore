# Idempotency

This document explains the current PayCore idempotency implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is intentionally still in-memory, and what is planned next.

## 1. Current Idempotency Scope

### Implemented

The Go API currently supports a local idempotency foundation:

- Idempotency record entity in `internal/idempotency/record.go`.
- Idempotency repository interface in `internal/idempotency/repository.go`.
- Idempotency service in `internal/idempotency/service.go`.
- In-memory idempotency repository adapter in `internal/idempotency/adapters/memory/repository.go`.
- Request hashing with SHA-256 over HTTP method, URL path, and request body.
- Idempotency statuses:
  - `IN_PROGRESS`
  - `COMPLETED`
  - `FAILED`
- Local 24-hour default TTL for idempotency records.
- Duplicate key handling.
- Same key and same request hash response replay.
- Same key and different request hash conflict rejection.
- Expired key rejection.
- In-progress duplicate request rejection.
- Payment authorization integration for `POST /payments/authorize`.
- Payment capture integration for `POST /payments/{payment_id}/capture`.
- Handler-level response recording for successful replay.
- Unit tests for record behavior, memory repository behavior, service behavior, payment handler behavior, and router behavior.

### Not Implemented Yet

These are planned but not currently implemented:

- PostgreSQL durable idempotency records.
- Redis idempotency response cache.
- Request hash canonicalization beyond raw request body hashing.
- Durable recovery for `IN_PROGRESS` records after process crash.
- Failed response persistence policy.
- Idempotency metrics.
- Idempotency cleanup worker.

### Public Endpoints

Idempotency is currently enforced on:

```text
POST /payments/authorize
POST /payments/{payment_id}/capture
```

### Required Header

```http
Idempotency-Key: <key>
```

## 2. Runtime Flow

### App Startup

When running:

```bash
go run ./cmd/paycore-api
```

the current application creates:

```text
idempotency memory repository
  |
  v
idempotency service
  |
  v
payment handler with idempotency
```

The idempotency repository is in-memory and resets when the process restarts.

### Package Boundary

Idempotency code is owned by its own feature package:

```text
internal/idempotency
  |
  +--> record.go
  +--> repository.go
  +--> service.go
  |
  +--> adapters/memory/repository.go
```

The payment handler depends on the idempotency service, but payment business rules remain inside the payment service.

## 3. Authorization Idempotency Flow

### Current Request

```http
POST /payments/authorize
Content-Type: application/json
Idempotency-Key: demo-key-1
```

```json
{
  "merchant_id": "merchant-1",
  "payer_id": "payer-1",
  "amount": 4000,
  "currency": "USD"
}
```

### Step-by-Step

1. Client sends `POST /payments/authorize`.
2. Payment handler reads the request body.
3. Payment handler requires `Idempotency-Key`.
4. Payment handler hashes the HTTP method, URL path, and request body with SHA-256.
5. Idempotency service creates an `IN_PROGRESS` record when the key is new.
6. Payment handler continues to authorize the payment.
7. Payment handler records the final HTTP status and response body.
8. Idempotency service marks the record `COMPLETED`.
9. A later request with the same key and same request hash returns the stored response.
10. A later request with the same key and different request hash returns `409`.

### Diagram

```text
Client
  |
  | POST /payments/authorize
  | Idempotency-Key: demo-key-1
  v
payment.Handler
  |
  +--> HashRequest
  |
  +--> IdempotencyService.StartRequest
  |       |
  |       +--> CreateRecord
  |       +--> or replay completed response
  |       +--> or reject key conflict
  |
  +--> Payment Service
  |
  +--> responseRecorder
  |
  +--> IdempotencyService.CompleteRequest
  |
  v
HTTP response
```

## 4. Capture Idempotency Flow

### Current Request

```http
POST /payments/pay_123/capture
Idempotency-Key: capture-key-1
```

### Step-by-Step

1. Client sends `POST /payments/{payment_id}/capture`.
2. Payment handler reads the payment id from the chi route parameter.
3. Payment handler requires `Idempotency-Key`.
4. Payment handler hashes the HTTP method, URL path, and empty request body.
5. Idempotency service creates an `IN_PROGRESS` record when the key is new.
6. Payment handler continues to capture the payment.
7. Payment handler records the final HTTP status and response body.
8. Idempotency service marks the record `COMPLETED`.
9. A later request with the same key and same payment path returns the stored response.
10. A later request with the same key and a different payment path returns `409`.

### Diagram

```text
Client
  |
  | POST /payments/{payment_id}/capture
  | Idempotency-Key: capture-key-1
  v
payment.Handler
  |
  +--> HashRequest(method, path, body)
  |
  +--> IdempotencyService.StartRequest
  |       |
  |       +--> CreateRecord
  |       +--> or replay completed response
  |       +--> or reject key conflict
  |
  +--> Payment Service
  |
  +--> responseRecorder
  |
  +--> IdempotencyService.CompleteRequest
  |
  v
HTTP response
```

## 5. Failure Path

Current idempotency failures include:

```text
idempotency.ErrInvalidKey
idempotency.ErrInvalidRequestHash
idempotency.ErrRequestHashMismatch
idempotency.ErrExpiredIdempotencyKey
idempotency.ErrRequestInProgress
idempotency.ErrRecordNotFound
```

Current HTTP error mapping:

```text
missing Idempotency-Key       -> HTTP 400
same key, different request   -> HTTP 409
expired idempotency key       -> HTTP 409
same key still in progress    -> HTTP 409
invalid idempotency input     -> HTTP 400
```

## 6. Persistence

### Current In-Memory Adapter

The current adapter stores idempotency records in memory:

```go
map[string]idempotency.Record
```

It uses a mutex for concurrent map access and checks `context.Context` before work.

This adapter is useful for local API behavior and tests. It is not durable. Restarting the API loses idempotency history.

### Planned PostgreSQL Records

PostgreSQL persistence is planned but not implemented.

Planned durable fields:

- idempotency key
- request hash
- status
- response status code
- response body
- created timestamp
- updated timestamp
- expiry timestamp

The durable PostgreSQL implementation must participate in the same transaction as payment state, payer balance mutation, and outbox event creation.

### Planned Redis Cache

Redis response caching is planned but not implemented.

Redis may replay completed responses faster, but PostgreSQL remains authoritative. If Redis is unavailable for idempotency response caching, PayCore should fall back to PostgreSQL records.

## 7. Tests

Current tests cover:

- record creation
- default TTL behavior
- record completion
- response body cloning
- request body hashing
- method and path request hashing
- expiry checks
- memory repository create/get/update behavior
- duplicate key rejection
- missing record rejection
- context cancellation behavior
- service start and complete behavior
- completed response replay
- key conflict rejection
- in-progress duplicate rejection
- expired key rejection
- payment authorization missing-key rejection
- payment authorization replay
- payment authorization key conflict rejection
- payment capture missing-key rejection
- payment capture replay
- payment capture key conflict rejection across different payment paths

Run:

```bash
go test ./...
```

## 8. File Guide

`internal/idempotency/record.go`

Defines `Record`, statuses, record construction, completion, expiry checks, and request hashing.

`internal/idempotency/repository.go`

Defines the idempotency repository interface and repository-level errors.

`internal/idempotency/service.go`

Coordinates starting idempotent requests, replaying completed responses, rejecting conflicts, and completing records.

`internal/idempotency/adapters/memory/repository.go`

Provides the current non-durable in-memory idempotency repository implementation.

`internal/payment/response_recorder.go`

Captures response status and body for payment authorization and capture replay.

## Checklist

- [x] Add idempotency record entity.
- [x] Add idempotency repository interface.
- [x] Add in-memory idempotency repository.
- [x] Add idempotency service.
- [x] Enforce `Idempotency-Key` on payment authorization.
- [x] Replay same key and same request hash.
- [x] Reject same key and different request hash.
- [x] Enforce `Idempotency-Key` on payment capture.
- [x] Add PostgreSQL idempotency record migration.
- [ ] Add PostgreSQL durable idempotency records.
- [ ] Add Redis idempotency response cache.
- [ ] Add idempotency metrics.
