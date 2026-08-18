// Package main also hosts the HTTP server lifecycle.
//
// @file      cmd/bot/server.go
// @for       Build + run the /api/v1 HTTP server and drain on shutdown.
// @uses      context, errors, fmt, log/slog, net/http, time, internal/config
// @reason    Keeps main.go under the 250-line limit (§1.1); the serve/stop
// lifecycle is a single cohesive unit split from the composition root.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-17
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
)

// serve builds the HTTP server, runs it, and drains gracefully on ctx
// cancellation (SIGINT/SIGTERM). It returns a non-nil error only on a server
// failure or a failed graceful shutdown.
func serve(ctx context.Context, cfg *config.Config, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.WebhookPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("bot-order stopped cleanly")
		return nil
	}
}
