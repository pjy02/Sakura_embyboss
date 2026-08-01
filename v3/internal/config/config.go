package config

import (
	"encoding/base64"
	"encoding/hex"
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
	Role                   Role
	Environment            string
	LogLevel               string
	HTTPAddress            string
	HealthAddress          string
	DatabaseURL            string
	RedisAddress           string
	RedisPassword          string
	RedisDatabase          int
	InternalAPIURL         string
	ShutdownTimeout        time.Duration
	DependencyTimeout      time.Duration
	SessionTTL             time.Duration
	SessionCookie          string
	CookieSecure           bool
	CredentialMasterKey    string
	InternalBotToken       string
	BootstrapAdminUsername string
	BootstrapAdminPassword string
	TelegramBotToken       string
	TelegramAPIBase        string
	WorkerPollInterval     time.Duration
	WorkerLeaseDuration    time.Duration
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
	sessionTTL, err := envDuration("SAKURA_V3_SESSION_TTL", 168*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := envBool("SAKURA_V3_COOKIE_SECURE", true)
	if err != nil {
		return Config{}, err
	}
	workerPoll, err := envDuration("SAKURA_V3_WORKER_POLL_INTERVAL", time.Second)
	if err != nil {
		return Config{}, err
	}
	workerLease, err := envDuration("SAKURA_V3_WORKER_LEASE_DURATION", 90*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Role:                   role,
		Environment:            env("SAKURA_V3_ENVIRONMENT", "development"),
		LogLevel:               strings.ToLower(env("SAKURA_V3_LOG_LEVEL", "info")),
		HTTPAddress:            env("SAKURA_V3_HTTP_ADDR", ":8080"),
		HealthAddress:          env("SAKURA_V3_HEALTH_ADDR", defaultHealthAddress(role)),
		DatabaseURL:            strings.TrimSpace(os.Getenv("SAKURA_V3_DATABASE_URL")),
		RedisAddress:           strings.TrimSpace(os.Getenv("SAKURA_V3_REDIS_ADDRESS")),
		RedisPassword:          os.Getenv("SAKURA_V3_REDIS_PASSWORD"),
		RedisDatabase:          redisDatabase,
		InternalAPIURL:         strings.TrimRight(env("SAKURA_V3_INTERNAL_API_URL", "http://127.0.0.1:8080"), "/"),
		ShutdownTimeout:        shutdownTimeout,
		DependencyTimeout:      dependencyTimeout,
		SessionTTL:             sessionTTL,
		SessionCookie:          env("SAKURA_V3_SESSION_COOKIE", "sakura_v3_session"),
		CookieSecure:           cookieSecure,
		CredentialMasterKey:    strings.TrimSpace(os.Getenv("SAKURA_V3_CREDENTIAL_MASTER_KEY")),
		InternalBotToken:       strings.TrimSpace(os.Getenv("SAKURA_V3_INTERNAL_BOT_TOKEN")),
		BootstrapAdminUsername: strings.TrimSpace(os.Getenv("SAKURA_V3_BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapAdminPassword: os.Getenv("SAKURA_V3_BOOTSTRAP_ADMIN_PASSWORD"),
		TelegramBotToken:       strings.TrimSpace(os.Getenv("SAKURA_V3_TELEGRAM_BOT_TOKEN")),
		TelegramAPIBase:        strings.TrimRight(env("SAKURA_V3_TELEGRAM_API_BASE", "https://api.telegram.org"), "/"),
		WorkerPollInterval:     workerPoll,
		WorkerLeaseDuration:    workerLease,
	}
	// Do not merely avoid using unrelated secrets: remove them from process
	// configuration so accidental future logging or wiring cannot expose them.
	switch role {
	case RoleAPI:
		cfg.TelegramBotToken = ""
	case RoleWorker:
		cfg.InternalBotToken = ""
		cfg.BootstrapAdminUsername = ""
		cfg.BootstrapAdminPassword = ""
		cfg.TelegramBotToken = ""
	case RoleBot:
		cfg.DatabaseURL = ""
		cfg.RedisAddress = ""
		cfg.RedisPassword = ""
		cfg.RedisDatabase = 0
		cfg.CredentialMasterKey = ""
		cfg.BootstrapAdminUsername = ""
		cfg.BootstrapAdminPassword = ""
	case RoleMigrate:
		cfg.RedisAddress = ""
		cfg.RedisPassword = ""
		cfg.RedisDatabase = 0
		cfg.CredentialMasterKey = ""
		cfg.InternalBotToken = ""
		cfg.BootstrapAdminUsername = ""
		cfg.BootstrapAdminPassword = ""
		cfg.TelegramBotToken = ""
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
	if c.Role == RoleAPI || c.Role == RoleWorker {
		if !validMasterKey(c.CredentialMasterKey) {
			return errors.New("SAKURA_V3_CREDENTIAL_MASTER_KEY must be 32 bytes encoded as 64 hex or unpadded base64")
		}
	}
	if c.Role == RoleAPI {
		if len(c.InternalBotToken) < 32 {
			return errors.New("SAKURA_V3_INTERNAL_BOT_TOKEN must contain at least 32 characters")
		}
		if (c.BootstrapAdminUsername == "") != (c.BootstrapAdminPassword == "") {
			return errors.New("bootstrap admin username and password must be configured together")
		}
	}
	if c.Role == RoleBot {
		parsed, err := url.Parse(c.InternalAPIURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("SAKURA_V3_INTERNAL_API_URL must be an absolute HTTP URL")
		}
		if len(c.InternalBotToken) < 32 {
			return errors.New("SAKURA_V3_INTERNAL_BOT_TOKEN must contain at least 32 characters")
		}
		parsed, err = url.Parse(c.TelegramAPIBase)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("SAKURA_V3_TELEGRAM_API_BASE must be an absolute HTTP URL")
		}
	}
	if (c.Role == RoleWorker || c.Role == RoleBot) && c.HealthAddress == "" {
		return errors.New("SAKURA_V3_HEALTH_ADDR must not be empty")
	}
	return nil
}

func validMasterKey(value string) bool {
	if len(value) == 64 {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == 32
	}
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
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

func envBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}
