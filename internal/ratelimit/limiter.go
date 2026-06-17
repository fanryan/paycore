package ratelimit

import (
	"context"
	"errors"
	"time"
)

var ErrLimiterUnavailable = errors.New("rate limiter unavailable")

type Result struct {
	Allowed    bool
	Limit      int64
	Remaining  int64
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string) (Result, error)
}

type NoopLimiter struct{}

func (NoopLimiter) Allow(ctx context.Context, key string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	return Result{
		Allowed:   true,
		Limit:     0,
		Remaining: 0,
	}, nil
}
