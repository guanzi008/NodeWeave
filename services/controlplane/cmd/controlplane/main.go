package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nodeweave/services/controlplane/internal/config"
	"nodeweave/services/controlplane/internal/httpapi"
	"nodeweave/services/controlplane/internal/store"
)

func main() {
	cfg := config.Load()
	dataStore, err := store.Open(cfg)
	if err != nil {
		log.Fatalf("open store failed: %v", err)
	}
	defer func() {
		if closeErr := dataStore.Close(); closeErr != nil {
			log.Printf("store close failed: %v", closeErr)
		}
	}()

	handler := httpapi.New(cfg, dataStore)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("control plane listening on %s", cfg.Address)
		log.Printf("storage driver %s", cfg.StorageDriver)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
