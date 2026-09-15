package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hashchain/internal/config"
	"hashchain/internal/handler"
	"hashchain/internal/repository/memory"
	"hashchain/internal/router"
	"hashchain/internal/service"
)

func main() {
	cfg := config.Load()
	p := cfg.Params
	log.Printf("consensus params: difficulty=%d (min %d, max %d) retarget_interval=%d target_block_time=%s",
		p.InitialDifficulty, p.MinDifficulty, p.MaxDifficulty, p.RetargetInterval, p.TargetBlockTime)

	// In-memory chain (no database): the point of this project is the data
	// structure and the proof-of-work. Restarting the node starts a new chain.
	repo := memory.NewChainStore()
	svc, err := service.NewChainService(context.Background(), repo, p)
	if err != nil {
		log.Fatalf("start node: %v", err)
	}
	genesis, _ := svc.Latest()
	log.Printf("genesis %s (nonce %d, %d attempts, %s)",
		genesis.Hash, genesis.Nonce, genesis.Attempts, genesis.MinedIn.Round(time.Millisecond))

	h := handler.NewChainHandler(svc, cfg)

	// Every request context derives from baseCtx. Cancelling it at shutdown
	// stops in-flight mining right away instead of waiting out MineTimeout.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router.New(h),
		BaseContext:  func(net.Listener) context.Context { return baseCtx },
		ReadTimeout:  5 * time.Second,
		WriteTimeout: cfg.MineTimeout + 5*time.Second, // mining endpoints respond only when done
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// Block until SIGINT/SIGTERM arrives on the channel.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	if svc.StopAutoMiner() {
		log.Println("auto-miner stopped")
	}
	cancelBase() // abort requests that are still mining

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}
