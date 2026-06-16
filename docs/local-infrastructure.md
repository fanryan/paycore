# Local Infrastructure

This document explains the current PayCore local infrastructure setup as it exists today. It is written for resume and interview preparation, so it focuses on what runs locally, what each dependency is for, what is intentionally not wired into the app yet, and what is planned next.

## 1. Current Infrastructure Scope

### Implemented

The repository currently provides Docker Compose services for:

- PostgreSQL in `docker-compose.yml`.
- Redis in `docker-compose.yml`.
- Persistent Docker volumes for PostgreSQL and Redis.
- Health checks for PostgreSQL and Redis.
- Local environment template in `.env.example`.
- PostgreSQL merchant, payer, payment, hold, and idempotency schema migrations.

Current services:

```text
paycore-postgres
paycore-redis
```

### Not Implemented Yet

These are planned but not currently implemented:

- Application runtime connection to PostgreSQL.
- Application runtime connection to Redis.
- PostgreSQL payment, idempotency, settlement, and outbox migrations.
- Runtime wiring from the API to PostgreSQL repository adapters.
- Redis rate limiter.
- Redis idempotency response cache.
- Kafka broker.
- Outbox publisher.
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

The PayCore API does not yet connect to PostgreSQL or Redis at runtime.

Current API state is still in memory:

```text
merchant memory repository
payer memory repository
payment memory repository
idempotency memory repository
```

Docker Compose exists now so upcoming PostgreSQL and Redis work can be implemented against repeatable local services.

## 5. Configuration

Current `.env.example` values:

```bash
PAYCORE_ENV=local
PAYCORE_HTTP_ADDR=:8080
PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS=5
PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS=10

PAYCORE_DATABASE_URL=postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable
PAYCORE_REDIS_ADDR=localhost:6379
```

The app currently loads the database URL and Redis address into shared configuration, but does not connect to PostgreSQL or Redis at runtime yet.

## 6. Tests

Current automated tests do not require Docker Compose.

Existing tests use in-memory repositories and can run with:

```bash
go test ./...
```

PostgreSQL repository adapter tests run against local PostgreSQL when `PAYCORE_DATABASE_URL` is set. Redis integration tests are planned once Redis-backed adapters exist.

Schema migrations are plain SQL and are applied by the local `paycore-migrate` command.

Run:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

## 7. File Guide

`docker-compose.yml`

Defines local PostgreSQL and Redis services, ports, volumes, and health checks.

`.env.example`

Documents local environment variables for API runtime and planned database/cache connections.

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
- [x] Add PostgreSQL merchant and payer migrations.
- [x] Add PostgreSQL payment and idempotency migrations.
- [x] Add migration runner.
- [x] Add PostgreSQL repository adapters.
- [x] Wire API runtime to PostgreSQL repository adapters.
- [ ] Add Redis rate limiter.
- [ ] Add Redis idempotency response cache.
- [ ] Add Kafka service.
- [ ] Add Prometheus and Grafana services.
