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
	"github.com/pjy02/Sakura_embyboss/v3/internal/logging"
	runservice "github.com/pjy02/Sakura_embyboss/v3/internal/run"
	"github.com/pjy02/Sakura_embyboss/v3/internal/upstream"
	"github.com/pjy02/Sakura_embyboss/v3/internal/version"
)

func main() {
	if err := execute(); err != nil {
		slog.Error("Bot stopped with an error", "error", err)
		os.Exit(1)
	}
}

func execute() error {
	cfg, err := config.Load(config.RoleBot)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "bot", cfg.Environment, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	apiProbe := upstream.NewHTTPProbe(cfg.InternalAPIURL, cfg.DependencyTimeout)
	mux := http.NewServeMux()
	health.New("bot", cfg.DependencyTimeout,
		health.Probe{Name: "api", Check: apiProbe.Ping},
	).Register(mux)
	server := &http.Server{Addr: cfg.HealthAddress, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	logger.Info("Sakura v3 Bot shell starting", "version", version.Version, "commit", version.Commit)
	return runservice.Group(ctx, logger,
		runservice.HTTPServer(server, cfg.ShutdownTimeout, logger),
		runservice.Heartbeat("bot-adapter", 30*time.Second, logger),
	)
}
