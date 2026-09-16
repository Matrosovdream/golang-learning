package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"coinledger/internal/config"
	"coinledger/internal/domain"
	"coinledger/internal/handler"
	"coinledger/internal/mempool"
	"coinledger/internal/repository/boltstore"
	"coinledger/internal/router"
	"coinledger/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	p := cfg.Params
	log.Printf("chain id %d · difficulty %d · reward %d · min fee %d · %d genesis accounts",
		p.ChainID, p.Difficulty, p.BlockReward, p.MinFee, len(p.GenesisAlloc))
	if cfg.Miner == (domain.Address{}) {
		log.Printf("WARNING: MINER_ADDRESS not set — rewards and fees go to the zero address (burned)")
	}

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	store, err := boltstore.Open(filepath.Join(cfg.DataDir, "chain.db"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close() // releases bbolt's file lock

	node, err := service.NewNode(context.Background(), p, store, mempool.New(cfg.Pool),
		service.Options{Miner: cfg.Miner, RebuildState: cfg.RebuildState})
	if err != nil {
		store.Close()
		log.Fatalf("node: %v", err)
	}
	tip := node.Latest()
	log.Printf("tip: block %d %s", tip.Height, tip.Hash.Hex())

	// Every request context and the block producer derive from baseCtx, so
	// cancelling it at shutdown stops all mining at once.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	var producer sync.WaitGroup
	if cfg.BlockTime > 0 {
		log.Printf("block producer: one block every %s", cfg.BlockTime)
		producer.Go(func() { node.RunBlockProducer(baseCtx, cfg.BlockTime) })
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router.New(handler.NewNodeHandler(node, cfg.MineTimeout)),
		BaseContext:  func(net.Listener) context.Context { return baseCtx },
		ReadTimeout:  5 * time.Second,
		WriteTimeout: cfg.MineTimeout + 5*time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	cancelBase()    // stop mining
	producer.Wait() // the producer has returned: no block is half-committed
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("bye")
}
