package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env                     string
	HTTPAddr                string
	MetricsAddr             string
	HTTPReadHeaderTimeout   time.Duration
	HTTPShutdownTimeout     time.Duration
	DatabaseURL             string
	RedisAddr               string
	KafkaBrokers            string
	KafkaOutboxTopic        string
	OutboxPublisher         string
	RateLimitEnabled        bool
	RateLimitRequests       int64
	RateLimitWindow         time.Duration
	IdempotencyCacheEnabled bool
	IdempotencyCacheTTL     time.Duration
	RepositoryBackend       string
}

func Load() Config {
	return Config{
		Env:                     getenv("PAYCORE_ENV", "local"),
		HTTPAddr:                getenv("PAYCORE_HTTP_ADDR", ":8080"),
		MetricsAddr:             getenv("PAYCORE_METRICS_ADDR", ":9091"),
		HTTPReadHeaderTimeout:   durationSeconds("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", 5*time.Second),
		HTTPShutdownTimeout:     durationSeconds("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", 10*time.Second),
		DatabaseURL:             getenv("PAYCORE_DATABASE_URL", ""),
		RedisAddr:               getenv("PAYCORE_REDIS_ADDR", "localhost:6379"),
		KafkaBrokers:            getenv("PAYCORE_KAFKA_BROKERS", "localhost:9092"),
		KafkaOutboxTopic:        getenv("PAYCORE_KAFKA_OUTBOX_TOPIC", "paycore.outbox.events"),
		OutboxPublisher:         getenv("PAYCORE_OUTBOX_PUBLISHER", "logging"),
		RateLimitEnabled:        boolenv("PAYCORE_RATE_LIMIT_ENABLED", false),
		RateLimitRequests:       int64env("PAYCORE_RATE_LIMIT_REQUESTS", 60),
		RateLimitWindow:         durationSeconds("PAYCORE_RATE_LIMIT_WINDOW_SECONDS", time.Minute),
		IdempotencyCacheEnabled: boolenv("PAYCORE_IDEMPOTENCY_CACHE_ENABLED", false),
		IdempotencyCacheTTL:     durationSeconds("PAYCORE_IDEMPOTENCY_CACHE_TTL_SECONDS", 24*time.Hour),
		RepositoryBackend:       getenv("PAYCORE_REPOSITORY_BACKEND", "memory"),
	}
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func durationSeconds(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}

	return time.Duration(seconds) * time.Second
}

func boolenv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func int64env(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}
