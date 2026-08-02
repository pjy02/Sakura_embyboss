package legacyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

type Report struct {
	Mode              string              `json:"mode"`
	CanonicalAccounts int                 `json:"canonical_accounts"`
	LegacyAccounts    int                 `json:"legacy_accounts"`
	Identities        int                 `json:"identities"`
	MembershipPlans   int                 `json:"membership_plans"`
	Memberships       int                 `json:"memberships"`
	Tags              int                 `json:"tags"`
	TagAssignments    int                 `json:"tag_assignments"`
	Wallets           int                 `json:"wallets"`
	WalletTotal       int64               `json:"wallet_total"`
	Credentials       int                 `json:"credentials"`
	EmbyInstances     int                 `json:"emby_instances"`
	EmbyBindings      int                 `json:"emby_bindings"`
	Settings          int                 `json:"settings"`
	AuditLogs         int                 `json:"audit_logs"`
	SupportTickets    int                 `json:"support_tickets"`
	TicketMessages    int                 `json:"ticket_messages"`
	Notifications     int                 `json:"notifications"`
	NotificationPrefs int                 `json:"notification_preferences"`
	LedgerEntries     int                 `json:"ledger_entries"`
	InvitationCodes   int                 `json:"invitation_codes"`
	RechargeProducts  int                 `json:"recharge_products"`
	RechargeOrders    int                 `json:"recharge_orders"`
	RoleMembers       int                 `json:"role_members"`
	TableImports      []TableImportReport `json:"table_imports"`
	Conflicts         []string            `json:"conflicts"`
	StartedAt         time.Time           `json:"started_at"`
	FinishedAt        time.Time           `json:"finished_at"`
}
type Importer struct {
	source             *sql.DB
	target             *pgxpool.Pool
	apply              bool
	v2CredentialMaster string
	v3Vault            *security.Vault
}

func New(sourceDSN string, target *pgxpool.Pool, apply bool) (*Importer, error) {
	source, err := sql.Open("mysql", sourceDSN)
	if err != nil {
		return nil, err
	}
	return &Importer{source: source, target: target, apply: apply}, nil
}
func (i *Importer) Close() { i.source.Close() }
func (i *Importer) ConfigureCredentials(v2Master, v3Master string) error {
	vault, err := security.NewVault(v3Master)
	if err != nil {
		return err
	}
	i.v2CredentialMaster, i.v3Vault = v2Master, vault
	return nil
}

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
type legacyPlan struct {
	ID, DurationDays, SortOrder, Revision int
	Code, Name                            string
	Description, Entitlements             sql.NullString
	Enabled, IsDefault                    bool
}
type legacyMembership struct {
	ID, AccountID, Source, CreatedByKind, CreatedByID string
	PlanID                                            int
	Status                                            string
	Starts, Created, Updated                          time.Time
	Expires                                           sql.NullTime
}
type legacyTag struct {
	ID          int
	Name, Color string
	Description sql.NullString
}
type legacyTagAssignment struct {
	AccountID string
	TagID     int
}
type legacyWallet struct {
	AccountID, BalanceType string
	Balance                int64
}
type legacyLedger struct {
	ID                                                 int64
	SourceTransactionID                                sql.NullInt64
	AccountID, BalanceType, Reason, ActorKind, ActorID string
	Amount                                             int64
	Created                                            time.Time
}
type legacyInvite struct {
	Code            string
	IssuerTG        int64
	Days            int
	UsedTG          sql.NullInt64
	UsedAt, Expires sql.NullTime
	Status          sql.NullString
	IssuerAccount   sql.NullString
}
type legacyRechargeProduct struct {
	ID                                  int
	Name                                string
	Description                         sql.NullString
	Price, Coins, Bonus, Sort, Revision int64
	Enabled                             bool
	Created, Updated                    time.Time
}
type legacyRechargeOrder struct {
	ID, Number, ProductName, Method, Status string
	TG                                      int64
	ProductID                               sql.NullInt64
	Reference                               sql.NullString
	Price, Coins, Bonus                     int64
	Paid                                    sql.NullTime
	Created, Updated                        time.Time
}
type legacyRoleMember struct {
	RoleName string
	TG       int64
}
type legacyCredential struct {
	ID, Name, Provider, Kind, Ciphertext string
	Metadata                             sql.NullString
	Active                               bool
	Created, Updated                     time.Time
}
type legacyEmbyInstance struct {
	ID, Name, BaseURL, CredentialID, Status string
	Enabled, IsDefault, VerifyTLS           bool
	Priority                                int
	Created, Updated                        time.Time
}
type legacyEmbyBinding struct {
	ID, AccountID, InstanceID, RemoteUserID, Username, Status string
	IsPrimary                                                 bool
	Expires, LastSynced                                       sql.NullTime
	Created, Updated                                          time.Time
}
type legacySetting struct {
	Key, Value, ValueType, UpdatedByKind, UpdatedByID string
	Secret                                            bool
	Revision                                          int
	Updated                                           time.Time
}
type legacyAudit struct {
	ID                                                  int64
	RequestID, ActorName, ResourceID, Detail, IPAddress sql.NullString
	ActorKind, ActorID, Action, ResourceType, Outcome   string
	Created                                             time.Time
}
type legacyTicket struct {
	ID, Number, Subject, Category, Priority, Status string
	TG                                              int64
	Assignee                                        sql.NullInt64
	Resolved, Closed                                sql.NullTime
	Created, Updated                                time.Time
}
type legacyTicketMessage struct {
	ID                         int64
	TicketID, SenderKind, Body string
	SenderTG                   sql.NullInt64
	Internal                   bool
	Created                    time.Time
}
type legacyNotification struct {
	ID, Category, Title, Body, Severity string
	TG                                  int64
	ActionURL, Metadata                 sql.NullString
	ReadAt                              sql.NullTime
	Created                             time.Time
}
type legacyNotificationPreference struct {
	TG            int64
	Category      string
	Web, Telegram bool
	Updated       time.Time
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
	plans, err := i.readPlans(ctx)
	if err != nil {
		return report, fmt.Errorf("read membership plans: %w", err)
	}
	memberships, err := i.readMemberships(ctx)
	if err != nil {
		return report, fmt.Errorf("read memberships: %w", err)
	}
	tags, err := i.readTags(ctx)
	if err != nil {
		return report, fmt.Errorf("read tags: %w", err)
	}
	assignments, err := i.readTagAssignments(ctx)
	if err != nil {
		return report, fmt.Errorf("read tag assignments: %w", err)
	}
	wallets, err := i.readWallets(ctx)
	if err != nil {
		return report, fmt.Errorf("read wallets: %w", err)
	}
	ledgerEntries, err := i.readLedgerEntries(ctx)
	if err != nil {
		return report, fmt.Errorf("read account ledger: %w", err)
	}
	invites, err := i.readInvites(ctx)
	if err != nil {
		return report, fmt.Errorf("read invitation codes: %w", err)
	}
	rechargeProducts, err := i.readRechargeProducts(ctx)
	if err != nil {
		return report, fmt.Errorf("read recharge products: %w", err)
	}
	rechargeOrders, err := i.readRechargeOrders(ctx)
	if err != nil {
		return report, fmt.Errorf("read recharge orders: %w", err)
	}
	roleMembers, err := i.readRoleMembers(ctx)
	if err != nil {
		return report, fmt.Errorf("read role members: %w", err)
	}
	credentials, err := i.readCredentials(ctx)
	if err != nil {
		return report, fmt.Errorf("read managed credentials: %w", err)
	}
	instances, err := i.readEmbyInstances(ctx)
	if err != nil {
		return report, fmt.Errorf("read Emby instances: %w", err)
	}
	bindings, err := i.readEmbyBindings(ctx)
	if err != nil {
		return report, fmt.Errorf("read Emby bindings: %w", err)
	}
	settings, err := i.readSettings(ctx)
	if err != nil {
		return report, fmt.Errorf("read dynamic settings: %w", err)
	}
	auditLogs, err := i.readAuditLogs(ctx)
	if err != nil {
		return report, fmt.Errorf("read audit logs: %w", err)
	}
	tickets, err := i.readTickets(ctx)
	if err != nil {
		return report, fmt.Errorf("read support tickets: %w", err)
	}
	ticketMessages, err := i.readTicketMessages(ctx)
	if err != nil {
		return report, fmt.Errorf("read ticket messages: %w", err)
	}
	notifications, err := i.readNotifications(ctx)
	if err != nil {
		return report, fmt.Errorf("read notifications: %w", err)
	}
	preferences, err := i.readNotificationPreferences(ctx)
	if err != nil {
		return report, fmt.Errorf("read notification preferences: %w", err)
	}
	adapterRows, err := i.readAdapterTables(ctx)
	if err != nil {
		return report, fmt.Errorf("read legacy domain tables: %w", err)
	}
	report.CanonicalAccounts = len(accounts)
	report.LegacyAccounts = len(legacy)
	report.Identities = len(identities)
	report.MembershipPlans, report.Memberships = len(plans), len(memberships)
	report.Tags, report.TagAssignments, report.Wallets = len(tags), len(assignments), len(wallets)
	for _, wallet := range wallets {
		report.WalletTotal += wallet.Balance
	}
	report.Credentials, report.EmbyInstances, report.EmbyBindings = len(credentials), len(instances), len(bindings)
	report.Settings, report.AuditLogs = len(settings), len(auditLogs)
	report.SupportTickets, report.TicketMessages = len(tickets), len(ticketMessages)
	report.Notifications, report.NotificationPrefs = len(notifications), len(preferences)
	report.LedgerEntries = len(ledgerEntries)
	report.InvitationCodes = len(invites)
	report.RechargeProducts, report.RechargeOrders = len(rechargeProducts), len(rechargeOrders)
	report.RoleMembers = len(roleMembers)
	report.TableImports = summarizeAdapterRows(adapterRows)
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
	credentialNames := map[string]string{}
	if len(credentials) > 0 && (i.v2CredentialMaster == "" || i.v3Vault == nil) {
		return report, fmt.Errorf("SAKURA_V2_CREDENTIAL_MASTER_KEY and SAKURA_V3_CREDENTIAL_MASTER_KEY are required to re-encrypt managed credentials")
	}
	for _, item := range credentials {
		plaintext, decryptErr := decryptV2Credential(i.v2CredentialMaster, item.Ciphertext)
		if decryptErr != nil {
			return report, fmt.Errorf("decrypt credential %s: %w", item.Name, decryptErr)
		}
		ciphertext, nonce, keyVersion, encryptErr := i.v3Vault.Encrypt(plaintext)
		for index := range plaintext {
			plaintext[index] = 0
		}
		if encryptErr != nil {
			return report, encryptErr
		}
		credentialID := parseOrDeterministicUUID(item.ID, "sakura-v2-credential:")
		metadata := map[string]any{"source": "v2", "provider": item.Provider, "active": item.Active}
		if item.Metadata.Valid && json.Valid([]byte(item.Metadata.String)) {
			var legacyMetadata any
			_ = json.Unmarshal([]byte(item.Metadata.String), &legacyMetadata)
			metadata["legacy_metadata"] = legacyMetadata
		}
		if _, err = tx.Exec(ctx, `INSERT INTO credentials(id,name,kind,ciphertext,nonce,key_version,metadata,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,'system:legacy-import',$8,$9) ON CONFLICT(name) DO NOTHING`, credentialID, item.Name, item.Kind, ciphertext, nonce, keyVersion, metadata, item.Created, item.Updated); err != nil {
			return report, err
		}
		credentialNames[item.ID] = item.Name
	}
	instanceMap := map[string]uuid.UUID{}
	for _, item := range instances {
		credentialName, ok := credentialNames[item.CredentialID]
		if !ok {
			report.Conflicts = append(report.Conflicts, "Emby instance credential missing: "+item.ID)
			continue
		}
		instanceID := parseOrDeterministicUUID(item.ID, "sakura-v2-emby-instance:")
		status := normalizeInstanceStatus(item.Status, item.Enabled)
		_, err = tx.Exec(ctx, `INSERT INTO emby_instances(id,name,base_url,credential_name,enabled,is_default,verify_tls,priority,status,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,$10,$11) ON CONFLICT DO NOTHING`, instanceID, item.Name, item.BaseURL, credentialName, item.Enabled, item.IsDefault, item.VerifyTLS, item.Priority, status, item.Created, item.Updated)
		if err != nil {
			return report, err
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM emby_instances WHERE name=$1`, item.Name).Scan(&instanceID); err != nil {
			return report, err
		}
		instanceMap[item.ID] = instanceID
	}
	for _, item := range bindings {
		accountID, accountOK := idMap[item.AccountID]
		instanceID, instanceOK := instanceMap[item.InstanceID]
		if !accountOK || !instanceOK {
			report.Conflicts = append(report.Conflicts, "Emby binding dependency missing: "+item.ID)
			continue
		}
		bindingID := parseOrDeterministicUUID(item.ID, "sakura-v2-emby-binding:")
		_, err = tx.Exec(ctx, `INSERT INTO emby_account_bindings(id,account_id,instance_id,remote_user_id,remote_username,status,origin,is_primary,expires_at,last_synced_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'legacy_import',$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`, bindingID, accountID, instanceID, item.RemoteUserID, item.Username, normalizeBindingStatus(item.Status), item.IsPrimary, nullTime(item.Expires), nullTime(item.LastSynced), item.Created, item.Updated)
		if err != nil {
			return report, err
		}
	}
	for _, item := range settings {
		if item.Secret {
			report.Conflicts = append(report.Conflicts, "secret dynamic setting requires credential-center migration: "+item.Key)
			continue
		}
		raw := json.RawMessage(item.Value)
		if !json.Valid(raw) {
			report.Conflicts = append(report.Conflicts, "invalid setting JSON: "+item.Key)
			continue
		}
		valueType := normalizeSettingType(item.ValueType, raw)
		actor := fallback(item.UpdatedByKind, "system") + ":" + fallback(item.UpdatedByID, "legacy-import")
		revision := max(item.Revision, 1)
		_, err = tx.Exec(ctx, `INSERT INTO dynamic_settings(key,value,value_type,revision,updated_by,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,value_type=EXCLUDED.value_type,revision=GREATEST(dynamic_settings.revision,EXCLUDED.revision),updated_by=EXCLUDED.updated_by,updated_at=EXCLUDED.updated_at`, item.Key, raw, valueType, revision, actor, item.Updated)
		if err != nil {
			return report, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason,created_at) VALUES($1,$2,$3,$4,$5,'v2 final import',$6) ON CONFLICT(setting_key,revision) DO NOTHING`, item.Key, revision, raw, valueType, actor, item.Updated)
		if err != nil {
			return report, err
		}
	}
	for _, item := range auditLogs {
		requestID := fmt.Sprintf("legacy-v2-audit-%d", item.ID)
		details := map[string]any{"source": "v2", "outcome": item.Outcome}
		if item.RequestID.Valid {
			details["legacy_request_id"] = item.RequestID.String
		}
		if item.ActorName.Valid {
			details["actor_name"] = item.ActorName.String
		}
		if item.Detail.Valid && json.Valid([]byte(item.Detail.String)) {
			var detail any
			_ = json.Unmarshal([]byte(item.Detail.String), &detail)
			details["detail"] = detail
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,ip_address,details,created_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9 WHERE NOT EXISTS(SELECT 1 FROM audit_logs WHERE request_id=$6)`, item.ActorKind, item.ActorID, item.Action, item.ResourceType, nullString(item.ResourceID), requestID, nullString(item.IPAddress), details, item.Created)
		if err != nil {
			return report, err
		}
	}
	for _, item := range roleMembers {
		accountID, ok := tgMap[item.TG]
		if !ok {
			report.Conflicts = append(report.Conflicts, fmt.Sprintf("role member account missing: %d", item.TG))
			continue
		}
		var roleID uuid.UUID
		switch strings.ToLower(item.RoleName) {
		case "owner":
			roleID = uuid.MustParse("00000000-0000-4000-8000-000000000001")
		case "admin", "administrator":
			roleID = uuid.MustParse("00000000-0000-4000-8000-000000000002")
		case "user":
			roleID = uuid.MustParse("00000000-0000-4000-8000-000000000003")
		default:
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,$2,'system:legacy-import') ON CONFLICT DO NOTHING`, accountID, roleID); err != nil {
			return report, err
		}
	}
	for _, item := range tickets {
		accountID, ok := tgMap[item.TG]
		if !ok {
			report.Conflicts = append(report.Conflicts, "ticket account missing: "+item.ID)
			continue
		}
		var assigned any
		if item.Assignee.Valid {
			if value, found := tgMap[item.Assignee.Int64]; found {
				assigned = value
			}
		}
		ticketID := parseOrDeterministicUUID(item.ID, "sakura-v2-ticket:")
		_, err = tx.Exec(ctx, `INSERT INTO support_tickets(id,ticket_no,account_id,subject,category,priority,status,assigned_to,last_public_reply_at,resolved_at,closed_at,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,$12,$13) ON CONFLICT(ticket_no) DO NOTHING`, ticketID, item.Number, accountID, item.Subject, item.Category, normalizeTicketPriority(item.Priority), normalizeTicketStatus(item.Status), assigned, item.Updated, nullTime(item.Resolved), nullTime(item.Closed), item.Created, item.Updated)
		if err != nil {
			return report, err
		}
	}
	for _, item := range ticketMessages {
		ticketID := parseOrDeterministicUUID(item.TicketID, "sakura-v2-ticket:")
		messageID := deterministicUUID(fmt.Sprintf("sakura-v2-ticket-message:%d", item.ID))
		var author any
		label := item.SenderKind
		if item.SenderTG.Valid {
			label = fmt.Sprintf("%s:%d", item.SenderKind, item.SenderTG.Int64)
			if value, ok := tgMap[item.SenderTG.Int64]; ok {
				author = value
			}
		}
		attachments := []map[string]any{{"source": "v2", "legacy_id": item.ID}}
		_, err = tx.Exec(ctx, `INSERT INTO ticket_messages(id,ticket_id,author_account_id,author_label,body,is_internal,attachments,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO NOTHING`, messageID, ticketID, author, label, item.Body, item.Internal, attachments, item.Created)
		if err != nil {
			return report, err
		}
	}
	for _, item := range notifications {
		accountID, ok := tgMap[item.TG]
		if !ok {
			report.Conflicts = append(report.Conflicts, "notification account missing: "+item.ID)
			continue
		}
		notificationID := parseOrDeterministicUUID(item.ID, "sakura-v2-notification:")
		status := "unread"
		var readAt any
		if item.ReadAt.Valid {
			status = "read"
			readAt = item.ReadAt.Time
		}
		metadata := map[string]any{"source": "v2", "category": item.Category, "severity": item.Severity}
		if item.ActionURL.Valid {
			metadata["action_url"] = item.ActionURL.String
		}
		if item.Metadata.Valid && json.Valid([]byte(item.Metadata.String)) {
			var value any
			_ = json.Unmarshal([]byte(item.Metadata.String), &value)
			metadata["legacy_metadata"] = value
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_notifications(id,account_id,title,body,channel,status,delivery_status,delivered_at,metadata,created_at,read_at) VALUES($1,$2,$3,$4,'in_app',$5,'sent',$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, notificationID, accountID, item.Title, item.Body, status, item.Created, metadata, item.Created, readAt)
		if err != nil {
			return report, err
		}
	}
	for _, item := range preferences {
		accountID, ok := tgMap[item.TG]
		if !ok {
			continue
		}
		for _, channel := range []struct {
			name    string
			enabled bool
		}{{"in_app", item.Web}, {"telegram", item.Telegram}} {
			_, err = tx.Exec(ctx, `INSERT INTO notification_preferences(account_id,event_key,channel,enabled,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id,event_key,channel) DO UPDATE SET enabled=EXCLUDED.enabled,updated_at=EXCLUDED.updated_at`, accountID, item.Category, channel.name, channel.enabled, item.Updated)
			if err != nil {
				return report, err
			}
		}
	}
	planMap := map[int]uuid.UUID{}
	for _, plan := range plans {
		planID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("sakura-v2-plan:%d", plan.ID)))
		planMap[plan.ID] = planID
		planCode := legacyPlanCode(plan.ID, plan.Code)
		days := plan.DurationDays
		if days < 1 {
			days = 1
		}
		if days > 3650 {
			days = 3650
		}
		entitlements := json.RawMessage("{}")
		if plan.Entitlements.Valid && json.Valid([]byte(plan.Entitlements.String)) {
			entitlements = json.RawMessage(plan.Entitlements.String)
		}
		_, err = tx.Exec(ctx, `INSERT INTO membership_plans(id,code,name,description,duration_days,entitlements,enabled,is_default,sort_order,revision)
			VALUES($1,$2,$3,$4,$5,$6,$7,FALSE,$8,$9) ON CONFLICT(code) DO NOTHING`, planID, planCode, plan.Name, nullString(plan.Description), days, entitlements, plan.Enabled, plan.SortOrder, max(plan.Revision, 1))
		if err != nil {
			return report, err
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM membership_plans WHERE code=$1`, planCode).Scan(&planID); err != nil {
			return report, err
		}
		planMap[plan.ID] = planID
	}
	invitePlanMap := map[int]uuid.UUID{}
	for _, item := range invites {
		days := item.Days
		if days < 1 {
			days = 1
		}
		if days > 3650 {
			days = 3650
		}
		planID, ok := invitePlanMap[days]
		if !ok {
			code := fmt.Sprintf("v2-invite-%d-days", days)
			planID = deterministicUUID("sakura-v2-invite-plan:" + code)
			_, err = tx.Exec(ctx, `INSERT INTO membership_plans(id,code,name,duration_days,entitlements,enabled,is_default,sort_order,revision) VALUES($1,$2,$3,$4,'{}',TRUE,FALSE,900,1) ON CONFLICT(code) DO NOTHING`, planID, code, fmt.Sprintf("v2 邀请码 %d 天", days), days)
			if err != nil {
				return report, err
			}
			if err = tx.QueryRow(ctx, `SELECT id FROM membership_plans WHERE code=$1`, code).Scan(&planID); err != nil {
				return report, err
			}
			invitePlanMap[days] = planID
		}
		used := 0
		if item.UsedTG.Valid {
			used = 1
		}
		status := normalizeInviteStatus(item.Status, item.UsedTG, item.Expires)
		issuer := fmt.Sprintf("legacy:telegram:%d", item.IssuerTG)
		if item.IssuerAccount.Valid {
			issuer = "legacy:account:" + item.IssuerAccount.String
		}
		hint := item.Code
		if len(hint) > 8 {
			hint = hint[len(hint)-8:]
		}
		inviteID := deterministicUUID("sakura-v2-invite:" + item.Code)
		metadata := map[string]any{"source": "v2-rcode", "legacy_days": item.Days}
		if item.UsedTG.Valid {
			metadata["used_tg"] = item.UsedTG.Int64
		}
		_, err = tx.Exec(ctx, `INSERT INTO invitation_codes(id,code_hash,code_prefix,code_hint,kind,plan_id,max_uses,used_count,status,expires_at,issued_by,metadata) VALUES($1,$2,'V2',$3,'registration',$4,1,$5,$6,$7,$8,$9) ON CONFLICT(code_hash) DO NOTHING`, inviteID, security.HashToken(item.Code), hint, planID, used, status, nullTime(item.Expires), issuer, metadata)
		if err != nil {
			return report, err
		}
	}
	productMap := map[int64]uuid.UUID{}
	for _, item := range rechargeProducts {
		productID := deterministicUUID(fmt.Sprintf("sakura-v2-recharge-product:%d", item.ID))
		code := fmt.Sprintf("v2-product-%d", item.ID)
		_, err = tx.Exec(ctx, `INSERT INTO recharge_products(id,code,name,description,price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order,revision,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'CNY',$6,'POINTS',$7,$8,$9,$10,$11) ON CONFLICT(code) DO NOTHING`, productID, code, item.Name, nullString(item.Description), positiveOrOne(item.Price), positiveOrOne(item.Coins+item.Bonus), item.Enabled, item.Sort, max(int(item.Revision), 1), item.Created, item.Updated)
		if err != nil {
			return report, err
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM recharge_products WHERE code=$1`, code).Scan(&productID); err != nil {
			return report, err
		}
		productMap[int64(item.ID)] = productID
	}
	placeholderMap := map[string]uuid.UUID{}
	for _, item := range rechargeOrders {
		accountID, ok := tgMap[item.TG]
		if !ok {
			report.Conflicts = append(report.Conflicts, "recharge order account missing: "+item.ID)
			continue
		}
		var productID uuid.UUID
		if item.ProductID.Valid {
			productID = productMap[item.ProductID.Int64]
		}
		if productID == uuid.Nil {
			key := fmt.Sprintf("%s:%d:%d", item.ProductName, item.Price, item.Coins+item.Bonus)
			productID = placeholderMap[key]
			if productID == uuid.Nil {
				productID = deterministicUUID("sakura-v2-recharge-placeholder:" + key)
				code := "v2-order-product-" + strings.ReplaceAll(productID.String(), "-", "")[:20]
				_, err = tx.Exec(ctx, `INSERT INTO recharge_products(id,code,name,price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order,revision) VALUES($1,$2,$3,$4,'CNY',$5,'POINTS',FALSE,9999,1) ON CONFLICT(code) DO NOTHING`, productID, code, item.ProductName, positiveOrOne(item.Price), positiveOrOne(item.Coins+item.Bonus))
				if err != nil {
					return report, err
				}
				placeholderMap[key] = productID
			}
		}
		status := normalizeRechargeStatus(item.Status, item.Paid, item.Created)
		expires := item.Created.Add(30 * time.Minute)
		var paidAmount any
		if status == "paid" || status == "partially_refunded" || status == "refunded" {
			paidAmount = item.Price
		}
		orderID := parseOrDeterministicUUID(item.ID, "sakura-v2-recharge-order:")
		provider := normalizeProvider(item.Method)
		_, err = tx.Exec(ctx, `INSERT INTO recharge_orders(id,order_no,idempotency_key,account_id,product_id,provider,external_order_id,status,price_minor,payment_currency,grant_amount,wallet_currency,paid_amount_minor,expires_at,paid_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'CNY',$10,'POINTS',$11,$12,$13,$14,$15) ON CONFLICT DO NOTHING`, orderID, item.Number, "legacy-recharge:"+item.Number, accountID, productID, provider, nullString(item.Reference), status, item.Price, item.Coins+item.Bonus, paidAmount, expires, nullTime(item.Paid), item.Created, item.Updated)
		if err != nil {
			return report, err
		}
	}
	for _, item := range memberships {
		accountID, ok := idMap[item.AccountID]
		if !ok {
			continue
		}
		planID, ok := planMap[item.PlanID]
		if !ok {
			report.Conflicts = append(report.Conflicts, "membership plan missing: "+item.ID)
			continue
		}
		membershipID := deterministicUUID("sakura-v2-membership:" + item.ID)
		expires := item.Expires.Time
		if !item.Expires.Valid {
			expires = item.Starts.AddDate(0, 0, 3650)
		}
		status := normalizeMembershipStatus(item.Status, expires)
		_, err = tx.Exec(ctx, `INSERT INTO account_memberships(id,account_id,plan_id,status,starts_at,expires_at,source,source_ref,created_by,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,'legacy_import',$7,$8,$9,$10) ON CONFLICT DO NOTHING`, membershipID, accountID, planID, status, item.Starts, expires, item.ID, "system:legacy-import", item.Created, item.Updated)
		if err != nil {
			return report, err
		}
	}
	tagMap := map[int]uuid.UUID{}
	for _, tag := range tags {
		tagID := deterministicUUID(fmt.Sprintf("sakura-v2-tag:%d", tag.ID))
		tagMap[tag.ID] = tagID
		code := fmt.Sprintf("v2-tag-%d", tag.ID)
		_, err = tx.Exec(ctx, `INSERT INTO account_tags(id,code,name,color,description,created_by) VALUES($1,$2,$3,$4,$5,'system:legacy-import') ON CONFLICT(code) DO NOTHING`, tagID, code, tag.Name, fallback(tag.Color, "#8b7cf6"), nullString(tag.Description))
		if err != nil {
			return report, err
		}
	}
	for _, item := range assignments {
		accountID, accountOK := idMap[item.AccountID]
		tagID, tagOK := tagMap[item.TagID]
		if !accountOK || !tagOK {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_tag_assignments(account_id,tag_id,assigned_by) VALUES($1,$2,'system:legacy-import') ON CONFLICT DO NOTHING`, accountID, tagID); err != nil {
			return report, err
		}
	}
	for _, entry := range ledgerEntries {
		accountID, ok := idMap[entry.AccountID]
		if !ok {
			report.Conflicts = append(report.Conflicts, fmt.Sprintf("ledger account missing for entry %d", entry.ID))
			continue
		}
		if err = importLegacyLedger(ctx, tx, accountID, normalizeCurrency(entry.BalanceType), entry); err != nil {
			return report, err
		}
	}
	for _, wallet := range wallets {
		accountID, ok := idMap[wallet.AccountID]
		if !ok {
			report.Conflicts = append(report.Conflicts, "wallet account missing: "+wallet.AccountID)
			continue
		}
		if wallet.Balance < 0 {
			report.Conflicts = append(report.Conflicts, "negative wallet balance requires manual decision: "+wallet.AccountID+"/"+wallet.BalanceType)
			continue
		}
		if err = importWallet(ctx, tx, accountID, normalizeCurrency(wallet.BalanceType), wallet.Balance); err != nil {
			return report, err
		}
	}
	report.TableImports, err = i.importAdapterTables(ctx, tx, runID, adapterRows, tgMap, idMap, instanceMap)
	if err != nil {
		return report, err
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

type pgTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func importWallet(ctx context.Context, tx pgTx, accountID uuid.UUID, currency string, balance int64) error {
	systemID, walletLedgerID, err := ensureWalletAccounts(ctx, tx, accountID, currency)
	if err != nil {
		return err
	}
	// A later import may legitimately reduce a previously imported balance to
	// zero, so always calculate the delta before returning.
	var imported int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN e.side='credit' THEN e.amount ELSE -e.amount END),0)
		FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id=t.id
		WHERE t.status='posted' AND t.reference_type='legacy_wallet' AND t.reference_id=$1 AND t.currency=$2 AND e.ledger_account_id=$3`, accountID.String(), currency, walletLedgerID).Scan(&imported); err != nil {
		return err
	}
	delta := balance - imported
	if delta == 0 {
		return nil
	}
	key := fmt.Sprintf("legacy-wallet:%s:%s:balance:%d", accountID.String(), currency, balance)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ledger_transactions WHERE idempotency_key=$1)`, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	txnID := deterministicUUID("sakura-v3-" + key)
	transactionNo := "MIG-" + strings.ToUpper(strings.ReplaceAll(txnID.String(), "-", ""))
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_transactions(id,transaction_no,idempotency_key,kind,currency,status,reference_type,reference_id,description,actor) VALUES($1,$2,$3,'admin_adjustment',$4,'draft','legacy_wallet',$5,'v2 final balance import','system:legacy-import')`, txnID, transactionNo, key, currency, accountID.String()); err != nil {
		return err
	}
	amount := delta
	systemSide, walletSide := "debit", "credit"
	if amount < 0 {
		amount = -amount
		systemSide, walletSide = "credit", "debit"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,ledger_account_id,side,amount) VALUES($1,$2,$4,$6),($1,$3,$5,$6)`, txnID, systemID, walletLedgerID, systemSide, walletSide, amount); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE ledger_transactions SET status='posted',posted_at=NOW() WHERE id=$1`, txnID)
	return err
}

func ensureWalletAccounts(ctx context.Context, tx pgTx, accountID uuid.UUID, currency string) (uuid.UUID, uuid.UUID, error) {
	systemID := deterministicUUID("sakura-v3-migration-clearing:" + currency)
	walletLedgerID := deterministicUUID("sakura-v3-wallet-ledger:" + accountID.String() + ":" + currency)
	walletID := deterministicUUID("sakura-v3-wallet:" + accountID.String() + ":" + currency)
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_accounts(id,owner_type,owner_id,code,currency,normal_side) VALUES($1,'system','system','legacy_migration',$2,'debit') ON CONFLICT(owner_type,owner_id,code,currency) DO NOTHING`, systemID, currency); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_type='system' AND owner_id='system' AND code='legacy_migration' AND currency=$1`, currency).Scan(&systemID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_accounts(id,owner_type,owner_id,code,currency,normal_side) VALUES($1,'account',$2,'wallet',$3,'credit') ON CONFLICT(owner_type,owner_id,code,currency) DO NOTHING`, walletLedgerID, accountID.String(), currency); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM ledger_accounts WHERE owner_type='account' AND owner_id=$1 AND code='wallet' AND currency=$2`, accountID.String(), currency).Scan(&walletLedgerID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO wallets(id,account_id,ledger_account_id,currency) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,currency) DO NOTHING`, walletID, accountID, walletLedgerID, currency); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return systemID, walletLedgerID, nil
}

func importLegacyLedger(ctx context.Context, tx pgTx, accountID uuid.UUID, currency string, entry legacyLedger) error {
	if entry.Amount == 0 {
		return nil
	}
	systemID, walletID, err := ensureWalletAccounts(ctx, tx, accountID, currency)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("legacy-ledger:account_ledger_entries:%d", entry.ID)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ledger_transactions WHERE idempotency_key=$1)`, key).Scan(&exists); err != nil || exists {
		return err
	}
	txnID := deterministicUUID("sakura-v3-" + key)
	number := "MIG-H-" + strings.ToUpper(strings.ReplaceAll(txnID.String(), "-", ""))
	amount := entry.Amount
	systemSide, walletSide := "debit", "credit"
	if amount < 0 {
		amount = -amount
		systemSide, walletSide = "credit", "debit"
	}
	metadata := map[string]any{"source": "v2", "legacy_entry_id": entry.ID, "legacy_actor": entry.ActorKind + ":" + entry.ActorID}
	if entry.SourceTransactionID.Valid {
		metadata["legacy_source_transaction_id"] = entry.SourceTransactionID.Int64
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_transactions(id,transaction_no,idempotency_key,kind,currency,status,reference_type,reference_id,description,metadata,actor,created_at) VALUES($1,$2,$3,'admin_adjustment',$4,'draft','legacy_wallet',$5,$6,$7,$8,$9)`, txnID, number, key, currency, accountID.String(), entry.Reason, metadata, "legacy:"+entry.ActorKind+":"+entry.ActorID, entry.Created); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,ledger_account_id,side,amount,created_at) VALUES($1,$2,$4,$6,$7),($1,$3,$5,$6,$7)`, txnID, systemID, walletID, systemSide, walletSide, amount, entry.Created); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE ledger_transactions SET status='posted',posted_at=$2 WHERE id=$1`, txnID, entry.Created)
	return err
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

func (i *Importer) readPlans(ctx context.Context) ([]legacyPlan, error) {
	exists, err := i.tableExists(ctx, "membership_plans")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,code,name,description,duration_days,entitlements_json,enabled,is_default,sort_order,revision FROM membership_plans`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyPlan
	for rows.Next() {
		var x legacyPlan
		if err = rows.Scan(&x.ID, &x.Code, &x.Name, &x.Description, &x.DurationDays, &x.Entitlements, &x.Enabled, &x.IsDefault, &x.SortOrder, &x.Revision); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readMemberships(ctx context.Context) ([]legacyMembership, error) {
	exists, err := i.tableExists(ctx, "account_memberships")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,account_id,plan_id,status,starts_at,expires_at,source,created_by_kind,created_by_id,created_at,updated_at FROM account_memberships`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyMembership
	for rows.Next() {
		var x legacyMembership
		if err = rows.Scan(&x.ID, &x.AccountID, &x.PlanID, &x.Status, &x.Starts, &x.Expires, &x.Source, &x.CreatedByKind, &x.CreatedByID, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readTags(ctx context.Context) ([]legacyTag, error) {
	exists, err := i.tableExists(ctx, "account_tags")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,name,color,description FROM account_tags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyTag
	for rows.Next() {
		var x legacyTag
		if err = rows.Scan(&x.ID, &x.Name, &x.Color, &x.Description); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readTagAssignments(ctx context.Context) ([]legacyTagAssignment, error) {
	exists, err := i.tableExists(ctx, "account_tag_assignments")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT account_id,tag_id FROM account_tag_assignments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyTagAssignment
	for rows.Next() {
		var x legacyTagAssignment
		if err = rows.Scan(&x.AccountID, &x.TagID); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readWallets(ctx context.Context) ([]legacyWallet, error) {
	exists, err := i.tableExists(ctx, "account_wallets")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT account_id,balance_type,balance FROM account_wallets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyWallet
	for rows.Next() {
		var x legacyWallet
		if err = rows.Scan(&x.AccountID, &x.BalanceType, &x.Balance); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readLedgerEntries(ctx context.Context) ([]legacyLedger, error) {
	exists, err := i.tableExists(ctx, "account_ledger_entries")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,source_transaction_id,account_id,balance_type,amount,reason,actor_kind,actor_id,created_at FROM account_ledger_entries ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyLedger
	for rows.Next() {
		var x legacyLedger
		if err = rows.Scan(&x.ID, &x.SourceTransactionID, &x.AccountID, &x.BalanceType, &x.Amount, &x.Reason, &x.ActorKind, &x.ActorID, &x.Created); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readInvites(ctx context.Context) ([]legacyInvite, error) {
	exists, err := i.tableExists(ctx, "Rcode")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, "SELECT code,COALESCE(tg,0),COALESCE(us,1),used,usedtime,expires_at,status,issuer_account_id FROM `Rcode`")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyInvite
	for rows.Next() {
		var x legacyInvite
		if err = rows.Scan(&x.Code, &x.IssuerTG, &x.Days, &x.UsedTG, &x.UsedAt, &x.Expires, &x.Status, &x.IssuerAccount); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readRechargeProducts(ctx context.Context) ([]legacyRechargeProduct, error) {
	exists, err := i.tableExists(ctx, "recharge_products")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,name,description,amount_cents,coins,bonus_coins,enabled,sort_order,revision,created_at,updated_at FROM recharge_products`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyRechargeProduct
	for rows.Next() {
		var x legacyRechargeProduct
		if err = rows.Scan(&x.ID, &x.Name, &x.Description, &x.Price, &x.Coins, &x.Bonus, &x.Enabled, &x.Sort, &x.Revision, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readRechargeOrders(ctx context.Context) ([]legacyRechargeOrder, error) {
	exists, err := i.tableExists(ctx, "recharge_orders")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,order_no,tg,product_id,product_name,amount_cents,coins,bonus_coins,payment_method,payment_reference,status,paid_at,created_at,updated_at FROM recharge_orders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyRechargeOrder
	for rows.Next() {
		var x legacyRechargeOrder
		if err = rows.Scan(&x.ID, &x.Number, &x.TG, &x.ProductID, &x.ProductName, &x.Price, &x.Coins, &x.Bonus, &x.Method, &x.Reference, &x.Status, &x.Paid, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readRoleMembers(ctx context.Context) ([]legacyRoleMember, error) {
	roles, err := i.tableExists(ctx, "web_roles")
	if err != nil || !roles {
		return nil, err
	}
	members, err := i.tableExists(ctx, "web_role_members")
	if err != nil || !members {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT r.name,m.tg FROM web_role_members m JOIN web_roles r ON r.id=m.role_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyRoleMember
	for rows.Next() {
		var x legacyRoleMember
		if err = rows.Scan(&x.RoleName, &x.TG); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readCredentials(ctx context.Context) ([]legacyCredential, error) {
	exists, err := i.tableExists(ctx, "managed_credentials")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,name,provider,credential_type,ciphertext,metadata_json,active,created_at,updated_at FROM managed_credentials`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyCredential
	for rows.Next() {
		var x legacyCredential
		if err = rows.Scan(&x.ID, &x.Name, &x.Provider, &x.Kind, &x.Ciphertext, &x.Metadata, &x.Active, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readEmbyInstances(ctx context.Context) ([]legacyEmbyInstance, error) {
	exists, err := i.tableExists(ctx, "emby_instances")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,name,base_url,credential_id,enabled,is_default,verify_tls,priority,status,created_at,updated_at FROM emby_instances`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyEmbyInstance
	for rows.Next() {
		var x legacyEmbyInstance
		if err = rows.Scan(&x.ID, &x.Name, &x.BaseURL, &x.CredentialID, &x.Enabled, &x.IsDefault, &x.VerifyTLS, &x.Priority, &x.Status, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readEmbyBindings(ctx context.Context) ([]legacyEmbyBinding, error) {
	exists, err := i.tableExists(ctx, "account_emby_bindings")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,account_id,instance_id,emby_user_id,emby_username,status,is_primary,expires_at,last_synced_at,created_at,updated_at FROM account_emby_bindings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyEmbyBinding
	for rows.Next() {
		var x legacyEmbyBinding
		if err = rows.Scan(&x.ID, &x.AccountID, &x.InstanceID, &x.RemoteUserID, &x.Username, &x.Status, &x.IsPrimary, &x.Expires, &x.LastSynced, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readSettings(ctx context.Context) ([]legacySetting, error) {
	exists, err := i.tableExists(ctx, "dynamic_settings")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT setting_key,value_json,value_type,is_secret,revision,updated_by_kind,updated_by_id,updated_at FROM dynamic_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacySetting
	for rows.Next() {
		var x legacySetting
		if err = rows.Scan(&x.Key, &x.Value, &x.ValueType, &x.Secret, &x.Revision, &x.UpdatedByKind, &x.UpdatedByID, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (i *Importer) readAuditLogs(ctx context.Context) ([]legacyAudit, error) {
	exists, err := i.tableExists(ctx, "audit_logs")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,request_id,actor_kind,actor_id,actor_name,action,resource_type,resource_id,outcome,detail_json,ip_address,created_at FROM audit_logs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyAudit
	for rows.Next() {
		var x legacyAudit
		if err = rows.Scan(&x.ID, &x.RequestID, &x.ActorKind, &x.ActorID, &x.ActorName, &x.Action, &x.ResourceType, &x.ResourceID, &x.Outcome, &x.Detail, &x.IPAddress, &x.Created); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readTickets(ctx context.Context) ([]legacyTicket, error) {
	exists, err := i.tableExists(ctx, "support_tickets")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,ticket_no,tg,subject,category,priority,status,assignee_tg,resolved_at,closed_at,created_at,updated_at FROM support_tickets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyTicket
	for rows.Next() {
		var x legacyTicket
		if err = rows.Scan(&x.ID, &x.Number, &x.TG, &x.Subject, &x.Category, &x.Priority, &x.Status, &x.Assignee, &x.Resolved, &x.Closed, &x.Created, &x.Updated); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readTicketMessages(ctx context.Context) ([]legacyTicketMessage, error) {
	exists, err := i.tableExists(ctx, "ticket_messages")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,ticket_id,sender_kind,sender_tg,body,internal,created_at FROM ticket_messages`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyTicketMessage
	for rows.Next() {
		var x legacyTicketMessage
		if err = rows.Scan(&x.ID, &x.TicketID, &x.SenderKind, &x.SenderTG, &x.Body, &x.Internal, &x.Created); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readNotifications(ctx context.Context) ([]legacyNotification, error) {
	exists, err := i.tableExists(ctx, "user_notifications")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT id,tg,category,title,body,severity,action_url,metadata_json,read_at,created_at FROM user_notifications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyNotification
	for rows.Next() {
		var x legacyNotification
		if err = rows.Scan(&x.ID, &x.TG, &x.Category, &x.Title, &x.Body, &x.Severity, &x.ActionURL, &x.Metadata, &x.ReadAt, &x.Created); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (i *Importer) readNotificationPreferences(ctx context.Context) ([]legacyNotificationPreference, error) {
	exists, err := i.tableExists(ctx, "notification_preferences")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := i.source.QueryContext(ctx, `SELECT tg,category,web_enabled,telegram_enabled,updated_at FROM notification_preferences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyNotificationPreference
	for rows.Next() {
		var x legacyNotificationPreference
		if err = rows.Scan(&x.TG, &x.Category, &x.Web, &x.Telegram, &x.Updated); err != nil {
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
func normalizeMembershipStatus(value string, expires time.Time) string {
	if value == "grace" && expires.After(time.Now()) {
		return "grace"
	}
	if value == "active" && expires.After(time.Now()) {
		return "active"
	}
	if value == "expired" || expires.Before(time.Now()) {
		return "expired"
	}
	return "canceled"
}
func normalizeCurrency(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "coins", "points", "point":
		return "POINTS"
	case "registration_days", "days":
		return "REGISTRATION_DAYS"
	default:
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			return "POINTS"
		}
		if len(value) > 16 {
			value = value[:16]
		}
		return value
	}
}
func legacyPlanCode(id int, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fmt.Sprintf("plan-%d", id)
	}
	if len(value) > 61 {
		value = value[:61]
	}
	return "v2-" + value
}
func parseOrDeterministicUUID(value, namespace string) uuid.UUID {
	parsed, err := uuid.Parse(value)
	if err == nil {
		return parsed
	}
	return deterministicUUID(namespace + value)
}
func normalizeInstanceStatus(value string, enabled bool) string {
	if !enabled {
		return "disabled"
	}
	switch value {
	case "healthy", "degraded", "unhealthy":
		return value
	}
	return "unknown"
}
func normalizeBindingStatus(value string) string {
	switch value {
	case "active", "suspended", "missing", "error", "deleted":
		return value
	}
	return "active"
}
func normalizeSettingType(value string, raw []byte) string {
	switch value {
	case "boolean", "integer", "string", "object", "array":
		return value
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return "string"
	}
	switch decoded.(type) {
	case bool:
		return "boolean"
	case float64:
		return "integer"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}
func normalizeTicketPriority(value string) string {
	switch value {
	case "low", "normal", "high", "urgent":
		return value
	}
	return "normal"
}
func normalizeTicketStatus(value string) string {
	switch value {
	case "open", "waiting_user", "waiting_staff", "resolved", "closed":
		return value
	case "pending":
		return "open"
	}
	return "open"
}
func normalizeInviteStatus(value sql.NullString, used sql.NullInt64, expires sql.NullTime) string {
	if used.Valid {
		return "used"
	}
	if expires.Valid && expires.Time.Before(time.Now()) {
		return "expired"
	}
	if value.Valid {
		switch value.String {
		case "active", "used", "expired", "revoked":
			return value.String
		}
	}
	return "active"
}
func normalizeProvider(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "legacy"
	}
	var builder strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_' || character == '-' {
			builder.WriteRune(character)
		}
	}
	value = builder.String()
	if value == "" {
		return "legacy"
	}
	if len(value) > 40 {
		return value[:40]
	}
	return value
}
func normalizeRechargeStatus(value string, paid sql.NullTime, created time.Time) string {
	switch value {
	case "paid", "partially_refunded", "refunded", "canceled", "expired":
		return value
	case "credited", "approved", "completed":
		return "paid"
	}
	if paid.Valid {
		return "paid"
	}
	if created.Add(30 * time.Minute).Before(time.Now()) {
		return "expired"
	}
	return "pending"
}
func positiveOrOne(value int64) int64 {
	if value > 0 {
		return value
	}
	return 1
}
func deterministicUUID(value string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceURL, []byte(value)) }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
