package platform

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

var tagCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)

func (s *Service) SaveTag(ctx context.Context, id *uuid.UUID, code, name, color, description string, actor identity.Actor) (AccountTag, error) {
	code = normalize(code)
	name = strings.TrimSpace(name)
	color = strings.TrimSpace(color)
	description = strings.TrimSpace(description)
	if !tagCodePattern.MatchString(code) || name == "" || len(name) > 100 || len(color) > 16 || len(description) > 500 {
		return AccountTag{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountTag{}, err
	}
	defer tx.Rollback(ctx)
	tagID := uuid.New()
	action := "account_tag.create"
	if id != nil {
		tagID = *id
		action = "account_tag.update"
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_tags WHERE id=$1)`, tagID).Scan(&exists); err != nil || !exists {
			return AccountTag{}, identity.ErrNotFound
		}
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO account_tags(id,code,name,color,description,created_by) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6)`, tagID, code, name, color, description, actor.Label())
	} else {
		_, err = tx.Exec(ctx, `UPDATE account_tags SET code=$2,name=$3,color=NULLIF($4,''),description=NULLIF($5,''),updated_at=NOW() WHERE id=$1`, tagID, code, name, color, description)
	}
	if err != nil {
		return AccountTag{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "account_tag", tagID.String(), nil); err != nil {
		return AccountTag{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountTag{}, err
	}
	return s.GetTag(ctx, tagID)
}
func scanTag(row rowScanner) (AccountTag, error) {
	var item AccountTag
	err := row.Scan(&item.ID, &item.Code, &item.Name, &item.Color, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	return item, notFound(err)
}
func (s *Service) GetTag(ctx context.Context, id uuid.UUID) (AccountTag, error) {
	return scanTag(s.db.QueryRow(ctx, `SELECT id,code,name,COALESCE(color,''),COALESCE(description,''),created_at,updated_at FROM account_tags WHERE id=$1`, id))
}
func (s *Service) ListTags(ctx context.Context) ([]AccountTag, error) {
	rows, err := s.db.Query(ctx, `SELECT id,code,name,COALESCE(color,''),COALESCE(description,''),created_at,updated_at FROM account_tags ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountTag
	for rows.Next() {
		item, scanErr := scanTag(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) CreateBatch(ctx context.Context, operationType string, target BatchTarget, payload map[string]any, idempotencyKey string, maxAttempts int, actor identity.Actor) (BatchOperation, error) {
	operationType = normalize(operationType)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 160 || maxAttempts < 1 || maxAttempts > 20 {
		return BatchOperation{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchOperation{}, err
	}
	defer tx.Rollback(ctx)
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM batch_operations WHERE idempotency_key=$1`, idempotencyKey).Scan(&existing)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return BatchOperation{}, err
		}
		return s.GetBatch(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return BatchOperation{}, err
	}
	if err = s.validateBatchPayload(ctx, operationType, payload); err != nil {
		return BatchOperation{}, err
	}
	targets, err := s.resolveBatchTargetsTx(ctx, tx, target)
	if err != nil {
		return BatchOperation{}, err
	}
	maximum := s.dynamicIntTx(ctx, tx, "batch.max_targets", 10000)
	if len(targets) < 1 || len(targets) > maximum {
		return BatchOperation{}, identity.ErrInvalid
	}
	id := uuid.New()
	targetRaw := map[string]any{"account_ids": target.AccountIDs, "status": target.Status, "tag_ids": target.TagIDs}
	_, err = tx.Exec(ctx, `INSERT INTO batch_operations(id,operation_type,target_spec,payload,idempotency_key,total_count,max_attempts,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, operationType, jsonBytes(targetRaw), jsonBytes(payload), idempotencyKey, len(targets), maxAttempts, actor.Label())
	if err != nil {
		return BatchOperation{}, identity.ErrConflict
	}
	for _, accountID := range targets {
		if _, err = tx.Exec(ctx, `INSERT INTO batch_operation_items(operation_id,account_id) VALUES($1,$2)`, id, accountID); err != nil {
			return BatchOperation{}, err
		}
	}
	if err = audit(ctx, tx, actor, "batch.create", "batch_operation", id.String(), map[string]any{"operation_type": operationType, "targets": len(targets)}); err != nil {
		return BatchOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BatchOperation{}, err
	}
	return s.GetBatch(ctx, id)
}

func (s *Service) validateBatchPayload(ctx context.Context, operationType string, payload map[string]any) error {
	if payload == nil {
		return identity.ErrInvalid
	}
	switch operationType {
	case "tag_add", "tag_remove":
		id, err := uuid.Parse(fmt.Sprint(payload["tag_id"]))
		if err != nil {
			return identity.ErrInvalid
		}
		var exists bool
		if err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_tags WHERE id=$1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return identity.ErrNotFound
		}
	case "membership_adjust":
		planID, err := uuid.Parse(fmt.Sprint(payload["plan_id"]))
		if err != nil {
			return identity.ErrInvalid
		}
		days := numberInt(payload["duration_days"])
		if days < 1 || days > 3650 {
			return identity.ErrInvalid
		}
		var enabled bool
		if err = s.db.QueryRow(ctx, `SELECT enabled FROM membership_plans WHERE id=$1`, planID).Scan(&enabled); err != nil || !enabled {
			return identity.ErrNotFound
		}
	case "notification":
		title := strings.TrimSpace(fmt.Sprint(payload["title"]))
		body := strings.TrimSpace(fmt.Sprint(payload["body"]))
		channel := normalize(fmt.Sprint(payload["channel"]))
		eventKey := normalize(fmt.Sprint(payload["event_key"]))
		if channel == "" {
			channel = "in_app"
			payload["channel"] = channel
		}
		if eventKey == "" {
			eventKey = "broadcast.general"
			payload["event_key"] = eventKey
		}
		if title == "" || len(title) > 160 || body == "" || len(body) > 4000 || len(eventKey) > 80 || channel != "in_app" && channel != "telegram" {
			return identity.ErrInvalid
		}
	default:
		return identity.ErrInvalid
	}
	return nil
}
func numberInt(value any) int {
	switch item := value.(type) {
	case int:
		return item
	case int64:
		return int(item)
	case float64:
		if item != math.Trunc(item) || item > float64(^uint(0)>>1) || item < 0 {
			return 0
		}
		return int(item)
	default:
		return 0
	}
}

func (s *Service) resolveBatchTargetsTx(ctx context.Context, tx pgx.Tx, target BatchTarget) ([]uuid.UUID, error) {
	if len(target.AccountIDs) > 0 {
		rows, err := tx.Query(ctx, `SELECT id FROM accounts WHERE id=ANY($1::uuid[]) ORDER BY id`, target.AccountIDs)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		if len(out) != len(uniqueUUIDs(target.AccountIDs)) {
			return nil, identity.ErrNotFound
		}
		return out, rows.Err()
	}
	status := strings.TrimSpace(target.Status)
	if status == "" {
		status = "active"
	}
	rows, err := tx.Query(ctx, `SELECT a.id FROM accounts a WHERE ($1='' OR a.status=$1) AND (cardinality($2::uuid[])=0 OR (SELECT COUNT(DISTINCT x.tag_id) FROM account_tag_assignments x WHERE x.account_id=a.id AND x.tag_id=ANY($2::uuid[]))=cardinality($2::uuid[])) ORDER BY a.id`, status, target.TagIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func uniqueUUIDs(values []uuid.UUID) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func scanBatch(row rowScanner) (BatchOperation, error) {
	var item BatchOperation
	var targetRaw, payloadRaw []byte
	err := row.Scan(&item.ID, &item.OperationType, &item.Status, &targetRaw, &payloadRaw, &item.TotalCount, &item.ProcessedCount, &item.SucceededCount, &item.FailedCount, &item.Attempts, &item.MaxAttempts, &item.LastError, &item.CreatedBy, &item.CreatedAt, &item.StartedAt, &item.FinishedAt, &item.UpdatedAt)
	if err != nil {
		return BatchOperation{}, notFound(err)
	}
	item.TargetSpec = decodeJSON(targetRaw)
	item.Payload = decodeJSON(payloadRaw)
	return item, nil
}

const batchSelect = `SELECT id,operation_type,status,target_spec,payload,total_count,processed_count,succeeded_count,failed_count,attempts,max_attempts,COALESCE(last_error,''),created_by,created_at,started_at,finished_at,updated_at FROM batch_operations`

func (s *Service) GetBatch(ctx context.Context, id uuid.UUID) (BatchOperation, error) {
	return scanBatch(s.db.QueryRow(ctx, batchSelect+` WHERE id=$1`, id))
}
func (s *Service) ListBatches(ctx context.Context, status string, limit int) ([]BatchOperation, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, batchSelect+` WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchOperation
	for rows.Next() {
		item, scanErr := scanBatch(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListBatchItems(ctx context.Context, operationID uuid.UUID, status string, limit int) ([]BatchItem, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `SELECT id,operation_id,account_id,status,attempts,result,COALESCE(last_error,''),started_at,finished_at FROM batch_operation_items WHERE operation_id=$1 AND ($2='' OR status=$2) ORDER BY id LIMIT $3`, operationID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchItem
	for rows.Next() {
		var item BatchItem
		var raw []byte
		if err = rows.Scan(&item.ID, &item.OperationID, &item.AccountID, &item.Status, &item.Attempts, &raw, &item.LastError, &item.StartedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		item.Result = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) PauseBatch(ctx context.Context, id uuid.UUID, actor identity.Actor) (BatchOperation, error) {
	return s.batchTransition(ctx, id, "paused", []string{"pending", "retry", "running"}, "batch.pause", actor)
}
func (s *Service) ResumeBatch(ctx context.Context, id uuid.UUID, actor identity.Actor) (BatchOperation, error) {
	return s.batchTransition(ctx, id, "pending", []string{"paused"}, "batch.resume", actor)
}
func (s *Service) CancelBatch(ctx context.Context, id uuid.UUID, actor identity.Actor) (BatchOperation, error) {
	return s.batchTransition(ctx, id, "canceled", []string{"pending", "retry", "paused", "running", "failed"}, "batch.cancel", actor)
}
func (s *Service) RetryBatch(ctx context.Context, id uuid.UUID, actor identity.Actor) (BatchOperation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchOperation{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE batch_operations SET status='retry',attempts=0,lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,finished_at=NULL,updated_at=NOW() WHERE id=$1 AND status='failed'`, id)
	if err != nil {
		return BatchOperation{}, err
	}
	if tag.RowsAffected() != 1 {
		return BatchOperation{}, identity.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE batch_operation_items SET status='pending',last_error=NULL,started_at=NULL,finished_at=NULL WHERE operation_id=$1 AND status='failed'`, id); err != nil {
		return BatchOperation{}, err
	}
	if err = audit(ctx, tx, actor, "batch.retry", "batch_operation", id.String(), nil); err != nil {
		return BatchOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BatchOperation{}, err
	}
	return s.GetBatch(ctx, id)
}
func (s *Service) batchTransition(ctx context.Context, id uuid.UUID, next string, allowed []string, action string, actor identity.Actor) (BatchOperation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchOperation{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE batch_operations SET status=$2,lease_owner=NULL,lease_expires_at=NULL,finished_at=CASE WHEN $2='canceled' THEN NOW() ELSE finished_at END,updated_at=NOW() WHERE id=$1 AND status=ANY($3::text[])`, id, next, allowed)
	if err != nil {
		return BatchOperation{}, err
	}
	if tag.RowsAffected() != 1 {
		return BatchOperation{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "batch_operation", id.String(), nil); err != nil {
		return BatchOperation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BatchOperation{}, err
	}
	return s.GetBatch(ctx, id)
}

func (s *Service) ProcessNextBatch(ctx context.Context, workerID string, lease time.Duration) (bool, error) {
	if lease < 30*time.Second {
		lease = 90 * time.Second
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var operationID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM batch_operations WHERE status IN ('pending','retry') OR (status='running' AND lease_expires_at<NOW()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&operationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE batch_operations SET status='running',attempts=attempts+1,lease_owner=$2,lease_expires_at=NOW()+($3::double precision*INTERVAL '1 second'),started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE id=$1`, operationID, workerID, lease.Seconds())
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE batch_operation_items SET status='pending',started_at=NULL WHERE operation_id=$1 AND status='running'`, operationID); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	operation, err := s.GetBatch(ctx, operationID)
	if err != nil {
		return true, err
	}
	chunk := 50
	_ = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='batch.chunk_size'`).Scan(&chunk)
	if chunk < 1 || chunk > 500 {
		chunk = 50
	}
	rows, err := s.db.Query(ctx, `SELECT id,account_id FROM batch_operation_items WHERE operation_id=$1 AND status='pending' ORDER BY id LIMIT $2`, operationID, chunk)
	if err != nil {
		return true, err
	}
	type batchItem struct {
		id        int64
		accountID uuid.UUID
	}
	var items []batchItem
	for rows.Next() {
		var item batchItem
		if err = rows.Scan(&item.id, &item.accountID); err != nil {
			rows.Close()
			return true, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return true, err
	}
	for _, item := range items {
		var status string
		if err = s.db.QueryRow(ctx, `SELECT status FROM batch_operations WHERE id=$1`, operationID).Scan(&status); err != nil {
			return true, err
		}
		if status != "running" {
			return true, nil
		}
		if err = s.processBatchItem(ctx, operation, item.id, item.accountID, workerID); err != nil {
			_, _ = s.db.Exec(ctx, `UPDATE batch_operation_items SET status='failed',attempts=attempts+1,last_error=$2,finished_at=NOW() WHERE id=$1`, item.id, truncateError(err))
		}
		_, _ = s.db.Exec(ctx, `UPDATE batch_operations SET lease_expires_at=NOW()+($3::double precision*INTERVAL '1 second'),updated_at=NOW() WHERE id=$1 AND lease_owner=$2 AND status='running'`, operationID, workerID, lease.Seconds())
	}
	return true, s.finalizeBatchChunk(ctx, operationID, workerID)
}

func (s *Service) processBatchItem(ctx context.Context, operation BatchOperation, itemID int64, accountID uuid.UUID, workerID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE batch_operation_items SET status='running',attempts=attempts+1,started_at=NOW() WHERE id=$1 AND status='pending'`, itemID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return identity.ErrConflict
	}
	actor := identity.Actor{Kind: "system", ID: workerID}
	switch operation.OperationType {
	case "tag_add":
		tagID, parseErr := uuid.Parse(fmt.Sprint(operation.Payload["tag_id"]))
		if parseErr != nil {
			return PermanentError{Err: parseErr}
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_tag_assignments(account_id,tag_id,assigned_by) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, accountID, tagID, actor.Label())
	case "tag_remove":
		tagID, parseErr := uuid.Parse(fmt.Sprint(operation.Payload["tag_id"]))
		if parseErr != nil {
			return PermanentError{Err: parseErr}
		}
		_, err = tx.Exec(ctx, `DELETE FROM account_tag_assignments WHERE account_id=$1 AND tag_id=$2`, accountID, tagID)
	case "membership_adjust":
		planID, parseErr := uuid.Parse(fmt.Sprint(operation.Payload["plan_id"]))
		if parseErr != nil {
			return PermanentError{Err: parseErr}
		}
		membership, assignErr := s.assignMembershipTx(ctx, tx, accountID, planID, time.Now(), numberInt(operation.Payload["duration_days"]), "batch_adjustment", operation.ID.String(), actor)
		err = assignErr
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE batch_operation_items SET result=$2 WHERE id=$1`, itemID, jsonBytes(map[string]any{"membership_id": membership.ID, "expires_at": membership.ExpiresAt}))
		}
	case "notification":
		channel := normalize(fmt.Sprint(operation.Payload["channel"]))
		if channel == "" {
			channel = "in_app"
		}
		eventKey := normalize(fmt.Sprint(operation.Payload["event_key"]))
		if eventKey == "" {
			eventKey = "broadcast.general"
		}
		allowed, preferenceErr := notificationAllowedTx(ctx, tx, accountID, eventKey, channel)
		if preferenceErr != nil {
			return preferenceErr
		}
		if !allowed {
			_, err = tx.Exec(ctx, `UPDATE batch_operation_items SET result=result||$2::jsonb WHERE id=$1`, itemID, jsonBytes(map[string]any{"notification_skipped": true, "event_key": eventKey}))
		} else if channel == "telegram" {
			var telegramSubject string
			if lookupErr := tx.QueryRow(ctx, `SELECT subject FROM account_identities WHERE account_id=$1 AND kind='telegram' AND NOT disabled`, accountID).Scan(&telegramSubject); lookupErr != nil {
				return identity.ErrNotFound
			}
		}
		if err == nil && allowed {
			_, err = tx.Exec(ctx, `INSERT INTO account_notifications(id,account_id,batch_operation_id,title,body,channel,delivery_status,metadata) VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6='in_app' THEN 'sent' ELSE 'pending' END,$7) ON CONFLICT(batch_operation_id,account_id,channel) DO NOTHING`, uuid.New(), accountID, operation.ID, fmt.Sprint(operation.Payload["title"]), fmt.Sprint(operation.Payload["body"]), channel, jsonBytes(map[string]any{"batch": true, "event_key": eventKey}))
		}
	default:
		return PermanentError{Err: identity.ErrInvalid}
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE batch_operation_items SET status='succeeded',result=result||$2::jsonb,last_error=NULL,finished_at=NOW() WHERE id=$1`, itemID, jsonBytes(map[string]any{"processed": true}))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) finalizeBatchChunk(ctx context.Context, operationID uuid.UUID, workerID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var pending, running, succeeded, failed int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE status='pending'),COUNT(*) FILTER(WHERE status='running'),COUNT(*) FILTER(WHERE status='succeeded'),COUNT(*) FILTER(WHERE status='failed') FROM batch_operation_items WHERE operation_id=$1`, operationID).Scan(&pending, &running, &succeeded, &failed); err != nil {
		return err
	}
	if running > 0 {
		return errors.New("batch operation still has running items")
	}
	processed := succeeded + failed
	status := "pending"
	var finished any = nil
	if pending == 0 {
		status = "succeeded"
		if failed > 0 {
			status = "failed"
		}
		finished = time.Now()
	}
	_, err = tx.Exec(ctx, `UPDATE batch_operations SET status=$2,processed_count=$3,succeeded_count=$4,failed_count=$5,lease_owner=NULL,lease_expires_at=NULL,finished_at=$6,last_error=CASE WHEN $5>0 THEN $7 ELSE NULL END,updated_at=NOW() WHERE id=$1 AND lease_owner=$8`, operationID, status, processed, succeeded, failed, finished, fmt.Sprintf("%d target(s) failed", failed), workerID)
	if err != nil {
		return err
	}
	if pending == 0 {
		err = audit(ctx, tx, identity.Actor{Kind: "system", ID: workerID}, "batch.complete", "batch_operation", operationID.String(), map[string]any{"status": status, "succeeded": succeeded, "failed": failed})
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListNotifications(ctx context.Context, accountID uuid.UUID, status string, limit int) ([]Notification, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,account_id,batch_operation_id,title,body,channel,status,delivery_status,delivery_attempts,COALESCE(delivery_error,''),metadata,created_at,read_at FROM account_notifications WHERE account_id=$1 AND ($2='' OR status=$2) ORDER BY created_at DESC LIMIT $3`, accountID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var item Notification
		var raw []byte
		if err = rows.Scan(&item.ID, &item.AccountID, &item.BatchID, &item.Title, &item.Body, &item.Channel, &item.Status, &item.DeliveryStatus, &item.DeliveryAttempts, &item.DeliveryError, &raw, &item.CreatedAt, &item.ReadAt); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}
func (s *Service) MarkNotificationRead(ctx context.Context, accountID, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `UPDATE account_notifications SET status='read',read_at=COALESCE(read_at,NOW()) WHERE id=$1 AND account_id=$2`, id, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return identity.ErrNotFound
	}
	return nil
}

func (s *Service) ClaimTelegramNotification(ctx context.Context, workerID string, lease time.Duration) (TelegramNotificationDelivery, bool, error) {
	if workerID == "" {
		return TelegramNotificationDelivery{}, false, identity.ErrInvalid
	}
	if lease < 15*time.Second {
		lease = 45 * time.Second
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TelegramNotificationDelivery{}, false, err
	}
	defer tx.Rollback(ctx)
	var item TelegramNotificationDelivery
	var subject string
	err = tx.QueryRow(ctx, `SELECT n.id,i.subject,n.title,n.body FROM account_notifications n JOIN account_identities i ON i.account_id=n.account_id AND i.kind='telegram' AND NOT i.disabled WHERE n.channel='telegram' AND n.delivery_attempts<5 AND ((n.delivery_status IN ('pending','failed') AND n.next_delivery_at<=NOW()) OR (n.delivery_status='sending' AND n.delivery_lease_expires_at<NOW())) ORDER BY n.next_delivery_at,n.created_at FOR UPDATE OF n SKIP LOCKED LIMIT 1`).Scan(&item.NotificationID, &subject, &item.Title, &item.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return TelegramNotificationDelivery{}, false, nil
	}
	if err != nil {
		return TelegramNotificationDelivery{}, false, err
	}
	if _, err = fmt.Sscan(subject, &item.TelegramUserID); err != nil {
		return TelegramNotificationDelivery{}, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE account_notifications SET delivery_status='sending',delivery_attempts=delivery_attempts+1,delivery_lease_owner=$2,delivery_lease_expires_at=NOW()+($3::double precision*INTERVAL '1 second'),delivery_error=NULL WHERE id=$1`, item.NotificationID, workerID, lease.Seconds())
	if err != nil {
		return TelegramNotificationDelivery{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TelegramNotificationDelivery{}, false, err
	}
	return item, true, nil
}

func (s *Service) CompleteTelegramNotification(ctx context.Context, id uuid.UUID, workerID string, deliveryErr error) error {
	if workerID == "" {
		return identity.ErrInvalid
	}
	if deliveryErr == nil {
		tag, err := s.db.Exec(ctx, `UPDATE account_notifications SET delivery_status='sent',delivered_at=NOW(),delivery_lease_owner=NULL,delivery_lease_expires_at=NULL,delivery_error=NULL WHERE id=$1 AND delivery_status='sending' AND delivery_lease_owner=$2`, id, workerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return identity.ErrConflict
		}
		return nil
	}
	message := truncateError(deliveryErr)
	tag, err := s.db.Exec(ctx, `UPDATE account_notifications SET delivery_status='failed',next_delivery_at=NOW()+(LEAST(300,POWER(2,delivery_attempts))::double precision*INTERVAL '1 second'),delivery_lease_owner=NULL,delivery_lease_expires_at=NULL,delivery_error=$3 WHERE id=$1 AND delivery_status='sending' AND delivery_lease_owner=$2`, id, workerID, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return identity.ErrConflict
	}
	return nil
}
