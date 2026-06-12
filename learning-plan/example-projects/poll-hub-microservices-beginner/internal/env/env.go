// Package env reads configuration from environment variables.
package env

import "os"

// Get returns the value of key, or fallback when the variable is unset
// or empty. Every service is configured this way — same code locally
// (`go run`, all defaults) and in Compose (everything overridden).
func Get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
