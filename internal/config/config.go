package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env                   string
	HTTPAddr              string
	HTTPReadHeaderTimeout time.Duration
	HTTPShutdownTimeout   time.Duration
}

func Load() Config {
	return Config{
		Env:                   getenv("PAYCORE_ENV", "local"),
		HTTPAddr:              getenv("PAYCORE_HTTP_ADDR", ":8080"),
		HTTPReadHeaderTimeout: durationSeconds("PAYCORE_HTTP_READ_HEADER_TIMEOUT_SECONDS", 5*time.Second),
		HTTPShutdownTimeout:   durationSeconds("PAYCORE_HTTP_SHUTDOWN_TIMEOUT_SECONDS", 10*time.Second),
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
