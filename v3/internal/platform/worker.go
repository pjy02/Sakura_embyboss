package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

type Worker struct {
	db       *pgxpool.Pool
	vault    *security.Vault
	service  *Service
	logger   *slog.Logger
	id       string
	poll     time.Duration
	lease    time.Duration
	lastPlan time.Time
}

type claimedTask struct {
	ID       uuid.UUID
	Type     string
	Payload  map[string]any
	Attempts int
	Maximum  int
}

func NewWorker(db *pgxpool.Pool, vault *security.Vault, logger *slog.Logger, id string, poll, lease time.Duration) *Worker {
	if poll <= 0 {
		poll = time.Second
	}
	if lease < 30*time.Second {
		lease = 90 * time.Second
	}
	return &Worker{db: db, vault: vault, service: New(db, vault), logger: logger, id: id, poll: poll, lease: lease}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("platform worker started", "worker_id", w.id)
	for ctx.Err() == nil {
		worked, err := w.ProcessNext(ctx)
		if err != nil {
			w.logger.Warn("platform task cycle failed", "error", err)
		}
		batchWorked, batchErr := w.service.ProcessNextBatch(ctx, w.id, w.lease)
		if batchErr != nil {
			w.logger.Warn("batch operation cycle failed", "error", batchErr)
		}
		worked = worked || batchWorked
		automationWorked, automationErr := w.service.ProcessNextAutomation(ctx, w.id, w.lease)
		if automationErr != nil {
			w.logger.Warn("automation event cycle failed", "error", automationErr)
		}
		worked = worked || automationWorked
		if time.Since(w.lastPlan) >= 30*time.Second {
			if err = w.service.ScheduleDue(ctx, time.Now()); err != nil {
				w.logger.Warn("task scheduling failed", "error", err)
			}
			w.lastPlan = time.Now()
		}
		if !worked && !waitContext(ctx, w.poll) {
			return nil
		}
	}
	return nil
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (w *Worker) ProcessNext(ctx context.Context) (bool, error) {
	task, ok, err := w.claim(ctx)
	if err != nil || !ok {
		return ok, err
	}
	taskContext, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.keepLease(taskContext, task.ID)
	}()
	result, processErr := w.process(taskContext, task)
	cancel()
	<-done
	if processErr == nil {
		return true, w.finish(ctx, task, result)
	}
	w.logger.Warn("platform task failed", "task_id", task.ID, "task_type", task.Type, "attempt", task.Attempts, "error", processErr)
	return true, w.fail(ctx, task, processErr)
}

func (w *Worker) claim(ctx context.Context) (claimedTask, bool, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return claimedTask{}, false, err
	}
	defer tx.Rollback(ctx)
	var item claimedTask
	var raw []byte
	err = tx.QueryRow(ctx, `
		SELECT id,task_type,payload,attempts+1,max_attempts
		FROM platform_tasks
		WHERE ((status IN ('pending','retry') AND available_at<=NOW()) OR (status='running' AND lease_expires_at<NOW()))
		ORDER BY available_at,created_at
		FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&item.ID, &item.Type, &raw, &item.Attempts, &item.Maximum)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedTask{}, false, nil
	}
	if err != nil {
		return claimedTask{}, false, err
	}
	item.Payload = decodeJSON(raw)
	_, err = tx.Exec(ctx, `UPDATE platform_tasks SET status='running',attempts=$2,started_at=COALESCE(started_at,NOW()),lease_owner=$3,lease_expires_at=NOW()+($4::double precision*INTERVAL '1 second'),updated_at=NOW() WHERE id=$1`, item.ID, item.Attempts, w.id, w.lease.Seconds())
	if err != nil {
		return claimedTask{}, false, err
	}
	if item.Type == "emby.provision" {
		_, err = tx.Exec(ctx, `UPDATE emby_provision_requests SET status='provisioning',updated_at=NOW() WHERE task_id=$1 AND status IN ('pending','failed','provisioning')`, item.ID)
		if err != nil {
			return claimedTask{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return claimedTask{}, false, err
	}
	return item, true, nil
}

func (w *Worker) keepLease(ctx context.Context, taskID uuid.UUID) {
	interval := w.lease / 3
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := w.db.Exec(ctx, `UPDATE platform_tasks SET lease_expires_at=NOW()+($3::double precision*INTERVAL '1 second'),updated_at=NOW() WHERE id=$1 AND status='running' AND lease_owner=$2`, taskID, w.id, w.lease.Seconds())
			if err != nil || result.RowsAffected() != 1 {
				w.logger.Warn("task lease renewal failed", "task_id", taskID, "error", err)
				return
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, task claimedTask) (map[string]any, error) {
	if task.Type == "moviepilot.submit" {
		return w.submitMoviePilot(ctx, task)
	}
	if task.Type == "line.probe" {
		lineID, err := uuid.Parse(fmt.Sprint(task.Payload["line_id"]))
		if err != nil {
			return nil, PermanentError{Err: errors.New("line probe task has invalid line id")}
		}
		sample, err := w.service.ProbeLine(ctx, lineID, identity.Actor{Kind: "system", ID: w.id})
		if err != nil {
			return nil, err
		}
		return map[string]any{"line_id": lineID, "status": sample.Status, "latency_ms": sample.LatencyMS}, nil
	}
	instanceID, err := uuid.Parse(fmt.Sprint(task.Payload["instance_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("task has invalid instance id")}
	}
	instance, client, err := w.client(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	switch task.Type {
	case "emby.provision":
		return w.provision(ctx, task, instance, client)
	case "emby.sync", "emby.import":
		return w.sync(ctx, task, instance, client, strings.TrimPrefix(task.Type, "emby."))
	case "emby.reconcile":
		return w.reconcile(ctx, task, instance, client)
	case "emby.playback_sync":
		return w.playbackSync(ctx, task, instance, client)
	case "risk.action":
		return w.executeRiskAction(ctx, task, instance, client, false)
	case "risk.revert":
		return w.executeRiskAction(ctx, task, instance, client, true)
	case "media.match":
		return w.matchMedia(ctx, task, instance, client)
	case "entitlement.sync":
		return w.syncEntitlement(ctx, task, instance, client)
	case "emby.favorite":
		return w.applyFavorite(ctx, task, instance, client)
	case "emby.favorite_sync":
		return w.syncFavorites(ctx, task, instance, client)
	default:
		return nil, PermanentError{Err: fmt.Errorf("unsupported task type %q", task.Type)}
	}
}

func (w *Worker) syncEntitlement(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	bindingID, err := uuid.Parse(fmt.Sprint(task.Payload["binding_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("entitlement task has invalid binding id")}
	}
	var remoteID string
	if err = w.db.QueryRow(ctx, `SELECT remote_user_id FROM emby_account_bindings WHERE id=$1 AND instance_id=$2 AND status<>'deleted'`, bindingID, instance.ID).Scan(&remoteID); err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	users, err := client.users(ctx)
	if err != nil {
		return nil, err
	}
	var remote embyUser
	for _, candidate := range users {
		if candidate.ID == remoteID {
			remote = candidate
			break
		}
	}
	if remote.ID == "" {
		return nil, PermanentError{Err: identity.ErrNotFound}
	}
	rows, err := w.db.Query(ctx, `SELECT resource_key FROM account_entitlements WHERE binding_id=$1 AND resource_kind='emby_library' AND status='active' AND expires_at>NOW() ORDER BY resource_key`, bindingID)
	if err != nil {
		return nil, err
	}
	var managedFolders []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			rows.Close()
			return nil, err
		}
		managedFolders = append(managedFolders, value)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	policy := remote.Policy
	if policy == nil {
		policy = map[string]any{}
	}
	currentFolders := policyFolders(policy["EnabledFolders"])
	baselineFolders := append([]string(nil), currentFolders...)
	lastManagedFolders := []string{}
	var baselineRaw, managedRaw []byte
	managementErr := w.db.QueryRow(ctx, `SELECT baseline_folders,last_managed_folders FROM emby_policy_management WHERE binding_id=$1`, bindingID).Scan(&baselineRaw, &managedRaw)
	if managementErr == nil {
		baselineFolders = decodeStringList(baselineRaw)
		lastManagedFolders = decodeStringList(managedRaw)
	} else if !errors.Is(managementErr, pgx.ErrNoRows) {
		return nil, managementErr
	}
	desiredFolders := mergeFolderSets(baselineFolders, subtractFolderSet(currentFolders, lastManagedFolders), managedFolders)
	policy["EnableAllFolders"] = false
	policy["EnabledFolders"] = desiredFolders
	if err = client.setUserPolicy(ctx, remoteID, policy); err != nil {
		return nil, err
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO emby_policy_management(binding_id,baseline_folders,last_managed_folders) VALUES($1,$2,$3) ON CONFLICT(binding_id) DO UPDATE SET last_managed_folders=EXCLUDED.last_managed_folders,updated_at=NOW()`, bindingID, jsonBytes(baselineFolders), jsonBytes(managedFolders)); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_entitlements SET status=CASE WHEN expires_at<=NOW() THEN 'expired' ELSE status END,updated_at=NOW() WHERE binding_id=$1`, bindingID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"binding_id": bindingID, "enabled_folders": desiredFolders, "managed_folders": managedFolders}, nil
}

func policyFolders(value any) []string {
	result := []string{}
	switch values := value.(type) {
	case []string:
		result = append(result, values...)
	case []any:
		for _, item := range values {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
	}
	return mergeFolderSets(result)
}

func decodeStringList(raw []byte) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return []string{}
	}
	return mergeFolderSets(values)
}

func subtractFolderSet(values, removed []string) []string {
	remove := make(map[string]struct{}, len(removed))
	for _, item := range removed {
		remove[item] = struct{}{}
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if _, exists := remove[item]; !exists {
			result = append(result, item)
		}
	}
	return result
}

func mergeFolderSets(groups ...[]string) []string {
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, item := range group {
			item = strings.TrimSpace(item)
			if item != "" {
				seen[item] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for item := range seen {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func (w *Worker) applyFavorite(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	favoriteID, err := uuid.Parse(fmt.Sprint(task.Payload["favorite_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("favorite task has invalid favorite id")}
	}
	var remoteUserID, itemID string
	var desired bool
	err = w.db.QueryRow(ctx, `SELECT b.remote_user_id,f.remote_item_id,f.desired_favorite FROM emby_favorites f JOIN emby_account_bindings b ON b.id=f.binding_id WHERE f.id=$1 AND f.instance_id=$2`, favoriteID, instance.ID).Scan(&remoteUserID, &itemID, &desired)
	if err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	if err = client.setFavorite(ctx, remoteUserID, itemID, desired); err != nil {
		_, _ = w.db.Exec(ctx, `UPDATE emby_favorites SET sync_status='failed',last_error=$2,updated_at=NOW() WHERE id=$1`, favoriteID, truncateError(err))
		return nil, err
	}
	_, err = w.db.Exec(ctx, `UPDATE emby_favorites SET remote_favorite=desired_favorite,sync_status='synced',last_error=NULL,last_synced_at=NOW(),updated_at=NOW() WHERE id=$1`, favoriteID)
	return map[string]any{"favorite_id": favoriteID, "favorite": desired}, err
}

func (w *Worker) syncFavorites(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	bindingID, err := uuid.Parse(fmt.Sprint(task.Payload["binding_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("favorite sync task has invalid binding id")}
	}
	var accountID uuid.UUID
	var remoteUserID string
	if err = w.db.QueryRow(ctx, `SELECT account_id,remote_user_id FROM emby_account_bindings WHERE id=$1 AND instance_id=$2`, bindingID, instance.ID).Scan(&accountID, &remoteUserID); err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	items, err := client.favoriteItems(ctx, remoteUserID)
	if err != nil {
		return nil, err
	}
	seen := make([]string, 0, len(items))
	for _, raw := range items {
		id := fmt.Sprint(raw["Id"])
		title := fmt.Sprint(raw["Name"])
		if id == "" || title == "" {
			continue
		}
		seen = append(seen, id)
		_, err = w.db.Exec(ctx, `INSERT INTO emby_favorites(id,account_id,instance_id,binding_id,remote_item_id,title,media_type,image_tag,remote_snapshot,desired_favorite,remote_favorite,sync_status,last_synced_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,TRUE,TRUE,'synced',NOW()) ON CONFLICT(binding_id,remote_item_id) DO UPDATE SET title=EXCLUDED.title,media_type=EXCLUDED.media_type,image_tag=EXCLUDED.image_tag,remote_snapshot=EXCLUDED.remote_snapshot,desired_favorite=CASE WHEN emby_favorites.sync_status='pending' THEN emby_favorites.desired_favorite ELSE TRUE END,remote_favorite=TRUE,sync_status=CASE WHEN emby_favorites.sync_status='pending' THEN 'pending' ELSE 'synced' END,last_error=CASE WHEN emby_favorites.sync_status='pending' THEN emby_favorites.last_error ELSE NULL END,last_synced_at=NOW(),updated_at=NOW()`, uuid.New(), accountID, instance.ID, bindingID, id, title, fmt.Sprint(raw["Type"]), favoriteImageTag(raw), jsonBytes(raw))
		if err != nil {
			return nil, err
		}
	}
	_, err = w.db.Exec(ctx, `UPDATE emby_favorites SET remote_favorite=FALSE,desired_favorite=CASE WHEN sync_status='pending' THEN desired_favorite ELSE FALSE END,sync_status=CASE WHEN sync_status='pending' THEN 'pending' ELSE 'synced' END,last_synced_at=NOW(),updated_at=NOW() WHERE binding_id=$1 AND NOT (remote_item_id=ANY($2::text[]))`, bindingID, seen)
	return map[string]any{"binding_id": bindingID, "favorites": len(seen)}, err
}

func (w *Worker) client(ctx context.Context, instanceID uuid.UUID) (EmbyInstance, *embyClient, error) {
	instance, err := w.service.GetInstance(ctx, instanceID)
	if err != nil {
		return EmbyInstance{}, nil, PermanentError{Err: err}
	}
	if !instance.Enabled {
		return EmbyInstance{}, nil, PermanentError{Err: errors.New("Emby instance is disabled")}
	}
	var circuitOpenUntil *time.Time
	if err = w.db.QueryRow(ctx, `INSERT INTO emby_instance_runtime_health(instance_id) VALUES($1) ON CONFLICT(instance_id) DO UPDATE SET instance_id=EXCLUDED.instance_id RETURNING circuit_open_until`, instanceID).Scan(&circuitOpenUntil); err != nil {
		return EmbyInstance{}, nil, err
	}
	if circuitOpenUntil != nil && circuitOpenUntil.After(time.Now()) {
		return EmbyInstance{}, nil, fmt.Errorf("Emby instance circuit is open until %s", circuitOpenUntil.UTC().Format(time.RFC3339))
	}
	var ciphertext, nonce []byte
	var version int
	if err = w.db.QueryRow(ctx, `SELECT ciphertext,nonce,key_version FROM credentials WHERE name=$1`, instance.CredentialName).Scan(&ciphertext, &nonce, &version); err != nil {
		return EmbyInstance{}, nil, PermanentError{Err: errors.New("Emby credential is missing")}
	}
	secret, err := w.vault.Decrypt(ciphertext, nonce, version)
	if err != nil {
		return EmbyInstance{}, nil, PermanentError{Err: fmt.Errorf("Emby credential cannot be decrypted: %w", err)}
	}
	client, err := newEmbyClient(instance, string(secret))
	return instance, client, err
}

func (w *Worker) provision(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	requestID, err := uuid.Parse(fmt.Sprint(task.Payload["provision_request_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("provision task has invalid request id")}
	}
	var accountID uuid.UUID
	var username string
	var ciphertext, nonce []byte
	var keyVersion int
	var preflight bool
	var knownRemoteID *string
	err = w.db.QueryRow(ctx, `SELECT account_id,requested_username,password_ciphertext,password_nonce,password_key_version,preflight_completed,remote_user_id FROM emby_provision_requests WHERE id=$1`, requestID).Scan(&accountID, &username, &ciphertext, &nonce, &keyVersion, &preflight, &knownRemoteID)
	if err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	password, err := w.vault.Decrypt(ciphertext, nonce, keyVersion)
	if err != nil {
		return nil, PermanentError{Err: err}
	}
	users, err := client.users(ctx)
	if err != nil {
		return nil, err
	}
	var remote embyUser
	for _, candidate := range users {
		if normalize(candidate.Name) == normalize(username) {
			remote = candidate
			break
		}
	}
	if knownRemoteID != nil && *knownRemoteID != "" {
		for _, candidate := range users {
			if candidate.ID == *knownRemoteID {
				remote = candidate
				break
			}
		}
	}
	if remote.ID != "" && !preflight {
		return nil, PermanentError{Err: fmt.Errorf("Emby username %q already existed before provisioning", username)}
	}
	if remote.ID == "" {
		if !preflight {
			tag, markErr := w.db.Exec(ctx, `UPDATE emby_provision_requests SET preflight_completed=TRUE,updated_at=NOW() WHERE id=$1 AND NOT preflight_completed`, requestID)
			if markErr != nil || tag.RowsAffected() != 1 {
				return nil, fmt.Errorf("cannot reserve remote creation: %w", markErr)
			}
		}
		remote, err = client.createUser(ctx, username)
		if err != nil {
			return nil, err
		}
		if _, err = w.db.Exec(ctx, `UPDATE emby_provision_requests SET remote_user_id=$2,updated_at=NOW() WHERE id=$1`, requestID, remote.ID); err != nil {
			return nil, err
		}
	}
	if err = client.setPassword(ctx, remote.ID, string(password)); err != nil {
		return nil, err
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var binding Binding
	err = tx.QueryRow(ctx, `SELECT id,account_id,instance_id,remote_user_id,remote_username,status,origin,is_primary,remote_disabled,remote_snapshot,created_at,updated_at FROM emby_account_bindings WHERE instance_id=$1 AND remote_user_id=$2`, instance.ID, remote.ID).Scan(&binding.ID, &binding.AccountID, &binding.InstanceID, &binding.RemoteUserID, &binding.RemoteUsername, &binding.Status, &binding.Origin, &binding.IsPrimary, &binding.RemoteDisabled, &binding.RemoteSnapshot, &binding.CreatedAt, &binding.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		binding, err = w.service.createBindingTx(ctx, tx, accountID, instance.ID, remote.ID, username, false, remote.Raw, "provision", identity.Actor{Kind: "system", ID: w.id})
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO remote_emby_users(id,instance_id,remote_user_id,username,username_normalized,disabled,claim_status,binding_id,snapshot) VALUES($1,$2,$3,$4,$5,FALSE,'claimed',$6,$7) ON CONFLICT(instance_id,remote_user_id) DO UPDATE SET username=EXCLUDED.username,username_normalized=EXCLUDED.username_normalized,disabled=FALSE,claim_status='claimed',binding_id=EXCLUDED.binding_id,snapshot=EXCLUDED.snapshot,last_seen_at=NOW(),missing_since=NULL,updated_at=NOW()`, uuid.New(), instance.ID, remote.ID, username, normalize(username), binding.ID, jsonBytes(remote.Raw))
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE emby_provision_requests SET status='succeeded',remote_user_id=$2,last_error=NULL,updated_at=NOW() WHERE id=$1`, requestID, remote.ID)
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"binding_id": binding.ID, "remote_user_id": remote.ID, "username": username}, nil
}

func (w *Worker) sync(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient, kind string) (map[string]any, error) {
	info, latency, err := client.probe(ctx)
	if err != nil {
		w.markInstanceFailure(ctx, instance.ID, err)
		return nil, err
	}
	users, err := client.users(ctx)
	if err != nil {
		w.markInstanceFailure(ctx, instance.ID, err)
		return nil, err
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	seen := make([]string, 0, len(users))
	for _, user := range users {
		seen = append(seen, user.ID)
		_, err = tx.Exec(ctx, `INSERT INTO remote_emby_users(id,instance_id,remote_user_id,username,username_normalized,disabled,claim_status,snapshot) VALUES($1,$2,$3,$4,$5,$6,'unclaimed',$7) ON CONFLICT(instance_id,remote_user_id) DO UPDATE SET username=EXCLUDED.username,username_normalized=EXCLUDED.username_normalized,disabled=EXCLUDED.disabled,snapshot=EXCLUDED.snapshot,last_seen_at=NOW(),missing_since=NULL,claim_status=CASE WHEN remote_emby_users.binding_id IS NULL THEN 'unclaimed' ELSE 'claimed' END,updated_at=NOW()`, uuid.New(), instance.ID, user.ID, user.Name, normalize(user.Name), user.disabled(), jsonBytes(user.Raw))
		if err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `UPDATE emby_account_bindings SET remote_username=$3,remote_disabled=$4,remote_snapshot=$5,status=CASE WHEN $4 THEN 'suspended' ELSE 'active' END,last_synced_at=NOW(),last_error=NULL,updated_at=NOW() WHERE instance_id=$1 AND remote_user_id=$2`, instance.ID, user.ID, user.Name, user.disabled(), jsonBytes(user.Raw))
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE remote_emby_users SET claim_status='missing',missing_since=COALESCE(missing_since,NOW()),updated_at=NOW() WHERE instance_id=$1 AND NOT (remote_user_id=ANY($2::text[]))`, instance.ID, seen)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE emby_account_bindings SET status='missing',last_error='remote user missing',updated_at=NOW() WHERE instance_id=$1 AND NOT (remote_user_id=ANY($2::text[]))`, instance.ID, seen)
	}
	latencyMS := int(latency.Milliseconds())
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE emby_instances SET status='healthy',server_id=$2,server_version=$3,last_error=NULL,last_latency_ms=$4,last_checked_at=NOW(),last_snapshot_at=NOW(),updated_at=NOW() WHERE id=$1`, instance.ID, info.ID, info.Version, latencyMS)
	}
	counts, changes, countErr := snapshotCounts(ctx, tx, instance.ID)
	if err == nil {
		err = countErr
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO remote_state_snapshots(instance_id,task_id,snapshot_kind,status,remote_user_count,bound_user_count,unclaimed_user_count,missing_user_count,changes) VALUES($1,$2,$3,'succeeded',$4,$5,$6,$7,$8)`, instance.ID, task.ID, kind, len(users), counts[0], counts[1], counts[2], jsonBytes(changes))
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	w.markInstanceSuccess(ctx, instance.ID)
	return map[string]any{"remote_users": len(users), "bound_users": counts[0], "unclaimed_users": counts[1], "missing_users": counts[2]}, nil
}

func snapshotCounts(ctx context.Context, tx pgx.Tx, instanceID uuid.UUID) ([3]int, map[string]any, error) {
	var counts [3]int
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE binding_id IS NOT NULL),COUNT(*) FILTER (WHERE claim_status='unclaimed'),COUNT(*) FILTER (WHERE claim_status='missing') FROM remote_emby_users WHERE instance_id=$1`, instanceID).Scan(&counts[0], &counts[1], &counts[2])
	return counts, map[string]any{"captured_by": "worker"}, err
}

func (w *Worker) reconcile(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	users, err := client.users(ctx)
	if err != nil {
		w.markInstanceFailure(ctx, instance.ID, err)
		return nil, err
	}
	byID := make(map[string]embyUser, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}
	rows, err := w.db.Query(ctx, `SELECT b.id,b.remote_user_id,a.status,COALESCE(m.expires_at <= NOW(),TRUE) FROM emby_account_bindings b JOIN accounts a ON a.id=b.account_id LEFT JOIN LATERAL (SELECT expires_at FROM account_memberships WHERE account_id=a.id AND status IN ('active','grace') ORDER BY expires_at DESC LIMIT 1) m ON TRUE WHERE b.instance_id=$1 AND b.status<>'deleted'`, instance.ID)
	if err != nil {
		return nil, err
	}
	type desired struct {
		bindingID uuid.UUID
		remoteID  string
		disabled  bool
	}
	var items []desired
	for rows.Next() {
		var item desired
		var accountStatus string
		var expired bool
		if err = rows.Scan(&item.bindingID, &item.remoteID, &accountStatus, &expired); err != nil {
			rows.Close()
			return nil, err
		}
		item.disabled = accountStatus != "active" || expired
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	changed, missing := 0, 0
	for _, item := range items {
		remote, ok := byID[item.remoteID]
		if !ok {
			missing++
			_, _ = w.db.Exec(ctx, `UPDATE emby_account_bindings SET status='missing',last_error='remote user missing',updated_at=NOW() WHERE id=$1`, item.bindingID)
			continue
		}
		if remote.disabled() != item.disabled {
			if err = client.setDisabled(ctx, remote, item.disabled); err != nil {
				return nil, err
			}
			changed++
		}
		status := "active"
		if item.disabled {
			status = "suspended"
		}
		_, err = w.db.Exec(ctx, `UPDATE emby_account_bindings SET status=$2,remote_disabled=$3,last_synced_at=NOW(),last_error=NULL,updated_at=NOW() WHERE id=$1`, item.bindingID, status, item.disabled)
		if err != nil {
			return nil, err
		}
	}
	var counts [3]int
	if err = w.db.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE binding_id IS NOT NULL),COUNT(*) FILTER (WHERE claim_status='unclaimed'),COUNT(*) FILTER (WHERE claim_status='missing') FROM remote_emby_users WHERE instance_id=$1`, instance.ID).Scan(&counts[0], &counts[1], &counts[2]); err != nil {
		return nil, err
	}
	_, err = w.db.Exec(ctx, `INSERT INTO remote_state_snapshots(instance_id,task_id,snapshot_kind,status,remote_user_count,bound_user_count,unclaimed_user_count,missing_user_count,changes) VALUES($1,$2,'reconcile','succeeded',$3,$4,$5,$6,$7)`, instance.ID, task.ID, len(users), counts[0], counts[1], missing, jsonBytes(map[string]any{"changed": changed}))
	if err == nil {
		w.markInstanceSuccess(ctx, instance.ID)
	}
	return map[string]any{"changed": changed, "missing": missing}, err
}

func (w *Worker) markInstanceFailure(ctx context.Context, instanceID uuid.UUID, failure error) {
	message := truncateError(failure)
	_, _ = w.db.Exec(ctx, `UPDATE emby_instances SET status='unhealthy',last_error=$2,last_checked_at=NOW(),updated_at=NOW() WHERE id=$1`, instanceID, message)
	_, _ = w.db.Exec(ctx, `INSERT INTO remote_state_snapshots(instance_id,snapshot_kind,status,error_message) VALUES($1,'probe','failed',$2)`, instanceID, message)
	maximum, cooldown := 3, 120
	_ = w.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='risk.max_instance_failures'`).Scan(&maximum)
	_ = w.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='risk.circuit_cooldown_seconds'`).Scan(&cooldown)
	if maximum < 1 {
		maximum = 3
	}
	if cooldown < 10 {
		cooldown = 120
	}
	_, _ = w.db.Exec(ctx, `INSERT INTO emby_instance_runtime_health(instance_id,consecutive_failures,last_failure_at,last_error) VALUES($1,1,NOW(),$2) ON CONFLICT(instance_id) DO UPDATE SET consecutive_failures=emby_instance_runtime_health.consecutive_failures+1,last_failure_at=NOW(),last_error=$2,circuit_open_until=CASE WHEN emby_instance_runtime_health.consecutive_failures+1 >= $3 THEN NOW()+($4::double precision*INTERVAL '1 second') ELSE emby_instance_runtime_health.circuit_open_until END,updated_at=NOW()`, instanceID, message, maximum, cooldown)
}

func (w *Worker) markInstanceSuccess(ctx context.Context, instanceID uuid.UUID) {
	_, _ = w.db.Exec(ctx, `INSERT INTO emby_instance_runtime_health(instance_id,last_success_at) VALUES($1,NOW()) ON CONFLICT(instance_id) DO UPDATE SET consecutive_failures=0,circuit_open_until=NULL,last_success_at=NOW(),last_error=NULL,updated_at=NOW()`, instanceID)
}

func (w *Worker) playbackSync(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	sessions, err := client.sessions(ctx)
	if err != nil {
		w.markInstanceFailure(ctx, instance.ID, err)
		return nil, err
	}
	result, err := w.service.IngestPlaybackSnapshot(ctx, instance, sessions, identity.Actor{Kind: "system", ID: w.id})
	if err != nil {
		return nil, err
	}
	w.markInstanceSuccess(ctx, instance.ID)
	result["task_id"] = task.ID.String()
	return result, nil
}

func (w *Worker) executeRiskAction(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient, revert bool) (map[string]any, error) {
	actionID, err := uuid.Parse(fmt.Sprint(task.Payload["action_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("risk task has invalid action id")}
	}
	var actionType, status, remoteSessionID, remoteUserID string
	var beforeRaw, actionResultRaw []byte
	if err = w.db.QueryRow(ctx, `SELECT action_type,status,COALESCE(remote_session_id,''),COALESCE(remote_user_id,''),before_state,result FROM risk_actions WHERE id=$1 AND instance_id=$2`, actionID, instance.ID).Scan(&actionType, &status, &remoteSessionID, &remoteUserID, &beforeRaw, &actionResultRaw); err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	if !revert && status == "succeeded" || revert && status == "reverted" {
		return map[string]any{"action_id": actionID, "replayed": true}, nil
	}
	expected := []string{"pending", "failed"}
	nextStatus := "running"
	if revert {
		expected, nextStatus = []string{"revert_pending", "revert_failed"}, "reverting"
	}
	result, err := w.db.Exec(ctx, `UPDATE risk_actions SET status=$2,attempts=attempts+1,last_error=NULL,updated_at=NOW() WHERE id=$1 AND status=ANY($3)`, actionID, nextStatus, expected)
	if err != nil || result.RowsAffected() != 1 {
		return nil, PermanentError{Err: errors.New("risk action is not executable in its current state")}
	}
	actionResult := decodeJSON(actionResultRaw)
	effectKey := "action_effect_applied"
	if revert {
		effectKey = "revert_effect_applied"
	}
	effectApplied := boolValue(actionResult[effectKey])
	var remote embyUser
	if actionType == "disable_user" && !effectApplied {
		users, usersErr := client.users(ctx)
		if usersErr != nil {
			w.markInstanceFailure(ctx, instance.ID, usersErr)
			return nil, usersErr
		}
		for _, candidate := range users {
			if candidate.ID == remoteUserID {
				remote = candidate
				break
			}
		}
	}
	before := decodeJSON(beforeRaw)
	if !revert && !effectApplied {
		switch actionType {
		case "stop_session":
			if len(before) == 0 {
				before = map[string]any{"session_active": true}
				if _, err = w.db.Exec(ctx, `UPDATE risk_actions SET before_state=$2,updated_at=NOW() WHERE id=$1 AND status='running'`, actionID, jsonBytes(before)); err != nil {
					return nil, err
				}
			}
			if err = client.stopSession(ctx, remoteSessionID); err != nil {
				w.markInstanceFailure(ctx, instance.ID, err)
				return nil, err
			}
		case "disable_user":
			if remote.ID == "" {
				return nil, PermanentError{Err: errors.New("risk action remote user is missing")}
			}
			if len(before) == 0 {
				before = map[string]any{"disabled": remote.disabled()}
				if _, err = w.db.Exec(ctx, `UPDATE risk_actions SET before_state=$2,updated_at=NOW() WHERE id=$1 AND status='running'`, actionID, jsonBytes(before)); err != nil {
					return nil, err
				}
			}
			if !remote.disabled() {
				if err = client.setDisabled(ctx, remote, true); err != nil {
					w.markInstanceFailure(ctx, instance.ID, err)
					return nil, err
				}
			}
		default:
			return nil, PermanentError{Err: errors.New("unsupported risk action")}
		}
	} else if revert && !effectApplied {
		if actionType != "disable_user" {
			return nil, PermanentError{Err: errors.New("risk action is not reversible")}
		}
		if remote.ID == "" {
			return nil, PermanentError{Err: errors.New("risk action remote user is missing")}
		}
		wasDisabled, _ := before["disabled"].(bool)
		if remote.disabled() != wasDisabled {
			if err = client.setDisabled(ctx, remote, wasDisabled); err != nil {
				w.markInstanceFailure(ctx, instance.ID, err)
				return nil, err
			}
		}
	}
	remoteAfter := map[string]any{"disabled": actionType == "disable_user" && !revert, "session_stopped": actionType == "stop_session" && !revert, "restored": revert}
	if !effectApplied {
		if _, err = w.db.Exec(ctx, `UPDATE risk_actions SET after_state=$2,result=result||$3::jsonb,updated_at=NOW() WHERE id=$1`, actionID, jsonBytes(remoteAfter), jsonBytes(map[string]any{effectKey: true})); err != nil {
			return nil, err
		}
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	completedStatus, timelineType := "succeeded", "action_succeeded"
	if revert {
		completedStatus, timelineType = "reverted", "action_reverted"
		_, err = tx.Exec(ctx, `UPDATE risk_actions SET status=$2::varchar,before_state=CASE WHEN $2::varchar='succeeded' THEN $3 ELSE before_state END,after_state=$4,result=result||$5::jsonb,last_error=NULL,reverted_at=NOW(),updated_at=NOW() WHERE id=$1`, actionID, completedStatus, jsonBytes(before), jsonBytes(map[string]any{"restored": true}), jsonBytes(map[string]any{"task_id": task.ID}))
	} else {
		_, err = tx.Exec(ctx, `UPDATE risk_actions SET status=$2,before_state=$3,after_state=$4,result=result||$5::jsonb,last_error=NULL,executed_at=NOW(),updated_at=NOW() WHERE id=$1`, actionID, completedStatus, jsonBytes(before), jsonBytes(map[string]any{"disabled": actionType == "disable_user", "session_stopped": actionType == "stop_session"}), jsonBytes(map[string]any{"task_id": task.ID}))
	}
	if err != nil {
		return nil, err
	}
	var eventID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT event_id FROM risk_actions WHERE id=$1`, actionID).Scan(&eventID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) SELECT event_id,$2,$3,reason,$4 FROM risk_actions WHERE id=$1`, actionID, timelineType, "system:"+w.id, jsonBytes(map[string]any{"action_id": actionID, "action_type": actionType})); err != nil {
		return nil, err
	}
	if actionType == "disable_user" {
		disabled := !revert
		if previous, ok := before["disabled"].(bool); revert {
			disabled = previous && ok
		}
		statusValue := "active"
		if disabled {
			statusValue = "suspended"
		}
		_, err = tx.Exec(ctx, `UPDATE emby_account_bindings SET remote_disabled=$3,status=$4,last_synced_at=NOW(),last_error=NULL,updated_at=NOW() WHERE instance_id=$1 AND remote_user_id=$2`, instance.ID, remoteUserID, disabled, statusValue)
		if err != nil {
			return nil, err
		}
	}
	if err = audit(ctx, tx, identity.Actor{Kind: "system", ID: w.id}, "risk_action."+completedStatus, "risk_event", eventID.String(), map[string]any{"action_id": actionID, "action_type": actionType}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	w.markInstanceSuccess(ctx, instance.ID)
	return map[string]any{"action_id": actionID, "status": completedStatus}, nil
}

func (w *Worker) matchMedia(ctx context.Context, task claimedTask, instance EmbyInstance, client *embyClient) (map[string]any, error) {
	mediaID, err := uuid.Parse(fmt.Sprint(task.Payload["media_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("media match task has invalid media id")}
	}
	media, err := w.service.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, PermanentError{Err: err}
	}
	items, err := client.mediaByTMDB(ctx, media.ExternalID, media.MediaType)
	if err != nil {
		w.markInstanceFailure(ctx, instance.ID, err)
		return nil, err
	}
	status := "not_found"
	var remoteID, remoteTitle, remoteType string
	remote := map[string]any{}
	if len(items) > 0 {
		remote = items[0]
		remoteID, remoteTitle, remoteType = stringValue(remote["Id"]), stringValue(remote["Name"]), stringValue(remote["Type"])
		if remoteID != "" {
			status = "matched"
		}
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO media_matches(id,media_id,instance_id,status,remote_item_id,remote_title,remote_item_type,remote_snapshot,matched_at,last_checked_at) VALUES($1,$2,$3,$4::varchar,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,CASE WHEN $4::varchar='matched' THEN NOW() END,NOW()) ON CONFLICT(media_id,instance_id) DO UPDATE SET status=$4::varchar,remote_item_id=NULLIF($5,''),remote_title=NULLIF($6,''),remote_item_type=NULLIF($7,''),remote_snapshot=$8,last_error=NULL,matched_at=CASE WHEN $4::varchar='matched' THEN NOW() ELSE NULL END,last_checked_at=NOW(),updated_at=NOW()`, uuid.New(), mediaID, instance.ID, status, remoteID, remoteTitle, remoteType, jsonBytes(remote))
	if err != nil {
		return nil, err
	}
	if status == "matched" {
		rows, queryErr := tx.Query(ctx, `SELECT id,status FROM media_requests WHERE media_id=$1 AND status IN ('requested','approved','queued','downloading') FOR UPDATE`, mediaID)
		if queryErr != nil {
			return nil, queryErr
		}
		type requestState struct {
			id     uuid.UUID
			status string
		}
		var requests []requestState
		for rows.Next() {
			var request requestState
			if queryErr = rows.Scan(&request.id, &request.status); queryErr != nil {
				rows.Close()
				return nil, queryErr
			}
			requests = append(requests, request)
		}
		rows.Close()
		if queryErr = rows.Err(); queryErr != nil {
			return nil, queryErr
		}
		for _, request := range requests {
			_, err = tx.Exec(ctx, `UPDATE media_requests SET status='completed',resolution_reason=$2,resolved_by=$3,resolved_at=NOW(),revision=revision+1,updated_at=NOW() WHERE id=$1`, request.id, "已在 Emby 实例 "+instance.Name+" 匹配到媒体", "system:"+w.id)
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,from_status,to_status,actor,reason,details) VALUES($1,'emby_matched',$2,'completed',$3,$4,$5)`, request.id, request.status, "system:"+w.id, "Emby media matched", jsonBytes(map[string]any{"instance_id": instance.ID, "remote_item_id": remoteID}))
			}
			if err == nil {
				err = notifyRequestSubscribersTx(ctx, tx, request.id, "media_request.status", "求片已入库", media.Title+" 已可在 "+instance.Name+" 播放")
			}
			if err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	w.markInstanceSuccess(ctx, instance.ID)
	return map[string]any{"media_id": mediaID, "instance_id": instance.ID, "status": status, "remote_item_id": remoteID}, nil
}

func (w *Worker) submitMoviePilot(ctx context.Context, task claimedTask) (map[string]any, error) {
	jobID, err := uuid.Parse(fmt.Sprint(task.Payload["moviepilot_job_id"]))
	if err != nil {
		return nil, PermanentError{Err: errors.New("MoviePilot task has invalid job id")}
	}
	var mediaID uuid.UUID
	var status string
	if err = w.db.QueryRow(ctx, `SELECT media_id,status FROM moviepilot_jobs WHERE id=$1`, jobID).Scan(&mediaID, &status); err != nil {
		return nil, PermanentError{Err: notFound(err)}
	}
	if status == "submitted" || status == "downloading" || status == "completed" {
		return map[string]any{"moviepilot_job_id": jobID, "replayed": true}, nil
	}
	if status != "pending" && status != "failed" && status != "submitting" {
		return nil, PermanentError{Err: errors.New("MoviePilot job is not submitable")}
	}
	if _, err = w.db.Exec(ctx, `UPDATE moviepilot_jobs SET status='submitting',attempts=attempts+1,last_error=NULL,updated_at=NOW() WHERE id=$1`, jobID); err != nil {
		return nil, err
	}
	media, err := w.service.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, PermanentError{Err: err}
	}
	credentialName := w.service.dynamicString(ctx, "moviepilot.credential_name", "moviepilot.api_token")
	token, err := w.service.credentialSecret(ctx, credentialName)
	if err != nil {
		return nil, PermanentError{Err: err}
	}
	base := strings.TrimRight(w.service.dynamicString(ctx, "moviepilot.api_base_url", "http://moviepilot:3000"), "/")
	var jobPayloadRaw []byte
	if err = w.db.QueryRow(ctx, `SELECT payload FROM moviepilot_jobs WHERE id=$1`, jobID).Scan(&jobPayloadRaw); err != nil {
		return nil, err
	}
	jobPayload := decodeJSON(jobPayloadRaw)
	resource, _ := jobPayload["resource"].(map[string]any)
	downloadPayload := moviePilotDownloadPayload(resource, media, jobID)
	path := normalizedUpstreamPath(w.service.dynamicString(ctx, "moviepilot.submit_path", "/api/v1/download/add"), "/api/v1/download/add")
	result, err := postJSON(ctx, base+path, token, downloadPayload)
	if err != nil {
		return nil, err
	}
	if success, ok := result["success"].(bool); ok && !success {
		return nil, fmt.Errorf("MoviePilot rejected the download task")
	}
	externalID := stringValue(result["job_id"])
	if externalID == "" {
		externalID = stringValue(result["id"])
	}
	if data, ok := result["data"].(map[string]any); ok {
		if externalID == "" {
			externalID = stringValue(data["download_id"])
		}
		if externalID == "" {
			externalID = stringValue(data["job_id"])
		}
	}
	if externalID == "" {
		return nil, fmt.Errorf("MoviePilot accepted the request without returning a download id")
	}
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var requestID *uuid.UUID
	if err = tx.QueryRow(ctx, `UPDATE moviepilot_jobs SET status='submitted',external_job_id=$2,result=$3,last_error=NULL,submitted_at=NOW(),updated_at=NOW() WHERE id=$1 RETURNING request_id`, jobID, externalID, jsonBytes(result)).Scan(&requestID); err != nil {
		return nil, err
	}
	if requestID != nil {
		var previous string
		if err = tx.QueryRow(ctx, `SELECT status FROM media_requests WHERE id=$1 FOR UPDATE`, *requestID).Scan(&previous); err != nil {
			return nil, err
		}
		if previous != "completed" {
			if _, err = tx.Exec(ctx, `UPDATE media_requests SET status='downloading',revision=revision+1,updated_at=NOW() WHERE id=$1`, *requestID); err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,from_status,to_status,actor,reason,details) VALUES($1,'moviepilot_submitted',$2,'downloading',$3,'MoviePilot accepted the task',$4)`, *requestID, previous, "system:"+w.id, jsonBytes(map[string]any{"job_id": jobID, "external_job_id": externalID})); err != nil {
				return nil, err
			}
			if err = notifyRequestSubscribersTx(ctx, tx, *requestID, "media_request.status", "求片开始下载", media.Title+" 已提交 MoviePilot"); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"moviepilot_job_id": jobID, "external_job_id": externalID, "status": "submitted"}, nil
}

func moviePilotDownloadPayload(resource map[string]any, media Media, jobID uuid.UUID) map[string]any {
	torrent, ok := resource["torrent_info"].(map[string]any)
	if !ok || len(torrent) == 0 {
		torrent = resource
	}
	payload := make(map[string]any, len(torrent)+6)
	for key, value := range torrent {
		payload[key] = value
	}
	payload["torrent_in"] = torrent
	payload["tmdb_id"] = media.ExternalID
	payload["media_type"] = media.MediaType
	payload["title"] = media.Title
	payload["idempotency_key"] = jobID.String()
	return payload
}

func (w *Worker) finish(ctx context.Context, task claimedTask, result map[string]any) error {
	_, err := w.db.Exec(ctx, `UPDATE platform_tasks SET status='succeeded',result=$2,last_error=NULL,lease_owner=NULL,lease_expires_at=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1 AND lease_owner=$3`, task.ID, jsonBytes(result), w.id)
	return err
}

func (w *Worker) fail(ctx context.Context, task claimedTask, failure error) error {
	permanent := false
	var permanentErr PermanentError
	if errors.As(failure, &permanentErr) {
		permanent = true
	}
	status := "retry"
	if permanent {
		status = "failed"
	} else if task.Attempts >= task.Maximum {
		status = "dead"
	}
	delay := math.Min(math.Pow(2, float64(task.Attempts)), 300)
	message := truncateError(failure)
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE platform_tasks SET status=$2::varchar,last_error=$3,available_at=NOW()+($4::double precision*INTERVAL '1 second'),lease_owner=NULL,lease_expires_at=NULL,finished_at=CASE WHEN $2::varchar IN ('failed','dead') THEN NOW() ELSE NULL END,updated_at=NOW() WHERE id=$1 AND lease_owner=$5`, task.ID, status, message, delay, w.id)
	if err == nil && task.Type == "emby.provision" {
		_, err = tx.Exec(ctx, `UPDATE emby_provision_requests SET status=CASE WHEN $2::varchar IN ('failed','dead') THEN 'failed' ELSE 'pending' END,last_error=$3,updated_at=NOW() WHERE task_id=$1`, task.ID, status, message)
	}
	if err == nil && (task.Type == "risk.action" || task.Type == "risk.revert") {
		if actionID, parseErr := uuid.Parse(fmt.Sprint(task.Payload["action_id"])); parseErr == nil {
			actionStatus := "failed"
			timelineType := "action_failed"
			if task.Type == "risk.revert" {
				actionStatus = "revert_failed"
				timelineType = "revert_failed"
			}
			_, err = tx.Exec(ctx, `UPDATE risk_actions SET status=$2,last_error=$3,updated_at=NOW() WHERE id=$1`, actionID, actionStatus, message)
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) SELECT event_id,$2,$3,$4,$5 FROM risk_actions WHERE id=$1`, actionID, timelineType, "system:"+w.id, message, jsonBytes(map[string]any{"action_id": actionID, "task_id": task.ID, "attempt": task.Attempts, "task_status": status}))
			}
		}
	}
	if err == nil && task.Type == "moviepilot.submit" {
		if jobID, parseErr := uuid.Parse(fmt.Sprint(task.Payload["moviepilot_job_id"])); parseErr == nil {
			_, err = tx.Exec(ctx, `UPDATE moviepilot_jobs SET status='failed',last_error=$2,updated_at=NOW() WHERE id=$1`, jobID, message)
		}
	}
	if err == nil && task.Type == "media.match" {
		mediaID, mediaErr := uuid.Parse(fmt.Sprint(task.Payload["media_id"]))
		instanceID, instanceErr := uuid.Parse(fmt.Sprint(task.Payload["instance_id"]))
		if mediaErr == nil && instanceErr == nil {
			_, err = tx.Exec(ctx, `UPDATE media_matches SET status='failed',last_error=$3,last_checked_at=NOW(),updated_at=NOW() WHERE media_id=$1 AND instance_id=$2`, mediaID, instanceID, message)
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func truncateError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 1800 {
		message = message[:1800]
	}
	return message
}
