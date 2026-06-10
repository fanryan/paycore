# Payment

This document explains the current PayCore payment implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is intentionally still in-memory, and what is planned next.

## 1. Current Payment Scope

### Implemented

The Go API currently supports the payment foundation:

- Payment entity in `internal/payment/entity.go`.
- Authorization hold entity in `internal/payment/hold.go`.
- Payment repository interface in `internal/payment/repository.go`.
- Payment authorization service in `internal/payment/service.go`.
- In-memory payment repository adapter in `internal/payment/adapters/memory/repository.go`.
- Payment statuses:
  - `PENDING`
  - `AUTHORIZED`
  - `CAPTURED`
  - `SETTLED`
  - `EXPIRED`
  - `FAILED`
- Hold statuses:
  - `HELD`
  - `CAPTURED`
  - `RELEASED`
- Authorized payment creation.
- Authorization hold creation.
- Payment capture state transition.
- Payment expiry state transition.
- Payment settlement state transition.
- Hold capture transition.
- Hold release transition.
- Payer balance reserve, release, and held-capture behavior.
- Local random id generation through `internal/shared/id`.
- In-memory repository support for payments and holds.
- Internal authorization service that:
  - loads merchant
  - loads payer
  - validates merchant status
  - validates payer currency and available balance
  - creates a hold
  - creates an authorized payment
  - reserves payer balance
  - persists payer, hold, and payment through in-memory repositories
- Payment authorization HTTP handler in `internal/payment/handler.go`.
- `POST /payments/authorize` route composed through `internal/http/router.go`.
- Entity, hold, repository, and service tests.

### Not Implemented Yet

These are planned but not currently implemented:

- `POST /payments/{payment_id}/capture`.
- `GET /payments/{payment_id}`.
- Idempotency-key enforcement.
- Redis-backed rate limiting.
- Redis-backed idempotency response cache.
- PostgreSQL payment repository.
- PostgreSQL payer balance transaction.
- Payment database migrations.
- Transactional outbox event creation.
- Kafka event publishing.
- Authorization expiry worker.
- Durable crash recovery.

### Public Endpoints

```text
POST /payments/authorize
```

### Protected Endpoints

None currently.

Payment mutation endpoints will eventually require:

```http
Idempotency-Key: <key>
```

Authentication has not been implemented yet.

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
  +--> creates merchant memory repository
  +--> creates payer memory repository
  +--> creates merchant and payer handlers
  +--> creates internal/http router
  +--> starts net/http server
```

Payment dependencies are wired in `main.go` with the in-memory repository adapter.

### Payment Package Boundary

Payment code is feature-owned:

```text
internal/payment
  |
  +--> entity.go
  +--> hold.go
  +--> repository.go
  +--> service.go
  +--> handler.go
  |
  +--> adapters/memory/repository.go
```

The feature package owns payment lifecycle rules. The HTTP package will only compose the payment handler once it exists.

## 3. Authorization Service Flow

### Current Service Input

Current HTTP request:

```json
{
  "merchant_id": "merchant-1",
  "payer_id": "payer-1",
  "amount": 4000,
  "currency": "USD"
}
```

Current service input:

```go
payment.AuthorizePaymentInput{
    MerchantID:  "merchant-1",
    PayerID:     "payer-1",
    AmountMinor: 4000,
    Currency:    "USD",
}
```

### Step-by-Step

1. Client sends `POST /payments/authorize`.
2. Router sends the request to `payment.Handler`.
3. Handler decodes JSON into `AuthorizePaymentRequest`.
4. Handler calls `Service.AuthorizePayment(...)`.
5. Service loads the merchant through `merchant.MerchantRepository`.
6. Service checks `Merchant.CanCreatePayments()`.
7. Service loads the payer through `payer.PayerRepository`.
8. Service checks payer currency against the requested payment currency.
9. Service checks payer available balance through `Payer.CanAuthorize(...)`.
10. Service generates a local payment id with prefix `pay`.
11. Service generates a local hold id with prefix `hold`.
12. Service creates a `HELD` authorization hold.
13. Service creates an `AUTHORIZED` payment with a 15-minute expiry.
14. Service reserves payer funds by moving amount from available balance to held balance.
15. Service persists the updated payer.
16. Service persists the hold.
17. Service persists the payment.
18. Handler returns the authorization response as JSON.

### Diagram

```text
Client
  |
  | POST /payments/authorize
  v
internal/http router
  |
  v
payment.Handler
  |
  v
Payment Service
  |
  +--> MerchantRepository.GetMerchant
  |       |
  |       +--> Merchant.CanCreatePayments
  |
  +--> PayerRepository.GetPayer
  |       |
  |       +--> Payer.CanAuthorize
  |       +--> Payer.Reserve
  |
  +--> NewHold
  +--> NewAuthorizedPayment
  |
  +--> PayerRepository.UpdatePayer
  +--> PaymentRepository.CreateHold
  +--> PaymentRepository.CreatePayment
  |
  v
AuthorizePaymentResult
```

### Failure Path

Current authorization failures include:

```text
merchant.ErrMerchantNotFound
payer.ErrPayerNotFound
payment.ErrMerchantCannotCreatePayments
payment.ErrPayerCurrencyMismatch
payment.ErrInsufficientAvailableBalance
```

Current HTTP error mapping:

```text
missing merchant              -> HTTP 404
missing payer                 -> HTTP 404
inactive merchant             -> HTTP 422 or 409
currency mismatch             -> HTTP 422
insufficient available balance -> HTTP 422
idempotency conflict          -> planned HTTP 409
rate limit exceeded           -> planned HTTP 429
```

## 4. Payment State Machine

The current entity supports these transitions:

```text
AUTHORIZED -> CAPTURED
AUTHORIZED -> EXPIRED
CAPTURED   -> SETTLED
```

The PRD also describes:

```text
PENDING -> AUTHORIZED
PENDING -> FAILED
```

The current constructor creates authorized payments directly because the first implemented workflow is successful local authorization. Failed authorization records and explicit pending records are planned for later.

## 5. Hold Lifecycle

The current hold entity supports:

```text
HELD -> CAPTURED
HELD -> RELEASED
```

Holds are created during authorization. Capture and release are entity-level transitions only right now. The payment capture service has not been implemented yet.

## 6. Persistence

### Current In-Memory Adapter

The current adapter stores payment records and holds in memory:

```go
map[string]payment.Payment
map[string]payment.Hold
map[string]string // hold id by payment id
```

It uses a mutex for concurrent map access and checks `context.Context` before work.

This adapter is useful for local service development before PostgreSQL exists. It is not durable.

### Current Consistency Limitation

The authorization service writes payer, hold, and payment records through separate in-memory calls.

That means this local implementation does not provide transactionality. If a later write fails after an earlier write succeeds, partial state is possible.

This is acceptable for the current foundation only. PostgreSQL authorization must later run payer balance mutation, hold creation, payment creation, idempotency persistence, and outbox event creation in one transaction.

### Planned PostgreSQL Adapter

PostgreSQL persistence is planned but not implemented.

Planned files:

```text
internal/payment/adapters/postgres/repository.go
```

Planned durable records:

- payments
- payment holds
- idempotency records
- outbox events

## 7. Tests

Current tests cover:

- authorized payment construction
- payment validation
- capture, expire, and settle transitions
- hold construction
- hold validation
- hold capture and release transitions
- invalid hold transitions
- payment repository create/get/update behavior
- hold repository create/get/update behavior
- duplicate payment and hold errors
- context cancellation behavior
- successful authorization service flow
- payment authorization handler success path
- payment authorization handler error mapping
- router-level `/payments/authorize` wiring
- inactive merchant rejection
- missing merchant rejection
- missing payer rejection
- payer currency mismatch rejection
- insufficient available balance rejection

Run:

```bash
go test ./...
```

## 8. File Guide

`internal/payment/entity.go`

Defines `Payment`, payment statuses, authorized payment construction, and payment lifecycle transitions.

`internal/payment/hold.go`

Defines `Hold`, hold statuses, hold construction, and hold lifecycle transitions.

`internal/payment/repository.go`

Defines the payment repository interface and payment/hold repository errors.

`internal/payment/service.go`

Defines payment authorization orchestration across merchant, payer, payment, and hold state.

`internal/payment/handler.go`

Owns payment authorization HTTP request parsing, response mapping, and HTTP error mapping.

`internal/payment/adapters/memory/repository.go`

Provides the current non-durable in-memory payment repository implementation.

`internal/payment/adapters/postgres/repository.go`

Planned. Will own durable PostgreSQL payment and hold persistence.

## Checklist

- [x] Add payment entity.
- [x] Add authorization hold entity.
- [x] Add in-memory payment repository.
- [x] Add internal authorization service.
- [x] Add payment HTTP handler.
- [x] Register `POST /payments/authorize`.
- [ ] Add idempotency-key enforcement.
- [ ] Add Redis-backed rate limiting.
- [ ] Add PostgreSQL payment and hold migrations.
- [ ] Add durable authorization transaction.
- [ ] Add transactional outbox event for `payment.authorized`.
