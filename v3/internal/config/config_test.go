package config

import (
	"testing"
	"time"
)

func TestBotDoesNotRequireDatabaseOrRedis(t *testing.T) {
	t.Setenv("SAKURA_V3_DATABASE_URL", "postgres://must:not-leak@db:5432/sakura")
	t.Setenv("SAKURA_V3_REDIS_ADDRESS", "redis:6379")
	t.Setenv("SAKURA_V3_REDIS_PASSWORD", "must-not-leak")
	t.Setenv("SAKURA_V3_INTERNAL_API_URL", "http://api:8080")
	t.Setenv("SAKURA_V3_INTERNAL_BOT_TOKEN", "test-internal-token-at-least-32-characters")
	t.Setenv("SAKURA_V3_TELEGRAM_BOT_TOKEN", "telegram-token")
	t.Setenv("SAKURA_V3_CREDENTIAL_MASTER_KEY", "must-not-leak")
	cfg, err := Load(RoleBot)
	if err != nil {
		t.Fatalf("load bot config: %v", err)
	}
	if cfg.DatabaseURL != "" || cfg.RedisAddress != "" {
		t.Fatal("bot unexpectedly owns database or Redis configuration")
	}
	if cfg.CredentialMasterKey != "" || cfg.TelegramBotToken != "telegram-token" {
		t.Fatal("bot secret boundary was not applied correctly")
	}
}

func TestAPIRequiresPostgresAndRedis(t *testing.T) {
	t.Setenv("SAKURA_V3_DATABASE_URL", "")
	t.Setenv("SAKURA_V3_REDIS_ADDRESS", "")
	if _, err := Load(RoleAPI); err == nil {
		t.Fatal("expected missing PostgreSQL configuration to fail")
	}

	t.Setenv("SAKURA_V3_DATABASE_URL", "postgres://user:pass@db:5432/sakura")
	if _, err := Load(RoleAPI); err == nil {
		t.Fatal("expected missing Redis configuration to fail")
	}

	t.Setenv("SAKURA_V3_REDIS_ADDRESS", "redis:6379")
	t.Setenv("SAKURA_V3_CREDENTIAL_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("SAKURA_V3_INTERNAL_BOT_TOKEN", "test-internal-token-at-least-32-characters")
	cfg, err := Load(RoleAPI)
	if err != nil {
		t.Fatalf("load API config: %v", err)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("unexpected shutdown timeout: %s", cfg.ShutdownTimeout)
	}
}

func TestMigrateDoesNotRequireRedis(t *testing.T) {
	t.Setenv("SAKURA_V3_DATABASE_URL", "postgres://user:pass@db:5432/sakura")
	t.Setenv("SAKURA_V3_REDIS_ADDRESS", "")
	if _, err := Load(RoleMigrate); err != nil {
		t.Fatalf("migrate should only require PostgreSQL: %v", err)
	}
}

func TestWorkerOwnsCredentialKeyButNotBotSecrets(t *testing.T) {
	t.Setenv("SAKURA_V3_DATABASE_URL", "postgres://user:pass@db:5432/sakura")
	t.Setenv("SAKURA_V3_REDIS_ADDRESS", "redis:6379")
	t.Setenv("SAKURA_V3_CREDENTIAL_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("SAKURA_V3_INTERNAL_BOT_TOKEN", "must-not-leak-to-worker-at-least-32")
	t.Setenv("SAKURA_V3_TELEGRAM_BOT_TOKEN", "must-not-leak")
	cfg, err := Load(RoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CredentialMasterKey == "" || cfg.InternalBotToken != "" || cfg.TelegramBotToken != "" {
		t.Fatal("worker secret boundary is incorrect")
	}
}

func TestInvalidDurationIsRejected(t *testing.T) {
	t.Setenv("SAKURA_V3_DATABASE_URL", "postgres://user:pass@db:5432/sakura")
	t.Setenv("SAKURA_V3_SHUTDOWN_TIMEOUT", "never")
	if _, err := Load(RoleMigrate); err == nil {
		t.Fatal("expected invalid duration to fail")
	}
}
