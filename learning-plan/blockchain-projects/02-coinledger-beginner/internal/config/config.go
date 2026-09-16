package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"coinledger/internal/domain"
	"coinledger/internal/mempool"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port         string
	DataDir      string
	Params       domain.Params  // consensus: identical on every node of a network
	Pool         mempool.Config // policy: this node's choice
	Miner        domain.Address // policy: where this node's rewards go
	BlockTime    time.Duration  // 0 = mine only on POST /mine
	RebuildState bool
	MineTimeout  time.Duration
}

// Load reads configuration from env vars with defaults. It fails on values
// that would silently create a different chain (a bad address, a bad allocation).
func Load() (Config, error) {
	alloc, err := parseAlloc(os.Getenv("GENESIS_ALLOC"))
	if err != nil {
		return Config{}, err
	}
	miner := domain.Address{}
	if v := os.Getenv("MINER_ADDRESS"); v != "" {
		if miner, err = domain.ParseAddress(v); err != nil {
			return Config{}, fmt.Errorf("MINER_ADDRESS: %w", err)
		}
	}

	return Config{
		Port:    getenv("APP_PORT", "8080"),
		DataDir: getenv("DATA_DIR", "./data"),
		Params: domain.Params{
			ChainID:      uint64(atoi("CHAIN_ID", 1337, 1, 1<<31)),
			Difficulty:   uint8(atoi("DIFFICULTY", 18, 0, 40)),
			BlockReward:  uint64(atoi("BLOCK_REWARD", 50, 0, 1<<40)),
			MinFee:       uint64(atoi("MIN_FEE", 1, 0, 1<<40)),
			MaxBlockTxs:  atoi("MAX_BLOCK_TXS", 100, 1, 100_000),
			GenesisAlloc: alloc,
		},
		Pool: mempool.Config{
			MaxSize:     atoi("MEMPOOL_SIZE", 1000, 1, 1_000_000),
			MaxNonceGap: uint64(atoi("MAX_NONCE_GAP", 16, 0, 1_000)),
		},
		Miner:        miner,
		BlockTime:    time.Duration(atoi("BLOCK_TIME_MS", 0, 0, 3_600_000)) * time.Millisecond,
		RebuildState: os.Getenv("REBUILD_STATE") == "true",
		MineTimeout:  time.Duration(atoi("MINE_TIMEOUT_S", 60, 1, 3600)) * time.Second,
	}, nil
}

// parseAlloc reads "0xADDR=amount,0xADDR=amount".
func parseAlloc(s string) (map[domain.Address]uint64, error) {
	alloc := map[domain.Address]uint64{}
	if strings.TrimSpace(s) == "" {
		return alloc, nil
	}
	for _, pair := range strings.Split(s, ",") {
		addrStr, amountStr, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			return nil, fmt.Errorf("GENESIS_ALLOC: %q is not ADDRESS=AMOUNT", pair)
		}
		addr, err := domain.ParseAddress(addrStr)
		if err != nil {
			return nil, fmt.Errorf("GENESIS_ALLOC: %w", err)
		}
		amount, err := strconv.ParseUint(amountStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("GENESIS_ALLOC: amount %q: %w", amountStr, err)
		}
		alloc[addr] += amount
	}
	return alloc, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// atoi reads an integer env var, using fallback when unset, malformed, or outside [lo, hi].
func atoi(key string, fallback, lo, hi int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < lo || n > hi {
		return fallback
	}
	return n
}
