package identity

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

var usernamePattern = regexp.MustCompile(`^[^\s/\\<>]{3,32}$`)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUsernameTaken      = errors.New("username already exists")
	ErrAccountDisabled    = errors.New("account is not active")
	ErrNotFound           = errors.New("not found")
	ErrForbidden          = errors.New("forbidden")
	ErrConflict           = errors.New("conflict")
	ErrInvalid            = errors.New("invalid request")
)

type Actor struct{ Kind, ID, IP, RequestID string }

func (a Actor) Label() string { return a.Kind + ":" + a.ID }

type Account struct {
	ID          uuid.UUID  `json:"id"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	Revision    int64      `json:"revision"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Identities  []Identity `json:"identities,omitempty"`
	Roles       []string   `json:"roles,omitempty"`
}
type Identity struct {
	Kind       string     `json:"kind"`
	Subject    string     `json:"subject,omitempty"`
	Username   string     `json:"username,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}
type Principal struct {
	Actor       Actor
	AccountID   *uuid.UUID
	Permissions map[string]bool
	Scopes      map[string]bool
	CSRFHash    []byte
	SessionID   *uuid.UUID
}

func (p Principal) HasPermission(value string) bool { return p.Permissions[value] }
func (p Principal) HasScope(value string) bool      { return p.Scopes[value] }

type Service struct {
	db         *pgxpool.Pool
	sessionTTL time.Duration
	vault      *security.Vault
}

func New(db *pgxpool.Pool, ttl time.Duration, vault *security.Vault) *Service {
	return &Service{db: db, sessionTTL: ttl, vault: vault}
}

func normalizeUsername(value string) (string, string, error) {
	username := strings.TrimSpace(value)
	if !usernamePattern.MatchString(username) {
		return "", "", errors.New("username must contain 3-32 safe characters")
	}
	return username, strings.ToLower(username), nil
}
func jsonBytes(value any) []byte { body, _ := json.Marshal(value); return body }

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func audit(ctx context.Context, tx pgx.Tx, actor Actor, action, resourceType, resourceID string, details any) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,ip_address,details) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8)`, actor.Kind, actor.ID, action, resourceType, resourceID, actor.RequestID, actor.IP, jsonBytes(details))
	return err
}
