package platform

import (
	"time"

	"github.com/google/uuid"
)

type Media struct {
	ID             uuid.UUID      `json:"id"`
	Provider       string         `json:"provider"`
	ExternalID     int64          `json:"external_id"`
	MediaType      string         `json:"media_type"`
	Title          string         `json:"title"`
	OriginalTitle  string         `json:"original_title,omitempty"`
	Overview       string         `json:"overview,omitempty"`
	ReleaseDate    *time.Time     `json:"release_date,omitempty"`
	PosterPath     string         `json:"poster_path,omitempty"`
	BackdropPath   string         `json:"backdrop_path,omitempty"`
	Popularity     float64        `json:"popularity"`
	VoteAverage    float64        `json:"vote_average"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Available      bool           `json:"available"`
	AvailableCount int            `json:"available_instance_count"`
	LastRefreshed  time.Time      `json:"last_refreshed_at"`
}

type MediaMatch struct {
	ID             uuid.UUID      `json:"id"`
	MediaID        uuid.UUID      `json:"media_id"`
	InstanceID     uuid.UUID      `json:"instance_id"`
	InstanceName   string         `json:"instance_name"`
	Status         string         `json:"status"`
	RemoteItemID   string         `json:"remote_item_id,omitempty"`
	RemoteTitle    string         `json:"remote_title,omitempty"`
	RemoteItemType string         `json:"remote_item_type,omitempty"`
	RemoteSnapshot map[string]any `json:"remote_snapshot,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	MatchedAt      *time.Time     `json:"matched_at,omitempty"`
	LastCheckedAt  time.Time      `json:"last_checked_at"`
}

type MediaRequest struct {
	ID               uuid.UUID  `json:"id"`
	RequestNo        string     `json:"request_no"`
	Media            Media      `json:"media"`
	RequestedBy      uuid.UUID  `json:"requested_by"`
	Status           string     `json:"status"`
	Priority         int        `json:"priority"`
	Note             string     `json:"note,omitempty"`
	SubscriberCount  int        `json:"subscriber_count"`
	Subscribed       bool       `json:"subscribed"`
	Duplicate        bool       `json:"duplicate"`
	ResolutionReason string     `json:"resolution_reason,omitempty"`
	ResolvedBy       string     `json:"resolved_by,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Revision         int64      `json:"revision"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type MoviePilotJob struct {
	ID            uuid.UUID      `json:"id"`
	MediaID       uuid.UUID      `json:"media_id"`
	RequestID     *uuid.UUID     `json:"request_id,omitempty"`
	TaskID        *uuid.UUID     `json:"task_id,omitempty"`
	Status        string         `json:"status"`
	ExternalJobID string         `json:"external_job_id,omitempty"`
	Payload       map[string]any `json:"payload"`
	Result        map[string]any `json:"result"`
	Attempts      int            `json:"attempts"`
	LastError     string         `json:"last_error,omitempty"`
	SubmittedAt   *time.Time     `json:"submitted_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Duplicate     bool           `json:"duplicate"`
}

type Ticket struct {
	ID                 uuid.UUID  `json:"id"`
	TicketNo           string     `json:"ticket_no"`
	AccountID          uuid.UUID  `json:"account_id"`
	Subject            string     `json:"subject"`
	Category           string     `json:"category"`
	Priority           string     `json:"priority"`
	Status             string     `json:"status"`
	AssignedTo         *uuid.UUID `json:"assigned_to,omitempty"`
	LastPublicReplyAt  *time.Time `json:"last_public_reply_at,omitempty"`
	LastInternalNoteAt *time.Time `json:"last_internal_note_at,omitempty"`
	Revision           int64      `json:"revision"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type TicketMessage struct {
	ID              uuid.UUID  `json:"id"`
	TicketID        uuid.UUID  `json:"ticket_id"`
	AuthorAccountID *uuid.UUID `json:"author_account_id,omitempty"`
	AuthorLabel     string     `json:"author_label"`
	Body            string     `json:"body"`
	Internal        bool       `json:"internal"`
	Attachments     []any      `json:"attachments"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Review struct {
	ID               uuid.UUID  `json:"id"`
	Media            Media      `json:"media"`
	AccountID        uuid.UUID  `json:"account_id"`
	Rating           int        `json:"rating"`
	Title            string     `json:"title,omitempty"`
	Body             string     `json:"body"`
	ContainsSpoilers bool       `json:"contains_spoilers"`
	Status           string     `json:"status"`
	ModerationReason string     `json:"moderation_reason,omitempty"`
	ModeratedBy      string     `json:"moderated_by,omitempty"`
	ModeratedAt      *time.Time `json:"moderated_at,omitempty"`
	Revision         int64      `json:"revision"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type NotificationPreference struct {
	EventKey string `json:"event_key"`
	Channel  string `json:"channel"`
	Enabled  bool   `json:"enabled"`
}

type Broadcast struct {
	ID             uuid.UUID      `json:"id"`
	BatchOperation BatchOperation `json:"batch_operation"`
	Title          string         `json:"title"`
	Body           string         `json:"body"`
	EventKey       string         `json:"event_key"`
	Channel        string         `json:"channel"`
	TargetSpec     map[string]any `json:"target_spec"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
}

type AutomationRule struct {
	ID           uuid.UUID        `json:"id"`
	Code         string           `json:"code"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	TriggerEvent string           `json:"trigger_event"`
	Conditions   map[string]any   `json:"conditions"`
	Actions      []map[string]any `json:"actions"`
	Enabled      bool             `json:"enabled"`
	Priority     int              `json:"priority"`
	Revision     int64            `json:"revision"`
	CreatedBy    string           `json:"created_by"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type AutomationExecution struct {
	ID           int64          `json:"id"`
	EventID      uuid.UUID      `json:"event_id"`
	RuleID       uuid.UUID      `json:"rule_id"`
	RuleName     string         `json:"rule_name"`
	Status       string         `json:"status"`
	Result       map[string]any `json:"result"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}
