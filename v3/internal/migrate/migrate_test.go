package migrate

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestDiscoverMigrationsIsOrderedAndChecksummed(t *testing.T) {
	migrations, err := Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected embedded migrations")
	}
	for index, migration := range migrations {
		if len(migration.Checksum) != 64 {
			t.Fatalf("migration %d has invalid checksum", migration.Version)
		}
		if index > 0 && migrations[index-1].Version >= migration.Version {
			t.Fatal("migrations are not strictly ordered")
		}
	}
}

func TestStage3MigrationContainsDurablePlatformState(t *testing.T) {
	migrations, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	var stage3 string
	for _, migration := range migrations {
		if migration.Version == 3 {
			stage3 = migration.SQL
		}
	}
	for _, table := range []string{"membership_plans", "invitation_codes", "emby_instances", "emby_account_bindings", "remote_emby_users", "platform_tasks", "remote_state_snapshots"} {
		if !strings.Contains(stage3, "CREATE TABLE "+table) {
			t.Fatalf("stage 3 migration is missing %s", table)
		}
	}
}

func TestStage4MigrationContainsBalancedLedgerAndBatchState(t *testing.T) {
	migrations, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	var stage4 string
	for _, migration := range migrations {
		if migration.Version == 4 {
			stage4 = migration.SQL
		}
	}
	for _, table := range []string{"wallets", "ledger_transactions", "ledger_entries", "recharge_products", "recharge_orders", "recharge_refunds", "membership_purchases", "account_tags", "batch_operations", "account_notifications"} {
		if !strings.Contains(stage4, "CREATE TABLE "+table) {
			t.Fatalf("stage 4 migration is missing %s", table)
		}
	}
	for _, invariant := range []string{"sakura_ledger_entry_guard", "sakura_ledger_transaction_guard", "ledger transaction is not balanced"} {
		if !strings.Contains(stage4, invariant) {
			t.Fatalf("stage 4 migration is missing invariant %s", invariant)
		}
	}
}

func TestStage5MigrationContainsPlaybackDeviceAndTraceableRiskState(t *testing.T) {
	migrations, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	var stage5 string
	for _, migration := range migrations {
		if migration.Version == 5 {
			stage5 = migration.SQL
		}
	}
	for _, table := range []string{"emby_instance_runtime_health", "playback_sessions", "playback_history", "device_profiles", "device_access_rules", "risk_rules", "risk_events", "risk_actions", "risk_event_timeline"} {
		if !strings.Contains(stage5, "CREATE TABLE "+table) {
			t.Fatalf("stage 5 migration is missing %s", table)
		}
	}
	for _, invariant := range []string{"observation_mode", "before_state", "revert_pending", "dedupe_key"} {
		if !strings.Contains(stage5, invariant) {
			t.Fatalf("stage 5 migration is missing risk invariant %s", invariant)
		}
	}
}

func TestStage6MigrationContainsMediaCommunityAndAutomationState(t *testing.T) {
	migrations, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	var stage6 string
	for _, migration := range migrations {
		if migration.Version == 6 {
			stage6 = migration.SQL
		}
	}
	for _, table := range []string{"media_catalog", "media_matches", "media_requests", "media_request_subscriptions", "moviepilot_jobs", "support_tickets", "ticket_messages", "media_reviews", "notification_preferences", "broadcasts", "automation_rules", "automation_events", "automation_executions"} {
		if !strings.Contains(stage6, "CREATE TABLE "+table) {
			t.Fatalf("stage 6 migration is missing %s", table)
		}
	}
	for _, invariant := range []string{"media_requests_active_media_uidx", "moviepilot_jobs_active_media_uidx", "is_internal BOOLEAN", "event_key VARCHAR(255) NOT NULL UNIQUE"} {
		if !strings.Contains(stage6, invariant) {
			t.Fatalf("stage 6 migration is missing invariant %s", invariant)
		}
	}
}

func TestRunIsIdempotent(t *testing.T) {
	databaseURL := os.Getenv("SAKURA_V3_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SAKURA_V3_TEST_DATABASE_URL is not configured")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := New(databaseURL, logger)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("second migration run: %v", err)
	}

	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect for assertion: %v", err)
	}
	defer connection.Close(context.Background())
	var count int
	if err := connection.QueryRow(context.Background(), "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	migrations, _ := Discover()
	if count != len(migrations) {
		t.Fatalf("expected %d migration rows, got %d", len(migrations), count)
	}
}
