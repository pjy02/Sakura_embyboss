package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Report struct {
	Mode              string    `json:"mode"`
	CanonicalAccounts int       `json:"canonical_accounts"`
	LegacyAccounts    int       `json:"legacy_accounts"`
	Identities        int       `json:"identities"`
	Conflicts         []string  `json:"conflicts"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
}
type Importer struct {
	source *sql.DB
	target *pgxpool.Pool
	apply  bool
}

func New(sourceDSN string, target *pgxpool.Pool, apply bool) (*Importer, error) {
	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		return nil, err
	}
	return &Importer{source: source, target: target, apply: apply}, nil
}
func (i *Importer) Close() { i.source.Close() }

type legacyAccount struct {
	ID                  string
	TG                  int64
	DisplayName, Status string
	Created, Updated    time.Time
}
type legacyIdentity struct {
	ID, AccountID, Provider, Subject string
	Username, Normalized, Password   sql.NullString
	Verified                         sql.NullTime
}
type embyAccount struct {
	TG           int64
	EmbyID, Name sql.NullString
	Created      sql.NullTime
}

func (i *Importer) Run(ctx context.Context) (Report, error) {
	report := Report{Mode: "dry-run", StartedAt: time.Now()}
	if i.apply {
		report.Mode = "apply"
	}
	if err := i.source.PingContext(ctx); err != nil {
		return report, err
	}
	accounts, err := i.readAccounts(ctx)
	if err != nil {
		return report, fmt.Errorf("read canonical accounts: %w", err)
	}
	identities, err := i.readIdentities(ctx)
	if err != nil {
		return report, fmt.Errorf("read canonical identities: %w", err)
	}
	legacy, err := i.readEmby(ctx)
	if err != nil {
		return report, fmt.Errorf("read legacy emby accounts: %w", err)
	}
	report.CanonicalAccounts = len(accounts)
	report.LegacyAccounts = len(legacy)
	report.Identities = len(identities)
	report.Conflicts = analyze(accounts, identities)
	if !i.apply {
		report.FinishedAt = time.Now()
		return report, nil
	}
	if i.target == nil {
		return report, fmt.Errorf("target PostgreSQL is required in apply mode")
	}
	tx, err := i.target.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(ctx)
	runID := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO legacy_import_runs(id,source_fingerprint,mode,status) VALUES($1,$2,'apply','running')`, runID, "mysql-v2")
	if err != nil {
		return report, err
	}
	tgMap := map[int64]uuid.UUID{}
	idMap := map[string]uuid.UUID{}
	for _, a := range accounts {
		id, err := uuid.Parse(a.ID)
		if err != nil {
			id = uuid.NewSHA1(uuid.NameSpaceURL, []byte("sakura-v2-account:"+a.ID))
		}
		tgMap[a.TG] = id
		idMap[a.ID] = id
		status := normalizeStatus(a.Status)
		_, err = tx.Exec(ctx, `INSERT INTO accounts(id,display_name,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`, id, fallback(a.DisplayName, fmt.Sprintf("TG %d", a.TG)), status, a.Created, a.Updated)
		if err != nil {
			return report, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,'00000000-0000-4000-8000-000000000003','system:legacy-import') ON CONFLICT DO NOTHING`, id); err != nil {
			return report, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO legacy_account_mappings(source_kind,source_key,account_id,import_run_id) VALUES('v2-account',$1,$2,$3) ON CONFLICT DO NOTHING`, a.ID, id, runID); err != nil {
			return report, err
		}
	}
	for _, item := range identities {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			id = uuid.New()
		}
		accountID, ok := idMap[item.AccountID]
		var accountErr error
		if !ok {
			accountID, accountErr = uuid.Parse(item.AccountID)
		}
		if accountErr != nil {
			report.Conflicts = append(report.Conflicts, "identity has invalid account: "+item.ID)
			continue
		}
		var accountExists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1)`, accountID).Scan(&accountExists); err != nil {
			return report, err
		}
		if !accountExists {
			report.Conflicts = append(report.Conflicts, "identity skipped because account is missing: "+item.ID)
			continue
		}
		kind := item.Provider
		if kind != "local" && kind != "telegram" && kind != "emby" {
			kind = "legacy"
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,username_normalized,password_hash,verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, id, accountID, kind, item.Subject, nullString(item.Username), nullString(item.Normalized), nullString(item.Password), nullTime(item.Verified))
		if err != nil {
			return report, err
		}
	}
	for _, item := range legacy {
		accountID, ok := tgMap[item.TG]
		if !ok {
			accountID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("sakura-account:%d", item.TG)))
			tgMap[item.TG] = accountID
			created := time.Now()
			if item.Created.Valid {
				created = item.Created.Time
			}
			_, err = tx.Exec(ctx, `INSERT INTO accounts(id,display_name,status,created_at,updated_at) VALUES($1,$2,'active',$3,$3) ON CONFLICT(id) DO NOTHING`, accountID, fallback(item.Name.String, fmt.Sprintf("TG %d", item.TG)), created)
			if err != nil {
				return report, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,'00000000-0000-4000-8000-000000000003','system:legacy-import') ON CONFLICT DO NOTHING`, accountID); err != nil {
				return report, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,verified_at) VALUES($1,$2,'telegram',$3,NOW()) ON CONFLICT(kind,subject) DO NOTHING`, uuid.New(), accountID, fmt.Sprint(item.TG)); err != nil {
			return report, err
		}
		if item.EmbyID.Valid {
			if _, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,verified_at,metadata) VALUES($1,$2,'emby',$3,$4,NOW(),'{"source":"v2-emby"}') ON CONFLICT(kind,subject) DO NOTHING`, uuid.New(), accountID, item.EmbyID.String, nullString(item.Name)); err != nil {
				return report, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO legacy_account_mappings(source_kind,source_key,account_id,import_run_id) VALUES('v2-telegram',$1,$2,$3) ON CONFLICT DO NOTHING`, fmt.Sprint(item.TG), accountID, runID); err != nil {
			return report, err
		}
	}
	report.FinishedAt = time.Now()
	summary, _ := json.Marshal(report)
	_, err = tx.Exec(ctx, `UPDATE legacy_import_runs SET status='completed',summary=$2,finished_at=NOW() WHERE id=$1`, runID, summary)
	if err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	return report, nil
}

func (i *Importer) tableExists(ctx context.Context, name string) (bool, error) {
	var count int
	err := i.source.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, name).Scan(&count)
	return count > 0, err
}
func (i *Importer) readAccounts(ctx context.Context) ([]legacyAccount, error) {
	exists, err := i.tableExists(ctx, "accounts")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,legacy_tg,COALESCE(display_name,''),status,created_at,updated_at FROM accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyAccount
	for rows.Next() {
		var x legacyAccount
		if err = rows.Scan(&x.ID, &x.TG, &x.DisplayName, &x.Status, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readIdentities(ctx context.Context) ([]legacyIdentity, error) {
	exists, err := i.tableExists(ctx, "account_identities")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,account_id,provider,subject,username,username_normalized,password_hash,verified_at FROM account_identities`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyIdentity
	for rows.Next() {
		var x legacyIdentity
		if err = rows.Scan(&x.ID, &x.AccountID, &x.Provider, &x.Subject, &x.Username, &x.Normalized, &x.Password, &x.Verified); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readEmby(ctx context.Context) ([]embyAccount, error) {
	exists, err := i.tableExists(ctx, "emby")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := i.source.QueryContext(ctx, `SELECT tg,embyid,name,cr FROM emby`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []embyAccount
	for rows.Next() {
		var x embyAccount
		if err = rows.Scan(&x.TG, &x.EmbyID, &x.Name, &x.Created); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func analyze(accounts []legacyAccount, identities []legacyIdentity) []string {
	conflicts := []string{}
	accountIDs := map[string]bool{}
	tgOwners := map[int64]string{}
	for _, account := range accounts {
		accountIDs[account.ID] = true
		if _, err := uuid.Parse(account.ID); err != nil {
			conflicts = append(conflicts, "invalid account UUID will be deterministically remapped: "+account.ID)
		}
		if previous, ok := tgOwners[account.TG]; ok && previous != account.ID {
			conflicts = append(conflicts, fmt.Sprintf("duplicate Telegram ID %d on accounts %s and %s", account.TG, previous, account.ID))
		} else {
			tgOwners[account.TG] = account.ID
		}
	}
	for _, item := range identities {
		if !accountIDs[item.AccountID] {
			conflicts = append(conflicts, "identity references a missing canonical account: "+item.ID)
		}
	}
	return conflicts
}
func fallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func normalizeStatus(value string) string {
	switch value {
	case "pending", "active", "suspended", "banned", "deleted":
		return value
	}
	return "active"
}
func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullTime(v sql.NullTime) any {
	if v.Valid {
		return v.Time
	}
	return nil
}
