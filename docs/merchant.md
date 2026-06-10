# Merchant

This document explains the current PayCore merchant implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is intentionally still in-memory, and what is planned next.

## 1. Current Merchant Scope

### Implemented

The Go API currently supports the merchant foundation:

- Merchant entity in `internal/merchant/entity.go`.
- Merchant repository interface in `internal/merchant/repository.go`.
- Merchant service in `internal/merchant/service.go`.
- In-memory merchant repository adapter in `internal/merchant/adapters/memory/repository.go`.
- Merchant statuses:
  - `ACTIVE`
  - `SUSPENDED`
  - `CLOSED`
- New merchants start as `ACTIVE`.
- Merchant id validation.
- Merchant name trimming and validation.
- Settlement currency normalization through `internal/shared/currency`.
- Settlement currency validation as a 3-letter currency code.
- `Merchant.CanCreatePayments()` guard for payment creation eligibility.
- Repository errors for not-found and duplicate merchant records.
- Entity, service, and in-memory repository tests.

### Not Implemented Yet

These are planned but not currently implemented:

- Merchant HTTP handler.
- Merchant routes in `internal/http/router.go`.
- Public or protected merchant API endpoints.
- PostgreSQL merchant repository.
- Merchant database migrations.
- Merchant authentication and authorization rules.
- Merchant status update endpoints.
- Merchant rate-limit tiers.
- Merchant integration with payment authorization.

### Public Endpoints

None currently.

The only public API endpoints today are system endpoints:

```text
GET /healthz
GET /readyz
GET /version
```

### Protected Endpoints

None currently.

Authentication and protected merchant administration endpoints have not been implemented yet.

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

Merchant dependencies are not wired into `main.go` yet because merchant HTTP endpoints have not been introduced.

### Merchant Package Boundary

Merchant code is feature-owned:

```text
internal/merchant
  |
  +--> entity.go
  +--> repository.go
  +--> service.go
  |
  +--> adapters/memory/repository.go
```

The feature package owns merchant rules. The HTTP package will only compose the merchant handler once it exists.

## 3. Create Merchant Service Flow

### Current Service Input

There is no HTTP request contract yet. The current service input is:

```go
merchant.CreateMerchantInput{
    ID:                 "merchant-1",
    Name:               "Demo Merchant",
    SettlementCurrency: "USD",
}
```

### Step-by-Step

1. Caller invokes `MerchantService.CreateMerchant(...)`.
2. `MerchantService` calls `NewMerchant(...)`.
3. `NewMerchant` trims the merchant id.
4. `NewMerchant` trims the merchant name.
5. Settlement currency is normalized through `currency.NormalizeCurrency(...)`.
6. Required fields are validated.
7. Settlement currency shape is validated.
8. Merchant status is initialized to `ACTIVE`.
9. Created and updated timestamps are stored in UTC.
10. `MerchantService` calls `MerchantRepository.CreateMerchant(...)`.
11. The current memory adapter stores the merchant in a map keyed by merchant id.
12. The service returns the created merchant.

### Diagram

```text
Caller
  |
  | CreateMerchantInput
  v
MerchantService
  |
  +--> NewMerchant
  |     |
  |     +--> trim id and name
  |     +--> normalize settlement currency
  |     +--> validate required fields
  |     +--> status = ACTIVE
  |
  v
MerchantRepository
  |
  v
Memory merchant adapter
  |
  v
map[merchant_id]Merchant
```

### Failure Path

Entity validation currently returns an error for:

- blank merchant id
- blank merchant name
- invalid settlement currency

Repository operations currently return:

```text
ErrDuplicateMerchant
ErrMerchantNotFound
```

HTTP error mapping has not been implemented yet. Planned mapping:

```text
validation error       -> HTTP 400
ErrDuplicateMerchant   -> HTTP 409
ErrMerchantNotFound    -> HTTP 404
```

## 4. Planned Merchant HTTP Flow

This section is a placeholder for the upcoming handler implementation.

Planned endpoint:

```text
POST /merchants
GET  /merchants
GET  /merchants/{merchant_id}
```

Planned handler flow:

```text
Client
  |
  | POST /merchants
  v
internal/http router
  |
  v
merchant.Handler
  |
  +--> decode request JSON
  +--> validate request shape
  +--> call MerchantService
  +--> map service/domain errors
  |
  v
JSON response
```

The handler should live in:

```text
internal/merchant/handler.go
```

The router should only register it. Business rules should stay in the merchant entity and service.

## 5. Persistence

### Current In-Memory Adapter

The current adapter stores merchant records in memory:

```go
map[string]merchant.Merchant
```

It uses a mutex for concurrent map access and checks `context.Context` before work.

This adapter is useful for local API development before PostgreSQL exists. It is not durable.

### Planned PostgreSQL Adapter

PostgreSQL persistence is planned but not implemented.

Planned file:

```text
internal/merchant/adapters/postgres/repository.go
```

Planned durable fields:

- merchant id
- name
- status
- settlement currency
- optional rate-limit tier
- created timestamp
- updated timestamp

## 6. Tests

Current tests cover:

- merchant creation defaults
- merchant required-field validation
- settlement currency normalization and validation
- active/suspended/closed payment eligibility
- service create/get/list behavior
- repository not-found behavior
- in-memory duplicate detection
- in-memory context cancellation behavior

Run:

```bash
go test ./...
```

## 7. File Guide

`internal/merchant/entity.go`

Defines `Merchant`, `MerchantStatus`, `NewMerchant`, and `CanCreatePayments`.

`internal/merchant/repository.go`

Defines `MerchantRepository`, `ErrMerchantNotFound`, and `ErrDuplicateMerchant`.

`internal/merchant/service.go`

Defines `MerchantService` and coordinates merchant creation and repository reads.

`internal/merchant/adapters/memory/repository.go`

Provides the current non-durable in-memory repository implementation.

`internal/merchant/handler.go`

Planned. Will own merchant HTTP request parsing, response mapping, and HTTP error mapping.

`internal/merchant/adapters/postgres/repository.go`

Planned. Will own durable PostgreSQL merchant persistence.

## Checklist

- [ ] Add merchant HTTP handler.
- [ ] Register merchant routes in `internal/http/router.go`.
- [ ] Add merchant handler tests.
- [ ] Add PostgreSQL migration for merchants.
- [ ] Add PostgreSQL merchant repository.
- [ ] Document final merchant request and response contracts.
