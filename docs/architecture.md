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
- Request ID middleware using `X-Request-ID`.
- Structured JSON request logging.
- Structured JSON error response shape.
- Configuration loading from environment variables.
- Unit tests for HTTP routing and configuration loading.

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
