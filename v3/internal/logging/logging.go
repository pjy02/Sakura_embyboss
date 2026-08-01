package logging

import (
	"io"
	"log/slog"
	"strings"
)

func New(output io.Writer, service, environment, level string) *slog.Logger {
	var parsed slog.Level
	switch strings.ToLower(level) {
	case "debug":
		parsed = slog.LevelDebug
	case "warn":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		parsed = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: parsed})
	return slog.New(handler).With("service", service, "environment", environment)
}
