package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/comput3ai/comput3/backend/integrations/keeperhub"
	"github.com/comput3ai/comput3/backend/integrations/zerog"
	"github.com/comput3ai/comput3/backend/internal/api"
	"github.com/comput3ai/comput3/backend/internal/auth"
	"github.com/comput3ai/comput3/backend/internal/config"
	"github.com/comput3ai/comput3/backend/internal/container"
	"github.com/comput3ai/comput3/backend/internal/scanner"
	"github.com/comput3ai/comput3/backend/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	if err := db.Migrate(context.Background()); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	mgr, err := container.NewManager(cfg.DockerHost)
	if err != nil {
		log.Fatalf("container manager: %v", err)
	}

	sc := scanner.New(cfg.AnthropicAPIKey, cfg.ScanModel)

	authSvc := auth.New(cfg.JWTSecret)

	zeroGClient, err := zerog.New(cfg.ZeroG_RPC_URL, cfg.ZeroG_PrivateKey, cfg.ZeroG_FlowAddress)
	if err != nil {
		log.Fatalf("zerog: %v", err)
	}

	keeperClient, err := keeperhub.New(cfg.KeeperHub_Endpoint, cfg.KeeperHub_PrivateKey)
	if err != nil {
		log.Fatalf("keeperhub: %v", err)
	}

	srv := api.NewServer(cfg, db, mgr, sc, authSvc, keeperClient, zeroGClient)

	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			authSvc.GCNonces()
		}
	}()

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("[server] listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("[server] shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Printf("[server] shutdown error: %v", err)
	}
	log.Println("[server] stopped")
}
