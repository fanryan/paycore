package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/idempotency"
	idempotencyredis "github.com/fanryan/paycore/internal/idempotency/adapters/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestNewCacheValidatesConfig(t *testing.T) {
	_, err := idempotencyredis.NewCache(idempotencyredis.Config{
		TTL: time.Hour,
	})
	if !errors.Is(err, idempotencyredis.ErrClientRequired) {
		t.Fatalf("expected ErrClientRequired, got %v", err)
	}

	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()

	_, err = idempotencyredis.NewCache(idempotencyredis.Config{
		Client: client,
	})
	if !errors.Is(err, idempotencyredis.ErrTTLRequired) {
		t.Fatalf("expected ErrTTLRequired, got %v", err)
	}
}

func TestCacheStoresAndLoadsCompletedResponse(t *testing.T) {
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

	prefix := "paycore:test:idempotency:response"
	t.Cleanup(func() {
		_ = client.Del(ctx, prefix+":idem-key-1:hash-1").Err()
	})

	cache, err := idempotencyredis.NewCache(idempotencyredis.Config{
		Client: client,
		Prefix: prefix,
		TTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	response := idempotency.CachedResponse{
		Key:          "idem-key-1",
		RequestHash:  "hash-1",
		ResponseCode: 201,
		ResponseBody: []byte(`{"payment_id":"payment-1"}`),
	}

	if err := cache.SetResponse(ctx, response, time.Minute); err != nil {
		t.Fatalf("set response: %v", err)
	}

	loaded, err := cache.GetResponse(ctx, "idem-key-1", "hash-1")
	if err != nil {
		t.Fatalf("get response: %v", err)
	}

	if loaded.Key != response.Key {
		t.Fatalf("expected key %q, got %q", response.Key, loaded.Key)
	}

	if loaded.RequestHash != response.RequestHash {
		t.Fatalf("expected request hash %q, got %q", response.RequestHash, loaded.RequestHash)
	}

	if loaded.ResponseCode != response.ResponseCode {
		t.Fatalf("expected response code %d, got %d", response.ResponseCode, loaded.ResponseCode)
	}

	if string(loaded.ResponseBody) != string(response.ResponseBody) {
		t.Fatalf("expected response body %s, got %s", string(response.ResponseBody), string(loaded.ResponseBody))
	}
}

func TestCacheReturnsNotFoundForMissingResponse(t *testing.T) {
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

	cache, err := idempotencyredis.NewCache(idempotencyredis.Config{
		Client: client,
		Prefix: "paycore:test:idempotency:missing",
		TTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}

	_, err = cache.GetResponse(ctx, "missing-key", "missing-hash")
	if !errors.Is(err, idempotency.ErrCachedResponseNotFound) {
		t.Fatalf("expected ErrCachedResponseNotFound, got %v", err)
	}
}
