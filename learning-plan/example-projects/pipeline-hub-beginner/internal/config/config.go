package config

import (
	"os"
	"strconv"
)

// Config holds runtime settings, all read from the environment at startup.
type Config struct {
	Port        string
	WorkerCount int // tokenizer fan-out width (bounds concurrency)
	MaxBytes    int // reject input larger than this many bytes
	DefaultTopN int // how many top words to return when the client omits top_n
	MinWordLen  int // default minimum word length to keep
}

// Returns Config by value (not a pointer): it's tiny and copied once at startup.
func Load() Config {
	// A composite literal with named fields; any field left out would be zero.
	return Config{
		Port:        getenv("APP_PORT", "8080"),
		WorkerCount: atoi(getenv("WORKER_COUNT", "4"), 4),
		MaxBytes:    atoi(getenv("MAX_BYTES", "1048576"), 1048576),
		DefaultTopN: atoi(getenv("DEFAULT_TOP_N", "10"), 10),
		MinWordLen:  atoi(getenv("MIN_WORD_LEN", "2"), 2),
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
