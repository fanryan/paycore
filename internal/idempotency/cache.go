package idempotency

import (
	"context"
	"errors"
	"time"
)

var ErrCachedResponseNotFound = errors.New("cached idempotency response not found")

type CachedResponse struct {
	Key          string
	RequestHash  string
	ResponseCode int
	ResponseBody []byte
}

type Cache interface {
	GetResponse(ctx context.Context, key string, requestHash string) (CachedResponse, error)
	SetResponse(ctx context.Context, response CachedResponse, ttl time.Duration) error
}

type NoopCache struct{}

func (NoopCache) GetResponse(ctx context.Context, key string, requestHash string) (CachedResponse, error) {
	if err := ctx.Err(); err != nil {
		return CachedResponse{}, err
	}

	return CachedResponse{}, ErrCachedResponseNotFound
}

func (NoopCache) SetResponse(ctx context.Context, response CachedResponse, ttl time.Duration) error {
	return ctx.Err()
}
