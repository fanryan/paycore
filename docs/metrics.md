# Metrics

This document explains the current PayCore Prometheus metrics implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, and what is still planned.

## 1. Current Feature Scope

### Implemented

- Prometheus client dependency in `go.mod`.
- Shared metrics registry in `internal/shared/metrics/metrics.go`.
- Shared metrics HTTP server helper in `internal/shared/metrics/server.go`.
- API metrics endpoint at `GET /metrics`.
- Worker metrics endpoints at `/metrics` on `PAYCORE_METRICS_ADDR`.
- Prometheus service in `docker-compose.yml`.
- Prometheus scrape configuration in `prometheus.yml`.
- HTTP request counter:
  - `paycore_http_requests_total{method,route,status}`
- HTTP request duration histogram:
  - `paycore_http_request_duration_seconds{method,route,status}`
- Settlement metrics:
  - `paycore_settlement_batch_total{status}`
  - `paycore_settlement_batch_duration_seconds{status}`
  - `paycore_settlement_payments_total`
  - `paycore_settlement_recovered_batches_total`
- Outbox metrics:
  - `paycore_outbox_claimed_events_total{publisher}`
  - `paycore_outbox_publish_attempts_total{publisher}`
  - `paycore_outbox_publish_failures_total{publisher}`
  - `paycore_outbox_events_published_total{publisher}`
  - `paycore_outbox_pending_events`
  - `paycore_outbox_publish_lag_seconds`
- Rate-limit metrics:
  - `paycore_rate_limit_allowed_total`
  - `paycore_rate_limit_rejected_total`
  - `paycore_rate_limit_redis_errors_total`
  - `paycore_rate_limit_check_duration_seconds{result}`
- Idempotency cache metrics:
  - `paycore_idempotency_cache_hits_total`
  - `paycore_idempotency_cache_misses_total`
  - `paycore_idempotency_cache_errors_total`
  - `paycore_idempotency_postgres_fallback_total`
- Payment lifecycle metrics:
  - `paycore_authorization_total{result}`
  - `paycore_authorization_latency_seconds{result}`
  - `paycore_capture_total{result}`
  - `paycore_capture_latency_seconds{result}`
- Payer balance conflict metrics:
  - `paycore_payer_version_conflicts_total`
- Go runtime and process collectors:
  - Go runtime metrics
  - process metrics
- Tests for:
  - API `/metrics` route wiring
  - HTTP route-pattern metric labels
  - concrete settlement collector output
  - concrete outbox collector output
  - concrete payment collector output
  - settlement service metrics through a fake recorder
  - payment service metrics through a fake recorder
  - outbox worker metrics through a fake recorder

### Not Implemented Yet

- Grafana dashboards.

### Public Endpoints

The API exposes:

```text
GET /metrics
```

Worker commands expose:

```text
GET /metrics
```

on the address configured by `PAYCORE_METRICS_ADDR`.

Prometheus UI is available locally at:

```text
http://localhost:9090
```

Prometheus targets are available locally at:

```text
http://localhost:9090/targets
```

### Protected Endpoints Or Protected By Default

Metrics endpoints are not authenticated yet. In production, metrics should be exposed only on an internal network, sidecar port, or protected route.

## 2. Runtime Flow

### App Startup

API command:

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
  +--> creates metrics registry
  +--> wires /metrics into internal/http router
  +--> starts API server
```

Outbox worker command:

```bash
PAYCORE_METRICS_ADDR=:9091 go run ./cmd/paycore-outbox-worker
```

Settlement worker command:

```bash
PAYCORE_METRICS_ADDR=:9093 go run ./cmd/paycore-settlement-worker
```

Prometheus command:

```bash
docker compose up -d prometheus
```

```text
worker main()
  |
  +--> loads shared config
  +--> creates metrics registry
  +--> starts metrics server on PAYCORE_METRICS_ADDR
  +--> wires metrics recorder into service/worker
  +--> runs worker logic
```

Current Prometheus scrape targets assume the API and workers run on the host machine:

```text
host.docker.internal:8080
host.docker.internal:9091
host.docker.internal:9093
```

### Feature Package Boundary

```text
internal/shared/metrics
  |
  +--> metrics.go
  +--> metrics_test.go
  +--> server.go

internal/http
  |
  +--> router.go
  +--> middleware.go

internal/settlement
  |
  +--> service.go

internal/outbox
  |
  +--> worker.go
```

`internal/shared/metrics` owns Prometheus-specific collectors and HTTP exposition. Feature packages depend only on small recorder interfaces, so settlement and outbox logic are not tied directly to Prometheus.

## 3. Main Metrics Flow

### HTTP Request Metrics

Request metrics are recorded by `internal/http` middleware.

### Step-by-Step

1. A request enters the chi router.
2. Metrics middleware records the start time.
3. The route handler runs.
4. Metrics middleware reads the response status.
5. Metrics middleware resolves the chi route pattern.
6. Metrics middleware increments request count and observes duration.

### Diagram

```text
HTTP request
  |
  v
chi router
  |
  v
metrics middleware
  |
  +--> next handler
  +--> status code
  +--> chi route pattern
  +--> Prometheus counter/histogram
```

### Failure Path

If a handler returns an error response, the metric still records the final HTTP status code.

If a route is not matched, the route label is `unmatched`.

## 4. Worker Metrics Flow

### Outbox Worker

The outbox worker records metrics after each processed batch:

1. Claim pending outbox events.
2. Publish each event.
3. Mark each event published or failed.
4. Record claimed, attempted, published, and failed counts.
5. Record current publishable pending events and oldest-event lag.

### Settlement Worker

The settlement service records metrics when:

1. A settlement batch completes.
2. A settlement batch fails.
3. Stale settlement batches are recovered.
4. Payments are settled.

## Validation And Errors

Metric labels are intentionally low-cardinality:

- HTTP route labels use chi route patterns, not raw request paths.
- Outbox labels use publisher backend names such as `logging` or `kafka`.
- Settlement labels use stable batch statuses.
- Rate-limit labels use stable result values such as `allowed`, `rejected`, and `redis_error`.
- Payment labels use stable result values such as `success`, `payer_not_found`, `payer_version_conflict`, `insufficient_balance`, `not_capturable`, and `authorization_expired`.

Do not add labels for raw IDs, idempotency keys, payer IDs, merchant IDs, payment IDs, request IDs, or error strings.

## Persistence

Metrics are process-local and scraped by Prometheus. They are not stored in PostgreSQL.

Each PayCore process owns its own metrics registry:

- API process exposes API HTTP metrics.
- Outbox worker process exposes outbox publish metrics.
- Settlement worker process exposes settlement metrics.

## Tests

Run metrics tests:

```bash
go test ./internal/shared/metrics
```

Run related tests:

```bash
go test ./internal/http ./internal/outbox ./internal/settlement
```

Run all tests:

```bash
go test ./...
```

## File Guide

`internal/shared/metrics/metrics.go`

Defines the Prometheus registry, collectors, and recorder methods.

`internal/shared/metrics/server.go`

Starts a small `/metrics` HTTP server for worker commands.

`internal/http/middleware.go`

Records HTTP request metrics after route handling.

`internal/http/router.go`

Wires the API `/metrics` endpoint.

`internal/settlement/service.go`

Records settlement batch, payment, and stale recovery metrics through a recorder interface.

`internal/outbox/worker.go`

Records outbox batch metrics through a recorder interface.

`docker-compose.yml`

Defines the local Prometheus service.

`prometheus.yml`

Defines local scrape targets for host-run PayCore processes.

## Checklist

- [x] Add Prometheus client dependency.
- [x] Add shared metrics registry.
- [x] Expose API `/metrics`.
- [x] Add HTTP request count and duration metrics.
- [x] Add settlement metrics.
- [x] Add outbox metrics.
- [x] Expose worker `/metrics` endpoints.
- [x] Add Prometheus to Docker Compose.
- [x] Add scrape configuration.
- [x] Add Redis rate-limit metrics.
- [x] Add Redis idempotency cache metrics.
- [x] Add outbox lag metrics.
- [ ] Add dashboards or dashboard screenshots.
