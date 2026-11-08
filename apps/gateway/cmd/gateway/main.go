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
	corestore "github.com/sb0rka/sb0rka/packages/core/store"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
	"github.com/sb0rka/ir/apps/gateway/internal/service"
	"github.com/sb0rka/ir/apps/gateway/internal/sources"
	httptransport "github.com/sb0rka/ir/apps/gateway/internal/transport/http"
	"github.com/sb0rka/ir/packages/common"
	"github.com/sb0rka/ir/packages/common/agentaccess"
)

type commonSecretResolver struct {
	client  *common.ConfigurationClient
	catalog *sources.Catalog
}

func (resolver commonSecretResolver) Resolve(
	ctx context.Context,
	bearer, projectID string,
	names ...string,
) (map[string]capability.Access, error) {
	snapshot, err := resolver.client.Snapshot(ctx, bearer, projectID)
	if err != nil {
		return nil, err
	}
	return resolver.catalog.ResolveCredentials(snapshot.Values, names...)
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	catalog, err := sources.Default()
	if err != nil {
		return fmt.Errorf("source catalog: %w", err)
	}
	cfg, err := config.Load(catalog)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log, err := corelog.New(cfg.Log, "gateway")
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	providerRegistry, err := buildRegistry(catalog, cfg)
	if err != nil {
		return fmt.Errorf("build provider registry: %w", err)
	}
	configurationClient, err := common.NewConfigurationClient(cfg.Sb0rkaAPIBaseURL)
	if err != nil {
		return fmt.Errorf("configuration client: %w", err)
	}
	secretResolver := commonSecretResolver{client: configurationClient, catalog: catalog}

	gatewayService := service.New(providerRegistry, secretResolver, cfg.Server.RequestTimeout, cfg.Server.SourceTimeout, service.WithEventPagination(cfg.CursorSigningKey, sources.ConfigurationFingerprint(cfg.Sources, sources.BuildOptions{SkipTLSVerify: cfg.SkipTLSVerify})))
	if err := gatewayService.ValidateEventPagination(); err != nil {
		return err
	}
	var options []httptransport.HandlerOption
	if cfg.Database.URI != "" {
		pool, err := corestore.NewPool(cfg.Database.URI, cfg.Database.MaxConns, cfg.Database.ConnMaxLifetime)
		if err != nil {
			return fmt.Errorf("configure agent access database: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("connect agent access database: %w", err)
		}
		cipher, err := agentaccess.NewCipher(cfg.GatewayConfigurationKey)
		if err != nil {
			return fmt.Errorf("configure agent source encryption: %w", err)
		}
		options = append(options, httptransport.WithAgentAccess(agentaccess.NewRepository(pool.Pgx()), cipher))
	}
	handler := httptransport.NewHandler(cfg, log, gatewayService, options...)
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
			"auth_audience", "sb0rka-ir-gateway",
			"registered_sources", len(providerRegistry.Sources()),
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

func buildRegistry(catalog *sources.Catalog, cfg config.Config) (*registry.Registry, error) {
	return catalog.Build(cfg.Sources, sources.BuildOptions{SkipTLSVerify: cfg.SkipTLSVerify})
}
