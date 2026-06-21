# Architecture Tradeoffs

This document captures cross-cutting PayCore architecture decisions. It is written for resume and interview preparation, so each section explains what was chosen, what alternatives existed, why the choice fits PayCore, and how to frame the tradeoff clearly.

## Decision Index

| Area | Decision |
| --- | --- |
| Service Boundary | Go HTTP service with feature-first packages |
| Persistence | PostgreSQL as durable source of truth |
| Transactions | Service-owned transaction orchestration with context propagation |
| Idempotency | Durable idempotency records plus optional Redis response cache |
| Rate Limiting | Redis fixed-window limiter that fails closed for payment mutations |
| Balances | Integer minor units and optimistic concurrency on payer balances |
| Payment Lifecycle | Holds for authorization, capture, expiry, and settlement transitions |
| Events | Transactional outbox instead of direct Kafka publish in request path |
| Publishing | Claim-based asynchronous outbox publishing |
| Settlement | Idempotent settlement batches with stale-batch recovery |
| Observability | Prometheus metrics on API and worker processes |
| Local Infrastructure | Docker Compose for PostgreSQL, Redis, Kafka, and Prometheus |
| Load Testing | k6 scenarios for happy path, idempotency, rate limiting, contention, and backlog |

## 1. Go HTTP Service With Feature-First Packages

**Decision**

PayCore is implemented as a Go HTTP service with feature-owned packages such as `internal/merchant`, `internal/payer`, `internal/payment`, `internal/outbox`, and `internal/settlement`. The central router remains in `internal/http`.

**Alternative**

A layered package layout such as `domain`, `usecase`, `repository`, and `transport` could have been used globally.

**Why PayCore Chose This**

Feature-first packages keep entity, service, repository interface, handler, and adapters close together. This makes payment, settlement, and outbox behavior easier to evolve without jumping across generic layers.

**Tradeoff**

Feature-first layout improves ownership and readability, but shared patterns must be kept disciplined so packages do not drift into different styles.

**Interview Framing**

> I kept the router and middleware centralized, but let each feature own its service, handler, repository contract, and adapters. That gives clear feature boundaries without losing one HTTP composition root.

## 2. PostgreSQL As Durable Source Of Truth

**Decision**

PostgreSQL owns durable merchant, payer, payment, hold, idempotency, outbox, and settlement state.

**Alternative**

Redis or Kafka could have been used as primary state stores for some workflows.

**Why PayCore Chose This**

Payment lifecycle state and balances need transactional correctness, constraints, and recovery semantics. PostgreSQL gives durable commits, relational constraints, row locks, and indexes.

**Tradeoff**

PostgreSQL is simpler and safer for correctness, but it can become the throughput bottleneck if the system grows. PayCore mitigates this with focused indexes, short transactions, and asynchronous Kafka publishing.

**Interview Framing**

> Redis and Kafka help the system move faster, but PostgreSQL is the source of truth because balances, holds, idempotency records, and settlements need durable transactional semantics.

## 3. Service-Owned Transactions With Context Propagation

**Decision**

PayCore uses a `Transactor` abstraction in `internal/shared/db`. Services own transaction orchestration, and Postgres repositories read the active transaction from `context.Context`.

**Alternative**

Repositories could expose large atomic methods such as `AuthorizePaymentAtomic`.

**Why PayCore Chose This**

Authorization, capture, expiry, settlement, and outbox writes span multiple repositories. Keeping orchestration in services preserves business flow visibility while repositories stay focused on persistence operations.

**Tradeoff**

Context-propagated transactions require repository discipline and tests, but they avoid bloated repository interfaces and keep clean architecture boundaries intact.

**Interview Framing**

> The service owns the business transaction, repositories own SQL. Context propagation lets multiple repositories participate in one Postgres transaction without moving business rules into the persistence layer.

## 4. Durable Idempotency Records Plus Redis Cache

**Decision**

Payment mutation endpoints require `Idempotency-Key`. Durable idempotency records live in PostgreSQL, while Redis can cache completed responses for faster replay.

**Alternative**

Only Redis could have been used for idempotency.

**Why PayCore Chose This**

Correctness must survive Redis restarts and cache evictions. PostgreSQL records preserve request hashes, status, response code, response body, and expiry. Redis is an optimization.

**Tradeoff**

Durable idempotency adds a database write to mutation paths, but it gives reliable replay and conflict detection.

**Interview Framing**

> Redis accelerates idempotency replay, but PostgreSQL decides correctness. If Redis is unavailable, PayCore falls back to durable records.

## 5. Redis Rate Limiter Fails Closed

**Decision**

Payment mutation routes can use a Redis fixed-window rate limiter. If Redis is unavailable while rate limiting is enabled, PayCore rejects the request.

**Alternative**

The API could fail open and allow requests when Redis is unavailable.

**Why PayCore Chose This**

Payment mutation endpoints affect balances and durable state. During limiter uncertainty, rejecting requests is safer than allowing uncontrolled traffic.

**Tradeoff**

Failing closed protects the system but can reduce availability if Redis is down.

**Interview Framing**

> For payment mutations, I chose fail-closed rate limiting because overload protection is part of correctness. A temporary `503` is safer than uncontrolled balance mutation traffic.

## 6. Integer Minor Units And Optimistic Balance Concurrency

**Decision**

Money is represented as integer minor units. Payer balance updates use optimistic concurrency through a version column.

**Alternative**

Floating point values or implicit last-write-wins updates could have been used.

**Why PayCore Chose This**

Integer minor units avoid rounding errors. Optimistic concurrency detects concurrent updates to the same payer balance and returns stable `PAYER_VERSION_CONFLICT` errors.

**Tradeoff**

Optimistic concurrency can produce retryable conflicts under heavy contention, but this is visible through metrics and load-test scenarios.

**Interview Framing**

> I store money as integer minor units and use payer versions to prevent lost updates. Under contention the system returns explicit conflicts instead of silently corrupting balances.

## 7. Holds Before Capture

**Decision**

Authorization creates a hold and reserves payer funds. Capture converts held funds. Expiry releases held funds.

**Alternative**

Authorization could immediately debit funds without a separate hold model.

**Why PayCore Chose This**

The hold model matches real payment gateway lifecycle behavior and makes state transitions explicit: `AUTHORIZED`, `CAPTURED`, `EXPIRED`, and `SETTLED`.

**Tradeoff**

Holds add another table and more transaction steps, but they make expiry and capture semantics clear.

**Interview Framing**

> I modeled authorization as a hold because capture and expiry are different outcomes. That makes balance movement auditable and keeps the state machine honest.

## 8. Transactional Outbox Instead Of Direct Kafka Publish

**Decision**

Payment and settlement services write outbox events in the same PostgreSQL transaction as business state changes. A separate worker publishes events to Kafka.

**Alternative**

The API could publish directly to Kafka before or after committing the database transaction.

**Why PayCore Chose This**

Direct publishing risks split-brain behavior: a Kafka event without committed state, or committed state without an event. The transactional outbox preserves atomic state-plus-event intent.

**Tradeoff**

Outbox publishing is eventually consistent and at-least-once, so consumers must tolerate duplicates.

**Interview Framing**

> The outbox pattern keeps database state and event intent atomic. Kafka publishing happens after commit, so consumers see lifecycle events without the request path depending on Kafka availability.

## 9. Claim-Based Outbox Publishing

**Decision**

Outbox workers claim pending events with database locking and publish asynchronously.

**Alternative**

A single worker could scan and publish without explicit claiming.

**Why PayCore Chose This**

Claiming enables multiple workers and crash recovery. Events can be retried if publishing fails.

**Tradeoff**

Claiming adds state and retry logic, but provides operational control and backlog visibility.

**Interview Framing**

> Claim-based publishing lets workers scale horizontally and recover after crashes while keeping publishing at-least-once.

## 10. Idempotent Settlement With Recovery

**Decision**

Settlement processing creates batches, claims captured payments, writes line items, marks payments `SETTLED`, emits outbox events, and can recover stale `PROCESSING` batches.

**Alternative**

Settlement could be a simple report over captured payments without durable batch state.

**Why PayCore Chose This**

Durable batch state prevents double settlement and makes crash recovery explicit.

**Tradeoff**

Settlement state adds complexity, but it supports replay, auditability, and operational safety.

**Interview Framing**

> Settlement is idempotent because every payment can belong to at most one batch. If a worker crashes mid-batch, recovery resumes stale processing instead of creating duplicates.

## 11. Prometheus Metrics

**Decision**

PayCore exposes Prometheus metrics from the API and worker commands.

**Alternative**

The project could rely only on logs or ad hoc command output.

**Why PayCore Chose This**

Metrics make behavior visible under load: request latency, authorization and capture results, idempotency cache behavior, rate limiting, outbox lag, settlement counts, and payer version conflicts.

**Tradeoff**

Metrics need label discipline. PayCore uses stable low-cardinality labels and avoids raw IDs.

**Interview Framing**

> Metrics are designed around operational questions: is the API slow, are idempotency replays working, are rate limits firing, is the outbox backing up, and are payer conflicts increasing?

## 12. Local-First Docker Compose Infrastructure

**Decision**

Docker Compose provides local PostgreSQL, Redis, Kafka, and Prometheus.

**Alternative**

Managed cloud services could have been used from the start.

**Why PayCore Chose This**

The project is intended to be reproducible locally for development, demos, and interviews.

**Tradeoff**

Local infrastructure is not production infrastructure, but it proves integration behavior and lowers setup friction.

**Interview Framing**

> I used Docker Compose to make the full system runnable locally: durable state, cache, event broker, and metrics all work without external accounts.

## 13. k6 Load Testing

**Decision**

PayCore uses k6 scripts for happy-path payments, idempotency replay/conflict, rate-limit pressure, payer contention, and settlement/outbox backlog generation.

**Alternative**

Only unit and integration tests could have been used.

**Why PayCore Chose This**

Load tests validate behavior under concurrency and produce measurable latency, throughput, and error-path results.

**Tradeoff**

Load tests are environment-sensitive and should be treated as local baselines, not universal capacity claims.

**Interview Framing**

> The load tests are not just throughput checks. They deliberately exercise payment guarantees: idempotent replay, conflict detection, rate limiting, balance contention, and backlog behavior.
