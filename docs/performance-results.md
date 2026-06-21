# Performance Results

This document summarizes the current PayCore performance and load-test results. It is written for resume and interview preparation, so it focuses on what was tested, what the numbers mean, and what they do not mean.

## 1. Current Feature Scope

### Implemented

- k6 happy-path payment load test.
- k6 idempotency replay/conflict load test.
- k6 Redis rate-limit pressure load test.
- k6 payer optimistic-lock contention load test.
- k6 settlement/outbox backlog generation load test.
- Sequential load-test runner in `loadtest/run_all.sh`.
- Prometheus metrics for API, payment lifecycle, rate limiting, idempotency cache, outbox, settlement, and payer version conflicts.
- Initial local baseline results documented in `docs/load-testing.md`.

### Out Of Scope

- CI performance gate.
- Long-duration soak test.
- Grafana dashboards.
- Historical benchmark artifacts.
- Automated database reset before each benchmark run.
- Separate benchmark profiles for memory mode and PostgreSQL mode.

## 2. Runtime Flow

### Load Test Command

Run the full local suite:

```bash
bash loadtest/run_all.sh
```

Run a single scenario:

```bash
k6 run loadtest/payment_happy_path.js
```

### Environment

The documented results are local-development baselines. They should not be treated as production capacity claims.

The current suite assumes:

- API server running locally.
- PostgreSQL migrations applied.
- Redis available when rate limiting is enabled.
- Kafka available if outbox publishing is being tested separately.
- k6 running either locally or through Docker.

## 3. Summary Results

The latest documented suite results:

| Scenario | VUs | Completed Iterations | Throughput | HTTP Error Rate | Checks Succeeded | p95 Latency |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `payment_happy_path.js` | 5 | 25 | 18.86 reqs/sec | 0.00% | 100.00% (150/150) | 38.84 ms |
| `idempotency_replay.js` | 3 | 15 | 14.45 reqs/sec | 20.00% | 100.00% (120/120) | 16.50 ms |
| `rate_limit_pressure.js` | 10 | 454 | 89.30 reqs/sec | 57.23% | 100.00% (908/908) | 20.35 ms |
| `payer_contention.js` | 20 | 1,699 | 335.98 reqs/sec | 60.96% | 100.00% (3400/3400) | 18.85 ms |
| `settlement_outbox_backlog.js` | 10 | 386 | 302.95 reqs/sec | 0.00% | 100.00% (2316/2316) | 26.61 ms |

## 4. Interpretation

### Happy Path

The happy-path scenario verifies that the full local lifecycle can run successfully:

```text
merchant create
payer create
authorize
capture
```

The p95 latency remained below 40 ms in the recorded suite run.

### Idempotency Replay

The idempotency scenario intentionally produces `409 IDEMPOTENCY_KEY_CONFLICT` responses. k6 counts HTTP status codes above 399 as failed requests, so the scenario's `20.00%` HTTP error rate is expected.

The important result is:

```text
100% scenario-specific checks passed
```

This proves that same-key/same-payload replay and same-key/different-payload conflict behavior worked under load.

### Rate Limiting

The rate-limit scenario intentionally drives enough traffic to trigger `429 RATE_LIMIT_EXCEEDED`.

The high HTTP error rate is expected because the limiter is doing its job. The relevant checks are:

- allowed or rejected responses are valid
- rejected responses use the stable rate-limit error code

### Payer Contention

The payer contention scenario intentionally drives concurrent authorizations against one payer. Version conflicts are expected.

The important behavior is that PayCore returns explicit `PAYER_VERSION_CONFLICT` responses instead of silently losing balance updates.

### Settlement And Outbox Backlog

The backlog scenario creates captured payments and outbox events quickly. This is useful for observing:

- outbox pending count
- outbox publish lag
- settlement batch processing later

## 5. Metrics To Pair With Results

Prometheus metrics to inspect during or after a run:

```text
paycore_http_requests_total
paycore_http_request_duration_seconds
paycore_authorization_total
paycore_authorization_latency_seconds
paycore_capture_total
paycore_capture_latency_seconds
paycore_payer_version_conflicts_total
paycore_rate_limit_allowed_total
paycore_rate_limit_rejected_total
paycore_rate_limit_redis_errors_total
paycore_idempotency_cache_hits_total
paycore_idempotency_cache_misses_total
paycore_idempotency_postgres_fallback_total
paycore_outbox_pending_events
paycore_outbox_publish_lag_seconds
paycore_settlement_batch_total
paycore_settlement_payments_total
```

## 6. Limitations

Current results are local baselines, not production capacity numbers.

Important limitations:

- Local machine CPU, disk, and Docker configuration affect results.
- The database is not reset automatically between runs.
- The suite runs short tests, not long soak tests.
- Some scenarios intentionally create high HTTP error rates.
- There are no Grafana dashboards or alert thresholds yet.
- There is no separate memory-mode benchmark profile.

## 7. Future Work

High-value next performance improvements:

- Add a database reset script for repeatable benchmark runs.
- Add a longer soak test.
- Add Prometheus query examples for p95 latency and outbox lag.
- Add Grafana dashboard JSON.
- Store benchmark output artifacts under a dated reports directory.
- Add separate profiles for memory mode, PostgreSQL-only mode, and PostgreSQL + Redis + Kafka mode.

## File Guide

- `loadtest/payment_happy_path.js` owns the happy-path payment benchmark.
- `loadtest/idempotency_replay.js` owns idempotency replay/conflict load behavior.
- `loadtest/rate_limit_pressure.js` owns Redis rate-limit pressure behavior.
- `loadtest/payer_contention.js` owns optimistic-lock contention behavior.
- `loadtest/settlement_outbox_backlog.js` owns captured-payment backlog generation.
- `loadtest/run_all.sh` runs the suite sequentially.
- `docs/load-testing.md` explains how to run the tests.
- `docs/performance-results.md` summarizes the current results and interpretation.

