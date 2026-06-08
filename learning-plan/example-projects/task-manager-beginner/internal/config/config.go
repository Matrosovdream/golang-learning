package config

import "os"

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from env vars, falling back to sensible defaults
// so the service runs under docker compose unchanged.
func Load() Config {
	return Config{
		Port:        getenv("APP_PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://tasks:tasks@localhost:5432/tasks?sslmode=disable"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
