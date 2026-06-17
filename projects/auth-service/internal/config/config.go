package config

import (
	"os"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	TokenTTL    time.Duration
}

func Load() Config {

	return Config{
		Port:        getenv("APP_PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://auth:auth@localhost:5432/auth?sslmode=disable"),
		JWTSecret:   getenv("JWT_SECRET", "dev-secret-change-me"),
		TokenTTL:    getDuration("TOKEN_TTL", time.Hour),
	}

}

func getenv(key, fallback string) string {

	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback

}

func getDuration(key string, fallback time.Duration) time.Duration {

	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback

}
