package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pjy02/Sakura_embyboss/v3/internal/config"
	"github.com/pjy02/Sakura_embyboss/v3/internal/health"
	"github.com/pjy02/Sakura_embyboss/v3/internal/httpapi"
	"github.com/pjy02/Sakura_embyboss/v3/internal/logging"
	"github.com/pjy02/Sakura_embyboss/v3/internal/postgres"
	"github.com/pjy02/Sakura_embyboss/v3/internal/redisstore"
	runservice "github.com/pjy02/Sakura_embyboss/v3/internal/run"
	"github.com/pjy02/Sakura_embyboss/v3/internal/version"
)

func main() {
	if err := execute(); err != nil {
		slog.Error("API stopped with an error", "error", err)
		os.Exit(1)
	}
}

func execute() error {
	cfg, err := config.Load(config.RoleAPI)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "api", cfg.Environment, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()
	cache := redisstore.New(cfg.RedisAddress, cfg.RedisPassword, cfg.RedisDatabase)
	defer cache.Close()

	handler := httpapi.New(httpapi.Options{
		Environment:  cfg.Environment,
		Version:      version.Version,
		Logger:       logger,
		ProbeTimeout: cfg.DependencyTimeout,
		Probes: []health.Probe{
			{Name: "postgres", Check: database.Ping},
			{Name: "redis", Check: cache.Ping},
		},
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("Sakura v3 API starting", "version", version.Version, "commit", version.Commit)
	return runservice.Group(ctx, logger, runservice.HTTPServer(server, cfg.ShutdownTimeout, logger))
}
