package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"personalcloud/internal/app"
	"personalcloud/internal/config"
	"personalcloud/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	storage, err := store.Open(cfg.StorePath())
	if err != nil {
		logger.Error("no se pudo abrir el estado persistente", "error", err)
		os.Exit(1)
	}
	defer storage.Close()

	application, err := app.New(cfg, storage, logger)
	if err != nil {
		logger.Error("no se pudo iniciar la aplicación", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor iniciado", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("apagando servidor")
	case err := <-errCh:
		logger.Error("servidor detenido por error", "error", err)
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("apagado incompleto", "error", err)
		os.Exit(1)
	}
}
