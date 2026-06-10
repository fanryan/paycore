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

internal/shared/config
  -> environment-backed application configuration

internal/shared/currency
  -> currency normalization and validation helpers
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
```

Merchant and payer handlers currently use in-memory repositories. Their state is not durable and is reset when the API process restarts.
