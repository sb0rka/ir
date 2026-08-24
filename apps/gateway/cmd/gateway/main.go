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
	"strconv"
	"syscall"
	"time"

	corelog "github.com/sb0rka/sb0rka/packages/core/log"

	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/maxpatrol"
	"github.com/sb0rka/ir/apps/gateway/internal/adapters/proxy/ptnad"
	"github.com/sb0rka/ir/apps/gateway/internal/config"
	gatewayproxy "github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
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

	providerRegistry, err := buildRegistry(cfg)
	if err != nil {
		return fmt.Errorf("build provider registry: %w", err)
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
	gatewayService := service.New(providerRegistry, secretResolver, cfg.Server.RequestTimeout, cfg.Server.SourceTimeout)
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

func buildRegistry(cfg config.Config) (*registry.Registry, error) {
	enabled := make(map[string]bool)
	for _, sources := range cfg.ProjectSources {
		for source := range sources {
			enabled[source] = true
		}
	}
	providers := make([]registry.Provider, 0, len(enabled))
	if enabled[config.PTMaxPatrolSIEM] {
		source := cfg.Sources[config.PTMaxPatrolSIEM]
		provider, err := maxpatrol.NewProvider(maxpatrol.ClientConfig{
			SIEM: gatewayproxy.HTTPClientConfig{
				BaseURL: source.BaseURL, Timeout: source.Timeout, TLSCAFile: source.TLSCAFile, SkipTLSVerify: cfg.SkipTLSVerify,
			},
			Incidents: gatewayproxy.HTTPClientConfig{
				BaseURL: source.IncidentsBaseURL, Timeout: source.Timeout, TLSCAFile: source.TLSCAFile, SkipTLSVerify: cfg.SkipTLSVerify,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("configure %s: %w", config.PTMaxPatrolSIEM, err)
		}
		providers = append(providers, provider)
	}
	if enabled[config.PTNAD] {
		source := cfg.Sources[config.PTNAD]
		transport, err := gatewayproxy.NewHTTPClient(gatewayproxy.HTTPClientConfig{
			BaseURL: source.BaseURL, Timeout: source.Timeout, TLSCAFile: source.TLSCAFile, SkipTLSVerify: cfg.SkipTLSVerify,
		})
		if err != nil {
			return nil, fmt.Errorf("configure %s transport: %w", config.PTNAD, err)
		}
		client, err := ptnad.NewClient(ptnad.Config{BaseURL: source.BaseURL, HTTPClient: transport.Client})
		if err != nil {
			return nil, fmt.Errorf("configure %s client: %w", config.PTNAD, err)
		}
		storeIDs := make([]int64, 0, len(source.StoreIDs))
		for _, value := range source.StoreIDs {
			storeID, parseErr := strconv.ParseInt(value, 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("configure %s store %q: %w", config.PTNAD, value, parseErr)
			}
			storeIDs = append(storeIDs, storeID)
		}
		provider, err := ptnad.NewProvider(client, storeIDs)
		if err != nil {
			return nil, fmt.Errorf("configure %s provider: %w", config.PTNAD, err)
		}
		providers = append(providers, provider.RegistryProvider())
	}
	return registry.New(providers...)
}
