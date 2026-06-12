# Payment

This document explains the current PayCore payment implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is intentionally still in-memory, and what is planned next.

## 1. Current Payment Scope

### Implemented

The Go API currently supports the payment foundation:

- Payment entity in `internal/payment/entity.go`.
- Authorization hold entity in `internal/payment/hold.go`.
- Payment repository interface in `internal/payment/repository.go`.
- Payment authorization service in `internal/payment/service.go`.
- Payment capture service in `internal/payment/service.go`.
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
- Internal capture service that:
  - loads payment
  - loads payment hold
  - loads payer
  - validates the payment is still authorized
  - rejects expired authorizations
  - captures the payment
  - captures the hold
  - deducts payer held balance
  - persists payer, hold, and payment through in-memory repositories
- Payment authorization HTTP handler in `internal/payment/handler.go`.
- Payment capture HTTP handler in `internal/payment/handler.go`.
- `POST /payments/authorize` route composed through `internal/http/router.go`.
- `POST /payments/{payment_id}/capture` route composed through `internal/http/router.go`.
- Entity, hold, repository, service, handler, and router tests.

### Not Implemented Yet

These are planned but not currently implemented:

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
POST /payments/{payment_id}/capture
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
  +--> creates payment repository, service, and handler
  +--> creates internal/http chi router
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

The feature package owns payment lifecycle rules. The HTTP package composes payment routes through the central chi router.

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

Holds are created during authorization. Capture moves the hold from `HELD` to `CAPTURED` and deducts the captured amount from the payer held balance. Release exists at the entity and payer balance level for future authorization expiry or void flows.

## 6. Capture Service Flow

### Current Service Input

Current HTTP request:

```http
POST /payments/pay_123/capture
```

Current service input:

```go
payment.CapturePaymentInput{
    PaymentID: "pay_123",
}
```

### Step-by-Step

1. Client sends `POST /payments/{payment_id}/capture`.
2. Router sends the request to `payment.Handler`.
3. Handler reads `payment_id` from the chi route parameter.
4. Handler calls `Service.CapturePayment(...)`.
5. Service loads the payment.
6. Service loads the hold by payment id.
7. Service loads the payer referenced by the payment.
8. Service rejects payments that are not `AUTHORIZED`.
9. Service rejects authorizations past their expiry time.
10. Service transitions the payment to `CAPTURED`.
11. Service transitions the hold to `CAPTURED`.
12. Service deducts the payment amount from payer held balance.
13. Service persists the updated payer.
14. Service persists the captured hold.
15. Service persists the captured payment.
16. Handler returns the capture response as JSON.

### Diagram

```text
Client
  |
  | POST /payments/{payment_id}/capture
  v
internal/http chi router
  |
  v
payment.Handler
  |
  v
Payment Service
  |
  +--> PaymentRepository.GetPayment
  +--> PaymentRepository.GetHoldByPaymentID
  +--> PayerRepository.GetPayer
  |
  +--> Payment.Capture
  +--> Hold.Capture
  +--> Payer.CaptureHeld
  |
  +--> PayerRepository.UpdatePayer
  +--> PaymentRepository.UpdateHold
  +--> PaymentRepository.UpdatePayment
  |
  v
CapturePaymentResult
```

### Failure Path

Current capture failures include:

```text
payment.ErrPaymentNotFound
payment.ErrHoldNotFound
payer.ErrPayerNotFound
payment.ErrPaymentNotCapturable
payment.ErrAuthorizationExpired
```

Current HTTP error mapping:

```text
missing payment         -> HTTP 404
missing hold            -> HTTP 404
missing payer           -> HTTP 404
not capturable          -> HTTP 409
authorization expired   -> HTTP 422
idempotency conflict    -> planned HTTP 409
rate limit exceeded     -> planned HTTP 429
```

## 7. Persistence

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

The authorization and capture services write payer, hold, and payment records through separate in-memory calls.

That means this local implementation does not provide transactionality. If a later write fails after an earlier write succeeds, partial state is possible.

This is acceptable for the current foundation only. PostgreSQL authorization and capture must later run payer balance mutation, hold mutation, payment mutation, idempotency persistence, and outbox event creation in one transaction.

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

## 8. Tests

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
- successful capture service flow
- payment authorization handler success path
- payment authorization handler error mapping
- payment capture handler success path
- payment capture handler error mapping
- router-level `/payments/authorize` wiring
- router-level `/payments/{payment_id}/capture` wiring
- inactive merchant rejection
- missing merchant rejection
- missing payer rejection
- payer currency mismatch rejection
- insufficient available balance rejection
- missing payment on capture rejection
- missing hold on capture rejection
- non-capturable payment rejection
- expired authorization rejection

Run:

```bash
go test ./...
```

## 9. File Guide

`internal/payment/entity.go`

Defines `Payment`, payment statuses, authorized payment construction, and payment lifecycle transitions.

`internal/payment/hold.go`

Defines `Hold`, hold statuses, hold construction, and hold lifecycle transitions.

`internal/payment/repository.go`

Defines the payment repository interface and payment/hold repository errors.

`internal/payment/service.go`

Defines payment authorization orchestration across merchant, payer, payment, and hold state.
It also defines payment capture orchestration across payment, hold, and payer balance state.

`internal/payment/handler.go`

Owns payment authorization and capture HTTP request parsing, response mapping, and HTTP error mapping.

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
- [x] Add payment capture service.
- [x] Register `POST /payments/{payment_id}/capture`.
- [ ] Add idempotency-key enforcement.
- [ ] Add Redis-backed rate limiting.
- [ ] Add PostgreSQL payment and hold migrations.
- [ ] Add durable authorization transaction.
- [ ] Add transactional outbox event for `payment.authorized`.
