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

	corelog "github.com/sb0rka/sb0rka/packages/core/log"

	"github.com/sb0rka/ir/apps/investigations/internal/authclient"
	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/gatewayclient"
	"github.com/sb0rka/ir/apps/investigations/internal/server"
	"github.com/sb0rka/ir/apps/investigations/internal/somclient"
	"github.com/sb0rka/ir/apps/investigations/internal/store/psql"
	"github.com/sb0rka/ir/apps/investigations/internal/transport"
	"github.com/sb0rka/ir/packages/common"
)

const serviceName = "ir-api"

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
	log, err := corelog.New(cfg.Log, serviceName)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := psql.New(ctx, cfg.Database.URI, cfg.Database.MaxConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		return err
	}

	var secrets *common.SecretsClient
	if cfg.Platform.APIBaseURL != "" {
		secrets, err = common.NewSecretsClient(common.SecretsConfig{BaseURL: cfg.Platform.APIBaseURL})
		if err != nil {
			return err
		}
	}

	api := server.New(db, log,
		somclient.New(somclient.Config{
			APIBaseURL:     cfg.SOM.APIBaseURL,
			RelayBaseURL:   cfg.SOM.RelayBaseURL,
			HostID:         cfg.SOM.HostID,
			RepoID:         cfg.SOM.RepoID,
			RepoParentPath: cfg.SOM.RepoParentPath,
			RepoFolderName: cfg.SOM.RepoFolderName,
			TargetBranch:   cfg.SOM.TargetBranch,
			Executor:       cfg.SOM.Executor,
		}),
		secrets,
		gatewayclient.New(gatewayclient.Config{BaseURL: cfg.Gateway.BaseURL}),
		authclient.New(authclient.Config{BaseURL: cfg.Platform.AuthBaseURL}),
		cfg.Prompt,
	)
	handler := transport.NewHandler(transport.Dependencies{
		Cfg:    cfg.Server,
		Log:    log,
		Server: api,
	})

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
