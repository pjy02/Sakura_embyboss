package migrate

import (
	"context"
	"io"
	"log/slog"
	"os"
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
