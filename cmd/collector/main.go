// Command collector is the cloud metadata collection service.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func main() {
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &level})).With("service", "collector")
	cfg, err := server.LoadConfig(os.Getenv, "TANA_COLLECTOR", "8081")
	if err != nil {
		logger.Error("startup_failed", "error", err)
		os.Exit(1)
	}
	level.Set(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, cfg, logger, httpapi.NewHandler(logger)); err != nil {
		logger.Error("server_failed", "error", err)
		os.Exit(1)
	}
}
