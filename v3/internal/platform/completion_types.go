package platform

import (
	"time"

	"github.com/google/uuid"
)

type EntitlementCode struct {
	ID           uuid.UUID      `json:"id"`
	Code         string         `json:"code,omitempty"`
	CodeHint     string         `json:"code_hint"`
	InstanceID   *uuid.UUID     `json:"instance_id,omitempty"`
	ResourceKind string         `json:"resource_kind"`
	ResourceKey  string         `json:"resource_key"`
	DurationDays int            `json:"duration_days"`
	Status       string         `json:"status"`
	IssuedBy     string         `json:"issued_by"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type AccountEntitlement struct {
	ID           uuid.UUID      `json:"id"`
	AccountID    uuid.UUID      `json:"account_id"`
	InstanceID   *uuid.UUID     `json:"instance_id,omitempty"`
	BindingID    *uuid.UUID     `json:"binding_id,omitempty"`
	ResourceKind string         `json:"resource_kind"`
	ResourceKey  string         `json:"resource_key"`
	Status       string         `json:"status"`
	SourceCodeID *uuid.UUID     `json:"source_code_id,omitempty"`
	StartsAt     time.Time      `json:"starts_at"`
	ExpiresAt    time.Time      `json:"expires_at"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type LineEndpoint struct {
	ID            uuid.UUID      `json:"id"`
	Name          string         `json:"name"`
	BaseURL       string         `json:"base_url"`
	Region        string         `json:"region,omitempty"`
	Carrier       string         `json:"carrier,omitempty"`
	Audience      string         `json:"audience"`
	Weight        int            `json:"weight"`
	SortOrder     int            `json:"sort_order"`
	Enabled       bool           `json:"enabled"`
	Maintenance   bool           `json:"maintenance"`
	Revision      int64          `json:"revision"`
	LastStatus    string         `json:"last_status"`
	LastLatencyMS *int           `json:"last_latency_ms,omitempty"`
	LastError     string         `json:"last_error,omitempty"`
	LastCheckedAt *time.Time     `json:"last_checked_at,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type LineProbeSample struct {
	ID           int64     `json:"id"`
	LineID       uuid.UUID `json:"line_id"`
	Status       string    `json:"status"`
	HTTPStatus   *int      `json:"http_status,omitempty"`
	LatencyMS    *int      `json:"latency_ms,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CheckedBy    string    `json:"checked_by"`
	CheckedAt    time.Time `json:"checked_at"`
}

type ReviewReport struct {
	ID                uuid.UUID  `json:"id"`
	ReviewID          uuid.UUID  `json:"review_id"`
	ReporterAccountID uuid.UUID  `json:"reporter_account_id"`
	Reason            string     `json:"reason"`
	Detail            string     `json:"detail,omitempty"`
	Status            string     `json:"status"`
	Resolution        string     `json:"resolution,omitempty"`
	ResolvedBy        string     `json:"resolved_by,omitempty"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type EmbyFavorite struct {
	ID              uuid.UUID      `json:"id"`
	AccountID       uuid.UUID      `json:"account_id"`
	InstanceID      uuid.UUID      `json:"instance_id"`
	InstanceName    string         `json:"instance_name"`
	BindingID       uuid.UUID      `json:"binding_id"`
	MediaID         *uuid.UUID     `json:"media_id,omitempty"`
	RemoteItemID    string         `json:"remote_item_id"`
	Title           string         `json:"title"`
	MediaType       string         `json:"media_type,omitempty"`
	ImageTag        string         `json:"image_tag,omitempty"`
	RemoteSnapshot  map[string]any `json:"remote_snapshot"`
	DesiredFavorite bool           `json:"desired_favorite"`
	RemoteFavorite  bool           `json:"remote_favorite"`
	SyncStatus      string         `json:"sync_status"`
	LastError       string         `json:"last_error,omitempty"`
	LastSyncedAt    *time.Time     `json:"last_synced_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type IntegrationProbe struct {
	ID           uuid.UUID      `json:"id"`
	Integration  string         `json:"integration"`
	Target       string         `json:"target"`
	Status       string         `json:"status"`
	LatencyMS    int            `json:"latency_ms"`
	Detail       map[string]any `json:"detail"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CheckedBy    string         `json:"checked_by"`
	CheckedAt    time.Time      `json:"checked_at"`
}
