// Command tana is the local comic management and reader application.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/server"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &level})).With("service", "tana")
	cfg, err := server.LoadConfig(os.Getenv, "TANA", "8080")
	if err != nil {
		logger.Error("startup_failed", "error", err)
		return err
	}
	level.Set(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := local.New(ctx, logger)
	if err != nil {
		logger.Error("startup_failed", "error", err)
		return err
	}
	defer app.Close()
	if err := server.Run(ctx, cfg, logger, app.Handler()); err != nil {
		logger.Error("server_failed", "error", err)
		return err
	}
	return nil
}
