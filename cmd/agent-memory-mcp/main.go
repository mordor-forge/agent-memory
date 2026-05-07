package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/mcpserver"
	"github.com/mordor-forge/agent-memory/internal/recall"
	"github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("MEMORY_DATABASE_URL is required for MCP server")
	}

	store, err := cockroach.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close()

	embedder, err := embed.NewRuntimeEmbedder(cfg, store)
	if err != nil {
		return err
	}
	recallService, err := recall.NewService(embedder, store)
	if err != nil {
		return err
	}

	srv := mcpserver.NewWithOptions(store, store, store, recallService, mcpserver.Options{
		Version:            version.Current().Version,
		EmbedderProvider:   cfg.EmbeddingProvider,
		EmbedderModel:      embedder.Model(),
		EmbedderDimensions: embedder.Dimensions(),
	})
	return srv.Run(ctx)
}
