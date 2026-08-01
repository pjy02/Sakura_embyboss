package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	apiContract "github.com/pjy02/Sakura_embyboss/v3/api"
	"github.com/pjy02/Sakura_embyboss/v3/internal/health"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

type Options struct {
	Environment      string
	Version          string
	Logger           *slog.Logger
	Probes           []health.Probe
	ProbeTimeout     time.Duration
	Identity         *identity.Service
	SessionCookie    string
	CookieSecure     bool
	InternalBotToken string
}

func New(options Options) http.Handler {
	mux := http.NewServeMux()
	health.New("api", options.ProbeTimeout, options.Probes...).Register(mux)
	if options.Identity != nil {
		registerIdentityRoutes(mux, options)
	}
	mux.HandleFunc("GET /api/v3/system/info", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{
			"name":        "Sakura",
			"version":     options.Version,
			"service":     "api",
			"environment": options.Environment,
		})
	})
	mux.HandleFunc("GET /openapi.yaml", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-cache")
		_, _ = writer.Write(apiContract.OpenAPISpec)
	})
	mux.HandleFunc("GET /", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{
			"name":    "Sakura v3 API",
			"openapi": "/openapi.yaml",
		})
	})
	return requestLog(options.Logger, securityHeaders(options.CookieSecure, mux))
}

func securityHeaders(httpsOnly bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		if httpsOnly {
			writer.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(writer, request)
	})
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		next.ServeHTTP(writer, request)
		logger.Debug("HTTP request",
			"method", request.Method,
			"path", request.URL.Path,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
