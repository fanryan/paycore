# Payment

This document explains the current PayCore payment implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, what is durable in PostgreSQL mode, and what is planned next.

## 1. Current Payment Scope

### Implemented

The Go API currently supports the payment foundation:

- Payment entity in `internal/payment/entity.go`.
- Authorization hold entity in `internal/payment/hold.go`.
- Payment repository interface in `internal/payment/repository.go`.
- Payment authorization service in `internal/payment/service.go`.
- Payment capture service in `internal/payment/service.go`.
- Payment authorization expiry service in `internal/payment/service.go`.
- In-memory payment repository adapter in `internal/payment/adapters/memory/repository.go`.
- PostgreSQL payment and hold repository adapter in `internal/payment/adapters/postgres/repository.go`.
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
- In-memory `Idempotency-Key` enforcement for `POST /payments/authorize`.
- In-memory `Idempotency-Key` enforcement for `POST /payments/{payment_id}/capture`.
- Internal authorization service that:
  - loads merchant
  - loads payer
  - validates merchant status
  - validates payer currency and available balance
  - creates a hold
  - creates an authorized payment
  - reserves payer balance
  - persists payer, payment, and hold through configured repositories
- Internal capture service that:
  - loads payment
  - loads payment hold
  - loads payer
  - validates the payment is still authorized
  - rejects expired authorizations
  - captures the payment
  - captures the hold
  - deducts payer held balance
  - persists payer, hold, and payment through configured repositories
- Internal expiry service that:
  - finds expired `AUTHORIZED` payments
  - releases the payer held balance back to available balance
  - transitions the hold to `RELEASED`
  - transitions the payment to `EXPIRED`
  - writes a `payment.expired` outbox event
- Payment authorization HTTP handler in `internal/payment/handler.go`.
- Payment capture HTTP handler in `internal/payment/handler.go`.
- Payment authorization response recording in `internal/payment/response_recorder.go` for local idempotency replay.
- `POST /payments/authorize` route composed through `internal/http/router.go`.
- `POST /payments/{payment_id}/capture` route composed through `internal/http/router.go`.
- Runtime repository backend switch through `PAYCORE_REPOSITORY_BACKEND=memory|postgres`.
- Service-level transaction orchestration through `internal/shared/db.Transactor`.
- Transactional outbox events for `payment.authorized`, `payment.captured`, `payment.expired`, and `payment.settled`.
- Redis-backed rate limiting on payment mutation routes.
- Redis-backed idempotency response cache for replay acceleration.
- Postgres-backed HTTP smoke test in `cmd/paycore-api/main_test.go`.
- Entity, hold, repository, service, handler, and router tests.

### Not Implemented Yet

These are planned but not currently implemented:

- `GET /payments/{payment_id}`.
- Single PostgreSQL transaction that also includes idempotency completion.
- Durable crash recovery.

### Public Endpoints

```text
POST /payments/authorize
POST /payments/{payment_id}/capture
```

### Protected Endpoints

None currently.

Payment authorization and capture currently require:

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
  +--> creates memory repositories by default
  +--> creates PostgreSQL repositories when PAYCORE_REPOSITORY_BACKEND=postgres
  +--> creates merchant and payer handlers
  +--> creates payment repository, service, and handler
  +--> creates internal/http chi router
  +--> starts net/http server
```

Payment dependencies are wired in `main.go`. Memory repositories are the default. PostgreSQL repositories are enabled with `PAYCORE_REPOSITORY_BACKEND=postgres` and `PAYCORE_DATABASE_URL`.

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
  +--> adapters/postgres/repository.go
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
3. Handler reads the request body.
4. Handler requires `Idempotency-Key`.
5. Handler hashes the request body.
6. Handler starts an idempotency record.
7. If the same key and request hash completed before, handler replays the stored response.
8. If the same key is reused with a different request hash, handler returns `409`.
9. Handler decodes JSON into `AuthorizePaymentRequest`.
10. Handler calls `Service.AuthorizePayment(...)`.
11. Service loads the merchant through `merchant.MerchantRepository`.
12. Service checks `Merchant.CanCreatePayments()`.
13. Service loads the payer through `payer.PayerRepository`.
14. Service checks payer currency against the requested payment currency.
15. Service checks payer available balance through `Payer.CanAuthorize(...)`.
16. Service generates a local payment id with prefix `pay`.
17. Service generates a local hold id with prefix `hold`.
18. Service creates a `HELD` authorization hold.
19. Service creates an `AUTHORIZED` payment with a 15-minute expiry.
20. Service reserves payer funds by moving amount from available balance to held balance.
21. Service persists the updated payer.
22. Service persists the payment.
23. Service persists the hold.
24. Handler records the successful response against the idempotency key.
25. Handler returns the authorization response as JSON.

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
  +--> IdempotencyService.StartRequest
  |       |
  |       +--> create in-memory IN_PROGRESS record
  |       +--> replay completed response when key/hash match
  |       +--> reject key/hash mismatch
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
  +--> PaymentRepository.CreatePayment
  +--> PaymentRepository.CreateHold
  |
  v
AuthorizePaymentResult
  |
  v
IdempotencyService.CompleteRequest
```

### Failure Path

Current authorization failures include:

```text
merchant.ErrMerchantNotFound
payer.ErrPayerNotFound
payer.ErrPayerVersionConflict
payment.ErrMerchantCannotCreatePayments
payment.ErrPayerCurrencyMismatch
payment.ErrInsufficientAvailableBalance
idempotency.ErrRequestHashMismatch
idempotency.ErrExpiredIdempotencyKey
idempotency.ErrRequestInProgress
```

Current HTTP error mapping:

```text
missing merchant              -> HTTP 404
missing payer                 -> HTTP 404
payer version conflict        -> HTTP 409
inactive merchant             -> HTTP 422 or 409
currency mismatch             -> HTTP 422
insufficient available balance -> HTTP 422
missing idempotency key       -> HTTP 400
idempotency conflict          -> HTTP 409
idempotency key expired       -> HTTP 409
idempotency request in flight -> HTTP 409
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

Holds are created during authorization. Capture moves the hold from `HELD` to `CAPTURED` and deducts the captured amount from the payer held balance. Expiry moves the hold from `HELD` to `RELEASED` and releases the held amount back to the payer available balance.

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
4. Handler requires `Idempotency-Key`.
5. Handler hashes the HTTP method, URL path, and request body.
6. Handler starts an idempotency record.
7. If the same key and request hash completed before, handler replays the stored response.
8. If the same key is reused for a different payment path, handler returns `409`.
9. Handler calls `Service.CapturePayment(...)`.
10. Service loads the payment.
11. Service loads the hold by payment id.
12. Service loads the payer referenced by the payment.
13. Service rejects payments that are not `AUTHORIZED`.
14. Service rejects authorizations past their expiry time.
15. Service transitions the payment to `CAPTURED`.
16. Service transitions the hold to `CAPTURED`.
17. Service deducts the payment amount from payer held balance.
18. Service persists the updated payer.
19. Service persists the captured hold.
20. Service persists the captured payment.
21. Handler records the successful response against the idempotency key.
22. Handler returns the capture response as JSON.

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
  +--> IdempotencyService.StartRequest
  |       |
  |       +--> create in-memory IN_PROGRESS record
  |       +--> replay completed response when key/hash match
  |       +--> reject key/hash mismatch
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
  |
  v
IdempotencyService.CompleteRequest
```

### Failure Path

Current capture failures include:

```text
payment.ErrPaymentNotFound
payment.ErrHoldNotFound
payer.ErrPayerNotFound
payer.ErrPayerVersionConflict
payment.ErrPaymentNotCapturable
payment.ErrAuthorizationExpired
idempotency.ErrRequestHashMismatch
idempotency.ErrExpiredIdempotencyKey
idempotency.ErrRequestInProgress
```

Current HTTP error mapping:

```text
missing payment         -> HTTP 404
missing hold            -> HTTP 404
missing payer           -> HTTP 404
payer version conflict  -> HTTP 409
not capturable          -> HTTP 409
authorization expired    -> HTTP 422
missing idempotency key  -> HTTP 400
idempotency conflict     -> HTTP 409
idempotency key expired  -> HTTP 409
idempotency in flight    -> HTTP 409
rate limit exceeded      -> planned HTTP 429
```

## 7. Expiry Service Flow

### Current Command

The expiry flow currently runs through a one-shot worker command:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-expiry-worker
```

The batch size is controlled by `PAYCORE_EXPIRY_LIMIT`.

### Step-by-Step

1. The expiry worker starts from `cmd/paycore-expiry-worker/main.go`.
2. The worker loads config and creates a PostgreSQL pool.
3. The worker wires merchant, payer, payment, and outbox PostgreSQL repositories.
4. The worker calls `Service.ExpireAuthorizedPayments(...)`.
5. The service starts a transaction through `internal/shared/db.Transactor`.
6. The payment repository claims expired `AUTHORIZED` payments with `FOR UPDATE SKIP LOCKED`.
7. For each expired payment, the service loads the hold and payer.
8. The payer held balance is released back to available balance.
9. The hold transitions from `HELD` to `RELEASED`.
10. The payment transitions from `AUTHORIZED` to `EXPIRED`.
11. The service persists payer, hold, and payment updates.
12. The service writes a `payment.expired` outbox event.
13. The transaction commits the state changes and outbox events together.

### Diagram

```text
paycore-expiry-worker
  |
  v
Payment Service
  |
  +--> PaymentRepository.ListExpiredAuthorizedPayments
  |       |
  |       +--> status = AUTHORIZED
  |       +--> expires_at <= now
  |       +--> FOR UPDATE SKIP LOCKED
  |
  +--> PaymentRepository.GetHoldByPaymentID
  +--> PayerRepository.GetPayer
  |
  +--> Payer.Release
  +--> Hold.Release
  +--> Payment.Expire
  |
  +--> PayerRepository.UpdatePayer
  +--> PaymentRepository.UpdateHold
  +--> PaymentRepository.UpdatePayment
  +--> OutboxRepository.CreateEvent(payment.expired)
```

### Failure Path

Expiry can fail if:

```text
payment.ErrHoldNotFound
payer.ErrPayerNotFound
payer.ErrPayerVersionConflict
invalid hold state
invalid payment state
database transaction error
```

In Postgres mode, a failure rolls back payer, hold, payment, and outbox mutations for the batch.

## 8. Persistence

### Current In-Memory Adapter

The current adapter stores payment records and holds in memory:

```go
map[string]payment.Payment
map[string]payment.Hold
map[string]string // hold id by payment id
```

It uses a mutex for concurrent map access and checks `context.Context` before work.

This adapter is useful for local service development before PostgreSQL exists. It is not durable.

The current idempotency adapter also stores records in memory. This supports local replay behavior while developing the API contract, but duplicate protection is lost on process restart.

### Current Transaction Boundary

The authorization, capture, and expiry services write payer, payment, and hold records through separate repository calls.

In memory mode, `db.NoopTransactor` runs the service callback without durable transactionality. That keeps unit tests and local memory mode simple.

In Postgres mode, `db.PostgresTransactor` starts one transaction, injects it into `context.Context`, and the Postgres payer/payment repositories execute against that transaction when present. This means payer balance mutation, payment mutation, and hold mutation commit or roll back together for authorization, capture, and expiry.

HTTP idempotency start/completion currently happens in the handler outside the payment service transaction. Outbox event creation now happens inside the payment service transaction with payer, payment, and hold mutations. A later milestone should move durable idempotency completion into the same transaction boundary.

### PostgreSQL Adapter

PostgreSQL persistence is implemented for payment and hold records in:

```text
internal/payment/adapters/postgres/repository.go
```

Current durable records:

- payments
- payment holds

Idempotency records are durable in Postgres mode through `internal/idempotency/adapters/postgres/repository.go`, but are not yet completed atomically with payment business state.

Outbox events are durable in Postgres mode through `internal/outbox/adapters/postgres/repository.go`. Authorization creates `payment.authorized`; capture creates `payment.captured`; expiry creates `payment.expired`.

## 9. Tests

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
- successful expiry service flow
- payment authorization handler success path
- payment authorization handler error mapping
- payment capture handler success path
- payment capture handler error mapping
- payment authorization idempotency missing-key rejection
- payment authorization idempotency replay
- payment authorization idempotency conflict rejection
- payment capture idempotency missing-key rejection
- payment capture idempotency replay
- payment capture idempotency conflict rejection across different payment paths
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
- expired authorization release of held balance
- expired authorization hold release
- expired authorization outbox event creation
- expired authorization lookup in memory and PostgreSQL repositories
- Postgres transactor commit behavior
- Postgres transactor rollback behavior
- nested Postgres transactor reuse behavior
- payment authorization outbox event creation
- payment capture outbox event creation
- payment expiry outbox event creation

Run:

```bash
go test ./...
```

## 10. File Guide

`internal/payment/entity.go`

Defines `Payment`, payment statuses, authorized payment construction, and payment lifecycle transitions.

`internal/payment/hold.go`

Defines `Hold`, hold statuses, hold construction, and hold lifecycle transitions.

`internal/payment/repository.go`

Defines the payment repository interface and payment/hold repository errors.

`internal/payment/service.go`

Defines payment authorization orchestration across merchant, payer, payment, and hold state.
It also defines payment capture orchestration across payment, hold, and payer balance state.
It also defines payment expiry orchestration across payment, hold, payer balance, and outbox state.
These flows use `internal/shared/db.Transactor` so Postgres mode can commit or roll back payer, payment, hold, and outbox mutations together.

`cmd/paycore-expiry-worker/main.go`

Runs one expiry batch against PostgreSQL repositories.

`internal/payment/handler.go`

Owns payment authorization and capture HTTP request parsing, response mapping, and HTTP error mapping.

`internal/payment/response_recorder.go`

Captures HTTP response status and body so successful authorization responses can be stored for idempotency replay.

`internal/payment/adapters/memory/repository.go`

Provides the current non-durable in-memory payment repository implementation.

`internal/payment/adapters/postgres/repository.go`

Owns durable PostgreSQL payment and hold persistence.

`internal/shared/db/transactor.go`

Defines the cross-feature `Transactor` interface, context transaction lookup, and no-op memory transactor.

`internal/shared/db/postgres_transactor.go`

Starts, commits, rolls back, and propagates PostgreSQL transactions through `context.Context`.

`internal/outbox/event.go`

Defines durable outbox event shape and pending event construction.

`internal/outbox/adapters/postgres/repository.go`

Persists outbox events and participates in context-propagated PostgreSQL transactions.

## Checklist

- [x] Add payment entity.
- [x] Add authorization hold entity.
- [x] Add in-memory payment repository.
- [x] Add internal authorization service.
- [x] Add payment HTTP handler.
- [x] Register `POST /payments/authorize`.
- [x] Add payment capture service.
- [x] Register `POST /payments/{payment_id}/capture`.
- [x] Add local idempotency-key enforcement for authorization.
- [x] Add local idempotency-key enforcement for capture.
- [x] Add Redis-backed rate limiting.
- [x] Add PostgreSQL payment and hold migrations.
- [x] Add PostgreSQL payment and hold repository.
- [x] Wire API runtime to PostgreSQL payment repository.
- [x] Add Postgres-backed HTTP lifecycle smoke test.
- [x] Add transaction boundary around authorization and capture business mutations.
- [x] Add durable authorization/capture transaction for payer, payment, and hold writes.
- [x] Add authorization expiry service.
- [x] Add one-shot authorization expiry worker.
- [x] Add durable expiry transaction for payer, payment, hold, and outbox writes.
- [ ] Move durable idempotency completion into the payment transaction boundary.
- [x] Add transactional outbox event for `payment.authorized`.
- [x] Add transactional outbox event for `payment.captured`.
- [x] Add transactional outbox event for `payment.expired`.
- [x] Add transactional outbox event for `payment.settled`.
