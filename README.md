# PayCore

PayCore is a production-inspired payment gateway and settlement engine built as a Go backend systems project.

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
- Baseline API performance testing and observability

## Capabilities

PayCore currently includes:

- Merchant and payer APIs
- Payment authorization, capture, and authorization expiry
- Integer minor-unit balances with authorization holds
- Durable PostgreSQL repositories and migrations
- Service-level PostgreSQL transactions with context propagation
- Durable idempotency records with optional Redis response caching
- Redis-backed fixed-window rate limiting for payment mutations
- Transactional outbox events for payment and settlement lifecycle changes
- Logging and Kafka outbox publishers
- Settlement batching with stale-batch recovery
- Prometheus metrics for API, payments, idempotency, rate limiting, outbox, settlement, and payer contention
- Docker Compose local infrastructure for PostgreSQL, Redis, Kafka, and Prometheus
- k6 load tests for happy path, idempotency replay, rate limiting, payer contention, and settlement/outbox backlog

Detailed implementation notes live in `docs/`. Start with `docs/architecture.md`, `docs/architecture-tradeoffs.md`, `docs/payment.md`, `docs/failure-modes.md`, and `docs/performance-results.md`.

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

## Repository Map

```text
paycore/
  cmd/
    paycore-api/                # HTTP API
    paycore-migrate/            # PostgreSQL migrations
    paycore-outbox-worker/      # Outbox publisher
    paycore-expiry-worker/      # Authorization expiry
    paycore-settlement-worker/  # Settlement batches
  internal/
    http/                       # Router and middleware
    merchant/                   # Merchant feature
    payer/                      # Payer balances
    payment/                    # Authorization, capture, expiry
    idempotency/                # Durable request replay
    outbox/                     # Transactional event publishing
    settlement/                 # Settlement batches and recovery
    ratelimit/                  # Redis admission control
    shared/                     # Config, db, metrics, helpers
  loadtest/                     # k6 scenarios
  migrations/                   # PostgreSQL schema
  docs/                         # Architecture and feature docs
```

## API Endpoints

```text
GET /healthz
GET /readyz
GET /version
GET /metrics
POST /merchants
GET /merchants
POST /payers
GET /payers
POST /payments/authorize
POST /payments/{payment_id}/capture
```

## Runtime Modes

- Memory repositories are the default for quick local API runs.
- PostgreSQL repositories are enabled with `PAYCORE_REPOSITORY_BACKEND=postgres`.
- Redis rate limiting is opt-in and fails closed when enabled but unavailable.
- Redis idempotency caching is opt-in and falls back to durable PostgreSQL records.
- Kafka publishing is opt-in through the outbox worker; PostgreSQL remains the source of truth.

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
docker exec paycore-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
curl http://localhost:9090/-/ready
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

Start the API server with Redis rate limiting enabled:

```bash
docker compose up -d redis
PAYCORE_RATE_LIMIT_ENABLED=true \
PAYCORE_REDIS_ADDR=localhost:6379 \
go run ./cmd/paycore-api
```

Start the API server with Redis idempotency response caching enabled:

```bash
docker compose up -d redis
PAYCORE_IDEMPOTENCY_CACHE_ENABLED=true \
PAYCORE_REDIS_ADDR=localhost:6379 \
go run ./cmd/paycore-api
```

Start the outbox worker with the default logging publisher:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
PAYCORE_METRICS_ADDR=:9091 \
go run ./cmd/paycore-outbox-worker
```

Start the outbox worker with Kafka publishing:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
PAYCORE_OUTBOX_PUBLISHER=kafka \
PAYCORE_KAFKA_BROKERS=localhost:9092 \
PAYCORE_KAFKA_OUTBOX_TOPIC=paycore.outbox.events \
PAYCORE_METRICS_ADDR=:9091 \
go run ./cmd/paycore-outbox-worker
```

Run migrations before starting the worker. The worker requires the `outbox_events` table from `migrations/000005_create_outbox_events.sql`.

Run one payment authorization expiry batch:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-expiry-worker
```

Run one settlement batch for the previous completed window:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
PAYCORE_METRICS_ADDR=:9093 \
go run ./cmd/paycore-settlement-worker
```

Override the settlement window:

```bash
PAYCORE_SETTLEMENT_WINDOW_MINUTES=30 \
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
PAYCORE_METRICS_ADDR=:9093 \
go run ./cmd/paycore-settlement-worker
```

The API listens on port `8080` by default.

Override the address:

```bash
PAYCORE_HTTP_ADDR=:9090 go run ./cmd/paycore-api
```

Test the current endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:8080/version
curl http://localhost:8080/metrics
```

Worker metrics are exposed on `PAYCORE_METRICS_ADDR`:

```bash
curl http://localhost:9091/metrics
curl http://localhost:9093/metrics
```

Prometheus runs on port `9090` when started through Docker Compose:

```bash
docker compose up -d prometheus
```

Prometheus targets are available at `http://localhost:9090/targets`.

The default `prometheus.yml` scrapes host-run PayCore processes at:

```text
host.docker.internal:8080  # API
host.docker.internal:9091  # outbox worker
host.docker.internal:9093  # settlement worker
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

To run the Kafka publisher integration test:

```bash
docker compose up -d kafka
PAYCORE_KAFKA_BROKERS=localhost:9092 go test ./internal/outbox/adapters/kafka
```

To run the Redis rate limiter integration test:

```bash
docker compose up -d redis
PAYCORE_REDIS_ADDR=localhost:6379 go test ./internal/ratelimit/adapters/redis
```

To run the Redis idempotency cache integration test:

```bash
docker compose up -d redis
PAYCORE_REDIS_ADDR=localhost:6379 go test ./internal/idempotency/adapters/redis
```

To run the settlement domain tests:

```bash
go test ./internal/settlement
```

To run the settlement service integration tests:

```bash
docker compose up -d postgres
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go test ./internal/settlement
```

To run settlement worker command tests:

```bash
go test ./cmd/paycore-settlement-worker
```

To run the settlement PostgreSQL adapter tests:

```bash
docker compose up -d postgres
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go test ./internal/settlement/adapters/postgres
```

To run the Postgres + Kafka outbox worker integration test:

```bash
docker compose up -d postgres kafka
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
PAYCORE_KAFKA_BROKERS=localhost:9092 \
go test ./internal/outbox
```

To run the k6 payment happy-path load test:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-api
```

In another terminal:

```bash
k6 run loadtest/payment_happy_path.js
```

To run the k6 idempotency replay load test:

```bash
k6 run loadtest/idempotency_replay.js
```

