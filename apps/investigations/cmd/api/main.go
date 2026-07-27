package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/server"
	"github.com/sb0rka/ir/apps/investigations/internal/store/psql"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.Log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := psql.New(ctx, cfg.Database.URI, cfg.Database.MaxOpenConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		return err
	}

	api := server.New(db, log)
	handler := transport.NewHandler(transport.Dependencies{
		Cfg:    cfg.Server,
		Log:    log,
		Server: api,
		Roles:  api,
	})

	// Роль по умолчанию делает валидный токен достаточным для работы, минуя
	// role_bindings. На стенде это удобно, в проде — дыра, поэтому о ней
	// слышно при каждом старте.
	if cfg.Server.DefaultRole != "" {
		log.Warn("default_role_enabled",
			"role", cfg.Server.DefaultRole,
			"note", "любой валидный токен получает эту роль; для прода INV_DEFAULT_ROLE должен быть пуст")
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Server.Addr, cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Заголовки без тела прикрыты ReadHeaderTimeout, но медленное тело
		// и простаивающее keep-alive держали бы соединение бесконечно.
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("api_started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		log.Info("api_shutting_down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func newLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	_ = level.UnmarshalText([]byte(cfg.Level))

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
