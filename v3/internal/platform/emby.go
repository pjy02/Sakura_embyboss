package platform

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

var embyUsernamePattern = regexp.MustCompile(`^[^\s/\\<>]{3,32}$`)

func (s *Service) ListInstances(ctx context.Context) ([]EmbyInstance, error) {
	rows, err := s.db.Query(ctx, `
		SELECT i.id,i.name,i.base_url,i.credential_name,i.enabled,i.is_default,i.verify_tls,i.priority,i.status,
		       COALESCE(i.server_id,''),COALESCE(i.server_version,''),COALESCE(i.last_error,''),i.last_latency_ms,
		       i.last_checked_at,i.last_snapshot_at,i.revision,
		       COUNT(DISTINCT b.id),COUNT(DISTINCT u.id) FILTER (WHERE u.claim_status='unclaimed'),i.created_at,i.updated_at
		FROM emby_instances i
		LEFT JOIN emby_account_bindings b ON b.instance_id=i.id AND b.status<>'deleted'
		LEFT JOIN remote_emby_users u ON u.instance_id=i.id
		GROUP BY i.id ORDER BY i.is_default DESC,i.priority,i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbyInstance
	for rows.Next() {
		var item EmbyInstance
		if err = rows.Scan(&item.ID, &item.Name, &item.BaseURL, &item.CredentialName, &item.Enabled, &item.IsDefault, &item.VerifyTLS, &item.Priority, &item.Status, &item.ServerID, &item.ServerVersion, &item.LastError, &item.LastLatencyMS, &item.LastCheckedAt, &item.LastSnapshotAt, &item.Revision, &item.BindingCount, &item.UnclaimedCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveInstance(ctx context.Context, id *uuid.UUID, name, baseURL, credentialName string, enabled, isDefault, verifyTLS bool, priority int, expectedRevision int64, actor identity.Actor) (EmbyInstance, error) {
	name = strings.TrimSpace(name)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/emby") {
		baseURL = strings.TrimRight(baseURL[:len(baseURL)-len("/emby")], "/")
	}
	credentialName = strings.TrimSpace(credentialName)
	parsed, err := url.Parse(baseURL)
	if err != nil || name == "" || len(name) > 120 || len(baseURL) > 512 || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Scheme != "http" && parsed.Scheme != "https" || credentialName == "" || priority < 0 || priority > 100000 {
		return EmbyInstance{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EmbyInstance{}, err
	}
	defer tx.Rollback(ctx)
	var credentialExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credentials WHERE name=$1 AND kind IN ('emby_api_token','api_token','emby'))`, credentialName).Scan(&credentialExists); err != nil {
		return EmbyInstance{}, err
	}
	if !credentialExists {
		return EmbyInstance{}, identity.ErrNotFound
	}
	instanceID := uuid.New()
	action := "emby_instance.create"
	if id != nil {
		instanceID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM emby_instances WHERE id=$1 FOR UPDATE`, instanceID).Scan(&revision); err != nil {
			return EmbyInstance{}, notFound(err)
		}
		if expectedRevision != revision {
			return EmbyInstance{}, identity.ErrConflict
		}
		action = "emby_instance.update"
	}
	if isDefault {
		if _, err = tx.Exec(ctx, `UPDATE emby_instances SET is_default=FALSE,updated_at=NOW() WHERE is_default AND id<>$1`, instanceID); err != nil {
			return EmbyInstance{}, err
		}
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO emby_instances(id,name,base_url,credential_name,enabled,is_default,verify_tls,priority,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $5 THEN 'unknown' ELSE 'disabled' END)`, instanceID, name, baseURL, credentialName, enabled, isDefault, verifyTLS, priority)
	} else {
		_, err = tx.Exec(ctx, `UPDATE emby_instances SET name=$2,base_url=$3,credential_name=$4,enabled=$5,is_default=$6,verify_tls=$7,priority=$8,status=CASE WHEN $5 THEN CASE WHEN status='disabled' THEN 'unknown' ELSE status END ELSE 'disabled' END,revision=revision+1,updated_at=NOW() WHERE id=$1`, instanceID, name, baseURL, credentialName, enabled, isDefault, verifyTLS, priority)
	}
	if err != nil {
		return EmbyInstance{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "emby_instance", instanceID.String(), map[string]any{"name": name, "base_url": baseURL, "enabled": enabled, "is_default": isDefault}); err != nil {
		return EmbyInstance{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EmbyInstance{}, err
	}
	return s.GetInstance(ctx, instanceID)
}

func (s *Service) GetInstance(ctx context.Context, id uuid.UUID) (EmbyInstance, error) {
	items, err := s.ListInstances(ctx)
	if err != nil {
		return EmbyInstance{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return EmbyInstance{}, identity.ErrNotFound
}

func (s *Service) ListBindings(ctx context.Context, accountID *uuid.UUID, instanceID *uuid.UUID, limit int) ([]Binding, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT b.id,b.account_id,b.instance_id,i.name,b.remote_user_id,b.remote_username,b.status,b.origin,b.is_primary,
		       b.expires_at,b.remote_disabled,b.remote_snapshot,b.claimed_at,b.last_synced_at,COALESCE(b.last_error,''),b.created_at,b.updated_at
		FROM emby_account_bindings b JOIN emby_instances i ON i.id=b.instance_id
		WHERE ($1::uuid IS NULL OR b.account_id=$1) AND ($2::uuid IS NULL OR b.instance_id=$2)
		ORDER BY b.created_at DESC LIMIT $3`, uuidQueryValue(accountID), uuidQueryValue(instanceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Binding
	for rows.Next() {
		var item Binding
		var raw []byte
		if err = rows.Scan(&item.ID, &item.AccountID, &item.InstanceID, &item.InstanceName, &item.RemoteUserID, &item.RemoteUsername, &item.Status, &item.Origin, &item.IsPrimary, &item.ExpiresAt, &item.RemoteDisabled, &raw, &item.ClaimedAt, &item.LastSyncedAt, &item.LastError, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.RemoteSnapshot = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) RequestProvisioning(ctx context.Context, accountID uuid.UUID, instanceID *uuid.UUID, username, invitationCode, idempotencyKey string, actor identity.Actor) (ProvisionResult, error) {
	username = strings.TrimSpace(username)
	normalizedUsername := normalize(username)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if !embyUsernamePattern.MatchString(username) || idempotencyKey == "" || len(idempotencyKey) > 160 || s.vault == nil {
		return ProvisionResult{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ProvisionResult{}, err
	}
	defer tx.Rollback(ctx)
	var replayTaskID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM platform_tasks WHERE idempotency_key=$1`, "provision:"+accountID.String()+":"+idempotencyKey).Scan(&replayTaskID)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return ProvisionResult{}, err
		}
		return s.GetProvisioning(ctx, accountID, replayTaskID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ProvisionResult{}, err
	}
	var accountStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&accountStatus); err != nil {
		return ProvisionResult{}, notFound(err)
	}
	if accountStatus != "active" {
		return ProvisionResult{}, identity.ErrForbidden
	}
	selectedInstanceID, err := selectInstanceTx(ctx, tx, instanceID)
	if err != nil {
		return ProvisionResult{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emby_account_bindings WHERE account_id=$1 AND instance_id=$2 AND status<>'deleted') OR EXISTS(SELECT 1 FROM emby_provision_requests WHERE account_id=$1 AND instance_id=$2 AND status IN ('pending','provisioning','succeeded'))`, accountID, selectedInstanceID).Scan(&exists); err != nil {
		return ProvisionResult{}, err
	}
	if exists {
		return ProvisionResult{}, identity.ErrConflict
	}
	var membership Membership
	var invitationID *uuid.UUID
	if strings.TrimSpace(invitationCode) != "" {
		membership, invitationID, err = s.redeemInvitationTx(ctx, tx, accountID, invitationCode, "provision-invite:"+accountID.String()+":"+idempotencyKey, actor)
	} else {
		membership, err = scanMembership(tx.QueryRow(ctx, `SELECT m.id,m.account_id,m.plan_id,p.code,p.name,m.status,m.starts_at,m.expires_at,m.source,p.entitlements FROM account_memberships m JOIN membership_plans p ON p.id=m.plan_id WHERE m.account_id=$1 AND m.status IN ('active','grace') AND m.expires_at>NOW() ORDER BY m.expires_at DESC LIMIT 1 FOR UPDATE OF m`, accountID))
	}
	if err != nil {
		return ProvisionResult{}, identity.ErrForbidden
	}
	maximum := entitlementInt(membership.Benefits, "max_emby_accounts", 1)
	var bindingCount int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM emby_account_bindings WHERE account_id=$1 AND status<>'deleted'`, accountID).Scan(&bindingCount); err != nil {
		return ProvisionResult{}, err
	}
	if bindingCount >= maximum {
		return ProvisionResult{}, identity.ErrForbidden
	}
	password, err := randomAlphaNumeric(16)
	if err != nil {
		return ProvisionResult{}, err
	}
	ciphertext, nonce, keyVersion, err := s.vault.Encrypt([]byte(password))
	if err != nil {
		return ProvisionResult{}, err
	}
	maxAttempts := s.dynamicIntTx(ctx, tx, "emby.task_max_attempts", 8)
	taskID := uuid.New()
	requestID := uuid.New()
	payload := map[string]any{"provision_request_id": requestID.String(), "instance_id": selectedInstanceID.String(), "account_id": accountID.String()}
	_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'emby.provision',$2,$3,$4,$5)`, taskID, "provision:"+accountID.String()+":"+idempotencyKey, jsonBytes(payload), maxAttempts, actor.Label())
	if err != nil {
		return ProvisionResult{}, identity.ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO emby_provision_requests(id,task_id,account_id,instance_id,membership_id,invitation_id,requested_username,requested_username_normalized,password_ciphertext,password_nonce,password_key_version,password_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW()+INTERVAL '24 hours')`, requestID, taskID, accountID, selectedInstanceID, membership.ID, invitationID, username, normalizedUsername, ciphertext, nonce, keyVersion)
	if err != nil {
		return ProvisionResult{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "emby.provision.request", "account", accountID.String(), map[string]any{"task_id": taskID, "instance_id": selectedInstanceID, "username": username}); err != nil {
		return ProvisionResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ProvisionResult{}, err
	}
	return s.GetProvisioning(ctx, accountID, taskID)
}

func selectInstanceTx(ctx context.Context, tx pgx.Tx, requested *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	if requested != nil {
		if err := tx.QueryRow(ctx, `SELECT id FROM emby_instances WHERE id=$1 AND enabled FOR SHARE`, *requested).Scan(&id); err != nil {
			return uuid.Nil, notFound(err)
		}
		return id, nil
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM emby_instances WHERE enabled ORDER BY is_default DESC,priority,id LIMIT 1 FOR SHARE`).Scan(&id); err != nil {
		return uuid.Nil, notFound(err)
	}
	return id, nil
}

func entitlementInt(value map[string]any, key string, fallback int) int {
	raw, ok := value[key]
	if !ok {
		return fallback
	}
	switch number := raw.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	default:
		return fallback
	}
}

func (s *Service) dynamicIntTx(ctx context.Context, tx pgx.Tx, key string, fallback int) int {
	var value int
	if tx.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key=$1`, key).Scan(&value) != nil {
		return fallback
	}
	return value
}

func (s *Service) GetProvisioning(ctx context.Context, accountID, taskID uuid.UUID) (ProvisionResult, error) {
	var result ProvisionResult
	var task Task
	var rawResult []byte
	var ciphertext, nonce []byte
	var keyVersion int
	var passwordExpires time.Time
	err := s.db.QueryRow(ctx, `
		SELECT t.id,t.task_type,t.status,t.result,t.attempts,t.max_attempts,COALESCE(t.last_error,''),t.created_at,t.started_at,t.finished_at,t.updated_at,
		       r.account_id,r.instance_id,r.requested_username,COALESCE(r.remote_user_id,''),r.password_ciphertext,r.password_nonce,r.password_key_version,r.password_expires_at
		FROM platform_tasks t JOIN emby_provision_requests r ON r.task_id=t.id
		WHERE t.id=$1 AND r.account_id=$2`, taskID, accountID).Scan(&task.ID, &task.TaskType, &task.Status, &rawResult, &task.Attempts, &task.MaxAttempts, &task.LastError, &task.CreatedAt, &task.StartedAt, &task.FinishedAt, &task.UpdatedAt, &result.AccountID, &result.InstanceID, &result.Username, &result.RemoteUserID, &ciphertext, &nonce, &keyVersion, &passwordExpires)
	if err != nil {
		return ProvisionResult{}, notFound(err)
	}
	task.Result = decodeJSON(rawResult)
	result.Task = task
	if task.Status == "succeeded" && time.Now().Before(passwordExpires) && s.vault != nil {
		password, decryptErr := s.vault.Decrypt(ciphertext, nonce, keyVersion)
		if decryptErr == nil {
			result.GeneratedPassword = string(password)
			result.PasswordExpiresAt = &passwordExpires
		}
	}
	return result, nil
}

func (s *Service) AccountIDByTelegram(ctx context.Context, telegramUserID int64) (uuid.UUID, error) {
	var accountID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT account_id FROM account_identities WHERE kind='telegram' AND subject=$1 AND NOT disabled`, fmt.Sprint(telegramUserID)).Scan(&accountID)
	if err != nil {
		return uuid.Nil, notFound(err)
	}
	return accountID, nil
}

func (s *Service) ListRemoteUsers(ctx context.Context, instanceID *uuid.UUID, claimStatus string, limit int) ([]RemoteUser, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT u.id,u.instance_id,i.name,u.remote_user_id,u.username,u.disabled,u.claim_status,u.binding_id,u.snapshot,u.first_seen_at,u.last_seen_at,u.missing_since,u.updated_at FROM remote_emby_users u JOIN emby_instances i ON i.id=u.instance_id WHERE ($1::uuid IS NULL OR u.instance_id=$1) AND ($2='' OR u.claim_status=$2) ORDER BY u.last_seen_at DESC LIMIT $3`, uuidQueryValue(instanceID), claimStatus, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RemoteUser
	for rows.Next() {
		var item RemoteUser
		var raw []byte
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.InstanceName, &item.RemoteUserID, &item.Username, &item.Disabled, &item.ClaimStatus, &item.BindingID, &raw, &item.FirstSeenAt, &item.LastSeenAt, &item.MissingSince, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Snapshot = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) GenerateClaimToken(ctx context.Context, remoteUserID uuid.UUID, expiresIn time.Duration, actor identity.Actor) (string, error) {
	if expiresIn < time.Minute || expiresIn > 7*24*time.Hour {
		return "", identity.ErrInvalid
	}
	token, err := security.RandomToken(24)
	if err != nil {
		return "", err
	}
	token = "claim_" + token
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT claim_status FROM remote_emby_users WHERE id=$1 FOR UPDATE`, remoteUserID).Scan(&status); err != nil {
		return "", notFound(err)
	}
	if status != "unclaimed" {
		return "", identity.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE emby_claim_tokens SET status='revoked' WHERE remote_user_id=$1 AND status='active'`, remoteUserID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO emby_claim_tokens(id,remote_user_id,token_hash,token_hint,expires_at,issued_by) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), remoteUserID, security.HashToken(token), token[len(token)-8:], time.Now().Add(expiresIn), actor.Label()); err != nil {
		return "", err
	}
	if err = audit(ctx, tx, actor, "emby.claim_token.generate", "remote_emby_user", remoteUserID.String(), nil); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) ClaimRemoteUser(ctx context.Context, accountID uuid.UUID, token string, actor identity.Actor) (Binding, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Binding{}, err
	}
	defer tx.Rollback(ctx)
	var tokenID, remoteID, instanceID uuid.UUID
	var remoteUserID, username, claimStatus string
	var disabled bool
	var snapshot []byte
	err = tx.QueryRow(ctx, `SELECT c.id,u.id,u.instance_id,u.remote_user_id,u.username,u.disabled,u.claim_status,u.snapshot FROM emby_claim_tokens c JOIN remote_emby_users u ON u.id=c.remote_user_id WHERE c.token_hash=$1 AND c.status='active' AND c.expires_at>NOW() FOR UPDATE OF c,u`, security.HashToken(strings.TrimSpace(token))).Scan(&tokenID, &remoteID, &instanceID, &remoteUserID, &username, &disabled, &claimStatus, &snapshot)
	if err != nil {
		return Binding{}, identity.ErrNotFound
	}
	if claimStatus != "unclaimed" {
		return Binding{}, identity.ErrConflict
	}
	if err = enforceBindingCapacityTx(ctx, tx, accountID); err != nil {
		return Binding{}, err
	}
	binding, err := s.createBindingTx(ctx, tx, accountID, instanceID, remoteUserID, username, disabled, decodeJSON(snapshot), "manual_claim", actor)
	if err != nil {
		return Binding{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE remote_emby_users SET claim_status='claimed',binding_id=$2,updated_at=NOW() WHERE id=$1`, remoteID, binding.ID); err != nil {
		return Binding{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE emby_claim_tokens SET status='used',used_by_account_id=$2,used_at=NOW() WHERE id=$1`, tokenID, accountID); err != nil {
		return Binding{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func enforceBindingCapacityTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT p.entitlements FROM account_memberships m JOIN membership_plans p ON p.id=m.plan_id WHERE m.account_id=$1 AND m.status IN ('active','grace') AND m.expires_at>NOW() ORDER BY m.expires_at DESC LIMIT 1 FOR SHARE OF m`, accountID).Scan(&raw); err != nil {
		return identity.ErrForbidden
	}
	maximum := entitlementInt(decodeJSON(raw), "max_emby_accounts", 1)
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM emby_account_bindings WHERE account_id=$1 AND status<>'deleted'`, accountID).Scan(&count); err != nil {
		return err
	}
	if count >= maximum {
		return identity.ErrForbidden
	}
	return nil
}

func (s *Service) AdminClaimRemoteUser(ctx context.Context, accountID, remoteID uuid.UUID, actor identity.Actor) (Binding, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Binding{}, err
	}
	defer tx.Rollback(ctx)
	var instanceID uuid.UUID
	var remoteUserID, username, claimStatus string
	var disabled bool
	var snapshot []byte
	if err = tx.QueryRow(ctx, `SELECT instance_id,remote_user_id,username,disabled,claim_status,snapshot FROM remote_emby_users WHERE id=$1 FOR UPDATE`, remoteID).Scan(&instanceID, &remoteUserID, &username, &disabled, &claimStatus, &snapshot); err != nil {
		return Binding{}, notFound(err)
	}
	if claimStatus != "unclaimed" {
		return Binding{}, identity.ErrConflict
	}
	binding, err := s.createBindingTx(ctx, tx, accountID, instanceID, remoteUserID, username, disabled, decodeJSON(snapshot), "remote_import", actor)
	if err != nil {
		return Binding{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE remote_emby_users SET claim_status='claimed',binding_id=$2,updated_at=NOW() WHERE id=$1`, remoteID, binding.ID); err != nil {
		return Binding{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func (s *Service) createBindingTx(ctx context.Context, tx pgx.Tx, accountID, instanceID uuid.UUID, remoteUserID, username string, disabled bool, snapshot map[string]any, origin string, actor identity.Actor) (Binding, error) {
	var accountStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM accounts WHERE id=$1`, accountID).Scan(&accountStatus); err != nil {
		return Binding{}, notFound(err)
	}
	if accountStatus != "active" {
		return Binding{}, identity.ErrForbidden
	}
	var primary bool
	if err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM emby_account_bindings WHERE account_id=$1 AND status<>'deleted')`, accountID).Scan(&primary); err != nil {
		return Binding{}, err
	}
	id := uuid.New()
	status := "active"
	if disabled {
		status = "suspended"
	}
	_, err := tx.Exec(ctx, `INSERT INTO emby_account_bindings(id,account_id,instance_id,remote_user_id,remote_username,status,origin,is_primary,remote_disabled,remote_snapshot,claimed_at,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())`, id, accountID, instanceID, remoteUserID, username, status, origin, primary, disabled, jsonBytes(snapshot))
	if err != nil {
		return Binding{}, identity.ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account_identities(id,account_id,kind,subject,username,verified_at,metadata) VALUES($1,$2,'emby',$3,$4,NOW(),$5) ON CONFLICT(kind,subject) DO NOTHING`, uuid.New(), accountID, instanceID.String()+":"+remoteUserID, username, jsonBytes(map[string]any{"instance_id": instanceID, "binding_id": id})); err != nil {
		return Binding{}, err
	}
	if err = audit(ctx, tx, actor, "emby.binding.claim", "emby_binding", id.String(), map[string]any{"account_id": accountID, "instance_id": instanceID, "remote_user_id": remoteUserID, "origin": origin}); err != nil {
		return Binding{}, err
	}
	return Binding{ID: id, AccountID: accountID, InstanceID: instanceID, RemoteUserID: remoteUserID, RemoteUsername: username, Status: status, Origin: origin, IsPrimary: primary, RemoteDisabled: &disabled, RemoteSnapshot: snapshot, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (s *Service) AdoptLegacyIdentities(ctx context.Context, instanceID uuid.UUID, actor identity.Actor) (map[string]int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var instanceExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emby_instances WHERE id=$1)`, instanceID).Scan(&instanceExists); err != nil || !instanceExists {
		return nil, identity.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT account_id,subject,COALESCE(username,subject) FROM account_identities WHERE kind='emby' AND subject NOT LIKE '%:%' AND NOT disabled ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	type legacyIdentity struct {
		accountID uuid.UUID
		remoteID  string
		username  string
	}
	var legacy []legacyIdentity
	defer rows.Close()
	for rows.Next() {
		var item legacyIdentity
		if err = rows.Scan(&item.accountID, &item.remoteID, &item.username); err != nil {
			return nil, err
		}
		legacy = append(legacy, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	created, skipped := 0, 0
	for _, item := range legacy {
		accountID, remoteUserID, username := item.accountID, item.remoteID, item.username
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emby_account_bindings WHERE account_id=$1 AND instance_id=$2) OR EXISTS(SELECT 1 FROM emby_account_bindings WHERE instance_id=$2 AND remote_user_id=$3)`, accountID, instanceID, remoteUserID).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			skipped++
			continue
		}
		binding, createErr := s.createBindingTx(ctx, tx, accountID, instanceID, remoteUserID, username, false, map[string]any{"source": "v2_identity"}, "legacy_import", actor)
		if createErr != nil {
			return nil, createErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO remote_emby_users(id,instance_id,remote_user_id,username,username_normalized,claim_status,binding_id,snapshot) VALUES($1,$2,$3,$4,$5,'claimed',$6,$7) ON CONFLICT(instance_id,remote_user_id) DO UPDATE SET claim_status='claimed',binding_id=EXCLUDED.binding_id,updated_at=NOW()`, uuid.New(), instanceID, remoteUserID, username, normalize(username), binding.ID, jsonBytes(map[string]any{"source": "v2_identity"}))
		if err != nil {
			return nil, err
		}
		created++
	}
	if err = audit(ctx, tx, actor, "emby.legacy.adopt", "emby_instance", instanceID.String(), map[string]any{"created": created, "skipped": skipped}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]int{"created": created, "skipped": skipped}, nil
}
