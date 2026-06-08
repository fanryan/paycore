# PayCore

PayCore is a local-first high-throughput payment gateway and settlement engine built in Go.

It models core payment infrastructure patterns:

- idempotent authorization and capture APIs
- Redis-backed rate limiting and idempotency response caching
- PostgreSQL-backed durable payment state
- payment holds and balance reservation
- settlement batch processing
- transactional outbox publishing to Kafka
- Prometheus observability

PayCore is designed to integrate downstream with LedgerFlow as the accounting system of record.

## Current Status

Initial repository setup.

## Planned Components

- PayCore HTTP API
- PostgreSQL repository layer
- Redis rate limiter
- Redis idempotency cache
- payment authorization and capture workflows
- settlement batch processor
- outbox publisher
- Prometheus metrics
- Docker Compose local environment