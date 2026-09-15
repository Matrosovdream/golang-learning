package config

import (
	"os"
	"strconv"
	"time"

	"hashchain/internal/domain"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port         string
	Params       domain.Params // consensus rules: every node on a network must agree on these
	MineTimeout  time.Duration // per-request limit for endpoints that mine
	MaxDataBytes int           // reject block data larger than this
}

// Load reads configuration from env vars, falling back to defaults so the node
// runs under docker compose (or plain `go run`) unchanged.
func Load() Config {
	p := domain.Params{
		InitialDifficulty: uint8(atoi("DIFFICULTY", 20, 0, 40)),
		MinDifficulty:     uint8(atoi("MIN_DIFFICULTY", 8, 0, 40)),
		MaxDifficulty:     uint8(atoi("MAX_DIFFICULTY", 32, 0, 40)),
		RetargetInterval:  uint64(atoi("RETARGET_INTERVAL", 0, 0, 10_000)),
		TargetBlockTime:   time.Duration(atoi("TARGET_BLOCK_TIME_MS", 2000, 1, 3_600_000)) * time.Millisecond,
	}
	// Keep the rules self-consistent instead of failing at startup.
	if p.RetargetInterval == 1 {
		p.RetargetInterval = 2 // a 1-block window would start at genesis; see NextDifficulty
	}
	p.MaxDifficulty = max(p.MaxDifficulty, p.MinDifficulty)
	p.InitialDifficulty = min(max(p.InitialDifficulty, p.MinDifficulty), p.MaxDifficulty)

	return Config{
		Port:         getenv("APP_PORT", "8080"),
		Params:       p,
		MineTimeout:  time.Duration(atoi("MINE_TIMEOUT_S", 60, 1, 3600)) * time.Second,
		MaxDataBytes: atoi("MAX_DATA_BYTES", 1024, 1, 1<<20),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// atoi reads an integer env var, using fallback when it is unset, malformed,
// or outside [lo, hi].
func atoi(key string, fallback, lo, hi int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < lo || n > hi {
		return fallback
	}
	return n
}
