package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/ratelimit"
	ratelimitredis "github.com/fanryan/paycore/internal/ratelimit/adapters/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestNewLimiterValidatesConfig(t *testing.T) {
	_, err := ratelimitredis.NewLimiter(ratelimitredis.Config{
		Limit:  1,
		Window: time.Minute,
	})
	if !errors.Is(err, ratelimitredis.ErrClientRequired) {
		t.Fatalf("expected ErrClientRequired, got %v", err)
	}

	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()

	_, err = ratelimitredis.NewLimiter(ratelimitredis.Config{
		Client: client,
		Window: time.Minute,
	})
	if !errors.Is(err, ratelimitredis.ErrLimitRequired) {
		t.Fatalf("expected ErrLimitRequired, got %v", err)
	}

	_, err = ratelimitredis.NewLimiter(ratelimitredis.Config{
		Client: client,
		Limit:  1,
	})
	if !errors.Is(err, ratelimitredis.ErrWindowRequired) {
		t.Fatalf("expected ErrWindowRequired, got %v", err)
	}
}

func TestLimiterAllowsUntilFixedWindowLimit(t *testing.T) {
	redisAddr := os.Getenv("PAYCORE_REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("PAYCORE_REDIS_ADDR is not set")
	}

	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}

	prefix := "paycore:test:rate_limit"
	t.Cleanup(func() {
		_ = client.Del(ctx, prefix+":payer-1").Err()
	})

	limiter, err := ratelimitredis.NewLimiter(ratelimitredis.Config{
		Client: client,
		Prefix: prefix,
		Limit:  2,
		Window: time.Minute,
	})
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}

	first, err := limiter.Allow(ctx, "payer-1")
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	assertAllowed(t, first, true, 1)

	second, err := limiter.Allow(ctx, "payer-1")
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	assertAllowed(t, second, true, 0)

	third, err := limiter.Allow(ctx, "payer-1")
	if err != nil {
		t.Fatalf("third allow: %v", err)
	}
	assertAllowed(t, third, false, 0)

	if third.RetryAfter <= 0 {
		t.Fatalf("expected retry after, got %s", third.RetryAfter)
	}
}

func assertAllowed(t *testing.T, result ratelimit.Result, allowed bool, remaining int64) {
	t.Helper()

	if result.Allowed != allowed {
		t.Fatalf("expected allowed %t, got %t", allowed, result.Allowed)
	}

	if result.Limit != 2 {
		t.Fatalf("expected limit 2, got %d", result.Limit)
	}

	if result.Remaining != remaining {
		t.Fatalf("expected remaining %d, got %d", remaining, result.Remaining)
	}
}
