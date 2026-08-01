package migrate

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/*.up.sql
var migrationFiles embed.FS

var migrationPattern = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.up\.sql$`)

type Migration struct {
	Version  int64
	Name     string
	Checksum string
	SQL      string
}

type Runner struct {
	databaseURL string
	logger      *slog.Logger
}

func New(databaseURL string, logger *slog.Logger) *Runner {
	return &Runner{databaseURL: databaseURL, logger: logger}
}

func Discover() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "sql")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int64]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationPattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version %s: %w", entry.Name(), err)
		}
		if existing, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d: %s and %s", version, existing, entry.Name())
		}
		body, err := migrationFiles.ReadFile(filepath.ToSlash("sql/" + entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(body)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     matches[2],
			Checksum: hex.EncodeToString(digest[:]),
			SQL:      string(body),
		})
		seen[version] = entry.Name()
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no embedded migrations found")
	}
	return migrations, nil
}

func (r *Runner) Run(ctx context.Context) error {
	config, err := pgx.ParseConfig(r.databaseURL)
	if err != nil {
		return fmt.Errorf("parse PostgreSQL configuration: %w", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}
	defer connection.Close(context.Background())

	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock(hashtext('sakura-v3-migrations'))"); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer connection.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('sakura-v3-migrations'))")

	if _, err := connection.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			checksum CHAR(64) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	migrations, err := Discover()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var checksum string
		err := connection.QueryRow(ctx,
			"SELECT checksum FROM schema_migrations WHERE version = $1",
			migration.Version,
		).Scan(&checksum)
		switch {
		case err == nil:
			if checksum != migration.Checksum {
				return fmt.Errorf("migration %d checksum changed after application", migration.Version)
			}
			r.logger.Debug("migration already applied", "version", migration.Version, "name", migration.Name)
			continue
		case err != pgx.ErrNoRows:
			return fmt.Errorf("read migration %d state: %w", migration.Version, err)
		}

		transaction, err := connection.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}
		if _, err := transaction.Exec(ctx, migration.SQL); err != nil {
			_ = transaction.Rollback(ctx)
			return fmt.Errorf("apply migration %d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := transaction.Exec(ctx,
			"INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)",
			migration.Version, migration.Name, migration.Checksum,
		); err != nil {
			_ = transaction.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
		if err := transaction.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
		r.logger.Info("migration applied", "version", migration.Version, "name", migration.Name)
	}
	return nil
}
