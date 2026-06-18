package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port           string
	MaxConcurrent  int           // process-wide cap on simultaneous checks
	RatePerSec     int           // token-bucket refill rate
	RateBurst      int           // token-bucket burst size
	MaxRetries     int           // attempts per check before giving up
	HistorySize    int           // results kept per monitor
	MaxMonitors    int           // registration cap
	DefaultTimeout time.Duration // per-attempt request timeout default
}

func Load() Config {
	return Config{
		Port:           getenv("APP_PORT", "8080"),
		MaxConcurrent:  atoi("MAX_CONCURRENT_CHECKS", 8),
		RatePerSec:     atoi("RATE_PER_SEC", 20),
		RateBurst:      atoi("RATE_BURST", 10),
		MaxRetries:     atoi("MAX_RETRIES", 2),
		HistorySize:    atoi("HISTORY_SIZE", 20),
		MaxMonitors:    atoi("MAX_MONITORS", 100),
		DefaultTimeout: time.Duration(atoi("DEFAULT_TIMEOUT_MS", 3000)) * time.Millisecond,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoi(key string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}
