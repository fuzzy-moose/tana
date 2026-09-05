package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Run listens until cancellation or a server failure, then drains active requests.
func Run(ctx context.Context, cfg Config, logger *slog.Logger, handler http.Handler) error {
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}
	return serve(ctx, listener, cfg, logger, handler)
}

func serve(ctx context.Context, listener net.Listener, cfg Config, logger *slog.Logger, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()
	logger.Info("server_listening", "addr", listener.Addr().String())

	var err error
	select {
	case <-ctx.Done():
	case err = <-serveErr:
		// Serve can fail while requests are still active; close those too.
		_ = srv.Close()
		return fmt.Errorf("serve HTTP: %w", err)
	}

	logger.Info("shutdown_started")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracePeriod)
	defer cancel()
	if err = srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown_failed", "error", err)
		closeErr := srv.Close()
		<-serveErr
		return errors.Join(fmt.Errorf("shutdown HTTP: %w", err), closeErr)
	}
	if err = <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	logger.Info("shutdown_complete")
	return nil
}
