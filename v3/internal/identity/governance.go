package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)

var apiScopeCatalog = map[string]string{
	"system:read": "Read public system metadata",
}

type RoleInfo struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	System      bool      `json:"system"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PermissionInfo struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

func (s *Service) ListPermissions(ctx context.Context) ([]PermissionInfo, error) {
	rows, err := s.db.Query(ctx, `SELECT code,description FROM permissions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PermissionInfo
	for rows.Next() {
		var item PermissionInfo
		if err = rows.Scan(&item.Code, &item.Description); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListRoles(ctx context.Context) ([]RoleInfo, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.id,r.code,r.name,r.system,r.created_at,r.updated_at,
		       COALESCE(array_agg(rp.permission_code ORDER BY rp.permission_code)
		         FILTER (WHERE rp.permission_code IS NOT NULL),ARRAY[]::varchar[])
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id
		GROUP BY r.id ORDER BY r.system DESC,r.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoleInfo
	for rows.Next() {
		var item RoleInfo
		if err = rows.Scan(&item.ID, &item.Code, &item.Name, &item.System, &item.CreatedAt, &item.UpdatedAt, &item.Permissions); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveRole(ctx context.Context, code, name string, permissions []string, actor Actor) (RoleInfo, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	name = strings.TrimSpace(name)
	if !roleCodePattern.MatchString(code) || name == "" || len(name) > 100 || len(permissions) == 0 {
		return RoleInfo{}, ErrInvalid
	}
	permissions = uniqueSorted(permissions)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RoleInfo{}, err
	}
	defer tx.Rollback(ctx)
	var roleID uuid.UUID
	var system bool
	err = tx.QueryRow(ctx, `SELECT id,system FROM roles WHERE code=$1 FOR UPDATE`, code).Scan(&roleID, &system)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		roleID = uuid.New()
		if _, err = tx.Exec(ctx, `INSERT INTO roles(id,code,name,system) VALUES($1,$2,$3,FALSE)`, roleID, code, name); err != nil {
			return RoleInfo{}, err
		}
	case err != nil:
		return RoleInfo{}, err
	case system:
		return RoleInfo{}, ErrForbidden
	default:
		if _, err = tx.Exec(ctx, `UPDATE roles SET name=$2,updated_at=NOW() WHERE id=$1`, roleID, name); err != nil {
			return RoleInfo{}, err
		}
	}
	var permissionCount int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM permissions WHERE code=ANY($1)`, permissions).Scan(&permissionCount); err != nil {
		return RoleInfo{}, err
	}
	if permissionCount != len(permissions) {
		return RoleInfo{}, fmt.Errorf("%w: unknown permission", ErrInvalid)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, roleID); err != nil {
		return RoleInfo{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code) SELECT $1,unnest($2::text[])`, roleID, permissions); err != nil {
		return RoleInfo{}, err
	}
	if err = audit(ctx, tx, actor, "role.save", "role", code, map[string]any{"permissions": permissions}); err != nil {
		return RoleInfo{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RoleInfo{}, err
	}
	roles, err := s.ListRoles(ctx)
	if err != nil {
		return RoleInfo{}, err
	}
	for _, role := range roles {
		if role.Code == code {
			return role, nil
		}
	}
	return RoleInfo{}, ErrNotFound
}

func (s *Service) AssignRoles(ctx context.Context, accountID uuid.UUID, codes []string, actor Actor) (Account, error) {
	codes = uniqueSorted(codes)
	if len(codes) == 0 {
		return Account{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	var accountStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&accountStatus); err != nil {
		return Account{}, mapNotFound(err)
	}
	// Locking the owner role serializes assignments that could otherwise remove
	// the last owner concurrently.
	var ownerRoleID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE code='owner' FOR UPDATE`).Scan(&ownerRoleID); err != nil {
		return Account{}, err
	}
	var roleIDs []uuid.UUID
	rows, err := tx.Query(ctx, `SELECT id FROM roles WHERE code=ANY($1) ORDER BY code`, codes)
	if err != nil {
		return Account{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return Account{}, err
		}
		roleIDs = append(roleIDs, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Account{}, err
	}
	rows.Close()
	if len(roleIDs) != len(codes) {
		return Account{}, fmt.Errorf("%w: unknown role", ErrInvalid)
	}
	var currentlyOwner bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_roles WHERE account_id=$1 AND role_id=$2)`, accountID, ownerRoleID).Scan(&currentlyOwner); err != nil {
		return Account{}, err
	}
	if currentlyOwner && accountStatus == "active" && !contains(codes, "owner") {
		var ownerCount int
		if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM account_roles ar JOIN accounts a ON a.id=ar.account_id WHERE ar.role_id=$1 AND a.status='active'`, ownerRoleID).Scan(&ownerCount); err != nil {
			return Account{}, err
		}
		if ownerCount <= 1 {
			return Account{}, ErrForbidden
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM account_roles WHERE account_id=$1`, accountID); err != nil {
		return Account{}, err
	}
	for _, roleID := range roleIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,$2,$3)`, accountID, roleID, actor.Label()); err != nil {
			return Account{}, err
		}
	}
	if err = audit(ctx, tx, actor, "account.roles.assign", "account", accountID.String(), map[string]any{"roles": codes}); err != nil {
		return Account{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, accountID)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type Setting struct {
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	ValueType string    `json:"value_type"`
	Revision  int64     `json:"revision"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) ListSettings(ctx context.Context) ([]Setting, error) {
	rows, err := s.db.Query(ctx, `SELECT key,value,value_type,revision,updated_by,updated_at FROM dynamic_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var item Setting
		var raw []byte
		if err = rows.Scan(&item.Key, &raw, &item.ValueType, &item.Revision, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Value)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Service) SettingHistory(ctx context.Context, key string) ([]Setting, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.setting_key,r.value,r.value_type,r.revision,r.actor,r.created_at
		FROM setting_revisions r
		WHERE r.setting_key=$1 ORDER BY r.revision DESC`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var item Setting
		var raw []byte
		if err = rows.Scan(&item.Key, &raw, &item.ValueType, &item.Revision, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Value)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Service) UpdateSetting(ctx context.Context, key string, value any, valueType, reason string, expected int64, actor Actor) (Setting, error) {
	return s.writeSetting(ctx, key, value, valueType, reason, expected, 0, actor)
}
func (s *Service) RollbackSetting(ctx context.Context, key string, target, expected int64, reason string, actor Actor) (Setting, error) {
	return s.writeSetting(ctx, key, nil, "", reason, expected, target, actor)
}
func (s *Service) writeSetting(ctx context.Context, key string, value any, valueType, reason string, expected, target int64, actor Actor) (Setting, error) {
	key = strings.TrimSpace(key)
	reason = strings.TrimSpace(reason)
	if key == "" || len(key) > 160 || reason == "" || len(reason) > 500 || expected < 1 || target < 0 {
		return Setting{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Setting{}, err
	}
	defer tx.Rollback(ctx)
	var current Setting
	var raw []byte
	if err = tx.QueryRow(ctx, `SELECT key,value,value_type,revision,updated_by,updated_at FROM dynamic_settings WHERE key=$1 FOR UPDATE`, key).Scan(&current.Key, &raw, &current.ValueType, &current.Revision, &current.UpdatedBy, &current.UpdatedAt); err != nil {
		return Setting{}, mapNotFound(err)
	}
	if expected != current.Revision {
		return Setting{}, ErrConflict
	}
	if target > 0 {
		if err = tx.QueryRow(ctx, `SELECT value,value_type FROM setting_revisions WHERE setting_key=$1 AND revision=$2`, key, target).Scan(&raw, &valueType); err != nil {
			return Setting{}, mapNotFound(err)
		}
	} else {
		raw = jsonBytes(value)
		if !validSettingValue(key, raw, valueType) {
			return Setting{}, fmt.Errorf("%w: setting value does not satisfy its schema", ErrInvalid)
		}
	}
	next := current.Revision + 1
	_, err = tx.Exec(ctx, `UPDATE dynamic_settings SET value=$2,value_type=$3,revision=$4,updated_by=$5,updated_at=NOW() WHERE key=$1`, key, raw, valueType, next, actor.Label())
	if err != nil {
		return Setting{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason) VALUES($1,$2,$3,$4,$5,$6)`, key, next, raw, valueType, actor.Label(), reason)
	if err != nil {
		return Setting{}, err
	}
	action := "setting.update"
	if target > 0 {
		action = "setting.rollback"
	}
	if err = audit(ctx, tx, actor, action, "setting", key, map[string]any{"revision": next, "target_revision": target}); err != nil {
		return Setting{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Setting{}, err
	}
	var parsed any
	_ = json.Unmarshal(raw, &parsed)
	return Setting{Key: key, Value: parsed, ValueType: valueType, Revision: next, UpdatedBy: actor.Label(), UpdatedAt: time.Now()}, nil
}
func validSettingType(raw []byte, kind string) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch kind {
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		n, ok := value.(float64)
		return ok && n == float64(int64(n))
	case "string":
		_, ok := value.(string)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	}
	return false
}

func validSettingValue(key string, raw []byte, kind string) bool {
	if !validSettingType(raw, kind) {
		return false
	}
	var value any
	_ = json.Unmarshal(raw, &value)
	switch key {
	case "auth.local_registration_enabled":
		return kind == "boolean"
	case "auth.session_ttl_hours":
		number, ok := value.(float64)
		return ok && kind == "integer" && number >= 1 && number <= 24*365
	default:
		return true
	}
}

type Credential struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Metadata  any       `json:"metadata"`
	Masked    string    `json:"masked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) PutCredential(ctx context.Context, name, kind, secret string, metadata any, actor Actor) (Credential, error) {
	if s.vault == nil {
		return Credential{}, errors.New("credential vault is unavailable")
	}
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	if name == "" || len(name) > 120 || kind == "" || len(kind) > 64 || secret == "" {
		return Credential{}, ErrInvalid
	}
	ciphertext, nonce, version, err := s.vault.Encrypt([]byte(secret))
	if err != nil {
		return Credential{}, err
	}
	id := uuid.New()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Credential{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO credentials(id,name,kind,ciphertext,nonce,key_version,metadata,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(name) DO UPDATE SET kind=EXCLUDED.kind,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,key_version=EXCLUDED.key_version,metadata=EXCLUDED.metadata,updated_at=NOW()`, id, name, kind, ciphertext, nonce, version, jsonBytes(metadata), actor.Label())
	if err != nil {
		return Credential{}, err
	}
	if err = audit(ctx, tx, actor, "credential.put", "credential", name, map[string]any{"kind": kind}); err != nil {
		return Credential{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Credential{}, err
	}
	return s.GetCredential(ctx, name)
}
func (s *Service) GetCredential(ctx context.Context, name string) (Credential, error) {
	var item Credential
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT id,name,kind,metadata,created_at,updated_at FROM credentials WHERE name=$1`, name).Scan(&item.ID, &item.Name, &item.Kind, &raw, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Credential{}, mapNotFound(err)
	}
	_ = json.Unmarshal(raw, &item.Metadata)
	item.Masked = "********"
	return item, nil
}
func (s *Service) ListCredentials(ctx context.Context) ([]Credential, error) {
	rows, err := s.db.Query(ctx, `SELECT name FROM credentials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Credential, 0, len(names))
	for _, name := range names {
		item, getErr := s.GetCredential(ctx, name)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) RevealCredential(ctx context.Context, name string, actor Actor) (string, error) {
	if s.vault == nil {
		return "", errors.New("credential vault is unavailable")
	}
	var ciphertext, nonce []byte
	var version int
	err := s.db.QueryRow(ctx, `SELECT ciphertext,nonce,key_version FROM credentials WHERE name=$1`, name).Scan(&ciphertext, &nonce, &version)
	if err != nil {
		return "", mapNotFound(err)
	}
	plaintext, err := s.vault.Decrypt(ciphertext, nonce, version)
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err = audit(ctx, tx, actor, "credential.reveal", "credential", name, nil); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Service) DeleteCredential(ctx context.Context, name string, actor Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM credentials WHERE name=$1`, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, actor, "credential.delete", "credential", name, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type APIClientResult struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token,omitempty"`
	Prefix    string    `json:"prefix"`
	Scopes    []string  `json:"scopes"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) CreateAPIClient(ctx context.Context, name string, scopes []string, actor Actor) (APIClientResult, error) {
	name = strings.TrimSpace(name)
	scopes = uniqueSorted(scopes)
	if name == "" || len(name) > 120 || len(scopes) == 0 {
		return APIClientResult{}, ErrInvalid
	}
	for _, scope := range scopes {
		if _, ok := apiScopeCatalog[scope]; !ok {
			return APIClientResult{}, fmt.Errorf("%w: unknown API scope %q", ErrInvalid, scope)
		}
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return APIClientResult{}, err
	}
	token = "sk_v3_" + token
	id := uuid.New()
	prefix := token[:14]
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return APIClientResult{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO api_clients(id,name,token_prefix,token_hash,scopes,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, name, prefix, security.HashToken(token), scopes, actor.ID)
	if err != nil {
		return APIClientResult{}, err
	}
	if err = audit(ctx, tx, actor, "api_client.create", "api_client", id.String(), map[string]any{"scopes": scopes}); err != nil {
		return APIClientResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return APIClientResult{}, err
	}
	return APIClientResult{ID: id, Name: name, Token: token, Prefix: prefix, Scopes: scopes, Active: true, CreatedAt: time.Now()}, nil
}

func APIScopeCatalog() map[string]string {
	result := make(map[string]string, len(apiScopeCatalog))
	for scope, description := range apiScopeCatalog {
		result[scope] = description
	}
	return result
}

func (s *Service) ListAPIClients(ctx context.Context) ([]APIClientResult, error) {
	rows, err := s.db.Query(ctx, `SELECT id,name,token_prefix,scopes,active AND revoked_at IS NULL,created_at FROM api_clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIClientResult
	for rows.Next() {
		var item APIClientResult
		if err = rows.Scan(&item.ID, &item.Name, &item.Prefix, &item.Scopes, &item.Active, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Service) RevokeAPIClient(ctx context.Context, id uuid.UUID, actor Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE api_clients SET active=FALSE,revoked_at=NOW() WHERE id=$1 AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, actor, "api_client.revoke", "api_client", id.String(), nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type LinkRequest struct {
	ID               uuid.UUID `json:"id"`
	Code             string    `json:"code,omitempty"`
	Status           string    `json:"status"`
	ExpiresAt        time.Time `json:"expires_at"`
	TelegramUserID   *int64    `json:"telegram_user_id,omitempty"`
	TelegramUsername string    `json:"telegram_username,omitempty"`
}

func (s *Service) StartTelegramLink(ctx context.Context, accountID uuid.UUID, actor Actor) (LinkRequest, error) {
	code, err := security.RandomToken(18)
	if err != nil {
		return LinkRequest{}, err
	}
	id := uuid.New()
	expires := time.Now().Add(10 * time.Minute)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LinkRequest{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE telegram_link_requests SET status='canceled' WHERE account_id=$1 AND status='pending'`, accountID); err != nil {
		return LinkRequest{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO telegram_link_requests(id,account_id,code_hash,expires_at) VALUES($1,$2,$3,$4)`, id, accountID, security.HashToken(code), expires); err != nil {
		return LinkRequest{}, err
	}
	if err = audit(ctx, tx, actor, "identity.telegram.link_requested", "account", accountID.String(), map[string]any{"request_id": id.String(), "expires_at": expires}); err != nil {
		return LinkRequest{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LinkRequest{}, err
	}
	return LinkRequest{ID: id, Code: code, Status: "pending", ExpiresAt: expires}, nil
}
func (s *Service) ConfirmTelegramLink(ctx context.Context, code string, tg int64, username string, actor Actor) error {
	code = strings.TrimSpace(code)
	username = strings.TrimSpace(username)
	if code == "" || len(code) > 128 || tg <= 0 || len(username) > 255 {
		return ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id, accountID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id,account_id FROM telegram_link_requests WHERE code_hash=$1 AND status='pending' AND expires_at>NOW() FOR UPDATE`, security.HashToken(code)).Scan(&id, &accountID)
	if err != nil {
		return mapNotFound(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,verified_at) VALUES($1,$2,'telegram',$3,$4,NOW())`, uuid.New(), accountID, fmt.Sprint(tg), username)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE telegram_link_requests SET status='confirmed',telegram_user_id=$2,telegram_username=$3,confirmed_at=NOW() WHERE id=$1`, id, tg, username)
	if err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "identity.telegram.bind", "account", accountID.String(), map[string]any{"telegram_user_id": tg}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) GetLinkRequest(ctx context.Context, id, accountID uuid.UUID) (LinkRequest, error) {
	var item LinkRequest
	item.ID = id
	err := s.db.QueryRow(ctx, `SELECT status,expires_at,telegram_user_id,COALESCE(telegram_username,'') FROM telegram_link_requests WHERE id=$1 AND account_id=$2`, id, accountID).Scan(&item.Status, &item.ExpiresAt, &item.TelegramUserID, &item.TelegramUsername)
	if err != nil {
		return LinkRequest{}, mapNotFound(err)
	}
	if item.Status == "pending" && time.Now().After(item.ExpiresAt) {
		item.Status = "expired"
	}
	return item, nil
}

func (s *Service) Audit(ctx context.Context, limit int) ([]map[string]any, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,actor_kind,actor_id,action,resource_type,resource_id,request_id,ip_address,details,created_at FROM audit_logs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var ak, ai, a, rt string
		var ri, rq, ip *string
		var raw []byte
		var at time.Time
		if err = rows.Scan(&id, &ak, &ai, &a, &rt, &ri, &rq, &ip, &raw, &at); err != nil {
			return nil, err
		}
		var details any
		_ = json.Unmarshal(raw, &details)
		out = append(out, map[string]any{"id": id, "actor_kind": ak, "actor_id": ai, "action": a, "resource_type": rt, "resource_id": ri, "request_id": rq, "ip_address": ip, "details": details, "created_at": at})
	}
	return out, rows.Err()
}
