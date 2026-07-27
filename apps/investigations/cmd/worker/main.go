package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/store/psql"
	"github.com/sb0rka/ir/apps/investigations/internal/worker"
)

// worker — второй entrypoint того же кода: фоновые задачи (затяжка датасета,
// прогон правил связывания, сборка отчётов). Масштабируется отдельно от api.
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

	opts := &slog.HandlerOptions{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, opts))

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

	loop := worker.New(worker.Options{
		ID:           workerID(cfg.Worker.ID),
		Kinds:        cfg.Worker.Kinds,
		PollInterval: cfg.Worker.PollInterval,
		Log:          log,
	})

	log.Info("worker_started", "kinds", cfg.Worker.Kinds)
	return loop.Run(ctx)
}

func workerID(configured string) string {
	if configured != "" {
		return configured
	}
	host, err := os.Hostname()
	if err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return host
}
