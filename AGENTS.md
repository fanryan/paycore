# PayCore Agent Guide

This file guides future PayCore work when prompts are split across many small steps or context is compacted.

PayCore should stay aligned with `/Users/fan/Downloads/paycore_prd_v2.md`.

## Collaboration Workflow

- Work step by step.
- For implementation files, output full copy-paste code in chat instead of editing directly.
- Implementation files include `cmd/`, `internal/`, migrations, Docker files, config files, scripts, and production source files.
- For tests, `README.md`, and files under `docs/`, edit directly in the repo when asked or when documentation/test updates are part of a milestone.
- Keep each step substantial enough to be useful, but not so large that the user cannot read and type through it.
- After each code step, provide exact commands to run.
- When a feature or major milestone is complete, run or ask the user to run tests, then commit and push.
- After a milestone commit, update `README.md` and relevant `docs/` files.

## Current Project Direction

PayCore is a Go backend infrastructure project for a local-first payment gateway and settlement engine.

The system should eventually include:

- HTTP API service in Go.
- Merchant and payer account APIs.
- Idempotent payment authorization.
- Idempotent payment capture.
- Redis-backed rate limiting.
- Redis-backed idempotency response caching.
- PostgreSQL-backed durable payment state.
- PostgreSQL-backed payer balances and holds.
- Optimistic concurrency control for balance mutation.
- Settlement batch processing with crash recovery.
- Transactional outbox.
- Kafka event publishing to LedgerFlow.
- Prometheus metrics.
- Docker Compose local infrastructure.
- Load testing and performance documentation.

PayCore must not integrate with real card networks, banks, or payment rails.

## Core Architecture Rules

- PostgreSQL is the durable source of truth.
- Redis improves latency and admission control, but correctness must not depend on Redis durability.
- Kafka propagates lifecycle events after durable commit. Kafka is not the source of truth.
- Payment mutation endpoints must require an `Idempotency-Key` header.
- Duplicate idempotency key with the same request hash returns the original response.
- Duplicate idempotency key with a different request hash returns `409 IDEMPOTENCY_KEY_CONFLICT`.
- Expired idempotency key reuse returns `IDEMPOTENCY_KEY_EXPIRED`.
- Payment mutation endpoints fail closed if Redis rate limiting is unavailable.
- Idempotency cache Redis failures should fall back to PostgreSQL durable idempotency records.
- Payer balances use integer minor units, not floating point.
- Balance mutations must be protected by transactional logic and optimistic concurrency.
- Settlement must be idempotent; a payment may appear in exactly one settlement batch.
- Outbox writes happen in the same PostgreSQL transaction as business state changes.
- Outbox publishing is asynchronous and at-least-once.
- LedgerFlow owns authoritative accounting records. PayCore owns gateway lifecycle state.

## Payment State Machine

Allowed payment transitions:

```text
PENDING -> AUTHORIZED
PENDING -> FAILED
AUTHORIZED -> CAPTURED
AUTHORIZED -> EXPIRED
CAPTURED -> SETTLED
```

Invalid state transitions should return `409 Conflict`.

Authorization creates a hold. Capture converts the hold. Expiry releases the hold.

The minimum capture implementation should require full capture. Partial capture is optional later.

## Endpoint Priorities

Build in this broad order unless the user asks otherwise:

1. API foundation and configuration.
2. Merchant and payer domain models.
3. Merchant and payer APIs.
4. Payment authorization and holds.
5. Payment capture and state machine enforcement.
6. Durable idempotency records.
7. Redis-backed rate limiting.
8. Redis-backed idempotency response caching.
9. PostgreSQL persistence.
10. Transactional outbox.
11. Kafka publishing.
12. Settlement batch processing.
13. Prometheus metrics.
14. Docker Compose local infrastructure.
15. Load testing and performance documentation.

## Coding Practices

- Prefer standard library Go until a dependency clearly pays for itself.
- Keep HTTP handlers thin; place business rules in domain/service packages.
- Keep persistence behind repository interfaces once PostgreSQL is introduced.
- Keep request/response DTOs separate from durable database models when the concepts start to diverge.
- Use explicit status constants for payment, hold, merchant, settlement, idempotency, and outbox states.
- Return structured JSON errors with stable `error_code` values.
- Preserve `request_id` through responses and logs.
- Use `context.Context` through API, service, repository, Redis, and Kafka boundaries.
- Use `time.Time` in UTC for persisted and API timestamps.
- Validate all external request payloads before mutation.
- Store money as `int64` minor units.
- Do not add broad abstractions before there is real duplication or a clear boundary.
- Keep tests close to the behavior being introduced in the current step.

## Package Layout

Use a feature-first package layout with one central HTTP composition root.

Preferred shape:

```text
cmd/paycore-api/main.go
internal/
  http/
    router.go
    middleware.go
  merchant/
    entity.go
    service.go
    repository.go
    handler.go
    adapters/
      memory/
        repository.go
      postgres/
        repository.go
  payer/
    entity.go
    service.go
    repository.go
    handler.go
    adapters/
      memory/
        repository.go
      postgres/
        repository.go
  shared/
    config/
    currency/
    db/
    logger/
```

Rules:

- Feature folders own their handler, service, repository interface, entity/domain behavior, and adapters.
- `internal/http/router.go` wires feature handlers together.
- `internal/http/middleware.go` owns cross-cutting HTTP middleware.
- `cmd/paycore-api/main.go` bootstraps configuration, logger, repositories, services, handlers, and router.
- Do not put feature-specific routers inside feature packages.
- Do not put business rules in middleware.
- Do not create placeholder packages such as `postgres`, `db`, or `logger` before there is real code to place there.

Middleware may contain:

- Request logging.
- Recovery and panic handling.
- Authentication.
- Request ID propagation.
- Rate limiting.
- CORS.
- Body size limits.

Feature handlers should own:

- Parsing request JSON.
- Validating request shape.
- Calling the feature service.
- Mapping feature/domain errors to HTTP responses.

## Testing Practices

- Add or update tests directly when the user asks for tests or when a milestone needs coverage.
- For HTTP foundation, use `httptest`.
- For domain state machines, prefer focused unit tests.
- For repository behavior, use real PostgreSQL integration tests once local infrastructure exists.
- For Redis behavior, test rate-limit and cache semantics with integration tests once Redis exists.
- For Kafka/outbox, test repository claiming separately from producer publishing, then add integration tests with real Kafka.
- Keep test names behavior-focused.
- Every milestone should end with `go test ./...` when Go is available in the user's environment.

## Documentation Style

Documentation should follow the style in `/Users/fan/ledgerflow/docs`.

There should be:

- `docs/architecture-tradeoffs.md` for cross-cutting architecture decisions.
- One Markdown file per major system design or feature.
- `README.md` for current status, run commands, current repo shape, and high-level target architecture.

Feature docs should follow the style of `/Users/fan/ledgerflow/docs/authentication.md`: explain the current implementation as it exists today, write for resume/interview review, and include explicit placeholders for planned sections that are not implemented yet.

Preferred feature-doc sections:

```md
# Feature Name

This document explains the current PayCore <feature> implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, and what is still planned.

## 1. Current Feature Scope

### Implemented

- Concrete implemented items with file paths, endpoint names, status values, tables, and tests where applicable.

### Not Implemented Yet

- Planned but not implemented items. Be explicit when a handler, route, adapter, migration, or external dependency does not exist yet.

### Public Endpoints

List public endpoints, or say none currently.

### Protected Endpoints Or Protected By Default

List protected endpoints and required headers, or say auth is not implemented yet.

## 2. Runtime Flow

### App Startup

Show the command and entrypoint.

```bash
go run ./cmd/paycore-api
```

```text
go run ./cmd/paycore-api
  |
  v
main()
  |
  +--> loads shared config
  +--> creates logger
  +--> wires dependencies
  +--> creates internal/http router
  +--> starts net/http server
```

### Feature Package Boundary

Explain which package owns the feature and how it relates to `internal/http`.

```text
internal/<feature>
  |
  +--> entity.go
  +--> repository.go
  +--> service.go
  +--> handler.go when implemented
  |
  +--> adapters/<adapter>/repository.go
```

## 3. Main Feature Flow

### Request Or Service Input

If HTTP exists, show the request. If HTTP does not exist yet, show the current service input and say no HTTP contract exists yet.

### Step-by-Step

Use numbered steps from handler/service entry to repository/database/output.

### Diagram

Use a plain text flow diagram.

### Failure Path

List domain/service/repository errors and current or planned HTTP mappings.

## 4. Additional Flow Or Planned HTTP Flow

Add secondary flows where relevant, such as refresh, reverse, capture, settlement, replay, authorization checks, or planned handler flow.

## Validation And Errors

List validation rules and stable error codes.

## Persistence

Describe tables, important columns, constraints, and indexes once they exist.

## Tests

List current test coverage and command to run it.

## File Guide

Explain which files own handler/controller, service, repository, model/entity, adapter, migration, and tests.

## Checklist

End with a concise checklist for planned next work.
```

Not every doc needs every section, but keep the structure recognizable. If something is not implemented, include a clearly labeled placeholder instead of pretending it exists.

## Architecture Tradeoff Doc Style

`docs/architecture-tradeoffs.md` should be interview-friendly.

Use this pattern:

```md
# Architecture Tradeoffs

Short paragraph explaining that the document captures cross-cutting decisions.

## Decision Index

| Area | Decision |
| --- | --- |
| Persistence | PostgreSQL as durable source of truth |

## 1. Decision Title

**Decision**

What PayCore chose.

**Alternative**

What else could have been chosen.

**Why PayCore Chose This**

Why this is a good fit for the project and PRD.

**Tradeoff**

Benefits and costs.

**Interview Framing**

> Concise explanation suitable for interviews.
```

Tradeoff topics should eventually include:

- Go HTTP service boundaries.
- PostgreSQL as source of truth.
- Redis for rate limiting and idempotency cache.
- Fail-closed Redis rate limiter behavior.
- Durable idempotency records plus request hashes.
- Optimistic concurrency for payer balances.
- Payment holds instead of immediate charge-only mutation.
- Transactional outbox instead of direct Kafka publish.
- Claim-based outbox publishing.
- Settlement idempotency and crash recovery.
- Prometheus metrics.
- Local-first Docker Compose infrastructure.

## README Rules

Keep `README.md` honest about current repo status.

It should include:

- What PayCore is.
- Current implemented status.
- Implemented endpoints.
- Run commands that actually work today.
- Test commands that actually work today.
- Current repository structure, not future-only structure.
- Target architecture clearly labeled as target/planned.
- Planned implementation sequence.
- Current and planned docs.

Do not claim Docker Compose, PostgreSQL, Redis, Kafka, Prometheus, payment endpoints, settlement, or outbox are implemented before they exist.

## Documentation Update Timing

Update docs at milestones:

- API foundation complete: README, architecture.
- Configuration complete: README, testing or architecture notes if useful.
- Merchant/payer APIs complete: dedicated docs.
- Authorization complete: payment lifecycle and idempotency docs.
- Capture complete: payment lifecycle docs.
- Redis rate limiting complete: rate-limiting doc and architecture tradeoffs.
- Redis idempotency cache complete: idempotency doc and architecture tradeoffs.
- PostgreSQL persistence complete: feature docs and architecture tradeoffs.
- Outbox/Kafka complete: outbox doc and architecture tradeoffs.
- Settlement complete: settlement doc and architecture tradeoffs.
- Metrics/load testing complete: testing and performance docs.

## Commit Practice

Commit after a coherent milestone, not after every tiny edit.

Good early milestone examples:

- API skeleton with tests.
- Configuration loading with tests.
- Merchant/payer in-memory APIs with tests.
- Authorization domain flow with tests.
- PostgreSQL schema and repository tests.

Before milestone commit:

```bash
go test ./...
git status --short
```

Then:

```bash
git add .
git commit -m "Short imperative milestone message"
git push origin main
```

## Current Local Notes

The current API command is:

```bash
go run ./cmd/paycore-api
```

The current implemented endpoints are:

```text
GET /healthz
GET /readyz
GET /version
```

The current service version constant is `0.1.0`.
