package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	relayconfig "nodeweave/services/relay/internal/config"
	relayservice "nodeweave/services/relay/internal/relay"
)

func main() {
	cfg := relayconfig.Load()
	service, err := relayservice.Listen(relayservice.Config{
		ListenAddress: cfg.ListenAddress,
		MappingTTL:    cfg.MappingTTL,
	})
	if err != nil {
		log.Fatalf("start relay: %v", err)
	}
	defer func() {
		_ = service.Close()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("relay listening on %s", service.Address())
	if err := service.Serve(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("relay stopped with error: %v", err)
	}
}
