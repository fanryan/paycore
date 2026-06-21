# Payer

This document explains the current PayCore payer implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, and how payer balances support payment authorization, capture, and expiry.

## 1. Current Payer Scope

### Implemented

The Go API currently supports the payer foundation:

- Payer entity in `internal/payer/entity.go`.
- Payer repository interface in `internal/payer/repository.go`.
- Payer service in `internal/payer/service.go`.
- Payer HTTP handler in `internal/payer/handler.go`.
- In-memory payer repository adapter in `internal/payer/adapters/memory/repository.go`.
- PostgreSQL payer repository adapter in `internal/payer/adapters/postgres/repository.go`.
- PostgreSQL payer table migration in `migrations/000002_create_payers.sql`.
- Available balance stored as integer minor units.
- Held balance stored as integer minor units.
- New payers start with held balance `0`.
- New payers start with version `0`.
- Payer id validation.
- Available balance validation.
- Currency normalization through `internal/shared/currency`.
- Currency validation as a 3-letter currency code.
- `Payer.CanAuthorize(...)` predicate for amount, currency, and available balance checks.
- Repository errors for not-found and duplicate payer records.
- Payer create and list routes composed through `internal/http/router.go`.
- Optimistic concurrency enforcement for payer updates in memory and PostgreSQL repositories.
- Entity, service, handler, router, and in-memory repository tests.

### Future Hardening

These items are outside the current portfolio milestone:

- `GET /payers/{payer_id}`.
- Dedicated payer balance adjustment endpoint.
- Authentication and protected payer administration endpoints.

### Public Endpoints

```text
GET /healthz
GET /readyz
GET /version
POST /payers
GET  /payers
```

### Protected Endpoints

None currently.

Authentication and protected payer administration endpoints are outside the current local systems milestone.

## 2. Runtime Flow

### App Startup

When running:

```bash
go run ./cmd/paycore-api
```

the current application starts from:

```text
cmd/paycore-api/main.go
```

Startup flow:

```text
go run ./cmd/paycore-api
  |
  v
main()
  |
  +--> loads shared config from environment
  +--> creates JSON slog logger
  +--> creates internal/http router
  +--> attaches request id middleware
  +--> attaches request logging middleware
  +--> starts net/http server
```

Payer dependencies are wired in `main.go`. Memory repositories are the default. PostgreSQL repositories are enabled with `PAYCORE_REPOSITORY_BACKEND=postgres` and `PAYCORE_DATABASE_URL`.

### Payer Package Boundary

Payer code is feature-owned:

```text
internal/payer
  |
  +--> entity.go
  +--> repository.go
  +--> service.go
  +--> handler.go
  |
  +--> adapters/memory/repository.go
  +--> adapters/postgres/repository.go
```

The feature package owns payer rules and HTTP request/response mapping. The central HTTP package composes the payer handler into the shared router.

## 3. Create Payer Service Flow

### Current Service Input

Current HTTP request:

```json
{
  "id": "payer-1",
  "available_balance_minor": 10000,
  "currency": "USD"
}
```

Current service input:

```go
payer.CreatePayerInput{
    ID:                    "payer-1",
    AvailableBalanceMinor: 10000,
    Currency:              "USD",
}
```

### Step-by-Step

1. Caller invokes `PayerService.CreatePayer(...)`.
2. `PayerService` calls `NewPayer(...)`.
3. `NewPayer` trims the payer id.
4. Currency is normalized through `currency.NormalizeCurrency(...)`.
5. Payer id is validated.
6. Available balance is checked to ensure it is not negative.
7. Currency shape is validated.
8. Held balance is initialized to `0`.
9. Version is initialized to `0`.
10. Created and updated timestamps are stored in UTC.
11. `PayerService` calls `PayerRepository.CreatePayer(...)`.
12. The current memory adapter stores the payer in a map keyed by payer id.
13. The service returns the created payer.

### Diagram

```text
Caller
  |
  | CreatePayerInput
  v
PayerService
  |
  +--> NewPayer
  |     |
  |     +--> trim id
  |     +--> normalize currency
  |     +--> validate available balance
  |     +--> held balance = 0
  |     +--> version = 0
  |
  v
PayerRepository
  |
  v
Memory payer adapter
  |
  v
map[payer_id]Payer
```

### Failure Path

Entity validation currently returns an error for:

- blank payer id
- negative available balance
- invalid currency

Repository operations currently return:

```text
ErrDuplicatePayer
ErrPayerNotFound
ErrPayerVersionConflict
```

Current HTTP error mapping:

```text
validation error    -> HTTP 400
ErrDuplicatePayer   -> HTTP 409
ErrPayerNotFound    -> HTTP 404
```

When payment authorization or capture hits a payer version conflict, the payment handler maps it to:

```text
PAYER_VERSION_CONFLICT -> HTTP 409
```

## 4. Authorization Predicate

`Payer.CanAuthorize(amountMinor, currency)` is the current entity-level guard used to model the first part of payment authorization.

It returns true only when:

- amount is positive
- requested currency matches payer currency after normalization
- available balance is greater than or equal to the requested amount

### Diagram

```text
Payment authorization use case
  |
  v
Payer.CanAuthorize(amount, currency)
  |
  +--> amount > 0
  +--> currency matches
  +--> available balance is sufficient
  |
  v
true or false
```

The full payment authorization flow uses this payer behavior to create a payment, create a hold, reserve available balance into held balance, persist idempotency state, and write an outbox event.

## 5. Payer HTTP Flow

Current endpoints:

```text
POST /payers
GET  /payers
```

The current public payer API supports creation and listing. Single-record lookup is available internally through the repository and service boundary.

Handler flow:

```text
Client
  |
  | POST /payers
  v
internal/http router
  |
  v
payer.Handler
  |
  +--> decode request JSON
  +--> validate request shape
  +--> call PayerService
  +--> map service/domain errors
  |
  v
JSON response
```

The handler lives in:

```text
internal/payer/handler.go
```

The router only registers it. Business rules stay in the payer entity and service.

## 6. Persistence

### Current In-Memory Adapter

The current adapter stores payer records in memory:

```go
map[string]payer.Payer
```

It uses a mutex for concurrent map access and checks `context.Context` before work.

This adapter is useful for local API development and unit tests. It is not durable.

### PostgreSQL Adapter

PostgreSQL persistence is implemented in:

```text
internal/payer/adapters/postgres/repository.go
```

Durable fields:

- payer id
- available balance minor
- held balance minor
- currency
- version
- created timestamp
- updated timestamp

The `version` field is used for optimistic concurrency control. Repository updates require the incoming payer version to be exactly one greater than the stored version. If another update has already advanced the stored version, the repository returns `ErrPayerVersionConflict`.

## 7. Tests

Current tests cover:

- payer creation defaults
- payer required-field validation
- negative balance rejection
- currency normalization and validation
- authorization predicate behavior
- service create/get/list behavior
- repository not-found behavior
- in-memory duplicate detection
- in-memory payer version conflict detection
- PostgreSQL duplicate detection
- PostgreSQL payer version conflict detection
- in-memory context cancellation behavior
- handler create/list behavior
- handler invalid JSON and duplicate error mapping
- router-level `/payers` wiring

Run:

```bash
go test ./...
```

## 8. File Guide

`internal/payer/entity.go`

Defines `Payer`, `NewPayer`, and `CanAuthorize`.

`internal/payer/repository.go`

Defines `PayerRepository`, `ErrPayerNotFound`, `ErrDuplicatePayer`, and `ErrPayerVersionConflict`.

`internal/payer/service.go`

Defines `PayerService` and coordinates payer creation and repository reads.

`internal/payer/adapters/memory/repository.go`

Provides the current non-durable in-memory repository implementation, including optimistic version checks.

`internal/payer/handler.go`

Owns payer HTTP request parsing, response mapping, and HTTP error mapping.

`internal/payer/adapters/postgres/repository.go`

Owns durable PostgreSQL payer persistence and optimistic concurrency behavior.
