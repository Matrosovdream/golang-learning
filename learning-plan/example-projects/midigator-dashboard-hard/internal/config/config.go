package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds runtime settings loaded from the environment. Defaults let the
// service run under docker compose unchanged.
type Config struct {
	Port        string
	DatabaseURL string
	RedisAddr   string

	JWTSecret string
	JWTTTL    time.Duration

	WebhookUsername string
	WebhookPassword string
}

// Load reads configuration from env vars with sensible fallbacks.
func Load() Config {
	ttl := getenvInt("JWT_TTL_MINUTES", 120)
	return Config{
		Port:            getenv("APP_PORT", "8080"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://midigator:midigator@localhost:5432/midigator?sslmode=disable"),
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		JWTSecret:       getenv("JWT_SECRET", "dev-insecure-change-me"),
		JWTTTL:          time.Duration(ttl) * time.Minute,
		WebhookUsername: getenv("MIDIGATOR_WEBHOOK_USERNAME", "midigator"),
		WebhookPassword: getenv("MIDIGATOR_WEBHOOK_PASSWORD", "webhook-secret"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
