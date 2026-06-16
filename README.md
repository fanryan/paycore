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
- PostgreSQL repository adapters implemented for merchant, payer, payment, holds, and idempotency records
- PostgreSQL merchant, payer, payment, hold, and idempotency schema migrations added
- Runtime repository backend switch implemented with `PAYCORE_REPOSITORY_BACKEND=memory|postgres`
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
- Postgres-backed HTTP smoke test added for merchant creation, payer creation, authorization, capture, and persisted payment reload

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

Runtime wiring to PostgreSQL repositories is available through `PAYCORE_REPOSITORY_BACKEND=postgres`. Memory repositories remain the default. Redis, Kafka, Prometheus, settlement processing, and outbox publishing have not been implemented yet.

Payment authorization and capture enforce `Idempotency-Key`. In memory mode, idempotency records are process-local. In Postgres mode, merchant, payer, payment, hold, and idempotency records use PostgreSQL repositories.

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

Start the API server with PostgreSQL repositories:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
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
| `PAYCORE_REPOSITORY_BACKEND` | `memory` | Repository backend: `memory` or `postgres` |
| `PAYCORE_DATABASE_URL` | empty | PostgreSQL connection string for migrations and repository adapters |
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

By default, tests use in-memory repositories and skip PostgreSQL integration paths unless `PAYCORE_DATABASE_URL` is set. To include Postgres adapter and API smoke coverage, start Postgres, run migrations, then test with the database URL:

```bash
docker compose up -d postgres
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go test ./...
```

## Current Repository Structure

```text
paycore/
  cmd/
    paycore-api/
      main.go
      main_test.go
    paycore-migrate/
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
        postgres/
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
        postgres/
          repository.go
          repository_test.go
    payer/
      entity.go
      handler.go
      repository.go
      service.go
      adapters/
        memory/
          repository.go
        postgres/
          repository.go
          repository_test.go
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
        postgres/
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
