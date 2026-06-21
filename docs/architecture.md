# PayCore Architecture

PayCore owns the payment gateway lifecycle:

- merchant and payer records
- payment authorization
- payment capture
- payment holds
- settlement batches
- lifecycle event emission

PostgreSQL is the durable source of truth.

Redis is used for fast admission control and response caching, but correctness must not depend on Redis durability.

Kafka is used to publish payment lifecycle events to downstream systems such as LedgerFlow.

## Current Implementation

The current repository contains the first API foundation:

- Go HTTP API entrypoint at `cmd/paycore-api`.
- Health endpoint: `GET /healthz`.
- Readiness endpoint: `GET /readyz`.
- Version endpoint: `GET /version`.
- Central HTTP composition package at `internal/http`.
- Chi-based central router for method routes and path parameters.
- Request ID middleware using `X-Request-ID`.
- Structured JSON request logging.
- Panic recovery middleware with structured JSON errors.
- Request body size limit middleware using a 1 MiB default.
- Prometheus `/metrics` endpoint for the API.
- Shared metrics registry and worker metrics server helper.
- HTTP request metrics with route-pattern labels.
- Settlement and outbox metrics.
- Structured JSON error response shape.
- Configuration loading from environment variables, including planned PostgreSQL and Redis settings.
- Shared currency normalization and validation.
- Merchant entity, service, repository interface, and in-memory adapter.
- Payer entity, service, repository interface, and in-memory adapter.
- Merchant and payer HTTP handlers.
- Merchant and payer routes composed through `internal/http`.
- Payer balance mutation methods for reserve, release, and held capture.
- Payment entity, authorization hold entity, repository interface, in-memory adapter, authorization service, and capture service.
- Payment authorization and capture HTTP handlers.
- In-memory idempotency record, repository interface, adapter, and service.
- Local `Idempotency-Key` enforcement for payment authorization and capture.
- Docker Compose local PostgreSQL and Redis services for upcoming infrastructure work.
- PostgreSQL merchant, payer, payment, hold, and idempotency schema migrations.
- Local PostgreSQL migration runner at `cmd/paycore-migrate`.
- Shared HTTP JSON response helper.
- Shared random id helper.
- Unit tests for HTTP routing, configuration loading, currency helpers, merchant behavior, and payer behavior.

Current supported configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PAYCORE_ENV` | `local` | Runtime environment label used in startup logs |
| `PAYCORE_HTTP_ADDR` | `:8080` | HTTP listen address |
| `PAYCORE_METRICS_ADDR` | `:9091` | Metrics listen address used by worker commands |
| `PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS` | `5` | HTTP read header timeout in seconds |
| `PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS` | `10` | Graceful shutdown timeout in seconds |
| `PAYCORE_DATABASE_URL` | empty | PostgreSQL connection string for migrations and repository adapters |
| `PAYCORE_REDIS_ADDR` | `localhost:6379` | Redis address loaded for upcoming rate limiting and cache adapters |

## High-Level Flow

```text
Client
  -> PayCore API
      -> Redis rate limiting
      -> Redis idempotency cache
      -> PostgreSQL durable state
      -> PostgreSQL outbox
          -> Kafka
              -> LedgerFlow
```

## Durability Rule

Redis may improve latency, but PostgreSQL remains authoritative for payment state, payer balances, idempotency records, settlement records, and outbox events.

## Package Layout

PayCore currently uses a feature-first internal package layout with a central HTTP composition root.

```text
cmd/paycore-api
  -> bootstraps config, logger, HTTP server, and dependencies

internal/http
  -> chi router, middleware, system endpoints, shared HTTP response helpers

internal/merchant
  -> merchant entity, service, repository interface, memory adapter

internal/payer
  -> payer entity, service, repository interface, memory adapter

internal/payment
  -> payment entity, hold entity, authorization and capture services, repository interface, memory adapter

internal/idempotency
  -> idempotency record, service, repository interface, memory adapter

internal/shared/config
  -> environment-backed application configuration

internal/shared/currency
  -> currency normalization and validation helpers

internal/shared/httpjson
  -> shared JSON response writer

internal/shared/id
  -> random local id generation
```

Feature packages own their local entity, service, repository interface, and adapters. The `internal/http` package wires feature handlers into one HTTP entrypoint as handlers are introduced.

Business rules should stay in feature entities and services. Middleware should stay limited to cross-cutting HTTP behavior such as request IDs, logging, recovery, authentication, rate limiting, CORS, and body size limits.

## Current Request Flow

Current system endpoints flow through the central HTTP package:

```text
Client
  |
  v
internal/http.Router
  |
  +--> request ID middleware
  +--> recovery middleware
  +--> logging middleware
  +--> body size limit middleware
  |
  +--> GET /healthz
  +--> GET /readyz
  +--> GET /version
  +--> GET /metrics
  +--> POST /merchants
  +--> GET /merchants
  +--> POST /payers
  +--> GET /payers
  +--> POST /payments/authorize
  +--> POST /payments/{payment_id}/capture
```

Merchant and payer handlers currently use in-memory repositories. Their state is not durable and is reset when the API process restarts.

## Current Payment Authorization Flow

Payment authorization is currently exposed through `POST /payments/authorize`.
It requires `Idempotency-Key` and stores local in-memory idempotency records.

```text
Caller
  |
  | POST /payments/authorize
  | Idempotency-Key: <key>
  v
internal/http chi router
  |
  v
Payment Handler
  |
  +--> hash request body
  +--> create or replay idempotency record
  |
  v
Payment Service
  |
  +--> load merchant
  +--> verify merchant can create payments
  +--> load payer
  +--> verify currency and available balance
  +--> generate payment id and hold id
  +--> create authorization hold
  +--> create AUTHORIZED payment
  +--> reserve payer balance
  +--> persist payer, hold, and payment in memory
  +--> complete idempotency record with response
```

## Current Payment Capture Flow

Payment capture is currently exposed through `POST /payments/{payment_id}/capture`.
It requires `Idempotency-Key` and stores local in-memory idempotency records.

```text
Caller
  |
  | POST /payments/{payment_id}/capture
  | Idempotency-Key: <key>
  v
internal/http chi router
  |
  v
Payment Handler
  |
  +--> hash method, path, and body
  +--> create or replay idempotency record
  |
  v
Payment Service
  |
  +--> load payment
  +--> load hold by payment id
  +--> load payer
  +--> verify payment is AUTHORIZED
  +--> verify authorization has not expired
  +--> mark payment CAPTURED
  +--> mark hold CAPTURED
  +--> deduct payer held balance
  +--> persist payer, hold, and payment through configured repositories
  +--> complete idempotency record with response
```

In memory mode, these flows remain process-local and are not durable. In Postgres mode, `payment.Service` uses `internal/shared/db.Transactor` so payer balance mutation, payment mutation, hold mutation, and outbox event creation share one database transaction through context propagation. HTTP idempotency record start/completion still happens outside the payment service transaction; folding idempotency completion into the same durable boundary is planned. Outbox publishing to Kafka is also planned.

## Current Local Infrastructure

Docker Compose currently provides local PostgreSQL and Redis services:

```text
paycore-postgres
paycore-redis
```

The API can run with memory repositories or PostgreSQL repositories. Memory remains the default. `PAYCORE_REPOSITORY_BACKEND=postgres` wires merchant, payer, payment, hold, and idempotency repositories to PostgreSQL. Redis rate limiting and Redis idempotency response caching are still planned.

The repository also includes initial plain SQL migrations for merchant, payer, payment, hold, and idempotency tables. They can be applied locally with `go run ./cmd/paycore-migrate`.
