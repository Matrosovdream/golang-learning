package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings. time.Duration is really an int64 of
// nanoseconds; multiplying a number by time.Millisecond turns it into one.
type Config struct {
	Port              string
	MaxConcurrency    int           // hard ceiling on simultaneous fetches (the semaphore size)
	DefaultMaxDepth   int           // how deep to follow links when the client omits one
	DefaultMaxPages   int           // page budget per crawl when the client omits one
	DefaultRatePerSec int           // token-bucket refill rate when the client omits one
	SitePages         int           // number of pages in the built-in mini-site
	FetchTimeout      time.Duration // per-page request timeout
}

// Returns Config by value (not a pointer): it's tiny and copied once at startup.
func Load() Config {
	// A composite literal with named fields; any field left out would be zero.
	return Config{
		Port:              getenv("APP_PORT", "8080"),
		MaxConcurrency:    atoi(getenv("MAX_CONCURRENCY", "8"), 8),
		DefaultMaxDepth:   atoi(getenv("DEFAULT_MAX_DEPTH", "3"), 3),
		DefaultMaxPages:   atoi(getenv("DEFAULT_MAX_PAGES", "100"), 100),
		DefaultRatePerSec: atoi(getenv("DEFAULT_RATE_PER_SEC", "50"), 50),
		SitePages:         atoi(getenv("SITE_PAGES", "30"), 30),
		FetchTimeout:      time.Duration(atoi(getenv("FETCH_TIMEOUT_MS", "5000"), 5000)) * time.Millisecond,
	}
}

// getenv returns the env var when set and non-empty, else fallback.
func getenv(key, fallback string) string {
	// if-with-init: v is scoped to this if/else only. os.Getenv returns "" if unset.
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// atoi parses s into a positive int, returning fallback on any failure.
func atoi(s string, fallback int) int {
	// strconv.Atoi returns (int, error); accept it only if err is nil AND n > 0.
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return fallback
}
