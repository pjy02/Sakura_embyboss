package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

type Service struct {
	db    *pgxpool.Pool
	vault *security.Vault
}

func New(db *pgxpool.Pool, vault *security.Vault) *Service {
	return &Service{db: db, vault: vault}
}

type MembershipPlan struct {
	ID           uuid.UUID      `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	DurationDays int            `json:"duration_days"`
	Entitlements map[string]any `json:"entitlements"`
	Enabled      bool           `json:"enabled"`
	IsDefault    bool           `json:"is_default"`
	SortOrder    int            `json:"sort_order"`
	Revision     int64          `json:"revision"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Membership struct {
	ID        uuid.UUID      `json:"id"`
	AccountID uuid.UUID      `json:"account_id"`
	PlanID    uuid.UUID      `json:"plan_id"`
	PlanCode  string         `json:"plan_code"`
	PlanName  string         `json:"plan_name"`
	Status    string         `json:"status"`
	StartsAt  time.Time      `json:"starts_at"`
	ExpiresAt time.Time      `json:"expires_at"`
	Source    string         `json:"source"`
	Benefits  map[string]any `json:"entitlements"`
}

type Invitation struct {
	ID        uuid.UUID  `json:"id"`
	Code      string     `json:"code,omitempty"`
	CodeHint  string     `json:"code_hint"`
	Kind      string     `json:"kind"`
	PlanID    uuid.UUID  `json:"plan_id"`
	PlanCode  string     `json:"plan_code"`
	MaxUses   int        `json:"max_uses"`
	UsedCount int        `json:"used_count"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type EmbyInstance struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	BaseURL        string     `json:"base_url"`
	CredentialName string     `json:"credential_name"`
	Enabled        bool       `json:"enabled"`
	IsDefault      bool       `json:"is_default"`
	VerifyTLS      bool       `json:"verify_tls"`
	Priority       int        `json:"priority"`
	Status         string     `json:"status"`
	ServerID       string     `json:"server_id,omitempty"`
	ServerVersion  string     `json:"server_version,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	LastLatencyMS  *int       `json:"last_latency_ms,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastSnapshotAt *time.Time `json:"last_snapshot_at,omitempty"`
	Revision       int64      `json:"revision"`
	BindingCount   int        `json:"binding_count"`
	UnclaimedCount int        `json:"unclaimed_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Binding struct {
	ID             uuid.UUID      `json:"id"`
	AccountID      uuid.UUID      `json:"account_id"`
	InstanceID     uuid.UUID      `json:"instance_id"`
	InstanceName   string         `json:"instance_name"`
	RemoteUserID   string         `json:"remote_user_id"`
	RemoteUsername string         `json:"remote_username"`
	Status         string         `json:"status"`
	Origin         string         `json:"origin"`
	IsPrimary      bool           `json:"is_primary"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	RemoteDisabled *bool          `json:"remote_disabled,omitempty"`
	RemoteSnapshot map[string]any `json:"remote_snapshot"`
	ClaimedAt      *time.Time     `json:"claimed_at,omitempty"`
	LastSyncedAt   *time.Time     `json:"last_synced_at,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type RemoteUser struct {
	ID           uuid.UUID      `json:"id"`
	InstanceID   uuid.UUID      `json:"instance_id"`
	InstanceName string         `json:"instance_name"`
	RemoteUserID string         `json:"remote_user_id"`
	Username     string         `json:"username"`
	Disabled     bool           `json:"disabled"`
	ClaimStatus  string         `json:"claim_status"`
	BindingID    *uuid.UUID     `json:"binding_id,omitempty"`
	Snapshot     map[string]any `json:"snapshot"`
	FirstSeenAt  time.Time      `json:"first_seen_at"`
	LastSeenAt   time.Time      `json:"last_seen_at"`
	MissingSince *time.Time     `json:"missing_since,omitempty"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type Task struct {
	ID             uuid.UUID      `json:"id"`
	TaskType       string         `json:"task_type"`
	Status         string         `json:"status"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Result         map[string]any `json:"result"`
	Attempts       int            `json:"attempts"`
	MaxAttempts    int            `json:"max_attempts"`
	LastError      string         `json:"last_error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type ProvisionResult struct {
	Task              Task       `json:"task"`
	AccountID         uuid.UUID  `json:"account_id"`
	InstanceID        uuid.UUID  `json:"instance_id"`
	Username          string     `json:"username"`
	RemoteUserID      string     `json:"remote_user_id,omitempty"`
	GeneratedPassword string     `json:"generated_password,omitempty"`
	PasswordExpiresAt *time.Time `json:"password_expires_at,omitempty"`
}

type Snapshot struct {
	ID                 int64          `json:"id"`
	InstanceID         uuid.UUID      `json:"instance_id"`
	TaskID             *uuid.UUID     `json:"task_id,omitempty"`
	Kind               string         `json:"snapshot_kind"`
	Status             string         `json:"status"`
	RemoteUserCount    int            `json:"remote_user_count"`
	BoundUserCount     int            `json:"bound_user_count"`
	UnclaimedUserCount int            `json:"unclaimed_user_count"`
	MissingUserCount   int            `json:"missing_user_count"`
	Changes            map[string]any `json:"changes"`
	ErrorMessage       string         `json:"error_message,omitempty"`
	CapturedAt         time.Time      `json:"captured_at"`
}

func jsonBytes(value any) []byte {
	body, _ := json.Marshal(value)
	return body
}

func decodeJSON(raw []byte) map[string]any {
	value := map[string]any{}
	_ = json.Unmarshal(raw, &value)
	return value
}

func audit(ctx context.Context, tx pgx.Tx, actor identity.Actor, action, resourceType, resourceID string, details any) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,ip_address,details) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8)`, actor.Kind, actor.ID, action, resourceType, resourceID, actor.RequestID, actor.IP, jsonBytes(details))
	return err
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrNotFound
	}
	return err
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func uuidQueryValue(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return *value
}

type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }
