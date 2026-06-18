package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port           string
	WorkerCount    int           // size of the health-check worker pool (bounds concurrency)
	DefaultTimeout time.Duration // per-URL request timeout when the client omits one
	MaxURLs        int           // reject a batch larger than this
}

// Load reads configuration from env vars, falling back to sensible defaults
// so the service runs under docker compose unchanged.
func Load() Config {
	return Config{
		Port:           getenv("APP_PORT", "8080"),
		WorkerCount:    atoi(getenv("WORKER_COUNT", "5"), 5),
		DefaultTimeout: time.Duration(atoi(getenv("DEFAULT_TIMEOUT_MS", "3000"), 3000)) * time.Millisecond,
		MaxURLs:        atoi(getenv("MAX_URLS", "50"), 50),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoi(s string, fallback int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return fallback
}
