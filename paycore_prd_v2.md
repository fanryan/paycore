# PayCore PRD v2
## High-Throughput Payment Gateway & Settlement Engine

---

# 1. Project Summary

## Project name
PayCore

## Full title
PayCore: High-Throughput Payment Gateway & Settlement Engine

## Project type
Backend infrastructure / fintech backend / high-throughput API system / distributed systems / event-driven architecture

## Primary goal
Build a high-throughput payment gateway in Go that supports idempotent payment authorization and capture, Redis-backed rate limiting, Redis-backed idempotency response caching, PostgreSQL-backed durable payment state, settlement batch processing, Kafka event emission to LedgerFlow, and Prometheus-based observability.

The project demonstrates:

- High-throughput backend API design in Go
- Payment authorization and capture state machines
- Redis-backed per-merchant and per-payer rate limiting
- Redis-backed idempotency response caching with PostgreSQL durability
- Optimistic concurrency control on payer balances
- Payment hold creation, conversion, and release
- Settlement batch processing with crash recovery
- Kafka event publishing through a transactional outbox
- Downstream integration with LedgerFlow as the accounting system of record
- Prometheus metrics for latency, throughput, Redis behavior, rate limiting, settlement, and outbox lag
- Load testing under concurrent payment authorization workloads

The project is intentionally local-first. It does not integrate with real card networks, banks, or payment rails.

---

# 2. Why This Project Exists

LedgerFlow demonstrates correctness-sensitive financial infrastructure: double-entry accounting, immutable ledger entries, idempotent event consumption, reconciliation, and ledger consistency.

PayCore adds a complementary engineering signal:

- Low-latency write APIs
- Go-based backend service implementation
- Redis-backed caching and admission control
- Payment lifecycle orchestration
- High-concurrency balance mutation handling
- Payment gateway reliability patterns
- Settlement batch recovery
- Operational metrics and performance benchmarking

Together, PayCore and LedgerFlow form a two-service financial infrastructure system:

```text
PayCore Payment Gateway
    -> authorizes, captures, and settles payments
    -> emits Kafka events
        -> LedgerFlow consumes events
            -> posts authoritative double-entry ledger records
```

This architectural separation is deliberate. PayCore owns the payment lifecycle and gateway behavior. LedgerFlow owns accounting correctness and ledger reconciliation.

## The interview answer

When asked why this project exists:

"Payment gateways have to make fast admission decisions while preserving correctness under retries, duplicate requests, burst traffic, and partial failures. I built PayCore to model that path: Redis protects high-traffic payment endpoints through rate limiting and idempotency caching, PostgreSQL remains the durable source of truth for payment state, Kafka propagates lifecycle events, and settlement jobs recover safely from crashes without double-settling payments."

---

# 3. Target Audience

## Primary audience

- Backend engineering recruiters
- Platform engineering hiring managers
- Fintech infrastructure teams
- Distributed systems interviewers
- Payments engineering teams

## Example target companies

- Stripe
- Wise
- Airwallex
- Visa
- Mastercard
- Coinbase
- ByteDance Global Payments
- ShopeePay / SeaMoney
- Goldman Sachs
- JPMC
- Morgan Stanley
- Capital One
- Bloomberg

## Interview themes supported

- High-throughput API design
- Redis rate limiting
- Redis caching tradeoffs
- Durable idempotency
- Payment authorization and capture
- Payment state machines
- Optimistic concurrency control
- Payment holds and balance reservation
- Transactional outbox
- Kafka event production
- Settlement batch recovery
- Redis vs PostgreSQL responsibility boundaries
- Prometheus metrics
- Load testing and latency analysis

---

# 4. High-Level Architecture

```text
Client
 |
 v
PayCore API (Go / HTTP)
 |
 |-- Request Validation
 |-- Request ID Middleware
 |-- Redis Rate Limiter
 |-- Redis Idempotency Cache
 |-- Merchant APIs
 |-- Payer APIs
 |-- Payment Authorization
 |-- Payment Capture
 |-- Settlement Batch Trigger
 |-- Payment Status Queries
 |-- Prometheus Metrics
 |
 +--> Redis
 |     |-- Rate-limit counters
 |     |-- Idempotency response cache
 |     |-- Optional settlement coordination lock
 |
 v
PostgreSQL
 |
 |-- Durable payment state
 |-- Merchant records
 |-- Payer balances
 |-- Payment holds
 |-- Durable idempotency records
 |-- Settlement batches
 |-- Settlement line items
 |-- Outbox events
 |
 +--> Outbox Publisher (Go worker / polling loop)
         |
         v
       Kafka
         |
         +--> payment.authorized
         +--> payment.captured
         +--> payment.failed
         +--> payment.settled
              |
              v
           LedgerFlow
```

---

# 5. Technology Stack

## Core stack

- Go
- PostgreSQL
- Redis
- Kafka
- Docker
- Prometheus

## Go components

- HTTP API server using `net/http` or `chi`
- Middleware for request IDs, structured logging, error handling, and metrics
- Redis client for rate limiting and idempotency cache
- PostgreSQL repository layer
- Outbox publisher worker
- Settlement batch worker
- Prometheus metrics server
- Load test runner or k6 scripts

## Shared infrastructure

- PostgreSQL as durable source of truth
- Redis for cache and admission control
- Apache Kafka for lifecycle event propagation
- Docker Compose for local development

## Serialization format

Kafka payloads use JSON.

Reasoning:

- Easier local debugging
- Easier interview walkthroughs
- Lower implementation overhead
- Consistent with LedgerFlow integration

A `shared/schemas` directory may be used to document event payloads and support future migration to Protobuf or Avro.

---

# 6. Service Boundaries and Design Philosophy

## PayCore owns

- Payment authorization
- Payment capture
- Payment holds
- Payer available balance mutations
- Merchant settlement batching
- Payment lifecycle events
- Gateway rate limiting
- Idempotent write APIs
- Payment gateway metrics

## LedgerFlow owns

- Authoritative accounting records
- Double-entry ledger posting
- Immutable ledger entries
- Ledger balance correctness
- Ledger reconciliation
- Downstream accounting auditability

## Redis owns

- Fast rate-limit counters
- Cached idempotency responses
- Optional short-lived settlement coordination lock

## PostgreSQL owns

- Durable payment state
- Durable payer balances
- Durable payment holds
- Durable idempotency records
- Durable settlement batches
- Durable outbox events

## Important rule

Redis improves latency and coordination, but correctness must not depend on Redis durability. Redis keys may expire, be evicted, or be lost without corrupting payment state. PostgreSQL remains authoritative.

---

# 7. Core Domain Concepts

## Payment state machine

```text
PENDING
  |
  v
AUTHORIZED
  |
  +--> CAPTURED
  |       |
  |       v
  |    SETTLED
  |
  +--> EXPIRED
  |
  +--> FAILED
```

Valid transitions:

- `PENDING -> AUTHORIZED`: authorization succeeds and funds are held.
- `PENDING -> FAILED`: authorization fails.
- `AUTHORIZED -> CAPTURED`: hold is converted into a charge.
- `AUTHORIZED -> EXPIRED`: authorization TTL expires before capture.
- `CAPTURED -> SETTLED`: payment is included in a settlement batch.

Invalid transitions return HTTP `409 Conflict`.

## Authorization hold

Authorization places a hold on the payer's available balance.

The hold is released if:

- Authorization expires before capture.
- Capture fails permanently.
- Partial capture is used and excess authorization amount must be released.

The hold is converted if:

- Capture succeeds.

## Two-phase payment model

Authorization and capture are separate operations:

- Authorization validates the payer and reserves funds.
- Capture converts the reservation into a charge.

This models real payment systems where funds may be reserved before final capture.

## Settlement batch

Settlement aggregates captured payments for merchants over a time window and computes net payout positions.

Settlement must be idempotent. If the settlement worker crashes midway, rerunning the batch must not double-settle any payment.

## Idempotency

All payment mutation endpoints require an `Idempotency-Key` header.

Behavior:

- Duplicate request with identical payload returns the original response.
- Duplicate request with different payload returns `409 IDEMPOTENCY_KEY_CONFLICT`.
- Idempotency keys expire after 24 hours.
- Redis may cache idempotent responses.
- PostgreSQL stores durable idempotency records.

## Rate limiting

PayCore uses Redis to protect payment write endpoints from burst traffic.

Rate limits apply to:

- Merchant-level authorization volume
- Payer-level authorization attempts
- Capture requests
- Optional global fallback limits

Rate limiting is enforced before expensive database mutation work.

---

# 8. Service Responsibilities

## PayCore API Service

Responsibilities:

- Merchant account creation and lookup
- Payer account creation and lookup
- Payment authorization
- Payment capture
- Payment status lookup
- Settlement batch triggering
- Settlement batch lookup
- Request validation
- Idempotency enforcement
- Redis idempotency cache lookup and population
- Redis-backed rate limiting
- PostgreSQL transaction orchestration
- Payment state-machine enforcement
- Payer balance optimistic concurrency
- Outbox event creation
- Prometheus metric emission
- Structured error responses

## Redis Rate Limiter

Responsibilities:

- Enforce per-merchant rate limits
- Enforce per-payer authorization attempt limits
- Enforce capture endpoint limits
- Return retry metadata where appropriate
- Emit allowed and rejected request metrics
- Fail closed for payment mutations if Redis is unavailable

## Redis Idempotency Cache

Responsibilities:

- Cache successful idempotent write responses
- Cache relevant failed idempotent responses where safe
- Reduce latency for duplicate retry requests
- Store request hash, response status, and response body
- Use TTL aligned with idempotency key expiry
- Fall back to PostgreSQL on cache miss

## PostgreSQL Repository Layer

Responsibilities:

- Persist merchant, payer, payment, hold, settlement, idempotency, and outbox records
- Enforce payment state transitions within transactions
- Enforce payer balance updates with optimistic concurrency
- Store durable idempotency records
- Store durable settlement batch state
- Store durable outbox events

## Outbox Publisher

Responsibilities:

- Poll unpublished outbox events
- Claim rows using claim-based locking
- Publish payment lifecycle events to Kafka
- Mark events as published after successful publish
- Retry failed publishes with backoff
- Track pending event count and publish lag
- Preserve events during Kafka outages

## Settlement Batch Engine

Responsibilities:

- Select captured payments eligible for settlement
- Claim payments atomically for a settlement batch
- Compute per-merchant net positions
- Create settlement batch and line items
- Mark payments as settled
- Write `payment.settled` outbox events
- Recover from stale or failed batch processing

## Metrics Server

Responsibilities:

- Expose `/metrics` in Prometheus format
- Track HTTP latency and throughput
- Track payment authorization and capture outcomes
- Track Redis rate-limit behavior
- Track idempotency cache hit and miss rates
- Track settlement batch duration
- Track outbox lag and pending events

---

# 9. Redis Usage

## Redis is used for

- Rate limiting
- Idempotency response caching
- Optional settlement coordination lock

## Redis is not used for

- Durable payment state
- Durable balance state
- Durable settlement records
- Durable idempotency source of truth
- Durable outbox events

## Redis key design

```text
rate_limit:merchant:{merchant_id}:authorize:{window}
rate_limit:payer:{payer_id}:authorize:{window}
rate_limit:merchant:{merchant_id}:capture:{window}
idempotency:response:{idempotency_key}
settlement_lock:{window_start}:{window_end}
```

## Redis idempotency cache value

```json
{
  "request_hash": "sha256",
  "response_status": 200,
  "response_body": {
    "payment_id": "uuid",
    "status": "AUTHORIZED"
  },
  "created_at": "2026-06-06T00:00:00Z",
  "expires_at": "2026-06-07T00:00:00Z"
}
```

## TTL policy

- Rate-limit keys expire after the configured rate-limit window.
- Idempotency cache entries expire after 24 hours.
- Settlement lock keys expire after a short lock TTL, such as 60 seconds.

## Redis failure policy

For payment mutation endpoints, Redis failure causes fail-closed behavior.

If Redis is unavailable during rate-limit enforcement:

- Return HTTP `503 Service Unavailable`.
- Error code: `RATE_LIMITER_UNAVAILABLE`.
- Do not process the payment mutation.

Reasoning:

- Payment writes are sensitive operations.
- Allowing unlimited writes during rate limiter failure creates a poor financial-systems default.
- Fail-closed behavior is safer and easier to explain.

For idempotency cache failures after rate-limit success:

- Fall back to PostgreSQL durable idempotency records if Redis read fails.
- Continue processing if PostgreSQL is available.
- Emit Redis failure metrics.

---

# 10. Rate Limiting Design

## Protected endpoints

- `POST /payments/authorize`
- `POST /payments/{payment_id}/capture`
- `POST /settlement-batches`

## Limit dimensions

### Merchant-level authorization limit

Protects the system from one merchant generating excessive authorization traffic.

Example:

```text
1000 authorization requests per merchant per minute
```

### Payer-level authorization attempt limit

Protects against repeated attempts against the same payer.

Example:

```text
20 authorization attempts per payer per minute
```

### Merchant-level capture limit

Protects capture endpoint from burst traffic.

Example:

```text
1000 capture requests per merchant per minute
```

## Algorithm

Use a fixed-window counter or sliding-window counter.

For local-first implementation, fixed-window counter is acceptable if documented. Sliding-window counter is stronger if time permits.

## Rate limit response

HTTP status:

```text
429 Too Many Requests
```

Response body:

```json
{
  "error_code": "RATE_LIMIT_EXCEEDED",
  "message": "Rate limit exceeded for merchant authorization requests",
  "request_id": "uuid",
  "timestamp": "2026-06-06T00:00:00Z",
  "retry_after_seconds": 30
}
```

---

# 11. Idempotency Design

## Idempotency requirement

All write operations require an `Idempotency-Key` header.

Protected operations:

- `POST /payments/authorize`
- `POST /payments/{payment_id}/capture`
- `POST /settlement-batches`

## Request hash

PayCore computes a deterministic hash from:

- HTTP method
- route pattern
- request body
- authenticated or supplied merchant context where relevant

The hash is stored with the idempotency key.

## Idempotency flow

```text
1. Receive write request with Idempotency-Key.
2. Check Redis idempotency response cache.
3. If Redis hit and request hash matches, return cached response.
4. If Redis hit and request hash differs, return 409 conflict.
5. If Redis miss, check PostgreSQL idempotency_keys.
6. If PostgreSQL hit and request hash matches, return stored response and repopulate Redis.
7. If PostgreSQL hit and request hash differs, return 409 conflict.
8. If key is new, process request inside PostgreSQL transaction.
9. Persist idempotency record and response in PostgreSQL.
10. Populate Redis cache with TTL.
11. Return response.
```

## Durable idempotency guarantee

PostgreSQL remains authoritative. If Redis evicts a cached response, PayCore can still return the correct original response from PostgreSQL.

## Expired idempotency keys

If an idempotency key has expired:

- PayCore rejects reuse of the expired key.
- Client must generate a new key.
- Error code: `IDEMPOTENCY_KEY_EXPIRED`.

Reasoning:

- Prevent ambiguity around delayed retries.
- Keep retry semantics explicit.
- Avoid accidental replay of stale payment operations.

---

# 12. Merchant and Payer Model

## Merchant accounts

Each merchant has:

- Merchant ID
- Name
- Status
- Settlement currency
- Optional rate-limit tier
- Created timestamp
- Updated timestamp

Valid statuses:

- `ACTIVE`
- `SUSPENDED`
- `CLOSED`

Rules:

- `ACTIVE` merchants may authorize, capture, and settle payments.
- `SUSPENDED` merchants cannot create new authorizations or captures.
- `CLOSED` merchants are immutable except for historical reads.

## Payer accounts

Each payer has:

- Payer ID
- Available balance
- Held balance
- Currency
- Version
- Created timestamp
- Updated timestamp

Balances are stored in minor currency units as `bigint` to avoid floating point errors.

---

# 13. Payment Authorization

## Endpoint

```text
POST /payments/authorize
```

## Requirements

Headers:

```text
Idempotency-Key: required
```

Request body:

```json
{
  "merchant_id": "uuid",
  "payer_id": "uuid",
  "amount": 10000,
  "currency": "USD"
}
```

## Behavior

1. Validate request payload.
2. Enforce Redis rate limits.
3. Check Redis idempotency cache.
4. Fall back to PostgreSQL idempotency record if needed.
5. Validate merchant status.
6. Validate payer currency.
7. Validate payer has sufficient available balance.
8. Create payment in `AUTHORIZED` status.
9. Create payment hold in `HELD` status.
10. Decrease payer available balance.
11. Increase payer held balance.
12. Store durable idempotency response.
13. Write outbox event `payment.authorized`.
14. Populate Redis idempotency cache.
15. Return authorization response.

## Validation rules

Reject if:

- amount is zero or negative
- merchant does not exist
- merchant is suspended or closed
- payer does not exist
- payer currency does not match request currency
- payer has insufficient available balance
- idempotency key conflicts with different payload
- rate limit is exceeded

## Success response

```json
{
  "payment_id": "uuid",
  "status": "AUTHORIZED",
  "merchant_id": "uuid",
  "payer_id": "uuid",
  "amount": 10000,
  "currency": "USD",
  "authorized_at": "2026-06-06T00:00:00Z",
  "expires_at": "2026-06-06T00:15:00Z"
}
```

---

# 14. Payment Capture

## Endpoint

```text
POST /payments/{payment_id}/capture
```

## Requirements

Headers:

```text
Idempotency-Key: required
```

Request body:

```json
{
  "capture_amount": 10000
}
```

## Behavior

1. Validate request payload.
2. Enforce Redis rate limits.
3. Check Redis idempotency cache.
4. Fall back to PostgreSQL idempotency record if needed.
5. Load payment and hold.
6. Validate payment is in `AUTHORIZED` status.
7. Validate authorization has not expired.
8. Validate capture amount is less than or equal to authorized amount.
9. Convert hold to charge.
10. Release excess hold if partial capture is supported.
11. Transition payment to `CAPTURED`.
12. Store durable idempotency response.
13. Write outbox event `payment.captured`.
14. Populate Redis idempotency cache.
15. Return capture response.

## Capture rules

- Only `AUTHORIZED` payments may be captured.
- `FAILED`, `EXPIRED`, `CAPTURED`, and `SETTLED` payments cannot be captured.
- Full capture is required for minimum implementation.
- Partial capture is optional if time permits.

## Success response

```json
{
  "payment_id": "uuid",
  "status": "CAPTURED",
  "captured_amount": 10000,
  "currency": "USD",
  "captured_at": "2026-06-06T00:05:00Z"
}
```

---

# 15. Authorization Expiry

## Purpose

Authorized payments should not remain indefinitely capturable.

## Expiry policy

Default authorization TTL:

```text
15 minutes
```

## Expiry behavior

When an authorization expires:

- Payment transitions from `AUTHORIZED` to `EXPIRED`.
- Payment hold transitions from `HELD` to `RELEASED`.
- Payer held balance decreases.
- Payer available balance increases.
- Outbox event `payment.failed` or `payment.expired` may be emitted.

## Implementation options

Minimum implementation:

- Check expiry lazily during capture.
- If expired, release hold in the capture transaction and return `422 AUTHORIZATION_EXPIRED`.

Optional implementation:

- Scheduled expiry worker scans expired authorizations and releases holds.

---

# 16. Settlement Batch Processing

## Endpoint

```text
POST /settlement-batches
```

## Purpose

Settlement groups captured payments into merchant payout batches.

## Request body

```json
{
  "window_start": "2026-06-06T00:00:00Z",
  "window_end": "2026-06-06T23:59:59Z"
}
```

## Batch lifecycle

```text
PENDING
  |
  v
PROCESSING
  |
  +--> COMPLETED
  |
  +--> FAILED
```

## Settlement flow

1. Optionally acquire Redis settlement coordination lock for the window.
2. Create settlement batch in PostgreSQL.
3. Claim eligible captured payments by setting `settlement_batch_id` atomically.
4. Compute per-merchant net positions.
5. Create settlement line items.
6. Mark payments as `SETTLED`.
7. Mark settlement batch as `COMPLETED`.
8. Write `payment.settled` outbox events for settled payments.
9. Release Redis coordination lock if used.

## Idempotency guarantee

Each payment may appear in exactly one settlement batch.

The `payments.settlement_batch_id` column prevents double settlement. Even if the worker crashes and restarts, already-claimed payments are not claimed again for another batch.

## Crash recovery

If the settlement worker crashes after claiming payments but before completing the batch:

- Batch remains `PROCESSING`.
- `locked_until` eventually expires.
- Another worker may reclaim the batch.
- Claimed payments remain associated with the same batch.
- Reprocessing continues safely.

## Net position calculation

For each merchant:

```text
net_position = sum(captured_payment_amounts) - sum(fees)
```

Fee support is optional. If omitted, net position equals the sum of captured payment amounts.

---

# 17. Transactional Outbox

## Purpose

PayCore must avoid dual-write inconsistency between PostgreSQL and Kafka.

Business state changes and outbox event records are written in the same PostgreSQL transaction. Kafka publishing happens asynchronously.

## Produced events

```text
payment.authorized
payment.captured
payment.failed
payment.settled
```

## Outbox table states

```text
PENDING
PROCESSING
PUBLISHED
FAILED
```

## Claim-based publishing

Outbox rows contain:

- `claimed_by`
- `locked_until`
- `attempt_count`
- `next_attempt_at`
- `published_at`
- `last_error`

## Publish flow

1. Publisher scans `PENDING` or retryable `FAILED` events.
2. Publisher claims rows by setting `claimed_by` and `locked_until`.
3. Publisher sends events to Kafka.
4. On success, publisher marks rows as `PUBLISHED`.
5. On failure, publisher increments attempt count and sets next retry time.
6. Stale `PROCESSING` rows become reclaimable after lock expiry.

## Delivery guarantee

The outbox provides at-least-once delivery to Kafka. Downstream consumers must be idempotent.

LedgerFlow consumes relevant PayCore events and uses event IDs or payment IDs for deduplication.

---

# 18. Kafka Event Schema

## payment.authorized

```json
{
  "event_id": "uuid",
  "event_type": "payment.authorized",
  "payment_id": "uuid",
  "merchant_id": "uuid",
  "payer_id": "uuid",
  "amount": 10000,
  "currency": "USD",
  "authorized_at": "2026-06-06T00:00:00Z",
  "expires_at": "2026-06-06T00:15:00Z"
}
```

## payment.captured

```json
{
  "event_id": "uuid",
  "event_type": "payment.captured",
  "payment_id": "uuid",
  "merchant_id": "uuid",
  "payer_id": "uuid",
  "amount": 10000,
  "currency": "USD",
  "captured_at": "2026-06-06T00:05:00Z"
}
```

## payment.failed

```json
{
  "event_id": "uuid",
  "event_type": "payment.failed",
  "payment_id": "uuid",
  "failure_reason": "INSUFFICIENT_BALANCE",
  "failed_at": "2026-06-06T00:00:00Z"
}
```

## payment.settled

```json
{
  "event_id": "uuid",
  "event_type": "payment.settled",
  "payment_id": "uuid",
  "merchant_id": "uuid",
  "settlement_batch_id": "uuid",
  "amount": 10000,
  "currency": "USD",
  "settled_at": "2026-06-06T23:00:00Z"
}
```

---

# 19. Data Model

## merchants

Fields:

- `id UUID PRIMARY KEY`
- `name TEXT NOT NULL`
- `status TEXT NOT NULL`
- `settlement_currency TEXT NOT NULL`
- `rate_limit_tier TEXT NULL`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Indexes:

- `idx_merchants_status` on `status`

## payers

Fields:

- `id UUID PRIMARY KEY`
- `available_balance BIGINT NOT NULL`
- `held_balance BIGINT NOT NULL DEFAULT 0`
- `currency TEXT NOT NULL`
- `version BIGINT NOT NULL DEFAULT 0`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Indexes:

- `idx_payers_currency` on `currency`

## payments

Fields:

- `id UUID PRIMARY KEY`
- `merchant_id UUID NOT NULL REFERENCES merchants(id)`
- `payer_id UUID NOT NULL REFERENCES payers(id)`
- `amount BIGINT NOT NULL`
- `currency TEXT NOT NULL`
- `status TEXT NOT NULL`
- `idempotency_key TEXT NOT NULL UNIQUE`
- `request_hash TEXT NOT NULL`
- `authorized_at TIMESTAMP NULL`
- `captured_at TIMESTAMP NULL`
- `expires_at TIMESTAMP NULL`
- `settled_at TIMESTAMP NULL`
- `settlement_batch_id UUID NULL REFERENCES settlement_batches(id)`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Indexes:

- `idx_payments_merchant_status` on `(merchant_id, status)`
- `idx_payments_payer_status` on `(payer_id, status)`
- `idx_payments_settlement_batch` on `settlement_batch_id`
- `idx_payments_captured_unsettled` on `(status, captured_at)` where `status = 'CAPTURED'`

## payment_holds

Fields:

- `id UUID PRIMARY KEY`
- `payment_id UUID NOT NULL REFERENCES payments(id)`
- `payer_id UUID NOT NULL REFERENCES payers(id)`
- `amount BIGINT NOT NULL`
- `status TEXT NOT NULL`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Valid statuses:

- `HELD`
- `RELEASED`
- `CONVERTED`

Indexes:

- `idx_payment_holds_payment_id` on `payment_id`
- `idx_payment_holds_payer_status` on `(payer_id, status)`

## settlement_batches

Fields:

- `id UUID PRIMARY KEY`
- `status TEXT NOT NULL`
- `window_start TIMESTAMP NOT NULL`
- `window_end TIMESTAMP NOT NULL`
- `claimed_by TEXT NULL`
- `locked_until TIMESTAMP NULL`
- `completed_at TIMESTAMP NULL`
- `last_error TEXT NULL`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Indexes:

- `idx_settlement_batches_status` on `status`
- `idx_settlement_batches_window` on `(window_start, window_end)`
- `idx_settlement_batches_stale_locks` on `locked_until` where `status = 'PROCESSING'`

## settlement_line_items

Fields:

- `id UUID PRIMARY KEY`
- `settlement_batch_id UUID NOT NULL REFERENCES settlement_batches(id)`
- `merchant_id UUID NOT NULL REFERENCES merchants(id)`
- `payment_id UUID NOT NULL REFERENCES payments(id)`
- `amount BIGINT NOT NULL`
- `fee_amount BIGINT NOT NULL DEFAULT 0`
- `net_amount BIGINT NOT NULL`
- `currency TEXT NOT NULL`
- `created_at TIMESTAMP NOT NULL`

Constraints:

- Unique constraint on `payment_id` to prevent duplicate settlement line items.

Indexes:

- `idx_settlement_line_items_batch` on `settlement_batch_id`
- `idx_settlement_line_items_merchant` on `merchant_id`

## idempotency_keys

Fields:

- `key TEXT PRIMARY KEY`
- `request_hash TEXT NOT NULL`
- `resource_type TEXT NOT NULL`
- `resource_id UUID NULL`
- `response_status INT NOT NULL`
- `response_body JSONB NOT NULL`
- `expires_at TIMESTAMP NOT NULL`
- `created_at TIMESTAMP NOT NULL`

Indexes:

- `idx_idempotency_keys_expires_at` on `expires_at`

## outbox_events

Fields:

- `id UUID PRIMARY KEY`
- `aggregate_type TEXT NOT NULL`
- `aggregate_id UUID NOT NULL`
- `event_type TEXT NOT NULL`
- `payload JSONB NOT NULL`
- `status TEXT NOT NULL`
- `claimed_by TEXT NULL`
- `locked_until TIMESTAMP NULL`
- `attempt_count INT NOT NULL DEFAULT 0`
- `next_attempt_at TIMESTAMP NOT NULL DEFAULT now()`
- `published_at TIMESTAMP NULL`
- `last_error TEXT NULL`
- `created_at TIMESTAMP NOT NULL`
- `updated_at TIMESTAMP NOT NULL`

Indexes:

- `idx_outbox_events_publishable` on `(status, next_attempt_at, created_at)` where `status in ('PENDING', 'FAILED')`
- `idx_outbox_events_stale_claims` on `locked_until` where `status = 'PROCESSING'`
- `idx_outbox_events_aggregate` on `(aggregate_type, aggregate_id)`

---

# 20. API Requirements

## Health and operational APIs

### GET /health

Returns process liveness.

### GET /ready

Checks readiness of:

- PostgreSQL
- Redis
- Kafka producer connectivity where feasible

### GET /metrics

Exposes Prometheus metrics.

---

## Merchant APIs

### POST /merchants

Creates a merchant.

Request:

```json
{
  "name": "Acme Store",
  "settlement_currency": "USD"
}
```

### GET /merchants/{merchant_id}

Returns merchant details.

---

## Payer APIs

### POST /payers

Creates a payer with initial balance.

Request:

```json
{
  "available_balance": 100000,
  "currency": "USD"
}
```

### GET /payers/{payer_id}

Returns payer balance and currency.

---

## Payment APIs

### POST /payments/authorize

Creates an authorization and payment hold.

Requirements:

- `Idempotency-Key` header required
- Redis rate limit check required
- Redis idempotency cache check required
- PostgreSQL durable idempotency required

### POST /payments/{payment_id}/capture

Captures an authorized payment.

Requirements:

- `Idempotency-Key` header required
- Redis rate limit check required
- Payment must be `AUTHORIZED`
- Authorization must not be expired

### GET /payments/{payment_id}

Returns payment details and lifecycle timestamps.

---

## Settlement APIs

### POST /settlement-batches

Triggers settlement for a time window.

Requirements:

- `Idempotency-Key` header required
- Uses batch idempotency to prevent duplicate batch creation
- May use Redis settlement coordination lock

### GET /settlement-batches/{batch_id}

Returns settlement batch status and line items.

---

# 21. HTTP Error Contract

## Error response format

```json
{
  "error_code": "INSUFFICIENT_BALANCE",
  "message": "Payer has insufficient available balance for authorization",
  "request_id": "uuid",
  "timestamp": "2026-06-06T00:00:00Z"
}
```

## Extended error response with retry metadata

```json
{
  "error_code": "RATE_LIMIT_EXCEEDED",
  "message": "Rate limit exceeded for merchant authorization requests",
  "request_id": "uuid",
  "timestamp": "2026-06-06T00:00:00Z",
  "retry_after_seconds": 30
}
```

## Status codes

- `400 Bad Request`: invalid payload, malformed ID, negative amount
- `404 Not Found`: unknown merchant, payer, payment, or settlement batch
- `409 Conflict`: idempotency mismatch, invalid state transition, optimistic lock exhaustion
- `422 Unprocessable Entity`: insufficient balance, suspended merchant, currency mismatch, expired authorization
- `429 Too Many Requests`: rate limit exceeded
- `503 Service Unavailable`: Redis unavailable for rate limiting, PostgreSQL unavailable, service not ready
- `500 Internal Server Error`: unexpected server failure

## Error codes

```text
INVALID_REQUEST
UNKNOWN_MERCHANT
UNKNOWN_PAYER
UNKNOWN_PAYMENT
UNKNOWN_SETTLEMENT_BATCH
MERCHANT_SUSPENDED
MERCHANT_CLOSED
INSUFFICIENT_BALANCE
CURRENCY_MISMATCH
INVALID_PAYMENT_STATE
AUTHORIZATION_EXPIRED
IDEMPOTENCY_KEY_REQUIRED
IDEMPOTENCY_KEY_CONFLICT
IDEMPOTENCY_KEY_EXPIRED
RATE_LIMIT_EXCEEDED
RATE_LIMITER_UNAVAILABLE
IDEMPOTENCY_CACHE_UNAVAILABLE
REDIS_OPERATION_FAILED
OPTIMISTIC_LOCK_CONFLICT
SETTLEMENT_BATCH_CONFLICT
SERVICE_NOT_READY
INTERNAL_ERROR
```

---

# 22. Concurrency Strategy

## Payer balance updates

PayCore uses optimistic concurrency control on payer balances.

Flow:

1. Read payer row and version.
2. Validate available balance.
3. Compute new available and held balances.
4. Attempt update using `WHERE id = ? AND version = ?`.
5. Increment version on success.
6. Retry on conflict with bounded retry policy.
7. Return `409 OPTIMISTIC_LOCK_CONFLICT` after retry exhaustion.

## Retry policy

- Max retries: 3
- Backoff: exponential or fixed short jittered backoff
- Conflicts are tracked through Prometheus metrics

## Payment state transitions

Payment state transitions happen inside PostgreSQL transactions.

State transitions use conditional updates where appropriate:

```sql
UPDATE payments
SET status = 'CAPTURED'
WHERE id = $1 AND status = 'AUTHORIZED';
```

If no row is updated, PayCore returns `409 INVALID_PAYMENT_STATE`.

## Settlement concurrency

Settlement uses PostgreSQL as the correctness mechanism.

Rules:

- A payment can belong to at most one settlement batch.
- `settlement_line_items.payment_id` is unique.
- `payments.settlement_batch_id` prevents double-claiming.

Redis settlement locks may reduce duplicate work, but PostgreSQL constraints preserve correctness.

---

# 23. Observability

## Prometheus metrics

### HTTP metrics

```text
paycore_http_requests_total
paycore_http_request_duration_seconds
paycore_http_in_flight_requests
```

Labels:

- method
- route
- status_code

### Payment metrics

```text
paycore_authorization_total
paycore_authorization_latency_seconds
paycore_capture_total
paycore_capture_latency_seconds
paycore_payment_state_transition_total
paycore_optimistic_lock_conflicts_total
```

Labels:

- status
- merchant_id optional for local debugging only
- reason

### Rate-limit metrics

```text
paycore_rate_limit_allowed_total
paycore_rate_limit_rejected_total
paycore_rate_limit_redis_errors_total
paycore_rate_limit_check_duration_seconds
```

Labels:

- limiter_type
- endpoint

### Idempotency cache metrics

```text
paycore_idempotency_cache_hits_total
paycore_idempotency_cache_misses_total
paycore_idempotency_cache_errors_total
paycore_idempotency_postgres_fallback_total
```

### Redis metrics

```text
paycore_redis_operation_duration_seconds
paycore_redis_operation_errors_total
```

Labels:

- operation

### Settlement metrics

```text
paycore_settlement_batch_total
paycore_settlement_batch_duration_seconds
paycore_settlement_payments_total
paycore_settlement_batch_failures_total
paycore_settlement_recovered_batches_total
```

### Outbox metrics

```text
paycore_outbox_pending_events
paycore_outbox_publish_lag_seconds
paycore_outbox_publish_attempts_total
paycore_outbox_publish_failures_total
```

## Structured logging

Logs include:

- request_id
- payment_id where available
- merchant_id where available
- payer_id where available
- settlement_batch_id where available
- idempotency_key hash only, not raw key
- error_code
- latency_ms

## Readiness checks

`GET /ready` reports readiness for:

- PostgreSQL
- Redis
- Kafka producer availability or configured degraded status

---

# 24. Load Testing Requirements

## Tooling

Use one of:

- k6 scripts
- custom Go load test binary
- vegeta

## Required load tests

### Concurrent authorization load

Simulate many concurrent authorization requests across multiple merchants and payers.

Measure:

- p50 latency
- p95 latency
- p99 latency
- success rate
- optimistic lock conflict rate
- Redis operation latency

### Idempotency retry load

Simulate repeated duplicate requests using the same idempotency key.

Measure:

- Redis cache hit rate
- duplicate response latency
- PostgreSQL fallback rate

### Rate-limit burst test

Simulate a merchant traffic spike above configured limits.

Measure:

- accepted requests
- rejected requests
- rejection latency
- correctness of `429` response behavior

### Redis outage test

Simulate Redis unavailability during payment writes.

Expected behavior:

- payment mutation endpoints fail closed
- service returns `503 RATE_LIMITER_UNAVAILABLE`
- no unauthorized database mutations occur

### Kafka outage test

Simulate Kafka unavailability during payment writes.

Expected behavior:

- payment state commits to PostgreSQL
- outbox event remains pending
- publisher retries after Kafka recovers

### Settlement crash recovery test

Simulate worker crash after payment claiming.

Expected behavior:

- no payment is double-settled
- stale batch is recoverable
- settlement eventually completes

---

# 25. Testing Requirements

## Unit tests

- request validation
- request hash generation
- payment state transition validation
- rate-limit decision logic
- idempotency cache behavior
- settlement net position calculation

## API tests

- merchant creation
- payer creation
- successful authorization
- insufficient balance authorization failure
- duplicate authorization with same idempotency key
- duplicate authorization with conflicting payload
- successful capture
- invalid capture state transition
- expired authorization capture failure
- rate limit exceeded response

## Repository tests

- optimistic payer balance update
- payment and hold creation in one transaction
- idempotency record persistence
- outbox event persistence
- settlement batch creation
- payment settlement claiming

## Redis integration tests

- rate limiter allows within limit
- rate limiter rejects above limit
- idempotency cache hit returns stored response
- cache miss falls back to PostgreSQL
- cache TTL expiry does not break durable idempotency

## Kafka/outbox tests

- outbox event created with payment transaction
- publisher claims rows correctly
- stale claims are reclaimable
- Kafka failure leaves event retryable
- duplicate publish is tolerated downstream

## Settlement tests

- captured payments are included in settlement batch
- each payment appears in only one settlement batch
- settlement net positions are correct
- crash after claim does not double-settle payments
- stale processing batch can be recovered

## Failure simulation

- Redis unavailable
- PostgreSQL optimistic conflict
- Kafka unavailable
- duplicate idempotency requests
- settlement worker crash
- high-concurrency authorization race

---

# 26. Repository Structure

```text
paycore/
  cmd/
    api/
      main.go
    worker/
      main.go
    loadtest/
      main.go

  internal/
    api/
      handlers/
      middleware/
      router.go
      errors.go

    domain/
      merchant/
      payer/
      payment/
      settlement/
      idempotency/

    redis/
      client.go
      rate_limiter.go
      idempotency_cache.go
      settlement_lock.go

    db/
      postgres.go
      migrations/
      repositories/

    outbox/
      publisher.go
      repository.go
      kafka_producer.go

    settlement/
      batch_engine.go
      recovery.go

    metrics/
      prometheus.go

    config/
      config.go

    logging/
      logger.go

  shared/
    schemas/
      payment.authorized.json
      payment.captured.json
      payment.failed.json
      payment.settled.json

  infrastructure/
    docker/
    kafka/
    redis/

  loadtests/
    authorization_burst.js
    idempotency_retry.js
    rate_limit_burst.js

  docs/
    architecture.md
    failure-modes.md
    performance-results.md

  docker-compose.yml
  README.md
```

---

# 27. Docker Compose Requirements

Local environment includes:

- PayCore API
- PayCore worker
- PostgreSQL
- Redis
- Kafka
- Optional Kafka UI
- Optional Prometheus

Minimum `docker compose up` behavior:

- PostgreSQL starts with migrations applied.
- Redis starts with default local config.
- Kafka starts with required topics created.
- PayCore API exposes `/health`, `/ready`, and `/metrics`.
- PayCore worker can publish outbox events.

Required Kafka topics:

```text
payment.authorized
payment.captured
payment.failed
payment.settled
```

---

# 28. Milestones

## Milestone 1: API foundation and infrastructure

Deliverables:

- Go HTTP API foundation
- router and middleware
- request ID propagation
- structured error contract
- PostgreSQL connection
- Redis connection
- Prometheus `/metrics`
- `/health` and `/ready`
- Docker Compose for PostgreSQL, Redis, Kafka, and API

## Milestone 2: Merchant, payer, and authorization flow

Deliverables:

- merchant creation and lookup
- payer creation and lookup
- Redis rate limiting for authorization
- Redis idempotency response cache
- PostgreSQL durable idempotency records
- payment authorization
- payment hold creation
- optimistic concurrency on payer balances
- `payment.authorized` outbox event

## Milestone 3: Capture flow and state machine

Deliverables:

- payment capture endpoint
- state-machine enforcement
- authorization expiry handling
- hold conversion
- idempotent capture
- `payment.captured` outbox event
- concurrent capture and invalid transition tests

## Milestone 4: Transactional outbox and Kafka publishing

Deliverables:

- outbox schema
- claim-based outbox publisher
- Kafka producer
- retry behavior
- stale claim recovery
- outbox lag metrics
- Kafka outage test

## Milestone 5: Settlement batch engine

Deliverables:

- settlement batch creation
- captured payment claiming
- merchant net position calculation
- settlement line items
- `payment.settled` events
- crash recovery behavior
- stale batch recovery tests

## Milestone 6: Observability and load testing

Deliverables:

- Prometheus metrics across API, Redis, payments, settlement, and outbox
- authorization load test
- idempotency retry load test
- rate-limit burst test
- Redis outage test
- performance results documented in README or `docs/performance-results.md`

---

# 29. Non-Goals

PayCore is not intended to be:

- A real card network integration
- A real bank payment rail integration
- A PCI-compliant payment processor
- A fraud detection platform
- A KYC or merchant onboarding system
- A full accounting ledger
- A replacement for LedgerFlow
- A full reconciliation engine
- A multi-currency FX platform
- A blockchain payment system
- A frontend application
- A cloud-native Kubernetes deployment
- A Grafana dashboard project, unless time permits

Redis is not used as a source of truth.

---

# 30. Success Criteria

The project is complete when:

- Payment authorization is idempotent and creates durable payment holds.
- Payment capture enforces valid state transitions.
- Payer balances remain correct under concurrent authorization attempts.
- Redis rate limiting protects payment write endpoints.
- Redis idempotency cache returns duplicate responses without reprocessing.
- PostgreSQL remains authoritative for payment state and idempotency.
- Settlement batches claim captured payments safely.
- Settlement crash recovery does not double-settle payments.
- Transactional outbox reliably emits payment lifecycle events to Kafka.
- LedgerFlow can consume `payment.captured` and `payment.settled` events.
- Prometheus exposes latency, throughput, Redis, rate-limit, settlement, and outbox metrics.
- Load tests document p50, p95, and p99 authorization latency.
- Redis outage behavior is documented and tested.
- Kafka outage behavior is documented and tested.
- `docker compose up` runs the local stack cleanly.

---

# 31. Resume Positioning

## Project title

PayCore — High-Throughput Payment Gateway & Settlement Engine

## Resume stack

Go, PostgreSQL, Redis, Kafka, Prometheus, Docker

## Resume bullets

- Built a high-throughput payment gateway in Go with idempotent authorization and capture APIs, enforcing payment state transitions and optimistic concurrency on payer balances under concurrent request load.

- Implemented Redis-backed per-merchant and per-payer rate limiting to protect payment write endpoints from burst traffic, with Prometheus metrics for allowed, rejected, and latency-sensitive request paths.

- Designed a Redis-backed idempotency response cache with PostgreSQL as the durable source of truth, reducing retry-path latency while preserving correctness under cache expiry or eviction.

- Built a settlement batch engine that atomically claims captured payments, computes merchant net positions, and recovers from mid-batch crashes without double-settling payments.

- Implemented a transactional outbox publisher that emits payment lifecycle events to Kafka for downstream LedgerFlow ledger posting, with outbox lag and publish metrics exposed through Prometheus.

---

# 32. How PayCore and LedgerFlow Relate

## The architecture in one sentence

PayCore handles the payment lifecycle and gateway behavior; LedgerFlow consumes PayCore events and maintains the authoritative accounting record.

## Why they are separate services

Separation keeps responsibilities clear:

- PayCore optimizes for fast, reliable payment gateway operations.
- LedgerFlow optimizes for accounting correctness and ledger auditability.

If PayCore authorizes, captures, or settles a payment, it emits lifecycle events through Kafka. LedgerFlow consumes those events and records the corresponding double-entry ledger transactions.

## Interview explanation

"PayCore is the upstream payment gateway. It handles authorization, capture, Redis-backed rate limiting, idempotency caching, settlement batching, and Kafka event emission. LedgerFlow is the downstream accounting system of record. It consumes PayCore events and posts double-entry ledger entries. I separated them because gateway concerns and ledger correctness are different problems: PayCore focuses on low-latency APIs and operational controls, while LedgerFlow focuses on immutable accounting records and reconciliation."

## Key failure modes handled

- Duplicate authorization requests with the same idempotency key
- Conflicting idempotency key reuse
- Burst traffic from one merchant
- Redis outage during payment mutation
- Concurrent authorization attempts against the same payer balance
- Kafka outage after payment state has committed
- Settlement worker crash after payment claiming
- Duplicate Kafka event delivery downstream

Each failure mode is handled explicitly so the project demonstrates backend systems reasoning rather than simple CRUD behavior.

