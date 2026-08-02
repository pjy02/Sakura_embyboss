package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func (s *Service) EnqueueInstanceTask(ctx context.Context, taskType string, instanceID uuid.UUID, idempotencyKey string, actor identity.Actor) (Task, error) {
	if taskType != "emby.sync" && taskType != "emby.reconcile" && taskType != "emby.import" && taskType != "emby.playback_sync" {
		return Task{}, identity.ErrInvalid
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 160 {
		return Task{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var enabled bool
	if err = tx.QueryRow(ctx, `SELECT enabled FROM emby_instances WHERE id=$1`, instanceID).Scan(&enabled); err != nil {
		return Task{}, notFound(err)
	}
	if !enabled {
		return Task{}, identity.ErrForbidden
	}
	fullKey := taskType + ":" + instanceID.String() + ":" + idempotencyKey
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM platform_tasks WHERE idempotency_key=$1`, fullKey).Scan(&existing)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Task{}, err
		}
		return s.GetTask(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Task{}, err
	}
	id := uuid.New()
	maxAttempts := s.dynamicIntTx(ctx, tx, "emby.task_max_attempts", 8)
	_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, taskType, fullKey, jsonBytes(map[string]any{"instance_id": instanceID.String()}), maxAttempts, actor.Label())
	if err != nil {
		return Task{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "task.enqueue", "platform_task", id.String(), map[string]any{"task_type": taskType, "instance_id": instanceID}); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, id)
}

func (s *Service) GetTask(ctx context.Context, id uuid.UUID) (Task, error) {
	var item Task
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT id,task_type,status,idempotency_key,result,attempts,max_attempts,COALESCE(last_error,''),created_at,started_at,finished_at,updated_at FROM platform_tasks WHERE id=$1`, id).Scan(&item.ID, &item.TaskType, &item.Status, &item.IdempotencyKey, &raw, &item.Attempts, &item.MaxAttempts, &item.LastError, &item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt)
	if err != nil {
		return Task{}, notFound(err)
	}
	item.Result = decodeJSON(raw)
	return item, nil
}

func (s *Service) ListTasks(ctx context.Context, instanceID *uuid.UUID, status string, limit int) ([]Task, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,task_type,status,idempotency_key,result,attempts,max_attempts,COALESCE(last_error,''),created_at,started_at,finished_at,updated_at FROM platform_tasks WHERE ($1='' OR status=$1) AND ($2::uuid IS NULL OR payload->>'instance_id'=$2::text) ORDER BY created_at DESC LIMIT $3`, status, uuidQueryValue(instanceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var item Task
		var raw []byte
		if err = rows.Scan(&item.ID, &item.TaskType, &item.Status, &item.IdempotencyKey, &raw, &item.Attempts, &item.MaxAttempts, &item.LastError, &item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Result = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) RetryTask(ctx context.Context, id uuid.UUID, actor identity.Actor) (Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE platform_tasks SET status='retry',attempts=0,available_at=NOW(),lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,finished_at=NULL,updated_at=NOW() WHERE id=$1 AND status IN ('failed','dead')`, id)
	if err != nil {
		return Task{}, err
	}
	if tag.RowsAffected() == 0 {
		return Task{}, identity.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE emby_provision_requests SET status='pending',last_error=NULL,updated_at=NOW() WHERE task_id=$1 AND status='failed'`, id); err != nil {
		return Task{}, err
	}
	if err = audit(ctx, tx, actor, "task.retry", "platform_task", id.String(), nil); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, id)
}

func (s *Service) ListSnapshots(ctx context.Context, instanceID uuid.UUID, limit int) ([]Snapshot, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,instance_id,task_id,snapshot_kind,status,remote_user_count,bound_user_count,unclaimed_user_count,missing_user_count,changes,COALESCE(error_message,''),captured_at FROM remote_state_snapshots WHERE instance_id=$1 ORDER BY captured_at DESC LIMIT $2`, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var item Snapshot
		var raw []byte
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.TaskID, &item.Kind, &item.Status, &item.RemoteUserCount, &item.BoundUserCount, &item.UnclaimedUserCount, &item.MissingUserCount, &raw, &item.ErrorMessage, &item.CapturedAt); err != nil {
			return nil, err
		}
		item.Changes = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ScheduleDue(ctx context.Context, now time.Time) error {
	rows, err := s.db.Query(ctx, `SELECT id FROM emby_instances WHERE enabled ORDER BY priority,id`)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var syncSeconds, reconcileSeconds, playbackSeconds int
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='emby.sync_interval_seconds'`).Scan(&syncSeconds); err != nil || syncSeconds < 30 {
		syncSeconds = 300
	}
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='emby.reconcile_interval_seconds'`).Scan(&reconcileSeconds); err != nil || reconcileSeconds < 60 {
		reconcileSeconds = 900
	}
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='playback.sync_interval_seconds'`).Scan(&playbackSeconds); err != nil || playbackSeconds < 10 {
		playbackSeconds = 30
	}
	actor := identity.Actor{Kind: "system", ID: "worker-scheduler"}
	for _, id := range ids {
		syncBucket := now.Unix() / int64(syncSeconds)
		if _, enqueueErr := s.EnqueueInstanceTask(ctx, "emby.sync", id, fmt.Sprintf("auto-%d", syncBucket), actor); enqueueErr != nil && !errors.Is(enqueueErr, identity.ErrConflict) {
			return enqueueErr
		}
		reconcileBucket := now.Unix() / int64(reconcileSeconds)
		if _, enqueueErr := s.EnqueueInstanceTask(ctx, "emby.reconcile", id, fmt.Sprintf("auto-%d", reconcileBucket), actor); enqueueErr != nil && !errors.Is(enqueueErr, identity.ErrConflict) {
			return enqueueErr
		}
		playbackBucket := now.Unix() / int64(playbackSeconds)
		if _, enqueueErr := s.EnqueueInstanceTask(ctx, "emby.playback_sync", id, fmt.Sprintf("auto-%d", playbackBucket), actor); enqueueErr != nil && !errors.Is(enqueueErr, identity.ErrConflict) {
			return enqueueErr
		}
	}
	var entitlementSeconds, favoriteSeconds int
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='entitlements.sync_interval_seconds'`).Scan(&entitlementSeconds); err != nil || entitlementSeconds < 60 {
		entitlementSeconds = 300
	}
	if err = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='favorites.sync_interval_seconds'`).Scan(&favoriteSeconds); err != nil || favoriteSeconds < 300 {
		favoriteSeconds = 3600
	}
	bindingRows, queryErr := s.db.Query(ctx, `SELECT b.id,b.instance_id,EXISTS(SELECT 1 FROM account_entitlements e WHERE e.binding_id=b.id) FROM emby_account_bindings b WHERE b.status<>'deleted'`)
	if queryErr != nil {
		return queryErr
	}
	type bindingTarget struct {
		id, instanceID  uuid.UUID
		hasEntitlements bool
	}
	var bindings []bindingTarget
	for bindingRows.Next() {
		var item bindingTarget
		if err = bindingRows.Scan(&item.id, &item.instanceID, &item.hasEntitlements); err != nil {
			bindingRows.Close()
			return err
		}
		bindings = append(bindings, item)
	}
	bindingRows.Close()
	if err = bindingRows.Err(); err != nil {
		return err
	}
	for _, binding := range bindings {
		if binding.hasEntitlements {
			entitlementBucket := now.Unix() / int64(entitlementSeconds)
			if err = s.enqueueBindingTask(ctx, "entitlement.sync", binding.id, binding.instanceID, fmt.Sprintf("auto-%d", entitlementBucket), actor); err != nil && !errors.Is(err, identity.ErrConflict) {
				return err
			}
		}
		favoriteBucket := now.Unix() / int64(favoriteSeconds)
		if err = s.enqueueBindingTask(ctx, "emby.favorite_sync", binding.id, binding.instanceID, fmt.Sprintf("auto-%d", favoriteBucket), actor); err != nil && !errors.Is(err, identity.ErrConflict) {
			return err
		}
	}
	lineProbeSeconds := s.dynamicInt(ctx, "lines.probe_interval_seconds", 300)
	if lineProbeSeconds < 30 {
		lineProbeSeconds = 300
	}
	lineRows, lineErr := s.db.Query(ctx, `SELECT id FROM line_endpoints WHERE enabled AND NOT maintenance ORDER BY id`)
	if lineErr != nil {
		return lineErr
	}
	var lineIDs []uuid.UUID
	for lineRows.Next() {
		var lineID uuid.UUID
		if err = lineRows.Scan(&lineID); err != nil {
			lineRows.Close()
			return err
		}
		lineIDs = append(lineIDs, lineID)
	}
	lineRows.Close()
	if err = lineRows.Err(); err != nil {
		return err
	}
	lineBucket := now.Unix() / int64(lineProbeSeconds)
	for _, lineID := range lineIDs {
		key := fmt.Sprintf("line.probe:%s:auto-%d", lineID, lineBucket)
		if _, err = s.db.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by) VALUES($1,'line.probe',$2,$3,$4) ON CONFLICT(idempotency_key) DO NOTHING`, uuid.New(), key, jsonBytes(map[string]any{"line_id": lineID.String()}), actor.Label()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enqueueBindingTask(ctx context.Context, taskType string, bindingID, instanceID uuid.UUID, bucket string, actor identity.Actor) error {
	key := taskType + ":" + bindingID.String() + ":" + bucket
	_, err := s.db.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(idempotency_key) DO NOTHING`, uuid.New(), taskType, key, jsonBytes(map[string]any{"instance_id": instanceID.String(), "binding_id": bindingID.String()}), actor.Label())
	return err
}
