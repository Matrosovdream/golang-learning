package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"p2pchain/internal/domain"
	"p2pchain/internal/mempool"
	"p2pchain/internal/p2p"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	Port         string
	DataDir      string
	Params       domain.Params // consensus: identical on every node of the network
	Pool         mempool.Config
	P2P          p2p.Config
	Miner        domain.Address
	Threads      int
	PeerSync     time.Duration
	RebuildState bool
	MineTimeout  time.Duration
}

// Load reads configuration from env vars with defaults.
func Load() (Config, error) {
	// The genesis allocation is derived from DEV_ACCOUNTS so every node in a
	// demo network computes an identical genesis without sharing a file.
	alloc, err := domain.DevAlloc(atoi("DEV_ACCOUNTS", 5, 0, 100), uint64(atoi("DEV_BALANCE", 1_000_000, 0, 1<<40)))
	if err != nil {
		return Config{}, err
	}

	miner := domain.Address{}
	switch v := os.Getenv("MINER_ADDRESS"); {
	case v != "":
		if miner, err = domain.ParseAddress(v); err != nil {
			return Config{}, fmt.Errorf("MINER_ADDRESS: %w", err)
		}
	default:
		// Default: this node's own dev account, derived from its seed index, so
		// each node in a demo earns its own rewards.
		if miner, err = domain.DevAddress(atoi("MINER_DEV_ACCOUNT", 0, 0, 100)); err != nil {
			return Config{}, err
		}
	}

	chainID := uint64(atoi("CHAIN_ID", 1337, 1, 1<<31))
	return Config{
		Port:    getenv("APP_PORT", "8080"),
		DataDir: getenv("DATA_DIR", "./data"),
		Params: domain.Params{
			ChainID:      chainID,
			Difficulty:   uint8(atoi("DIFFICULTY", 25, 0, 40)),
			BlockReward:  uint64(atoi("BLOCK_REWARD", 50, 0, 1<<40)),
			MinFee:       uint64(atoi("MIN_FEE", 1, 0, 1<<40)),
			MaxBlockTxs:  atoi("MAX_BLOCK_TXS", 100, 1, 100_000),
			GenesisAlloc: alloc,
		},
		Pool: mempool.Config{
			MaxSize:     atoi("MEMPOOL_SIZE", 1000, 1, 1_000_000),
			MaxNonceGap: uint64(atoi("MAX_NONCE_GAP", 16, 0, 1_000)),
		},
		P2P: p2p.Config{
			ListenPort: atoi("P2P_PORT", 4001, 0, 65535),
			KeySeed:    getenv("NODE_SEED", getenv("APP_PORT", "8080")),
			Bootstrap:  splitList(os.Getenv("BOOTSTRAP")),
			ChainID:    chainID,
		},
		Miner:        miner,
		Threads:      atoi("MINER_THREADS", 1, 0, 64),
		PeerSync:     time.Duration(atoi("PEER_SYNC_S", 10, 1, 3600)) * time.Second,
		RebuildState: os.Getenv("REBUILD_STATE") == "true",
		MineTimeout:  time.Duration(atoi("MINE_TIMEOUT_S", 120, 1, 3600)) * time.Second,
	}, nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoi(key string, fallback, lo, hi int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < lo || n > hi {
		return fallback
	}
	return n
}
