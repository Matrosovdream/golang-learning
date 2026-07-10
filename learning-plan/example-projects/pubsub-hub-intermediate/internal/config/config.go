package config

import (
	"os"
	"strconv"
)

// Config holds runtime settings, loaded once at startup from the environment.
type Config struct {
	Port         string
	SubBuffer    int // per-subscriber channel capacity (mailbox depth before drops start)
	MaxTopicLen  int // reject a topic name longer than this
	MaxBodyBytes int // reject a message body larger than this many bytes
}

// Returns Config by value (not a pointer): it's tiny and copied once at startup.
func Load() Config {
	// A composite literal with named fields; any field left out would be zero.
	return Config{
		Port:         getenv("APP_PORT", "8080"),
		SubBuffer:    atoi(getenv("SUB_BUFFER", "16"), 16),
		MaxTopicLen:  atoi(getenv("MAX_TOPIC_LEN", "100"), 100),
		MaxBodyBytes: atoi(getenv("MAX_BODY_BYTES", "65536"), 65536),
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
