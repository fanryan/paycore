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
- Request ID middleware using `X-Request-ID`.
- Structured JSON request logging.
- Structured JSON error response shape.
- Configuration loading from environment variables.
- Shared currency normalization and validation.
- Merchant entity, service, repository interface, and in-memory adapter.
- Payer entity, service, repository interface, and in-memory adapter.
- Merchant and payer HTTP handlers.
- Merchant and payer routes composed through `internal/http`.
- Payer balance mutation methods for reserve, release, and held capture.
- Payment entity, authorization hold entity, repository interface, in-memory adapter, and authorization service.
- Shared HTTP JSON response helper.
- Shared random id helper.
- Unit tests for HTTP routing, configuration loading, currency helpers, merchant behavior, and payer behavior.

Current supported configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PAYCORE_ENV` | `local` | Runtime environment label used in startup logs |
| `PAYCORE_HTTP_ADDR` | `:8080` | HTTP listen address |
| `PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS` | `5` | HTTP read header timeout in seconds |
| `PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS` | `10` | Graceful shutdown timeout in seconds |

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
  -> router, middleware, system endpoints, shared HTTP response helpers

internal/merchant
  -> merchant entity, service, repository interface, memory adapter

internal/payer
  -> payer entity, service, repository interface, memory adapter

internal/payment
  -> payment entity, hold entity, authorization service, repository interface, memory adapter

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
  +--> logging middleware
  |
  +--> GET /healthz
  +--> GET /readyz
  +--> GET /version
  +--> POST /merchants
  +--> GET /merchants
  +--> POST /payers
  +--> GET /payers
  +--> POST /payments/authorize
```

Merchant and payer handlers currently use in-memory repositories. Their state is not durable and is reset when the API process restarts.

## Current Payment Authorization Flow

Payment authorization is currently exposed through `POST /payments/authorize`.

```text
Caller
  |
  | POST /payments/authorize
  v
internal/http.Router
  |
  v
Payment Handler
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
```

Because this is still in-memory, this flow is not transactionally durable. It also does not yet enforce `Idempotency-Key` or Redis rate limiting. PostgreSQL will later make payer balance mutation, hold creation, payment creation, idempotency, and outbox event creation part of one transaction.
