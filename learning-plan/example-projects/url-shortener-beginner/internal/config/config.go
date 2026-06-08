package config

import "os"

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port        string
	DatabaseURL string
	BaseURL     string
}

// Load reads configuration from env vars, falling back to sensible defaults
// so the service runs under docker compose unchanged.
func Load() Config {
	return Config{
		Port:        getenv("APP_PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://shortener:shortener@localhost:5432/shortener?sslmode=disable"),
		BaseURL:     getenv("BASE_URL", "http://localhost:8080"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
