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
	"personalcloud/internal/privilege"
	"personalcloud/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if relaunched, err := privilege.Ensure(); err != nil {
		logger.Warn("no se pudo solicitar elevación automática; algunas operaciones de volumen pueden fallar", "error", err)
	} else if relaunched {
		return
	}
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
		// Los cuerpos pueden ser archivos grandes; ReadHeaderTimeout protege la fase previa
		// sin imponer un límite global que corte transferencias legítimas.
		IdleTimeout: 2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		tlsEnabled := cfg.TLSCertFile != ""
		logger.Info("servidor iniciado", "addr", cfg.Addr, "tls", tlsEnabled)
		var serveErr error
		if tlsEnabled {
			serveErr = server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			serveErr = server.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
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
