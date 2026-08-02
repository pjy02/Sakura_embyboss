package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

type SessionResult struct {
	Account   Account   `json:"account"`
	Token     string    `json:"-"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Service) RegisterLocal(ctx context.Context, username, password, displayName string, actor Actor) (Account, error) {
	return s.registerLocal(ctx, username, password, displayName, actor, true)
}

func (s *Service) registerLocal(ctx context.Context, username, password, displayName string, actor Actor, enforcePublicRegistration bool) (Account, error) {
	username, normalized, err := normalizeUsername(username)
	if err != nil {
		return Account{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return Account{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if displayName == "" {
		displayName = username
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 255 {
		return Account{}, fmt.Errorf("%w: display name must contain 1 to 255 bytes", ErrInvalid)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	if enforcePublicRegistration {
		var enabled bool
		if err = tx.QueryRow(ctx, `SELECT COALESCE((value #>> '{}')::boolean,false) FROM dynamic_settings WHERE key='auth.local_registration_enabled'`).Scan(&enabled); err != nil {
			return Account{}, err
		}
		if !enabled {
			return Account{}, ErrForbidden
		}
	}
	account := Account{ID: uuid.New(), DisplayName: displayName, Status: "active", Revision: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = tx.Exec(ctx, `INSERT INTO accounts(id,display_name,status) VALUES($1,$2,'active')`, account.ID, account.DisplayName)
	if err != nil {
		return Account{}, mapUnique(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,username_normalized,password_hash,verified_at) VALUES($1,$2,'local',$3,$4,$3,$5,NOW())`, uuid.New(), account.ID, normalized, username, passwordHash)
	if err != nil {
		return Account{}, mapUnique(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,'00000000-0000-4000-8000-000000000003',$2)`, account.ID, actor.Label())
	if err != nil {
		return Account{}, err
	}
	if err = audit(ctx, tx, actor, "account.register.local", "account", account.ID.String(), map[string]any{"username": username}); err != nil {
		return Account{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, account.ID)
}

func (s *Service) AuthenticateLocal(ctx context.Context, username, password, userAgent, ip string) (SessionResult, error) {
	_, normalized, err := normalizeUsername(username)
	if err != nil {
		return SessionResult{}, ErrInvalidCredentials
	}
	var accountID uuid.UUID
	var passwordHash, status string
	err = s.db.QueryRow(ctx, `SELECT a.id,i.password_hash,a.status FROM account_identities i JOIN accounts a ON a.id=i.account_id WHERE i.kind='local' AND i.username_normalized=$1 AND NOT i.disabled`, normalized).Scan(&accountID, &passwordHash, &status)
	if err != nil || !security.VerifyPassword(password, passwordHash) {
		return SessionResult{}, ErrInvalidCredentials
	}
	if status != "active" {
		return SessionResult{}, ErrAccountDisabled
	}
	if strings.HasPrefix(passwordHash, "scrypt$") {
		if upgraded, upgradeErr := security.HashPassword(password); upgradeErr == nil {
			_, _ = s.db.Exec(ctx, `UPDATE account_identities SET password_hash=$2,last_used_at=NOW(),updated_at=NOW() WHERE account_id=$1 AND kind='local' AND password_hash=$3`, accountID, upgraded, passwordHash)
		}
	} else {
		_, _ = s.db.Exec(ctx, `UPDATE account_identities SET last_used_at=NOW(),updated_at=NOW() WHERE account_id=$1 AND kind='local'`, accountID)
	}
	return s.createSession(ctx, accountID, userAgent, ip)
}

func (s *Service) createSession(ctx context.Context, accountID uuid.UUID, userAgent, ip string) (SessionResult, error) {
	token, err := security.RandomToken(32)
	if err != nil {
		return SessionResult{}, err
	}
	csrf, err := security.RandomToken(24)
	if err != nil {
		return SessionResult{}, err
	}
	ttl := s.sessionTTL
	var configuredHours int64
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::bigint FROM dynamic_settings WHERE key='auth.session_ttl_hours'`).Scan(&configuredHours); err == nil && configuredHours > 0 && configuredHours <= 24*365 {
		ttl = time.Duration(configuredHours) * time.Hour
	}
	expires := time.Now().Add(ttl)
	sessionID := uuid.New()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return SessionResult{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO sessions(id,account_id,token_hash,csrf_hash,user_agent,ip_address,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, sessionID, accountID, security.HashToken(token), security.HashToken(csrf), userAgent, ip, expires)
	if err != nil {
		return SessionResult{}, err
	}
	if err = audit(ctx, tx, Actor{Kind: "account", ID: accountID.String(), IP: ip}, "auth.session.create", "session", sessionID.String(), map[string]any{"expires_at": expires}); err != nil {
		return SessionResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SessionResult{}, err
	}
	account, err := s.GetAccount(ctx, accountID)
	return SessionResult{Account: account, Token: token, CSRFToken: csrf, ExpiresAt: expires}, err
}

func (s *Service) AuthenticateSession(ctx context.Context, token string) (Principal, error) {
	var principal Principal
	var accountID, sessionID uuid.UUID
	var status string
	err := s.db.QueryRow(ctx, `SELECT s.id,s.account_id,s.csrf_hash,a.status FROM sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW()`, security.HashToken(token)).Scan(&sessionID, &accountID, &principal.CSRFHash, &status)
	if err != nil || status != "active" {
		return Principal{}, ErrInvalidCredentials
	}
	principal.Actor = Actor{Kind: "account", ID: accountID.String()}
	principal.AccountID = &accountID
	principal.SessionID = &sessionID
	principal.Permissions = map[string]bool{}
	principal.Scopes = map[string]bool{}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT rp.permission_code FROM account_roles ar JOIN role_permissions rp ON rp.role_id=ar.role_id WHERE ar.account_id=$1`, accountID)
	if err != nil {
		return Principal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if rows.Scan(&value) == nil {
			principal.Permissions[value] = true
		}
	}
	_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at=NOW() WHERE id=$1 AND last_seen_at<NOW()-INTERVAL '5 minutes'`, sessionID)
	return principal, rows.Err()
}

// AuthenticateTelegram resolves a Telegram adapter request to the same account
// and RBAC permissions used by Web sessions. It does not create a session and is
// only exposed through the separately authenticated internal Bot API.
func (s *Service) AuthenticateTelegram(ctx context.Context, telegramUserID int64) (Principal, error) {
	if telegramUserID <= 0 {
		return Principal{}, ErrInvalidCredentials
	}
	var accountID uuid.UUID
	var status string
	err := s.db.QueryRow(ctx, `SELECT a.id,a.status FROM account_identities i JOIN accounts a ON a.id=i.account_id WHERE i.kind='telegram' AND i.subject=$1 AND NOT i.disabled`, fmt.Sprint(telegramUserID)).Scan(&accountID, &status)
	if err != nil || status != "active" {
		return Principal{}, ErrInvalidCredentials
	}
	principal := Principal{
		Actor:       Actor{Kind: "telegram", ID: fmt.Sprint(telegramUserID)},
		AccountID:   &accountID,
		Permissions: map[string]bool{},
		Scopes:      map[string]bool{},
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT rp.permission_code FROM account_roles ar JOIN role_permissions rp ON rp.role_id=ar.role_id WHERE ar.account_id=$1`, accountID)
	if err != nil {
		return Principal{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var permission string
		if err = rows.Scan(&permission); err != nil {
			return Principal{}, err
		}
		principal.Permissions[permission] = true
	}
	return principal, rows.Err()
}

func (s *Service) AuthenticateAPIClient(ctx context.Context, token string) (Principal, error) {
	var id uuid.UUID
	var scopes []string
	err := s.db.QueryRow(ctx, `SELECT id,scopes FROM api_clients WHERE token_hash=$1 AND active AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>NOW())`, security.HashToken(token)).Scan(&id, &scopes)
	if err != nil {
		return Principal{}, ErrInvalidCredentials
	}
	result := Principal{Actor: Actor{Kind: "api_client", ID: id.String()}, Permissions: map[string]bool{}, Scopes: map[string]bool{}}
	for _, scope := range scopes {
		result.Scopes[scope] = true
	}
	_, _ = s.db.Exec(ctx, `UPDATE api_clients SET last_used_at=NOW() WHERE id=$1`, id)
	return result, nil
}

func (s *Service) RevokeSession(ctx context.Context, principal Principal) error {
	if principal.SessionID == nil {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=NOW() WHERE id=$1`, *principal.SessionID); err != nil {
		return err
	}
	if err = audit(ctx, tx, principal.Actor, "auth.session.revoke", "session", principal.SessionID.String(), nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) GetAccount(ctx context.Context, id uuid.UUID) (Account, error) {
	var a Account
	err := s.db.QueryRow(ctx, `SELECT id,display_name,status,revision,created_at,updated_at FROM accounts WHERE id=$1`, id).Scan(&a.ID, &a.DisplayName, &a.Status, &a.Revision, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return Account{}, mapNotFound(err)
	}
	rows, err := s.db.Query(ctx, `SELECT kind,subject,COALESCE(username,''),verified_at FROM account_identities WHERE account_id=$1 AND NOT disabled ORDER BY kind`, id)
	if err != nil {
		return Account{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Identity
		if err = rows.Scan(&item.Kind, &item.Subject, &item.Username, &item.VerifiedAt); err != nil {
			return Account{}, err
		}
		if item.Kind == "local" {
			item.Subject = ""
		}
		a.Identities = append(a.Identities, item)
	}
	if err = rows.Err(); err != nil {
		return Account{}, err
	}
	roleRows, err := s.db.Query(ctx, `SELECT r.code FROM account_roles ar JOIN roles r ON r.id=ar.role_id WHERE ar.account_id=$1 ORDER BY r.code`, id)
	if err != nil {
		return Account{}, err
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var role string
		if err = roleRows.Scan(&role); err != nil {
			return Account{}, err
		}
		a.Roles = append(a.Roles, role)
	}
	if err = roleRows.Err(); err != nil {
		return Account{}, err
	}
	return a, nil
}

func (s *Service) ListAccounts(ctx context.Context, limit int) ([]Account, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM accounts ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := make([]Account, 0, len(ids))
	for _, id := range ids {
		a, getErr := s.GetAccount(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		result = append(result, a)
	}
	return result, nil
}

var transitions = map[string]map[string]bool{"pending": {"active": true, "deleted": true}, "active": {"suspended": true, "banned": true, "deleted": true}, "suspended": {"active": true, "banned": true, "deleted": true}, "banned": {"active": true, "deleted": true}}

func (s *Service) ChangeLifecycle(ctx context.Context, id uuid.UUID, to, reason string, actor Actor) (Account, error) {
	to = strings.TrimSpace(to)
	reason = strings.TrimSpace(reason)
	if len(reason) > 500 {
		return Account{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	var from string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT status,revision FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&from, &revision); err != nil {
		return Account{}, mapNotFound(err)
	}
	if !transitions[from][to] || reason == "" {
		return Account{}, ErrConflict
	}
	if from == "active" && to != "active" {
		var ownerRoleID uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE code='owner' FOR UPDATE`).Scan(&ownerRoleID); err != nil {
			return Account{}, err
		}
		var targetIsOwner bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_roles WHERE account_id=$1 AND role_id=$2)`, id, ownerRoleID).Scan(&targetIsOwner); err != nil {
			return Account{}, err
		}
		if targetIsOwner {
			var activeOwners int
			if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM account_roles ar JOIN accounts a ON a.id=ar.account_id WHERE ar.role_id=$1 AND a.status='active'`, ownerRoleID).Scan(&activeOwners); err != nil {
				return Account{}, err
			}
			if activeOwners <= 1 {
				return Account{}, ErrForbidden
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE accounts SET status=$2,revision=revision+1,updated_at=NOW(),deleted_at=CASE WHEN $2='deleted' THEN NOW() ELSE deleted_at END WHERE id=$1`, id, to)
	if err != nil {
		return Account{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_lifecycle_events(account_id,from_status,to_status,reason,actor) VALUES($1,$2,$3,$4,$5)`, id, from, to, reason, actor.Label())
	if err != nil {
		return Account{}, err
	}
	if to != "active" {
		_, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=NOW() WHERE account_id=$1 AND revoked_at IS NULL`, id)
		if err != nil {
			return Account{}, err
		}
	}
	if err = audit(ctx, tx, actor, "account.lifecycle.change", "account", id.String(), map[string]any{"from": from, "to": to, "reason": reason, "revision": revision}); err != nil {
		return Account{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return s.GetAccount(ctx, id)
}

func mapUnique(err error) error {
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Service) BootstrapOwner(ctx context.Context, username, password string) error {
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("both bootstrap username and password are required")
	}
	username, normalized, err := normalizeUsername(username)
	if err != nil {
		return fmt.Errorf("bootstrap username: %w", err)
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return fmt.Errorf("bootstrap password: %w", err)
	}
	connection, err := s.db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	if _, err = connection.Exec(ctx, `SELECT pg_advisory_lock(hashtext('sakura-v3-owner-bootstrap'))`); err != nil {
		return err
	}
	defer connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('sakura-v3-owner-bootstrap'))`)
	tx, err := connection.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM account_roles WHERE role_id='00000000-0000-4000-8000-000000000001'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	actor := Actor{Kind: "system", ID: "bootstrap"}
	accountID := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO accounts(id,display_name,status) VALUES($1,'Sakura Owner','active')`, accountID); err != nil {
		return mapUnique(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,username_normalized,password_hash,verified_at) VALUES($1,$2,'local',$3,$4,$3,$5,NOW())`, uuid.New(), accountID, normalized, username, passwordHash); err != nil {
		return mapUnique(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,assigned_by) VALUES($1,'00000000-0000-4000-8000-000000000001',$2)`, accountID, actor.Label()); err != nil {
		return err
	}
	if err = audit(ctx, tx, actor, "account.owner.bootstrap", "account", accountID.String(), nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
