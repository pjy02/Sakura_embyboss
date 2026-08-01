package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Component func(context.Context) error

func Group(parent context.Context, logger *slog.Logger, components ...Component) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	errorsChannel := make(chan error, len(components))
	var wait sync.WaitGroup
	for _, component := range components {
		component := component
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := component(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errorsChannel <- err
				cancel()
			}
		}()
	}

	select {
	case <-parent.Done():
		cancel()
	case err := <-errorsChannel:
		cancel()
		wait.Wait()
		return err
	}
	wait.Wait()
	logger.Info("service stopped")
	return nil
}

func HTTPServer(server *http.Server, shutdownTimeout time.Duration, logger *slog.Logger) Component {
	return func(ctx context.Context) error {
		result := make(chan error, 1)
		go func() {
			logger.Info("HTTP listener started", "address", server.Addr)
			result <- server.ListenAndServe()
		}()

		select {
		case err := <-result:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("HTTP server: %w", err)
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("HTTP shutdown: %w", err)
			}
			return nil
		}
	}
}

func Heartbeat(name string, interval time.Duration, logger *slog.Logger) Component {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		logger.Info("component started", "component", name)
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				logger.Debug("component heartbeat", "component", name)
			}
		}
	}
}
