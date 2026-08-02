package reconcile

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CountCheck struct {
	Source  int64 `json:"source"`
	Target  int64 `json:"target"`
	Missing int64 `json:"missing"`
}

type MoneyCheck struct {
	Currency   string `json:"currency"`
	Source     int64  `json:"source"`
	Imported   int64  `json:"imported"`
	Current    int64  `json:"current"`
	Difference int64  `json:"difference"`
}

type TableCheck struct {
	Name        string `json:"name"`
	Rows        int64  `json:"rows"`
	Disposition string `json:"disposition"`
	Implemented bool   `json:"implemented"`
}

type Report struct {
	GeneratedAt            time.Time    `json:"generated_at"`
	Accounts               CountCheck   `json:"accounts"`
	Memberships            CountCheck   `json:"memberships"`
	EmbyIdentities         CountCheck   `json:"emby_identities"`
	Credentials            CountCheck   `json:"credentials"`
	EmbyInstances          CountCheck   `json:"emby_instances"`
	InstanceBindings       CountCheck   `json:"instance_bindings"`
	Settings               CountCheck   `json:"settings"`
	AuditLogs              CountCheck   `json:"audit_logs"`
	SupportTickets         CountCheck   `json:"support_tickets"`
	TicketMessages         CountCheck   `json:"ticket_messages"`
	Notifications          CountCheck   `json:"notifications"`
	NotificationPrefs      CountCheck   `json:"notification_preferences"`
	LedgerHistory          CountCheck   `json:"ledger_history"`
	InvitationCodes        CountCheck   `json:"invitation_codes"`
	RechargeProducts       CountCheck   `json:"recharge_products"`
	RechargeOrders         CountCheck   `json:"recharge_orders"`
	RoleMembers            CountCheck   `json:"role_members"`
	UnsupportedCustomRoles int64        `json:"unsupported_custom_roles"`
	SecretSettings         int64        `json:"secret_settings_requiring_manual_migration"`
	EmbyBindings           int64        `json:"emby_bindings"`
	Wallets                []MoneyCheck `json:"wallets"`
	Tables                 []TableCheck `json:"tables"`
	UnbalancedTransactions int64        `json:"unbalanced_transactions"`
	LastImportStatus       string       `json:"last_import_status"`
	Pass                   bool         `json:"pass"`
	Blockers               []string     `json:"blockers"`
	Warnings               []string     `json:"warnings"`
}

type Checker struct {
	source *sql.DB
	target *pgxpool.Pool
}

func New(sourceDSN string, target *pgxpool.Pool) (*Checker, error) {
	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		return nil, err
	}
	return &Checker{source: source, target: target}, nil
}

func (c *Checker) Close() { _ = c.source.Close() }

func (c *Checker) Run(ctx context.Context) (Report, error) {
	report := Report{GeneratedAt: time.Now().UTC(), LastImportStatus: "missing"}
	if err := c.source.PingContext(ctx); err != nil {
		return report, fmt.Errorf("v2 mysql: %w", err)
	}
	if c.target == nil {
		return report, fmt.Errorf("v3 PostgreSQL is required")
	}

	canonical, err := c.countSource(ctx, "accounts", `SELECT COUNT(*) FROM accounts`)
	if err != nil {
		return report, err
	}
	legacy, err := c.countSource(ctx, "emby", `SELECT COUNT(*) FROM emby`)
	if err != nil {
		return report, err
	}
	report.Accounts.Source = canonical
	if canonical == 0 {
		report.Accounts.Source = legacy
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(DISTINCT account_id) FROM legacy_account_mappings WHERE source_kind=CASE WHEN $1>0 THEN 'v2-account' ELSE 'v2-telegram' END`, canonical).Scan(&report.Accounts.Target); err != nil {
		return report, err
	}
	report.Accounts.Missing = positive(report.Accounts.Source - report.Accounts.Target)

	report.Memberships.Source, err = c.countSource(ctx, "account_memberships", `SELECT COUNT(*) FROM account_memberships`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM account_memberships WHERE source='legacy_import'`).Scan(&report.Memberships.Target); err != nil {
		return report, err
	}
	report.Memberships.Missing = positive(report.Memberships.Source - report.Memberships.Target)

	embyIDs, err := c.sourceEmbyIDs(ctx)
	if err != nil {
		return report, err
	}
	report.EmbyIdentities.Source = int64(len(embyIDs))
	if err = c.target.QueryRow(ctx, `SELECT COUNT(DISTINCT subject) FROM account_identities WHERE kind='emby' AND subject=ANY($1)`, embyIDs).Scan(&report.EmbyIdentities.Target); err != nil {
		return report, err
	}
	report.EmbyIdentities.Missing = positive(report.EmbyIdentities.Source - report.EmbyIdentities.Target)
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM emby_account_bindings WHERE origin='legacy_import' AND status<>'deleted'`).Scan(&report.EmbyBindings); err != nil {
		return report, err
	}
	report.Credentials.Source, err = c.countSource(ctx, "managed_credentials", `SELECT COUNT(*) FROM managed_credentials`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM credentials WHERE metadata->>'source'='v2'`).Scan(&report.Credentials.Target); err != nil {
		return report, err
	}
	report.Credentials.Missing = positive(report.Credentials.Source - report.Credentials.Target)
	instanceNames, namesErr := c.sourceStrings(ctx, "emby_instances", `SELECT name FROM emby_instances ORDER BY name`)
	if namesErr != nil {
		return report, namesErr
	}
	report.EmbyInstances.Source = int64(len(instanceNames))
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM emby_instances WHERE name=ANY($1)`, instanceNames).Scan(&report.EmbyInstances.Target); err != nil {
		return report, err
	}
	report.EmbyInstances.Missing = positive(report.EmbyInstances.Source - report.EmbyInstances.Target)
	report.InstanceBindings.Source, err = c.countSource(ctx, "account_emby_bindings", `SELECT COUNT(*) FROM account_emby_bindings`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM emby_account_bindings WHERE origin='legacy_import'`).Scan(&report.InstanceBindings.Target); err != nil {
		return report, err
	}
	report.InstanceBindings.Missing = positive(report.InstanceBindings.Source - report.InstanceBindings.Target)
	settingKeys, settingsErr := c.sourceStrings(ctx, "dynamic_settings", `SELECT setting_key FROM dynamic_settings WHERE is_secret=0 ORDER BY setting_key`)
	if settingsErr != nil {
		return report, settingsErr
	}
	report.Settings.Source = int64(len(settingKeys))
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM dynamic_settings WHERE key=ANY($1)`, settingKeys).Scan(&report.Settings.Target); err != nil {
		return report, err
	}
	report.Settings.Missing = positive(report.Settings.Source - report.Settings.Target)
	report.AuditLogs.Source, err = c.countSource(ctx, "audit_logs", `SELECT COUNT(*) FROM audit_logs`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE request_id LIKE 'legacy-v2-audit-%'`).Scan(&report.AuditLogs.Target); err != nil {
		return report, err
	}
	report.AuditLogs.Missing = positive(report.AuditLogs.Source - report.AuditLogs.Target)
	report.SecretSettings, err = c.countSource(ctx, "dynamic_settings", `SELECT COUNT(*) FROM dynamic_settings WHERE is_secret=1`)
	if err != nil {
		return report, err
	}
	ticketNumbers, ticketErr := c.sourceStrings(ctx, "support_tickets", `SELECT ticket_no FROM support_tickets ORDER BY ticket_no`)
	if ticketErr != nil {
		return report, ticketErr
	}
	report.SupportTickets.Source = int64(len(ticketNumbers))
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM support_tickets WHERE ticket_no=ANY($1)`, ticketNumbers).Scan(&report.SupportTickets.Target); err != nil {
		return report, err
	}
	report.SupportTickets.Missing = positive(report.SupportTickets.Source - report.SupportTickets.Target)
	report.TicketMessages.Source, err = c.countSource(ctx, "ticket_messages", `SELECT COUNT(*) FROM ticket_messages`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM ticket_messages WHERE attachments @> '[{"source":"v2"}]'::jsonb`).Scan(&report.TicketMessages.Target); err != nil {
		return report, err
	}
	report.TicketMessages.Missing = positive(report.TicketMessages.Source - report.TicketMessages.Target)
	report.Notifications.Source, err = c.countSource(ctx, "user_notifications", `SELECT COUNT(*) FROM user_notifications`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM account_notifications WHERE metadata->>'source'='v2'`).Scan(&report.Notifications.Target); err != nil {
		return report, err
	}
	report.Notifications.Missing = positive(report.Notifications.Source - report.Notifications.Target)
	prefRows, err := c.countSource(ctx, "notification_preferences", `SELECT COUNT(*) FROM notification_preferences`)
	if err != nil {
		return report, err
	}
	report.NotificationPrefs.Source = prefRows * 2
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM notification_preferences WHERE account_id IN(SELECT account_id FROM legacy_account_mappings WHERE source_kind IN('v2-account','v2-telegram'))`).Scan(&report.NotificationPrefs.Target); err != nil {
		return report, err
	}
	report.NotificationPrefs.Missing = positive(report.NotificationPrefs.Source - report.NotificationPrefs.Target)
	report.LedgerHistory.Source, err = c.countSource(ctx, "account_ledger_entries", `SELECT COUNT(*) FROM account_ledger_entries WHERE amount<>0`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_transactions WHERE metadata->>'source'='v2' AND metadata ? 'legacy_entry_id'`).Scan(&report.LedgerHistory.Target); err != nil {
		return report, err
	}
	report.LedgerHistory.Missing = positive(report.LedgerHistory.Source - report.LedgerHistory.Target)
	report.InvitationCodes.Source, err = c.countSource(ctx, "Rcode", "SELECT COUNT(*) FROM `Rcode`")
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM invitation_codes WHERE metadata->>'source'='v2-rcode'`).Scan(&report.InvitationCodes.Target); err != nil {
		return report, err
	}
	report.InvitationCodes.Missing = positive(report.InvitationCodes.Source - report.InvitationCodes.Target)
	report.RechargeProducts.Source, err = c.countSource(ctx, "recharge_products", `SELECT COUNT(*) FROM recharge_products`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM recharge_products WHERE code LIKE 'v2-product-%'`).Scan(&report.RechargeProducts.Target); err != nil {
		return report, err
	}
	report.RechargeProducts.Missing = positive(report.RechargeProducts.Source - report.RechargeProducts.Target)
	orderNumbers, orderErr := c.sourceStrings(ctx, "recharge_orders", `SELECT order_no FROM recharge_orders ORDER BY order_no`)
	if orderErr != nil {
		return report, orderErr
	}
	report.RechargeOrders.Source = int64(len(orderNumbers))
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM recharge_orders WHERE order_no=ANY($1)`, orderNumbers).Scan(&report.RechargeOrders.Target); err != nil {
		return report, err
	}
	report.RechargeOrders.Missing = positive(report.RechargeOrders.Source - report.RechargeOrders.Target)
	report.RoleMembers.Source, err = c.countSource(ctx, "web_role_members", `SELECT COUNT(*) FROM web_role_members`)
	if err != nil {
		return report, err
	}
	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM account_roles WHERE assigned_by='system:legacy-import'`).Scan(&report.RoleMembers.Target); err != nil {
		return report, err
	}
	report.RoleMembers.Missing = positive(report.RoleMembers.Source - report.RoleMembers.Target)
	report.UnsupportedCustomRoles, err = c.countSource(ctx, "web_roles", `SELECT COUNT(*) FROM web_roles WHERE LOWER(name) NOT IN('owner','admin','administrator','user')`)
	if err != nil {
		return report, err
	}

	sourceWallets, err := c.sourceWalletTotals(ctx)
	if err != nil {
		return report, err
	}
	importedWallets, err := queryTotals(ctx, c.target, `SELECT t.currency,COALESCE(SUM(CASE WHEN e.side='credit' THEN e.amount ELSE -e.amount END),0)
		FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id JOIN ledger_accounts a ON a.id=e.ledger_account_id
		WHERE t.status='posted' AND t.reference_type='legacy_wallet' AND a.owner_type='account' AND a.code='wallet' GROUP BY t.currency`)
	if err != nil {
		return report, err
	}
	currentWallets, err := queryTotals(ctx, c.target, `SELECT a.currency,COALESCE(SUM(CASE WHEN t.status='posted' AND e.side='credit' THEN e.amount WHEN t.status='posted' AND e.side='debit' THEN -e.amount ELSE 0 END),0) FROM ledger_accounts a LEFT JOIN ledger_entries e ON e.ledger_account_id=a.id LEFT JOIN ledger_transactions t ON t.id=e.transaction_id WHERE a.owner_type='account' AND a.code='wallet' GROUP BY a.currency`)
	if err != nil {
		return report, err
	}
	keys := map[string]bool{}
	for key := range sourceWallets {
		keys[key] = true
	}
	for key := range importedWallets {
		keys[key] = true
	}
	for key := range currentWallets {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, currency := range ordered {
		report.Wallets = append(report.Wallets, MoneyCheck{Currency: currency, Source: sourceWallets[currency], Imported: importedWallets[currency], Current: currentWallets[currency], Difference: importedWallets[currency] - sourceWallets[currency]})
	}

	if err = c.target.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT t.id FROM ledger_transactions t LEFT JOIN ledger_entries e ON e.transaction_id=t.id WHERE t.status='posted' GROUP BY t.id HAVING COALESCE(SUM(e.amount) FILTER(WHERE e.side='debit'),0)<>COALESCE(SUM(e.amount) FILTER(WHERE e.side='credit'),0)) bad`).Scan(&report.UnbalancedTransactions); err != nil {
		return report, err
	}
	_ = c.target.QueryRow(ctx, `SELECT status FROM legacy_import_runs ORDER BY started_at DESC LIMIT 1`).Scan(&report.LastImportStatus)
	report.Tables, err = c.sourceTableCoverage(ctx)
	if err != nil {
		return report, err
	}
	report.Evaluate()
	return report, nil
}

func (r *Report) Evaluate() {
	r.Blockers = nil
	r.Warnings = nil
	if r.Accounts.Missing > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("missing %d accounts", r.Accounts.Missing))
	}
	if r.Memberships.Missing > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("missing %d memberships", r.Memberships.Missing))
	}
	if r.EmbyIdentities.Missing > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("missing %d Emby identities", r.EmbyIdentities.Missing))
	}
	for name, check := range map[string]CountCheck{"credentials": r.Credentials, "Emby instances": r.EmbyInstances, "Emby instance bindings": r.InstanceBindings, "settings": r.Settings, "audit logs": r.AuditLogs, "support tickets": r.SupportTickets, "ticket messages": r.TicketMessages, "notifications": r.Notifications, "notification preferences": r.NotificationPrefs, "ledger history": r.LedgerHistory, "invitation codes": r.InvitationCodes, "recharge products": r.RechargeProducts, "recharge orders": r.RechargeOrders, "role members": r.RoleMembers} {
		if check.Missing > 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf("missing %d %s", check.Missing, name))
		}
	}
	for _, wallet := range r.Wallets {
		if wallet.Difference != 0 {
			r.Blockers = append(r.Blockers, fmt.Sprintf("wallet %s differs by %d", wallet.Currency, wallet.Difference))
		}
		if wallet.Current != wallet.Imported {
			r.Warnings = append(r.Warnings, fmt.Sprintf("wallet %s current total differs from imported total by %d", wallet.Currency, wallet.Current-wallet.Imported))
		}
	}
	if r.UnbalancedTransactions > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("%d ledger transactions are unbalanced", r.UnbalancedTransactions))
	}
	if r.LastImportStatus != "completed" {
		r.Blockers = append(r.Blockers, "last import is not completed")
	}
	if r.SecretSettings > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("%d secret dynamic settings require credential-center migration", r.SecretSettings))
	}
	if r.UnsupportedCustomRoles > 0 {
		r.Blockers = append(r.Blockers, fmt.Sprintf("%d custom v2 roles require explicit permission mapping", r.UnsupportedCustomRoles))
	}
	if r.EmbyBindings < r.EmbyIdentities.Target {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d imported Emby identities are not adopted into instance bindings", r.EmbyIdentities.Target-r.EmbyBindings))
	}
	for _, table := range r.Tables {
		if table.Rows == 0 {
			continue
		}
		switch table.Disposition {
		case "pending_adapter", "unknown":
			r.Blockers = append(r.Blockers, fmt.Sprintf("v2 table %s has %d rows but no active-domain migration adapter", table.Name, table.Rows))
		case "archive", "rebuild", "invalidate":
			r.Warnings = append(r.Warnings, fmt.Sprintf("v2 table %s has %d rows and will be %s", table.Name, table.Rows, table.Disposition))
		}
	}
	r.Pass = len(r.Blockers) == 0
}

func (c *Checker) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := c.source.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, name).Scan(&n)
	return n > 0, err
}
func (c *Checker) countSource(ctx context.Context, table, query string) (int64, error) {
	exists, err := c.tableExists(ctx, table)
	if err != nil || !exists {
		return 0, err
	}
	var n int64
	err = c.source.QueryRowContext(ctx, query).Scan(&n)
	return n, err
}
func (c *Checker) sourceWalletTotals(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	exists, err := c.tableExists(ctx, "account_wallets")
	if err != nil || !exists {
		return out, err
	}
	rows, err := c.source.QueryContext(ctx, `SELECT balance_type,COALESCE(SUM(balance),0) FROM account_wallets GROUP BY balance_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value int64
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[normalizeCurrency(key)] += value
	}
	return out, rows.Err()
}
func (c *Checker) sourceEmbyIDs(ctx context.Context) ([]string, error) {
	exists, err := c.tableExists(ctx, "emby")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := c.source.QueryContext(ctx, `SELECT DISTINCT embyid FROM emby WHERE embyid IS NOT NULL AND embyid<>'' ORDER BY embyid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func (c *Checker) sourceStrings(ctx context.Context, table, query string) ([]string, error) {
	exists, err := c.tableExists(ctx, table)
	if err != nil || !exists {
		return nil, err
	}
	rows, err := c.source.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

var tableDisposition = map[string]string{
	"alembic_version": "archive",
	"accounts":        "transform", "account_identities": "transform", "membership_plans": "transform", "account_memberships": "transform",
	"account_tags": "transform", "account_tag_assignments": "transform", "account_wallets": "transform", "emby": "transform",
	"managed_credentials": "transform", "emby_instances": "transform", "account_emby_bindings": "transform",
	"dynamic_settings": "transform", "audit_logs": "transform",
	"support_tickets": "transform", "ticket_messages": "transform",
	"user_notifications": "transform", "notification_preferences": "transform",
	"account_ledger_entries": "transform",
	"Rcode":                  "transform",
	"recharge_products":      "transform", "recharge_orders": "transform",
	"web_roles": "transform", "web_role_members": "transform",
	"idempotency_records": "archive", "job_runs": "archive", "system_events": "archive", "automation_runs": "archive",
	"line_health_samples": "archive", "service_probes": "archive", "alert_deliveries": "archive",
	"web_sessions": "invalidate", "web_login_requests": "invalidate", "worker_heartbeats": "invalidate", "registration_state": "invalidate",
	"emby_favorites": "rebuild", "media_catalog_items": "rebuild",
	"account_lifecycle_events": "archive", "emby2": "pending_adapter",
	"partition_codes": "pending_adapter", "partition_grants": "pending_adapter", "point_transactions": "archive",
	"billing_entries": "archive",
	"line_endpoints":  "pending_adapter", "playback_sessions": "pending_adapter", "known_devices": "pending_adapter",
	"device_client_rules": "pending_adapter", "security_events": "pending_adapter", "risk_rules": "pending_adapter",
	"media_requests": "pending_adapter", "request_records": "pending_adapter", "media_reviews": "pending_adapter",
	"review_reactions": "pending_adapter", "review_reports": "pending_adapter",
	"automation_rules": "pending_adapter", "operation_tasks": "pending_adapter",
	"config_revisions": "pending_adapter", "api_clients": "pending_adapter",
}

func (c *Checker) sourceTableCoverage(ctx context.Context) ([]TableCheck, error) {
	rows, err := c.source.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	out := make([]TableCheck, 0, len(names))
	for _, name := range names {
		var count int64
		quoted := "`" + strings.ReplaceAll(name, "`", "``") + "`"
		if err = c.source.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoted).Scan(&count); err != nil {
			return nil, fmt.Errorf("count source table %s: %w", name, err)
		}
		disposition := tableDisposition[name]
		if disposition == "" {
			disposition = "unknown"
		}
		out = append(out, TableCheck{Name: name, Rows: count, Disposition: disposition, Implemented: disposition == "transform"})
	}
	return out, nil
}
func queryTotals(ctx context.Context, db *pgxpool.Pool, query string) (map[string]int64, error) {
	out := map[string]int64{}
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value int64
		if err = rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}
func normalizeCurrency(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "coins", "points", "point":
		return "POINTS"
	case "registration_days", "days":
		return "REGISTRATION_DAYS"
	}
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 16 {
		value = value[:16]
	}
	return value
}
func positive(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}
