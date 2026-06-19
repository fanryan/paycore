package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrClientRequired = errors.New("redis client is required")
	ErrTTLRequired    = errors.New("idempotency cache ttl must be positive")
)

type Cache struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

type Config struct {
	Client *goredis.Client
	Prefix string
	TTL    time.Duration
}

type cachedResponseDTO struct {
	Key          string `json:"key"`
	RequestHash  string `json:"request_hash"`
	ResponseCode int    `json:"response_code"`
	ResponseBody []byte `json:"response_body"`
}

func NewCache(config Config) (*Cache, error) {
	if config.Client == nil {
		return nil, ErrClientRequired
	}

	if config.TTL <= 0 {
		return nil, ErrTTLRequired
	}

	prefix := strings.TrimSpace(config.Prefix)
	if prefix == "" {
		prefix = "paycore:idempotency:response"
	}

	return &Cache{
		client: config.Client,
		prefix: prefix,
		ttl:    config.TTL,
	}, nil
}

func (c *Cache) GetResponse(ctx context.Context, key string, requestHash string) (idempotency.CachedResponse, error) {
	value, err := c.client.Get(ctx, c.cacheKey(key, requestHash)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return idempotency.CachedResponse{}, idempotency.ErrCachedResponseNotFound
	}
	if err != nil {
		return idempotency.CachedResponse{}, err
	}

	var dto cachedResponseDTO
	if err := json.Unmarshal(value, &dto); err != nil {
		return idempotency.CachedResponse{}, err
	}

	return idempotency.CachedResponse{
		Key:          dto.Key,
		RequestHash:  dto.RequestHash,
		ResponseCode: dto.ResponseCode,
		ResponseBody: append([]byte(nil), dto.ResponseBody...),
	}, nil
}

func (c *Cache) SetResponse(ctx context.Context, response idempotency.CachedResponse, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = c.ttl
	}

	dto := cachedResponseDTO{
		Key:          response.Key,
		RequestHash:  response.RequestHash,
		ResponseCode: response.ResponseCode,
		ResponseBody: append([]byte(nil), response.ResponseBody...),
	}

	value, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, c.cacheKey(response.Key, response.RequestHash), value, ttl).Err()
}

func (c *Cache) cacheKey(key string, requestHash string) string {
	return fmt.Sprintf("%s:%s:%s", c.prefix, strings.TrimSpace(key), strings.TrimSpace(requestHash))
}
