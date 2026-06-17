# Rate Limiting

This document explains the current PayCore rate limiting implementation as it exists today. It is written for resume and interview preparation, so it focuses on how the code works, what decisions were made, and what is still planned.

## 1. Current Feature Scope

### Implemented

- Rate limiter interface in `internal/ratelimit/limiter.go`.
- Redis fixed-window limiter in `internal/ratelimit/adapters/redis/limiter.go`.
- HTTP middleware in `internal/http/middleware.go`.
- Payment route wiring in `internal/http/router.go`.
- Runtime API wiring in `cmd/paycore-api/main.go`.
- Configuration loading in `internal/shared/config/config.go`.
- Local Redis service in `docker-compose.yml`.
- Router tests for allowed, exceeded, and unavailable limiter paths.
- Redis adapter constructor tests.
- Opt-in Redis integration test for fixed-window behavior.

### Not Implemented Yet

- Per-merchant or per-account tiered limits.
- Sliding-window or token-bucket smoothing.
- Redis cluster/sentinel configuration.
- Prometheus metrics for allowed, blocked, and unavailable checks.
- Admin endpoint for inspecting rate-limit state.

### Public Endpoints

None. Rate limiting is internal middleware.

### Protected Endpoints Or Protected By Default

Rate limiting applies only when `PAYCORE_RATE_LIMIT_ENABLED=true`.

Currently protected routes:

```text
POST /payments/authorize
POST /payments/{payment_id}/capture
```

Merchant and payer routes are not rate limited yet.

## 2. Runtime Flow

### App Startup

```bash
PAYCORE_RATE_LIMIT_ENABLED=true \
PAYCORE_REDIS_ADDR=localhost:6379 \
go run ./cmd/paycore-api
```

```text
go run ./cmd/paycore-api
  |
  v
main()
  |
  +--> loads shared config
  +--> creates repositories
  +--> if PAYCORE_RATE_LIMIT_ENABLED=true
  |       +--> creates Redis client
  |       +--> pings Redis
  |       +--> creates Redis rate limiter
  |
  +--> creates internal/http router
  +--> wraps payment routes with rate-limit middleware
  +--> starts net/http server
```

### Feature Package Boundary

```text
internal/ratelimit
  |
  +--> limiter.go
  |
  +--> adapters/redis/limiter.go

internal/http
  |
  +--> middleware.go
  +--> router.go
```

`internal/ratelimit` owns the limiter contract. The Redis adapter owns Redis operations. `internal/http` owns the HTTP middleware and route attachment.

## 3. Main Feature Flow

### Request

```bash
curl -i -X POST http://localhost:8080/payments/authorize \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-key-1' \
  -d '{"merchant_id":"merchant-1","payer_id":"payer-1","amount":4000,"currency":"usd"}'
```

### Step-by-Step

1. Request enters `internal/http/router.go`.
2. Payment mutation route is wrapped by `rateLimitMiddleware`.
3. Middleware builds a client key from `X-Forwarded-For`, `X-Real-IP`, or `RemoteAddr`.
4. Middleware calls `Limiter.Allow(ctx, key)`.
5. Redis limiter increments a fixed-window Redis key.
6. If the key is new, Redis expiry is set to the configured window.
7. If the count is within the limit, the request proceeds to the payment handler.
8. If the count exceeds the limit, middleware returns `429 RATE_LIMIT_EXCEEDED`.
9. If Redis is unavailable or the limiter errors, middleware returns `503 RATE_LIMITER_UNAVAILABLE`.

### Diagram

```text
POST /payments/authorize
  |
  v
rateLimitMiddleware
  |
  +--> Redis INCR paycore:rate_limit:<client-key>
  +--> Redis EXPIRE on first hit
  |
  +--> allowed -> payment handler
  +--> exceeded -> 429 RATE_LIMIT_EXCEEDED
  +--> Redis error -> 503 RATE_LIMITER_UNAVAILABLE
```

### Failure Path

PayCore fails closed when the limiter is unavailable.

```text
Redis unavailable
  |
  v
Limiter.Allow returns ErrLimiterUnavailable
  |
  v
HTTP 503 RATE_LIMITER_UNAVAILABLE
```

This matches the PRD requirement that payment mutation endpoints should not continue without admission control once Redis rate limiting is enabled.

## Validation And Errors

Current validation:

- Redis client is required.
- Rate limit must be positive.
- Rate-limit window must be positive.
- Empty client keys fall back to `anonymous`.

Stable HTTP errors:

```text
RATE_LIMIT_EXCEEDED
RATE_LIMITER_UNAVAILABLE
RATE_LIMIT_ERROR
```

## Persistence

Rate-limit counters are stored in Redis.

Current key shape:

```text
paycore:rate_limit:<client-key>
```

The key expires after `PAYCORE_RATE_LIMIT_WINDOW_SECONDS`.

Redis is not the durable source of truth for payment correctness. It is used here as an admission-control dependency.

## Tests

Current tests cover:

- HTTP route allows requests when limiter allows.
- HTTP route returns `429` when limiter denies.
- HTTP route returns `503` when limiter is unavailable.
- Redis limiter constructor validation.
- Redis fixed-window allow/deny behavior when `PAYCORE_REDIS_ADDR` is set.
- Config defaults and overrides.

Run default tests:

```bash
go test ./...
```

Run Redis integration test:

```bash
docker compose up -d redis
PAYCORE_REDIS_ADDR=localhost:6379 go test ./internal/ratelimit/adapters/redis
```

## File Guide

`internal/ratelimit/limiter.go`

Defines the limiter interface, result shape, no-op limiter, and unavailable error.

`internal/ratelimit/adapters/redis/limiter.go`

Implements Redis-backed fixed-window rate limiting.

`internal/http/middleware.go`

Maps limiter results to HTTP headers and JSON errors.

`internal/http/router.go`

Applies the rate-limit middleware to payment mutation routes.

`cmd/paycore-api/main.go`

Creates the Redis client and limiter when `PAYCORE_RATE_LIMIT_ENABLED=true`.

`internal/shared/config/config.go`

Loads rate-limit configuration from environment variables.

## Checklist

- [x] Add rate limiter interface.
- [x] Add Redis fixed-window limiter.
- [x] Wire limiter into payment mutation routes.
- [x] Fail closed when Redis is unavailable.
- [x] Add router tests.
- [x] Add Redis adapter tests.
- [ ] Add Prometheus metrics.
- [ ] Add per-merchant or per-tier limits.
- [ ] Consider token-bucket or sliding-window algorithm.
