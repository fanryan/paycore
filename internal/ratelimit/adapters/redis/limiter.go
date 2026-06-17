package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fanryan/paycore/internal/ratelimit"
	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrClientRequired = errors.New("redis client is required")
	ErrLimitRequired  = errors.New("rate limit must be positive")
	ErrWindowRequired = errors.New("rate limit window must be positive")
)

type Limiter struct {
	client *goredis.Client
	prefix string
	limit  int64
	window time.Duration
}

type Config struct {
	Client *goredis.Client
	Prefix string
	Limit  int64
	Window time.Duration
}

func NewLimiter(config Config) (*Limiter, error) {
	if config.Client == nil {
		return nil, ErrClientRequired
	}

	if config.Limit <= 0 {
		return nil, ErrLimitRequired
	}

	if config.Window <= 0 {
		return nil, ErrWindowRequired
	}

	prefix := strings.TrimSpace(config.Prefix)
	if prefix == "" {
		prefix = "paycore:rate_limit"
	}

	return &Limiter{
		client: config.Client,
		prefix: prefix,
		limit:  config.Limit,
		window: config.Window,
	}, nil
}

func (l *Limiter) Allow(ctx context.Context, key string) (ratelimit.Result, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "anonymous"
	}

	redisKey := fmt.Sprintf("%s:%s", l.prefix, key)

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return ratelimit.Result{}, ratelimit.ErrLimiterUnavailable
	}

	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, l.window).Err(); err != nil {
			return ratelimit.Result{}, ratelimit.ErrLimiterUnavailable
		}
	}

	remaining := l.limit - count
	if remaining < 0 {
		remaining = 0
	}

	result := ratelimit.Result{
		Allowed:   count <= l.limit,
		Limit:     l.limit,
		Remaining: remaining,
	}

	if !result.Allowed {
		ttl, err := l.client.TTL(ctx, redisKey).Result()
		if err != nil || ttl < 0 {
			ttl = l.window
		}

		result.RetryAfter = ttl
	}

	return result, nil
}
