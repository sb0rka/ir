package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	corelog "github.com/sb0rka/sb0rka/packages/core/log"

	adaptermock "github.com/sb0rka/ir/apps/gateway/internal/adapters/mock"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	httptransport "github.com/sb0rka/ir/apps/gateway/internal/transport/http"
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
		return fmt.Errorf("load config: %w", err)
	}
	log, err := corelog.New(cfg.Log, "gateway")
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	providerRegistry, mockStats, err := adaptermock.NewRegistry(adaptermock.Options{
		EventCount:    cfg.Mock.EventCount,
		EndpointCount: cfg.Mock.EndpointCount,
		HistoryDays:   cfg.Mock.HistoryDays,
	})
	if err != nil {
		return fmt.Errorf("build mock provider registry: %w", err)
	}
	gatewayService := service.New(providerRegistry, cfg.Server.RequestTimeout, cfg.Server.SourceTimeout)
	handler := httptransport.NewHandler(cfg, log, gatewayService)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Server.Addr, cfg.Server.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.Server.RequestTimeout + 5*time.Second,
		WriteTimeout:      cfg.Server.RequestTimeout + 5*time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("gateway_started",
			"addr", server.Addr,
			"auth_disabled", cfg.Auth.Disabled,
			"mock_events", mockStats.EventCount,
			"mock_hosts", mockStats.EndpointCount,
			"mock_history_days", cfg.Mock.HistoryDays,
		)
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	select {
	case serveErr := <-errCh:
		return serveErr
	case <-ctx.Done():
		log.Info("gateway_shutting_down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
