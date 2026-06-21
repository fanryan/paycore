# Load Testing

This document explains the current PayCore load testing setup as it exists today. It is written for resume and interview preparation, so it focuses on what the test exercises, how to run it locally, which metrics to inspect, and what is still planned.

## 1. Current Feature Scope

### Implemented

- k6 payment happy-path script in `loadtest/payment_happy_path.js`.
- The script exercises:
  - `POST /merchants`
  - `POST /payers`
  - `POST /payments/authorize`
  - `POST /payments/{payment_id}/capture`
- Payment mutation requests include unique `Idempotency-Key` headers.
- Unique merchant, payer, authorization key, and capture key values are generated per virtual user iteration.
- The script supports configurable base URL, virtual users, and duration through environment variables.

### Not Implemented Yet

- Automated CI load-test stage.
- Docker Compose k6 service.
- Rate-limit pressure scenario.
- Duplicate idempotency replay scenario.
- Payer optimistic-lock contention scenario.
- Settlement/outbox backlog scenario.

### Public Endpoints

The current load test targets public local API endpoints:

```text
POST /merchants
POST /payers
POST /payments/authorize
POST /payments/{payment_id}/capture
```

### Protected Endpoints Or Protected By Default

Payment mutation endpoints require:

```text
Idempotency-Key: <unique-key>
```

Auth is not implemented yet.

## 2. Runtime Flow

### App Startup

Start local infrastructure:

```bash
docker compose up -d
```

Apply migrations:

```bash
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' go run ./cmd/paycore-migrate
```

Start the API with PostgreSQL repositories:

```bash
PAYCORE_REPOSITORY_BACKEND=postgres \
PAYCORE_DATABASE_URL='postgres://paycore:paycore@localhost:5432/paycore?sslmode=disable' \
go run ./cmd/paycore-api
```

Run the happy-path load test:

```bash
k6 run loadtest/payment_happy_path.js
```

Override load shape:

```bash
PAYCORE_LOADTEST_VUS=20 \
PAYCORE_LOADTEST_DURATION=1m \
k6 run loadtest/payment_happy_path.js
```

Override API base URL:

```bash
PAYCORE_BASE_URL=http://localhost:8080 k6 run loadtest/payment_happy_path.js
```

### Feature Package Boundary

```text
loadtest/
  |
  +--> payment_happy_path.js

internal/http
  |
  +--> router.go
  +--> middleware.go

internal/merchant
internal/payer
internal/payment
```

Load tests stay outside production packages. They drive PayCore through the HTTP API so they exercise router, middleware, handlers, services, repositories, idempotency, and metrics together.

## 3. Main Load Test Flow

### Request Flow

Each k6 iteration:

1. Creates a unique merchant.
2. Creates a unique payer with `10000` minor units.
3. Authorizes a `4000` minor-unit payment.
4. Captures the authorized payment.
5. Sleeps briefly before the next iteration.

### Diagram

```text
k6 virtual user
  |
  +--> POST /merchants
  |
  +--> POST /payers
  |
  +--> POST /payments/authorize
  |      +--> Idempotency-Key
  |      +--> payer balance reserve
  |      +--> payment.authorized outbox event
  |
  +--> POST /payments/{payment_id}/capture
         +--> Idempotency-Key
         +--> held balance capture
         +--> payment.captured outbox event
```

### Failure Path

The script checks expected HTTP statuses:

- Merchant create should return `201`.
- Payer create should return `201`.
- Authorization should return `201`.
- Capture should return `200`.

If authorization fails, the iteration skips capture because no valid `payment_id` exists.

## 4. Metrics To Watch

Prometheus is available locally at:

```text
http://localhost:9090
```

Useful API metrics:

```text
paycore_http_requests_total
paycore_http_request_duration_seconds
paycore_authorization_total
paycore_authorization_latency_seconds
paycore_capture_total
paycore_capture_latency_seconds
paycore_payer_version_conflicts_total
paycore_idempotency_cache_hits_total
paycore_idempotency_cache_misses_total
paycore_rate_limit_allowed_total
paycore_rate_limit_rejected_total
```

Useful outbox and settlement metrics when workers are running:

```text
paycore_outbox_pending_events
paycore_outbox_publish_lag_seconds
paycore_outbox_events_published_total
paycore_settlement_batch_total
paycore_settlement_payments_total
```

## 5. Initial Baseline Result

This baseline was captured from the current `payment_happy_path.js` script with:

```text
5 VUs
30 seconds
PostgreSQL repository backend
unique merchant and payer per iteration
1 second sleep at the end of each iteration
```

### Functional Results

All checks passed:

| Check | Result |
| --- | --- |
| Merchant creation returned `201 Created` | 145 / 145 |
| Payer creation returned `201 Created` | 145 / 145 |
| Payment authorization returned `201 Created` | 145 / 145 |
| Authorization returned `payment_id` | 145 / 145 |
| Payment capture returned `200 OK` | 145 / 145 |
| Capture response status was `CAPTURED` | 145 / 145 |
| Total assertions | 870 / 870 |

### HTTP Performance

| Metric | Value |
| --- | --- |
| Total HTTP requests | 580 |
| Request rate | about 19 requests/sec |
| Failed requests | 0 / 580 |
| Average latency | 11.95 ms |
| Median latency | 9.34 ms |
| p90 latency | 21.07 ms |
| p95 latency | 27.06 ms |
| Minimum latency | 0.98 ms |
| Maximum latency | 123.32 ms |

The current p95 was well below the script threshold of `500 ms`.

### Throughput

| Metric | Value |
| --- | --- |
| Completed iterations | 145 |
| Iteration rate | about 4.75 iterations/sec |
| Average iteration duration | 1.05 seconds, including `sleep(1)` |
| Data received | 224 kB, about 7.3 kB/sec |
| Data sent | 151 kB, about 4.9 kB/sec |

This should be treated as an early local baseline, not a production capacity claim. The test currently creates fresh merchants and payers every iteration, so it measures end-to-end happy-path API behavior more than pure payment authorization throughput.

## Validation And Errors

The first happy-path test is intentionally simple:

- It uses unique IDs so duplicate-key errors do not dominate early results.
- It avoids intentionally hitting rate limits.
- It avoids intentionally reusing idempotency keys.
- It treats failed HTTP checks as load-test failures.

Later scenarios should deliberately test duplicate idempotency replay, conflicting idempotency keys, Redis rate-limit rejection, and payer balance contention.

## Persistence

When run against PostgreSQL mode, the test writes durable rows into:

- `merchants`
- `payers`
- `payments`
- `payment_holds`
- `idempotency_records`
- `outbox_events`

The script currently does not clean up data. For repeatable local benchmark runs, reset the local database or use a dedicated database.

## Tests

This load test is not part of `go test ./...`.

Run Go tests before load testing:

```bash
go test ./...
```

Run the load test manually:

```bash
k6 run loadtest/payment_happy_path.js
```

## File Guide

- `loadtest/payment_happy_path.js` owns the first k6 scenario.
- `docs/load-testing.md` explains setup, runtime flow, metrics, and planned scenarios.
- `internal/shared/metrics/metrics.go` owns Prometheus collectors used to inspect the run.
- `prometheus.yml` owns local scrape targets.
- `docker-compose.yml` owns local PostgreSQL, Redis, Kafka, and Prometheus infrastructure.

## Checklist

- [x] Add payment happy-path load test.
- [x] Document local load-test commands.
- [x] Add initial 5 VU / 30 second baseline result.
- [ ] Add duplicate idempotency replay scenario.
- [ ] Add Redis rate-limit pressure scenario.
- [ ] Add payer optimistic-lock contention scenario.
- [ ] Add settlement/outbox backlog scenario.
