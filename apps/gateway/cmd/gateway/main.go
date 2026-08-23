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
	"github.com/sb0rka/ir/packages/common"
)

type commonSecretResolver struct {
	client *common.SecretsClient
}

func (resolver commonSecretResolver) Resolve(ctx context.Context, bearer, projectID string, names ...string) (map[string]string, error) {
	snapshot, err := resolver.client.ResolveSnapshot(ctx, bearer, projectID, names...)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(snapshot.Values))
	for name, value := range snapshot.Values {
		values[name] = value.Value
	}
	return values, nil
}

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	providerRegistry, mockStats, err := adaptermock.NewRegistry(adaptermock.Options{
		EventCount:    cfg.Mock.EventCount,
		EndpointCount: cfg.Mock.EndpointCount,
		HistoryDays:   cfg.Mock.HistoryDays,
	})
	if err != nil {
		return fmt.Errorf("build mock provider registry: %w", err)
	}
	var secretResolver service.SecretResolver
	if cfg.Sb0rkaAPIBaseURL != "" {
		client, clientErr := common.NewSecretsClient(common.SecretsConfig{
			BaseURL:          cfg.Sb0rkaAPIBaseURL,
			Timeout:          cfg.Server.SourceTimeout,
			MaxResponseBytes: 1 << 20,
		})
		if clientErr != nil {
			return fmt.Errorf("build Sb0rka Secrets client: %w", clientErr)
		}
		secretResolver = commonSecretResolver{client: client}
	}
	gatewayService := service.New(providerRegistry, secretResolver, cfg.Server.RequestTimeout, cfg.Server.SourceTimeout, cfg.SkipTLSVerify)
	handler := httptransport.NewHandler(cfg, log, gatewayService)
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
