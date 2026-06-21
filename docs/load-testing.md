# Load Testing

This document explains the current PayCore load testing setup as it exists today. It is written for resume and interview preparation, so it focuses on what the test exercises, how to run it locally, which metrics to inspect, and what is still planned.

## 1. Current Feature Scope

### Implemented

- k6 payment happy-path script in `loadtest/payment_happy_path.js`.
- k6 idempotency replay script in `loadtest/idempotency_replay.js`.
- k6 Redis rate-limit pressure script in `loadtest/rate_limit_pressure.js`.
- k6 payer optimistic-lock contention script in `loadtest/payer_contention.js`.
- k6 settlement/outbox backlog generator script in `loadtest/settlement_outbox_backlog.js`.
- Shell runner for executing all load-test scenarios in `loadtest/run_all.sh`.
- The happy-path script exercises:
  - `POST /merchants`
  - `POST /payers`
  - `POST /payments/authorize`
  - `POST /payments/{payment_id}/capture`
- The idempotency replay script verifies:
  - same `Idempotency-Key` plus same request body replays the original response
  - same `Idempotency-Key` plus different request body returns `409 IDEMPOTENCY_KEY_CONFLICT`
- The rate-limit pressure script verifies payment mutation requests can be rejected with `429 RATE_LIMIT_EXCEEDED`.
- The payer contention script verifies concurrent balance mutations can return stable `409 PAYER_VERSION_CONFLICT` responses.
- The settlement/outbox backlog script generates captured payments and outbox events for worker backlog observation.
- Payment mutation requests include unique `Idempotency-Key` headers.
- Unique merchant, payer, authorization key, and capture key values are generated per virtual user iteration.
- The script supports configurable base URL, virtual users, and duration through environment variables.

### Not Implemented Yet

- Automated CI load-test stage.
- Docker Compose k6 service.

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

Run the idempotency replay load test:

```bash
k6 run loadtest/idempotency_replay.js
```

Run all load-test scenarios:

```bash
bash loadtest/run_all.sh
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
  +--> idempotency_replay.js
  +--> rate_limit_pressure.js
  +--> payer_contention.js
  +--> settlement_outbox_backlog.js
  +--> run_all.sh

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

## 4. Idempotency Replay Flow

### Request Flow

Each k6 iteration:

1. Creates a unique merchant.
2. Creates a unique payer with `10000` minor units.
3. Authorizes a `4000` minor-unit payment with a unique `Idempotency-Key`.
4. Repeats the same authorization request with the same key and expects the same `payment_id`.
5. Reuses the same key with a different amount and expects `409 IDEMPOTENCY_KEY_CONFLICT`.

### Diagram

```text
k6 virtual user
  |
  +--> POST /merchants
  |
  +--> POST /payers
  |
  +--> POST /payments/authorize
  |      +--> Idempotency-Key: key-1
  |      +--> amount: 4000
  |      +--> 201 Created
  |
  +--> POST /payments/authorize
  |      +--> Idempotency-Key: key-1
  |      +--> amount: 4000
  |      +--> 201 Created replay with same payment_id
  |
  +--> POST /payments/authorize
         +--> Idempotency-Key: key-1
         +--> amount: 5000
         +--> 409 IDEMPOTENCY_KEY_CONFLICT
```

### Failure Path

If the initial authorization fails, the iteration skips replay and conflict checks because there is no valid original response to compare against.

## 5. Metrics To Watch

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

## 6. Initial Baseline Result

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

## 7. Load Test Suite Result

This suite result was captured by running the current load-test scripts as a group.

| Scenario | VUs | Completed Iterations | Throughput | HTTP Error Rate | Checks Succeeded | p95 Latency |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `payment_happy_path.js` | 5 | 25 | 18.86 reqs/sec | 0.00% | 100.00% (150/150) | 38.84 ms |
| `idempotency_replay.js` | 3 | 15 | 14.45 reqs/sec | 20.00% | 100.00% (120/120) | 16.50 ms |
| `rate_limit_pressure.js` | 10 | 454 | 89.30 reqs/sec | 57.23% | 100.00% (908/908) | 20.35 ms |
| `payer_contention.js` | 20 | 1,699 | 335.98 reqs/sec | 60.96% | 100.00% (3400/3400) | 18.85 ms |
| `settlement_outbox_backlog.js` | 10 | 386 | 302.95 reqs/sec | 0.00% | 100.00% (2316/2316) | 26.61 ms |

The higher HTTP error rates in `idempotency_replay.js`, `rate_limit_pressure.js`, and `payer_contention.js` are expected. Those scenarios intentionally produce `409` and `429` responses to verify idempotency conflict handling, Redis rate limiting, and payer optimistic-lock conflict behavior. The important signal is that every scenario-specific check passed.

## Validation And Errors

The first happy-path test is intentionally simple:

- It uses unique IDs so duplicate-key errors do not dominate early results.
- It avoids intentionally hitting rate limits.
- It avoids intentionally reusing idempotency keys.
- It treats failed HTTP checks as load-test failures.

Later scenarios should deliberately test Redis rate-limit rejection and payer balance contention.

Some scenarios intentionally produce HTTP `409` or `429` responses:

- `idempotency_replay.js` expects one `409 IDEMPOTENCY_KEY_CONFLICT` per successful iteration, so its `http_req_failed` threshold allows the expected conflict rate.
- `payer_contention.js` intentionally drives concurrent updates against one payer, so it does not use an `http_req_failed` threshold. The scenario checks that conflicts return the stable `PAYER_VERSION_CONFLICT` error code instead.
- `rate_limit_pressure.js` intentionally allows `429 RATE_LIMIT_EXCEEDED` responses.

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

Run the idempotency replay scenario manually:

```bash
k6 run loadtest/idempotency_replay.js
```

## File Guide

- `loadtest/payment_happy_path.js` owns the first k6 scenario.
- `loadtest/idempotency_replay.js` owns the idempotency replay and conflict scenario.
- `loadtest/rate_limit_pressure.js` owns the Redis rate-limit pressure scenario.
- `loadtest/payer_contention.js` owns the optimistic-lock contention scenario.
- `loadtest/settlement_outbox_backlog.js` owns the captured-payment backlog generation scenario.
- `loadtest/run_all.sh` runs the load-test scripts sequentially through the k6 Docker image.
- `docs/load-testing.md` explains setup, runtime flow, metrics, and planned scenarios.
- `internal/shared/metrics/metrics.go` owns Prometheus collectors used to inspect the run.
- `prometheus.yml` owns local scrape targets.
- `docker-compose.yml` owns local PostgreSQL, Redis, Kafka, and Prometheus infrastructure.

## Checklist

- [x] Add payment happy-path load test.
- [x] Document local load-test commands.
- [x] Add initial 5 VU / 30 second baseline result.
- [x] Add duplicate idempotency replay scenario.
- [x] Add Redis rate-limit pressure scenario.
- [x] Add payer optimistic-lock contention scenario.
- [x] Add settlement/outbox backlog scenario.
