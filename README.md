# PayCore

PayCore is a production-inspired payment gateway and settlement engine built as a Go backend systems project.

The long-term goal is to model high-throughput payment infrastructure with idempotent payment authorization and capture, Redis-backed admission control, PostgreSQL-backed durable state, Kafka lifecycle event publishing, settlement batch processing, and Prometheus observability.

## Goals

PayCore is designed to demonstrate:

- Payment authorization and capture workflows
- Payment holds and balance reservation
- Durable idempotency guarantees
- Redis-backed rate limiting and admission control
- Redis-backed idempotency response caching
- Optimistic concurrency control on payer balances
- Settlement batch processing and crash recovery
- Transactional outbox publishing
- Event-driven integration with LedgerFlow
- High-throughput API design and observability

## Current Status

Current development stage:

- Initial repository setup completed
- Go module initialized
- PayCore API service skeleton implemented
- Health, readiness, and version endpoints implemented
- Request ID middleware implemented
- Structured JSON request logging implemented
- Panic recovery middleware implemented
- Request body size limit middleware implemented
- JSON error response shape introduced
- Configuration loading implemented for environment, HTTP server settings, PostgreSQL URL, and Redis address
- Feature-first package layout introduced for merchant and payer modules
- Merchant entity, service, repository interface, and in-memory adapter implemented
- Payer entity, service, repository interface, and in-memory adapter implemented
- PostgreSQL merchant, payer, payment, hold, and idempotency schema migrations added
- Merchant HTTP create and list endpoints implemented
- Payer HTTP create and list endpoints implemented
- Payer balance reservation, release, and held-capture behavior implemented
- Payment entity, authorization hold entity, repository interface, and in-memory adapter implemented
- In-memory idempotency record, service, repository interface, and memory adapter implemented
- Payment authorization HTTP endpoint implemented with local in-memory `Idempotency-Key` enforcement
- Payment capture service and HTTP endpoint implemented with local in-memory `Idempotency-Key` enforcement
- Shared currency normalization and validation implemented
- Shared random id helper implemented
- Central HTTP router migrated to chi for path parameters and feature route composition
- Docker Compose local PostgreSQL and Redis infrastructure added
- `.env.example` added for local runtime configuration
- HTTP API foundation and middleware tests added
- Configuration tests added
- Merchant and payer unit tests added
- Merchant and payer handler tests added
- Payment service, handler, repository, entity, hold, idempotency, and router tests added

Implemented endpoints:

```text
GET /healthz
GET /readyz
GET /version
POST /merchants
GET /merchants
POST /payers
GET /payers
POST /payments/authorize
POST /payments/{payment_id}/capture
```

Runtime integration with PostgreSQL, Redis, Kafka, Prometheus, settlement processing, and outbox publishing has not been implemented yet. Docker Compose currently starts local PostgreSQL and Redis for upcoming persistence and cache work.

Payment authorization and capture are currently local and in-memory. They enforce `Idempotency-Key` through an in-memory repository, but do not yet use durable PostgreSQL idempotency records, Redis response caching, Redis rate limiting, durable PostgreSQL payment transactions, or outbox event creation.

## Run Locally

Start local infrastructure:

```bash
docker compose up -d
docker compose ps
```

Optional health checks:

```bash
docker exec paycore-postgres pg_isready -U paycore -d paycore
docker exec paycore-redis redis-cli ping
```

Apply local PostgreSQL migrations:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

Start the API server:

```bash
go run ./cmd/paycore-api
```

The API listens on port `8080` by default.

Override the address:

```bash
PAYCORE_HTTP_ADDR=:9090 go run ./cmd/paycore-api
```

Supported local configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PAYCORE_ENV` | `local` | Runtime environment label used in startup logs |
| `PAYCORE_HTTP_ADDR` | `:8080` | HTTP listen address |
| `PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS` | `5` | HTTP read header timeout in seconds |
| `PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS` | `10` | Graceful shutdown timeout in seconds |
| `PAYCORE_DATABASE_URL` | empty | PostgreSQL connection string loaded for upcoming repository adapters |
| `PAYCORE_REDIS_ADDR` | `localhost:6379` | Redis address loaded for upcoming rate limiting and cache adapters |

Test the current endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/version
```

Create local in-memory records:

```bash
curl -i -X POST http://localhost:8080/merchants \
  -H 'Content-Type: application/json' \
  -d '{"id":"merchant-1","name":"Demo Merchant","settlement_currency":"usd"}'

curl -i -X POST http://localhost:8080/payers \
  -H 'Content-Type: application/json' \
  -d '{"id":"payer-1","available_balance_minor":10000,"currency":"usd"}'

curl -i -X POST http://localhost:8080/payments/authorize \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-key-1' \
  -d '{"merchant_id":"merchant-1","payer_id":"payer-1","amount":4000,"currency":"usd"}'
```

Capture an authorized payment:

```bash
curl -i -X POST http://localhost:8080/payments/<payment_id>/capture \
  -H 'Idempotency-Key: demo-capture-key-1'
```

## Test

Run all tests:

```bash
go test ./...
```

## Current Repository Structure

```text
paycore/
  cmd/
    paycore-api/
      main.go
  internal/
    idempotency/
      record.go
      record_test.go
      repository.go
      service.go
      service_test.go
      adapters/
        memory/
          repository.go
          repository_test.go
    http/
      middleware.go
      router.go
      router_test.go
      system_handler.go
    merchant/
      entity.go
      handler.go
      repository.go
      service.go
      adapters/
        memory/
          repository.go
    payer/
      entity.go
      handler.go
      repository.go
      service.go
      adapters/
        memory/
          repository.go
    payment/
      entity.go
      entity_test.go
      handler.go
      handler_test.go
      hold.go
      hold_test.go
      repository.go
      response_recorder.go
      service.go
      service_test.go
      adapters/
        memory/
          repository.go
          repository_test.go
    shared/
      config/
        config.go
        config_test.go
      currency/
        currency.go
        currency_test.go
      httpjson/
        response.go
      id/
        id.go
  docs/
    architecture.md
    idempotency.md
    local-infrastructure.md
    merchant.md
    payer.md
    payment.md
  migrations/
    000001_create_merchants.sql
    000002_create_payers.sql
    000003_create_payments.sql
    000004_create_idempotency_records.sql
  go.mod
  docker-compose.yml
  .env.example
  README.md
```

## Target Architecture

```text
Client
  |
  v
PayCore API Service
  |
  |-- Request Validation
  |-- Request ID Middleware
  |-- Redis Rate Limiter
  |-- Redis Idempotency Cache
  |-- Merchant APIs
  |-- Payer APIs
  |-- Payment Authorization
  |-- Payment Capture
  |-- Settlement APIs
  |-- Prometheus Metrics
  |
  +--> Redis
  |      |-- Rate Limiting
  |      |-- Idempotency Response Cache
  |
  v
PostgreSQL
  |
  |-- Durable Payment State
  |-- Durable Payer Balances
  |-- Durable Idempotency Records
  |-- Durable Settlement Records
  |-- Durable Outbox Events
  |
  +--> Outbox Publisher
          |
          v
        Kafka
          |
          v
      LedgerFlow
```

## Payment Lifecycle

```mermaid
stateDiagram-v2
    [*] --> PENDING
    PENDING --> AUTHORIZED
    PENDING --> FAILED
    AUTHORIZED --> CAPTURED
    AUTHORIZED --> EXPIRED
    CAPTURED --> SETTLED
```

## Planned Implementation Sequence

1. API foundation and configuration
2. Merchant and payer domain models
3. Merchant and payer APIs
4. Payment authorization and holds
5. Payment capture and state machine enforcement
6. Durable idempotency records
7. Redis-backed rate limiting
8. Redis-backed idempotency response caching
9. PostgreSQL persistence
10. Transactional outbox
11. Kafka publishing
12. Settlement batch processing
13. Prometheus metrics
14. Docker Compose local infrastructure
15. Load testing and performance documentation

## Documentation

Current documentation:

- `docs/architecture.md`
- `docs/idempotency.md`
- `docs/local-infrastructure.md`
- `docs/merchant.md`
- `docs/payer.md`
- `docs/payment.md`
- `docs/postgresql-migrations.md`

Planned documentation:

- `docs/architecture-tradeoffs.md`
- `docs/payment-lifecycle.md`
- `docs/idempotency.md`
- `docs/rate-limiting.md`
- `docs/settlement.md`
- `docs/outbox.md`
- `docs/failure-modes.md`
- `docs/performance-results.md`
