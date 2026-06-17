# Local Infrastructure

This document explains the current PayCore local infrastructure setup as it exists today. It is written for resume and interview preparation, so it focuses on what runs locally, what each dependency is for, what is wired into the app today, and what is planned next.

## 1. Current Infrastructure Scope

### Implemented

The repository currently provides Docker Compose services for:

- PostgreSQL in `docker-compose.yml`.
- Redis in `docker-compose.yml`.
- Kafka in `docker-compose.yml`.
- Persistent Docker volumes for PostgreSQL, Redis, and Kafka.
- Health checks for PostgreSQL, Redis, and Kafka.
- Local environment template in `.env.example`.
- PostgreSQL merchant, payer, payment, hold, idempotency, and outbox schema migrations.
- PostgreSQL repository runtime mode through `PAYCORE_REPOSITORY_BACKEND=postgres`.
- Kafka broker configuration loading through `PAYCORE_KAFKA_BROKERS`.
- Kafka outbox topic configuration loading through `PAYCORE_KAFKA_OUTBOX_TOPIC`.
- Outbox publisher selection through `PAYCORE_OUTBOX_PUBLISHER`.

Current services:

```text
paycore-postgres
paycore-redis
paycore-kafka
```

### Not Implemented Yet

These are planned but not currently implemented:

- Application runtime connection to Redis.
- PostgreSQL settlement migrations.
- Redis rate limiter.
- Redis idempotency response cache.
- Prometheus and Grafana.
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

Correctness must not depend on Redis durability. PostgreSQL remains authoritative for durable payment, balance, idempotency, settlement, and outbox state.

### Kafka

Kafka is planned for asynchronous lifecycle event delivery after durable PostgreSQL commits.

The local broker exists now so the outbox publisher adapter can be run against a repeatable dependency. The outbox worker defaults to a logging publisher, but can publish to Kafka when `PAYCORE_OUTBOX_PUBLISHER=kafka`.

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

Redis and Kafka are available in Docker Compose. The API does not yet use Redis-backed rate limiting or Redis-backed idempotency response caching. The outbox worker can publish to Kafka when explicitly configured.

## 5. Configuration

Current `.env.example` values:

```bash
PAYCORE_ENV=local
PAYCORE_HTTP_ADDR=:8080
PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS=5
PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS=10

PAYCORE_DATABASE_URL=postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable
PAYCORE_REDIS_ADDR=localhost:6379
PAYCORE_KAFKA_BROKERS=localhost:9092
PAYCORE_KAFKA_OUTBOX_TOPIC=paycore.outbox.events
PAYCORE_OUTBOX_PUBLISHER=logging
```

The app currently loads the database URL, Redis address, Kafka brokers, Kafka outbox topic, outbox publisher backend, and repository backend into shared configuration.

The API can run with PostgreSQL repositories when started with:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL=postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable \
go run ./cmd/paycore-api
```

Redis is available in Docker Compose, but Redis-backed rate limiting and idempotency response caching are not implemented yet.

Kafka is available in Docker Compose. Kafka-backed outbox publishing is implemented behind `PAYCORE_OUTBOX_PUBLISHER=kafka`, while `logging` remains the default local worker mode.

## 6. Tests

Default automated tests do not require Docker Compose.

Existing tests use in-memory repositories and can run with:

```bash
go test ./...
```

PostgreSQL repository adapter tests and the API Postgres smoke test run against local PostgreSQL when `PAYCORE_DATABASE_URL` is set. Redis integration tests are planned once Redis-backed adapters exist. Kafka publisher integration tests are planned after the publisher topic contract is finalized.

Schema migrations are plain SQL and are applied by the local `paycore-migrate` command.

Run:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

## 7. File Guide

`docker-compose.yml`

Defines local PostgreSQL, Redis, and Kafka services, ports, volumes, and health checks.

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
- [x] Add PostgreSQL merchant and payer migrations.
- [x] Add PostgreSQL payment and idempotency migrations.
- [x] Add migration runner.
- [x] Add PostgreSQL repository adapters.
- [x] Wire API runtime to PostgreSQL repository adapters.
- [x] Add Kafka service.
- [x] Add Kafka-backed outbox publisher.
- [ ] Add Redis rate limiter.
- [ ] Add Redis idempotency response cache.
- [ ] Add Prometheus and Grafana services.
