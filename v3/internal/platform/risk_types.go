package platform

import (
	"time"

	"github.com/google/uuid"
)

type PlaybackSession struct {
	ID                  uuid.UUID      `json:"id"`
	InstanceID          uuid.UUID      `json:"instance_id"`
	InstanceName        string         `json:"instance_name,omitempty"`
	BindingID           *uuid.UUID     `json:"binding_id,omitempty"`
	AccountID           *uuid.UUID     `json:"account_id,omitempty"`
	RemoteSessionID     string         `json:"remote_session_id"`
	PlaybackKey         string         `json:"playback_key"`
	RemoteUserID        string         `json:"remote_user_id,omitempty"`
	RemoteUsername      string         `json:"remote_username,omitempty"`
	ItemID              string         `json:"item_id"`
	ItemName            string         `json:"item_name"`
	ItemType            string         `json:"item_type,omitempty"`
	SeriesName          string         `json:"series_name,omitempty"`
	ClientName          string         `json:"client_name,omitempty"`
	DeviceName          string         `json:"device_name,omitempty"`
	DeviceID            string         `json:"device_id,omitempty"`
	Platform            string         `json:"platform,omitempty"`
	RemoteIP            string         `json:"remote_ip,omitempty"`
	PlayMethod          string         `json:"play_method,omitempty"`
	Transcoding         bool           `json:"transcoding"`
	Bitrate             int64          `json:"bitrate,omitempty"`
	PositionTicks       int64          `json:"position_ticks"`
	RuntimeTicks        int64          `json:"runtime_ticks,omitempty"`
	Paused              bool           `json:"paused"`
	DeviceDecision      string         `json:"device_decision"`
	MatchedDeviceRuleID *uuid.UUID     `json:"matched_device_rule_id,omitempty"`
	RawSnapshot         map[string]any `json:"raw_snapshot,omitempty"`
	FirstSeenAt         time.Time      `json:"first_seen_at"`
	LastSeenAt          time.Time      `json:"last_seen_at"`
}

type PlaybackHistory struct {
	PlaybackSession
	PeakBitrate      int64      `json:"peak_bitrate,omitempty"`
	MaxPositionTicks int64      `json:"max_position_ticks"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
}

type DeviceProfile struct {
	ID             uuid.UUID  `json:"id"`
	InstanceID     uuid.UUID  `json:"instance_id"`
	InstanceName   string     `json:"instance_name,omitempty"`
	BindingID      *uuid.UUID `json:"binding_id,omitempty"`
	AccountID      *uuid.UUID `json:"account_id,omitempty"`
	RemoteUserID   string     `json:"remote_user_id,omitempty"`
	DeviceKey      string     `json:"device_key"`
	DeviceID       string     `json:"device_id,omitempty"`
	DeviceName     string     `json:"device_name,omitempty"`
	ClientName     string     `json:"client_name,omitempty"`
	Platform       string     `json:"platform,omitempty"`
	FirstIP        string     `json:"first_ip,omitempty"`
	LastIP         string     `json:"last_ip,omitempty"`
	SessionCount   int64      `json:"session_count"`
	TranscodeCount int64      `json:"transcode_count"`
	MaximumBitrate int64      `json:"maximum_bitrate,omitempty"`
	AccessDecision string     `json:"access_decision"`
	MatchedRuleID  *uuid.UUID `json:"matched_rule_id,omitempty"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
}

type DeviceRule struct {
	ID              uuid.UUID  `json:"id"`
	InstanceID      *uuid.UUID `json:"instance_id,omitempty"`
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	Decision        string     `json:"decision"`
	MatchField      string     `json:"match_field"`
	MatchOperator   string     `json:"match_operator"`
	MatchValue      string     `json:"match_value"`
	Action          string     `json:"action"`
	ObservationMode bool       `json:"observation_mode"`
	Enabled         bool       `json:"enabled"`
	BuiltIn         bool       `json:"built_in"`
	Priority        int        `json:"priority"`
	Revision        int64      `json:"revision"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type RiskRule struct {
	ID              uuid.UUID      `json:"id"`
	InstanceID      *uuid.UUID     `json:"instance_id,omitempty"`
	Code            string         `json:"code"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	RuleType        string         `json:"rule_type"`
	Condition       map[string]any `json:"condition"`
	Severity        string         `json:"severity"`
	Action          string         `json:"action"`
	ObservationMode bool           `json:"observation_mode"`
	Enabled         bool           `json:"enabled"`
	CooldownSeconds int            `json:"cooldown_seconds"`
	Revision        int64          `json:"revision"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type RiskEvent struct {
	ID                uuid.UUID      `json:"id"`
	InstanceID        uuid.UUID      `json:"instance_id"`
	InstanceName      string         `json:"instance_name,omitempty"`
	RuleID            *uuid.UUID     `json:"rule_id,omitempty"`
	DeviceRuleID      *uuid.UUID     `json:"device_rule_id,omitempty"`
	PlaybackSessionID *uuid.UUID     `json:"playback_session_id,omitempty"`
	BindingID         *uuid.UUID     `json:"binding_id,omitempty"`
	AccountID         *uuid.UUID     `json:"account_id,omitempty"`
	Source            string         `json:"source"`
	Severity          string         `json:"severity"`
	Title             string         `json:"title"`
	Reason            string         `json:"reason"`
	Evidence          map[string]any `json:"evidence"`
	RuleSnapshot      map[string]any `json:"rule_snapshot"`
	ObservationMode   bool           `json:"observation_mode"`
	RecommendedAction string         `json:"recommended_action"`
	Status            string         `json:"status"`
	DispositionReason string         `json:"disposition_reason,omitempty"`
	DispositionBy     string         `json:"disposition_by,omitempty"`
	DispositionAt     *time.Time     `json:"disposition_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type RiskAction struct {
	ID              uuid.UUID      `json:"id"`
	EventID         uuid.UUID      `json:"event_id"`
	InstanceID      uuid.UUID      `json:"instance_id"`
	TaskID          *uuid.UUID     `json:"task_id,omitempty"`
	ActionType      string         `json:"action_type"`
	Status          string         `json:"status"`
	Reason          string         `json:"reason"`
	RemoteSessionID string         `json:"remote_session_id,omitempty"`
	RemoteUserID    string         `json:"remote_user_id,omitempty"`
	BeforeState     map[string]any `json:"before_state"`
	AfterState      map[string]any `json:"after_state"`
	Result          map[string]any `json:"result"`
	Attempts        int            `json:"attempts"`
	LastError       string         `json:"last_error,omitempty"`
	ExecutedAt      *time.Time     `json:"executed_at,omitempty"`
	RevertedAt      *time.Time     `json:"reverted_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type RiskTimelineEntry struct {
	ID        int64          `json:"id"`
	EventID   uuid.UUID      `json:"event_id"`
	EventType string         `json:"event_type"`
	Actor     string         `json:"actor"`
	Reason    string         `json:"reason,omitempty"`
	Details   map[string]any `json:"details"`
	CreatedAt time.Time      `json:"created_at"`
}

type RiskEventDetail struct {
	Event    RiskEvent           `json:"event"`
	Actions  []RiskAction        `json:"actions"`
	Timeline []RiskTimelineEntry `json:"timeline"`
}
