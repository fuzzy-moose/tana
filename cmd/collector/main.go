// Command collector is the cloud metadata collection service.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fuzzy-moose/tana/internal/cleanup"
	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func main() {
	dev := flag.Bool("dev", false, "run in local development mode and load .env")
	flag.Parse()
	if flag.NArg() != 0 {
		slog.Error("unexpected_positional_arguments")
		os.Exit(1)
	}
	if err := run(*dev); err != nil {
		os.Exit(1)
	}
}

func run(development bool) error {
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &level})).With("service", "collector")
	if err := server.LoadEnvironment(development); err != nil {
		logger.Error("startup_failed", "error", err)
		return err
	}
	cfg, err := collector.LoadConfig(os.Getenv)
	if err != nil {
		logger.Error("startup_failed", "error", err)
		return err
	}
	level.Set(cfg.Server.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := collector.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("startup_failed", "error", err)
		return err
	}
	defer cleanup.CloseAndLog(app, logger)
	if err := server.Run(ctx, cfg.Server, logger, httpapi.NewHandler(app)); err != nil {
		logger.Error("server_failed", "error", err)
		return err
	}
	return nil
}
