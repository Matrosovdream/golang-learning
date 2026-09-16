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

	"p2pchain/internal/chain"
	"p2pchain/internal/config"
	"p2pchain/internal/domain"
	"p2pchain/internal/events"
	"p2pchain/internal/handler"
	"p2pchain/internal/mempool"
	"p2pchain/internal/miner"
	"p2pchain/internal/p2p"
	"p2pchain/internal/repository/boltstore"
	"p2pchain/internal/router"
	"p2pchain/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	p := cfg.Params
	log.Printf("chain id %d · difficulty %d · reward %d · %d genesis accounts · miner %s · %d thread(s)",
		p.ChainID, p.Difficulty, p.BlockReward, len(p.GenesisAlloc), cfg.Miner.Hex(), cfg.Threads)

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	store, err := boltstore.Open(filepath.Join(cfg.DataDir, "chain.db"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewBus()
	pool := mempool.New(cfg.Pool)
	c, err := chain.New(ctx, p, store, pool, bus, chain.Options{RebuildState: cfg.RebuildState})
	if err != nil {
		store.Close()
		log.Fatalf("chain: %v", err)
	}
	tip, meta := c.Tip()
	log.Printf("tip: block %d %s (work %s)", tip.Height, tip.Hash.Hex(), meta.Work)

	node := service.NewNode(ctx, p, c, pool, bus)

	network, err := p2p.New(ctx, cfg.P2P, node, bus)
	if err != nil {
		store.Close()
		log.Fatalf("p2p: %v", err)
	}
	defer network.Close()
	node.AttachNetwork(network)
	log.Printf("p2p listening: %v", network.Addrs())

	m := miner.New(c, pool, bus, cfg.Miner, cfg.Threads, func(b domain.Block) {
		if err := network.BroadcastBlock(ctx, b); err != nil {
			log.Printf("broadcast block: %v", err)
		}
	})
	node.AttachMiner(m)

	var workers sync.WaitGroup
	workers.Go(func() { m.Run(ctx) })
	workers.Go(func() { node.RunPeerSync(ctx, cfg.PeerSync) })

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router.New(handler.NewNodeHandler(node, bus, cfg.MineTimeout)),
		BaseContext:  func(net.Listener) context.Context { return ctx },
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // /events streams for as long as the client wants
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Printf("http listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	cancel()       // stops mining, syncing, and in-flight requests
	workers.Wait() // no block is half-committed when this returns
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("bye")
}
