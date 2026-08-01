package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Role string

const (
	RoleAPI     Role = "api"
	RoleWorker  Role = "worker"
	RoleBot     Role = "bot"
	RoleMigrate Role = "migrate"
)

type Config struct {
	Role              Role
	Environment       string
	LogLevel          string
	HTTPAddress       string
	HealthAddress     string
	DatabaseURL       string
	RedisAddress      string
	RedisPassword     string
	RedisDatabase     int
	InternalAPIURL    string
	ShutdownTimeout   time.Duration
	DependencyTimeout time.Duration
}

func Load(role Role) (Config, error) {
	redisDatabase, err := envInt("SAKURA_V3_REDIS_DATABASE", 0)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := envDuration("SAKURA_V3_SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	dependencyTimeout, err := envDuration("SAKURA_V3_DEPENDENCY_TIMEOUT", 3*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Role:              role,
		Environment:       env("SAKURA_V3_ENVIRONMENT", "development"),
		LogLevel:          strings.ToLower(env("SAKURA_V3_LOG_LEVEL", "info")),
		HTTPAddress:       env("SAKURA_V3_HTTP_ADDR", ":8080"),
		HealthAddress:     env("SAKURA_V3_HEALTH_ADDR", defaultHealthAddress(role)),
		DatabaseURL:       strings.TrimSpace(os.Getenv("SAKURA_V3_DATABASE_URL")),
		RedisAddress:      strings.TrimSpace(os.Getenv("SAKURA_V3_REDIS_ADDRESS")),
		RedisPassword:     os.Getenv("SAKURA_V3_REDIS_PASSWORD"),
		RedisDatabase:     redisDatabase,
		InternalAPIURL:    strings.TrimRight(env("SAKURA_V3_INTERNAL_API_URL", "http://127.0.0.1:8080"), "/"),
		ShutdownTimeout:   shutdownTimeout,
		DependencyTimeout: dependencyTimeout,
	}
	// Do not merely avoid using unrelated secrets: remove them from process
	// configuration so accidental future logging or wiring cannot expose them.
	switch role {
	case RoleBot:
		cfg.DatabaseURL = ""
		cfg.RedisAddress = ""
		cfg.RedisPassword = ""
		cfg.RedisDatabase = 0
	case RoleMigrate:
		cfg.RedisAddress = ""
		cfg.RedisPassword = ""
		cfg.RedisDatabase = 0
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Role != RoleAPI && c.Role != RoleWorker && c.Role != RoleBot && c.Role != RoleMigrate {
		return fmt.Errorf("unsupported service role %q", c.Role)
	}
	if c.Environment == "" {
		return errors.New("SAKURA_V3_ENVIRONMENT must not be empty")
	}
	if _, ok := map[string]struct{}{"debug": {}, "info": {}, "warn": {}, "error": {}}[c.LogLevel]; !ok {
		return fmt.Errorf("invalid SAKURA_V3_LOG_LEVEL %q", c.LogLevel)
	}
	if c.ShutdownTimeout <= 0 || c.DependencyTimeout <= 0 {
		return errors.New("timeouts must be positive")
	}
	if c.Role == RoleAPI || c.Role == RoleWorker || c.Role == RoleMigrate {
		if c.DatabaseURL == "" {
			return errors.New("SAKURA_V3_DATABASE_URL is required")
		}
		parsed, err := url.Parse(c.DatabaseURL)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
			return errors.New("SAKURA_V3_DATABASE_URL must be a PostgreSQL URL")
		}
	}
	if c.Role == RoleAPI || c.Role == RoleWorker {
		if c.RedisAddress == "" {
			return errors.New("SAKURA_V3_REDIS_ADDRESS is required")
		}
	}
	if c.Role == RoleAPI && c.HTTPAddress == "" {
		return errors.New("SAKURA_V3_HTTP_ADDR must not be empty")
	}
	if c.Role == RoleBot {
		parsed, err := url.Parse(c.InternalAPIURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("SAKURA_V3_INTERNAL_API_URL must be an absolute HTTP URL")
		}
	}
	if (c.Role == RoleWorker || c.Role == RoleBot) && c.HealthAddress == "" {
		return errors.New("SAKURA_V3_HEALTH_ADDR must not be empty")
	}
	return nil
}

func defaultHealthAddress(role Role) string {
	switch role {
	case RoleWorker:
		return ":8081"
	case RoleBot:
		return ":8082"
	default:
		return ":8080"
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}
