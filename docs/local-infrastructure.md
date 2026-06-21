# Local Infrastructure

This document explains the current PayCore local infrastructure setup as it exists today. It is written for resume and interview preparation, so it focuses on what runs locally, what each dependency is for, what is wired into the app today, and what is planned next.

## 1. Current Infrastructure Scope

### Implemented

The repository currently provides Docker Compose services for:

- PostgreSQL in `docker-compose.yml`.
- Redis in `docker-compose.yml`.
- Kafka in `docker-compose.yml`.
- Prometheus in `docker-compose.yml`.
- Persistent Docker volumes for PostgreSQL, Redis, Kafka, and Prometheus.
- Health checks for PostgreSQL, Redis, Kafka, and Prometheus.
- Prometheus scrape configuration in `prometheus.yml`.
- Local environment template in `.env.example`.
- PostgreSQL merchant, payer, payment, hold, idempotency, outbox, and settlement schema migrations.
- PostgreSQL repository runtime mode through `PAYCORE_REPOSITORY_BACKEND=postgres`.
- Kafka broker configuration loading through `PAYCORE_KAFKA_BROKERS`.
- Kafka outbox topic configuration loading through `PAYCORE_KAFKA_OUTBOX_TOPIC`.
- Outbox publisher selection through `PAYCORE_OUTBOX_PUBLISHER`.
- Redis rate-limit configuration through `PAYCORE_RATE_LIMIT_ENABLED`, `PAYCORE_RATE_LIMIT_REQUESTS`, and `PAYCORE_RATE_LIMIT_WINDOW_SECONDS`.
- Redis idempotency cache configuration through `PAYCORE_IDEMPOTENCY_CACHE_ENABLED` and `PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS`.

Current services:

```text
paycore-postgres
paycore-redis
paycore-kafka
paycore-prometheus
```

### Not Implemented Yet

These are planned but not currently implemented:

- Grafana.
- Dockerized PayCore API service.

## 2. Local Services

### PostgreSQL

PostgreSQL is planned to become the durable source of truth for:

- merchant records
- payer balances
- payment state
- authorization holds
- idempotency records
- settlement records
- outbox events

Current local connection string:

```text
postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable
```

### Redis

Redis is planned for:

- rate limiting
- idempotency response caching
- fast admission-control checks

Redis-backed rate limiting is implemented for payment mutation routes when `PAYCORE_RATE_LIMIT_ENABLED=true`. Redis-backed idempotency response caching is implemented when `PAYCORE_IDEMPOTENCY_CACHE_ENABLED=true`. Correctness must not depend on Redis durability. PostgreSQL remains authoritative for durable payment, balance, idempotency, settlement, and outbox state.

### Kafka

Kafka is planned for asynchronous lifecycle event delivery after durable PostgreSQL commits.

The local broker exists now so the outbox publisher adapter can be run against a repeatable dependency. The outbox worker defaults to a logging publisher, but can publish to Kafka when `PAYCORE_OUTBOX_PUBLISHER=kafka`.

### Prometheus

Prometheus scrapes the API and worker metrics endpoints.

The current app processes run on the host, while Prometheus runs in Docker Compose. For that reason, `prometheus.yml` uses `host.docker.internal` targets:

```text
host.docker.internal:8080  # PayCore API
host.docker.internal:9091  # Outbox worker metrics
host.docker.internal:9092  # Settlement worker metrics
```

Prometheus UI:

```text
http://localhost:9090
```

Targets page:

```text
http://localhost:9090/targets
```

## 3. Runtime Flow

Current local infrastructure startup:

```bash
docker compose up -d
```

Health checks:

```bash
docker compose ps
docker exec paycore-postgres pg_isready -U paycore -d paycore
docker exec paycore-redis redis-cli ping
docker exec paycore-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
curl http://localhost:9090/-/ready
```

Shutdown:

```bash
docker compose down
```

Remove local volumes:

```bash
docker compose down -v
```

## 4. Current Application Relationship

The PayCore API can run with either memory repositories or PostgreSQL repositories.

Default API state is still in memory:

```text
merchant memory repository
payer memory repository
payment memory repository
idempotency memory repository
outbox memory repository
```

PostgreSQL runtime mode is available through `PAYCORE_REPOSITORY_BACKEND=postgres`.

```text
merchant postgres repository
payer postgres repository
payment postgres repository
idempotency postgres repository
outbox postgres repository
```

Redis and Kafka are available in Docker Compose. The API can use Redis-backed rate limiting and Redis-backed idempotency response caching when explicitly configured. The outbox worker can publish to Kafka when explicitly configured.

## 5. Configuration

Current `.env.example` values:

```bash
PAYCORE_ENV=local
PAYCORE_HTTP_ADDR=:8080
PAYCORE_METRICS_ADDR=:9091
PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS=5
PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS=10

PAYCORE_DATABASE_URL=postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable
PAYCORE_REDIS_ADDR=localhost:6379
PAYCORE_KAFKA_BROKERS=localhost:9092
PAYCORE_KAFKA_OUTBOX_TOPIC=paycore.outbox.events
PAYCORE_OUTBOX_PUBLISHER=logging
PAYCORE_RATE_LIMIT_ENABLED=false
PAYCORE_RATE_LIMIT_REQUESTS=60
PAYCORE_RATE_LIMIT_WINDOW_SECONDS=60
PAYCORE_IDEMPOTENCY_CACHE_ENABLED=false
PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS=86400
```

The app currently loads the database URL, Redis address, Kafka brokers, Kafka outbox topic, outbox publisher backend, metrics address, rate-limit settings, idempotency cache settings, and repository backend into shared configuration.

The API can run with PostgreSQL repositories when started with:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL=postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable \
go run ./cmd/paycore-api
```

Redis is available in Docker Compose. Redis-backed rate limiting is implemented for payment mutation routes when enabled. Redis-backed idempotency response caching is implemented for completed response replay when enabled.

Kafka is available in Docker Compose. Kafka-backed outbox publishing is implemented behind `PAYCORE_OUTBOX_PUBLISHER=kafka`, while `logging` remains the default local worker mode.

Prometheus is available in Docker Compose. The API exposes metrics on its API port at `/metrics`; worker commands expose metrics on `PAYCORE_METRICS_ADDR`.

## 6. Tests

Default automated tests do not require Docker Compose.

Existing tests use in-memory repositories and can run with:

```bash
go test ./...
```

PostgreSQL repository adapter tests and the API Postgres smoke test run against local PostgreSQL when `PAYCORE_DATABASE_URL` is set. Redis rate-limit and idempotency-cache adapter tests run against local Redis when `PAYCORE_REDIS_ADDR` is set. Kafka publisher integration tests run against local Kafka when `PAYCORE_KAFKA_BROKERS` is set.

Schema migrations are plain SQL and are applied by the local `paycore-migrate` command.

Run:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

## 7. File Guide

`docker-compose.yml`

Defines local PostgreSQL, Redis, Kafka, and Prometheus services, ports, volumes, and health checks.

`prometheus.yml`

Defines local scrape targets for host-run PayCore API and worker processes.

`.env.example`

Documents local environment variables for API runtime and planned database/cache/broker connections.

`docs/local-infrastructure.md`

Documents how local services fit into the project roadmap.

`cmd/paycore-migrate`

Applies local PostgreSQL migrations and records applied files in `schema_migrations`.

## Checklist

- [x] Add Docker Compose PostgreSQL service.
- [x] Add Docker Compose Redis service.
- [x] Add local persistent Docker volumes.
- [x] Add service health checks.
- [x] Add `.env.example`.
- [x] Add database config loading.
- [x] Add Redis config loading.
- [x] Add Kafka config loading.
- [x] Add Kafka outbox topic config loading.
- [x] Add outbox publisher backend config loading.
- [x] Add Redis rate-limit config loading.
- [x] Add Redis idempotency cache config loading.
- [x] Add PostgreSQL merchant and payer migrations.
- [x] Add PostgreSQL payment and idempotency migrations.
- [x] Add PostgreSQL settlement migrations.
- [x] Add migration runner.
- [x] Add PostgreSQL repository adapters.
- [x] Wire API runtime to PostgreSQL repository adapters.
- [x] Add Kafka service.
- [x] Add Kafka-backed outbox publisher.
- [x] Add Redis rate limiter.
- [x] Add Redis idempotency response cache.
- [x] Add Prometheus service.
- [x] Add Prometheus scrape configuration.
- [ ] Add Grafana service.
