package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pjy02/Sakura_embyboss/v3/internal/config"
	"github.com/pjy02/Sakura_embyboss/v3/internal/logging"
	"github.com/pjy02/Sakura_embyboss/v3/internal/migrate"
	"github.com/pjy02/Sakura_embyboss/v3/internal/version"
)

func main() {
	if err := execute(); err != nil {
		slog.Error("Migration failed", "error", err)
		os.Exit(1)
	}
}

func execute() error {
	cfg, err := config.Load(config.RoleMigrate)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "migrate", cfg.Environment, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("Sakura v3 migration starting", "version", version.Version, "commit", version.Commit)
	if err := migrate.New(cfg.DatabaseURL, logger).Run(ctx); err != nil {
		return err
	}
	logger.Info("Sakura v3 database is current")
	return nil
}
