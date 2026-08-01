package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func (s *Service) ListPlaybackSessions(ctx context.Context, instanceID, accountID *uuid.UUID, limit int) ([]PlaybackSession, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.instance_id,i.name,p.binding_id,p.account_id,p.remote_session_id,p.playback_key,COALESCE(p.remote_user_id,''),COALESCE(p.remote_username,''),p.item_id,p.item_name,COALESCE(p.item_type,''),COALESCE(p.series_name,''),COALESCE(p.client_name,''),COALESCE(p.device_name,''),COALESCE(p.device_id,''),COALESCE(p.platform,''),COALESCE(p.remote_ip,''),COALESCE(p.play_method,''),p.transcoding,COALESCE(p.bitrate,0),p.position_ticks,COALESCE(p.runtime_ticks,0),p.paused,p.device_decision,p.matched_device_rule_id,p.raw_snapshot,p.first_seen_at,p.last_seen_at FROM playback_sessions p JOIN emby_instances i ON i.id=p.instance_id WHERE ($1::uuid IS NULL OR p.instance_id=$1) AND ($2::uuid IS NULL OR p.account_id=$2) ORDER BY p.last_seen_at DESC LIMIT $3`, uuidQueryValue(instanceID), uuidQueryValue(accountID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaybackSession
	for rows.Next() {
		item, scanErr := scanPlaybackSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanPlaybackSession(row pgx.Row) (PlaybackSession, error) {
	var item PlaybackSession
	var raw []byte
	err := row.Scan(&item.ID, &item.InstanceID, &item.InstanceName, &item.BindingID, &item.AccountID, &item.RemoteSessionID, &item.PlaybackKey, &item.RemoteUserID, &item.RemoteUsername, &item.ItemID, &item.ItemName, &item.ItemType, &item.SeriesName, &item.ClientName, &item.DeviceName, &item.DeviceID, &item.Platform, &item.RemoteIP, &item.PlayMethod, &item.Transcoding, &item.Bitrate, &item.PositionTicks, &item.RuntimeTicks, &item.Paused, &item.DeviceDecision, &item.MatchedDeviceRuleID, &raw, &item.FirstSeenAt, &item.LastSeenAt)
	item.RawSnapshot = decodeJSON(raw)
	return item, err
}

func (s *Service) ListPlaybackHistory(ctx context.Context, instanceID, accountID *uuid.UUID, limit int) ([]PlaybackHistory, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT h.id,h.instance_id,i.name,h.binding_id,h.account_id,h.remote_session_id,h.playback_key,COALESCE(h.remote_user_id,''),COALESCE(h.remote_username,''),h.item_id,h.item_name,COALESCE(h.item_type,''),COALESCE(h.series_name,''),COALESCE(h.client_name,''),COALESCE(h.device_name,''),COALESCE(h.device_id,''),COALESCE(h.platform,''),COALESCE(h.remote_ip,''),COALESCE(h.play_method,''),h.transcoding,COALESCE(h.peak_bitrate,0),h.max_position_ticks,COALESCE(h.runtime_ticks,0),h.raw_snapshot,h.started_at,h.last_seen_at,h.ended_at FROM playback_history h JOIN emby_instances i ON i.id=h.instance_id WHERE ($1::uuid IS NULL OR h.instance_id=$1) AND ($2::uuid IS NULL OR h.account_id=$2) ORDER BY h.started_at DESC LIMIT $3`, uuidQueryValue(instanceID), uuidQueryValue(accountID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaybackHistory
	for rows.Next() {
		var item PlaybackHistory
		var raw []byte
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.InstanceName, &item.BindingID, &item.AccountID, &item.RemoteSessionID, &item.PlaybackKey, &item.RemoteUserID, &item.RemoteUsername, &item.ItemID, &item.ItemName, &item.ItemType, &item.SeriesName, &item.ClientName, &item.DeviceName, &item.DeviceID, &item.Platform, &item.RemoteIP, &item.PlayMethod, &item.Transcoding, &item.PeakBitrate, &item.MaxPositionTicks, &item.RuntimeTicks, &raw, &item.StartedAt, &item.LastSeenAt, &item.EndedAt); err != nil {
			return nil, err
		}
		item.RawSnapshot = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListDevices(ctx context.Context, instanceID, accountID *uuid.UUID, decision string, limit int) ([]DeviceProfile, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT d.id,d.instance_id,i.name,d.binding_id,d.account_id,COALESCE(d.remote_user_id,''),d.device_key,COALESCE(d.device_id,''),COALESCE(d.device_name,''),COALESCE(d.client_name,''),COALESCE(d.platform,''),COALESCE(d.first_ip,''),COALESCE(d.last_ip,''),d.session_count,d.transcode_count,COALESCE(d.maximum_bitrate,0),d.access_decision,d.matched_rule_id,d.first_seen_at,d.last_seen_at FROM device_profiles d JOIN emby_instances i ON i.id=d.instance_id WHERE ($1::uuid IS NULL OR d.instance_id=$1) AND ($2::uuid IS NULL OR d.account_id=$2) AND ($3='' OR d.access_decision=$3) ORDER BY d.last_seen_at DESC LIMIT $4`, uuidQueryValue(instanceID), uuidQueryValue(accountID), decision, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceProfile
	for rows.Next() {
		var item DeviceProfile
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.InstanceName, &item.BindingID, &item.AccountID, &item.RemoteUserID, &item.DeviceKey, &item.DeviceID, &item.DeviceName, &item.ClientName, &item.Platform, &item.FirstIP, &item.LastIP, &item.SessionCount, &item.TranscodeCount, &item.MaximumBitrate, &item.AccessDecision, &item.MatchedRuleID, &item.FirstSeenAt, &item.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListDeviceRules(ctx context.Context, instanceID *uuid.UUID) ([]DeviceRule, error) {
	rows, err := s.db.Query(ctx, `SELECT id,instance_id,name,COALESCE(description,''),decision,match_field,match_operator,match_value,action,observation_mode,enabled,built_in,priority,revision,created_by,created_at,updated_at FROM device_access_rules WHERE ($1::uuid IS NULL OR instance_id IS NULL OR instance_id=$1) ORDER BY CASE decision WHEN 'allow' THEN 0 ELSE 1 END,priority,id`, uuidQueryValue(instanceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceRule
	for rows.Next() {
		var item DeviceRule
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.Name, &item.Description, &item.Decision, &item.MatchField, &item.MatchOperator, &item.MatchValue, &item.Action, &item.ObservationMode, &item.Enabled, &item.BuiltIn, &item.Priority, &item.Revision, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveDeviceRule(ctx context.Context, id *uuid.UUID, item DeviceRule, expected int64, actor identity.Actor) (DeviceRule, error) {
	item.Name, item.MatchValue = strings.TrimSpace(item.Name), strings.TrimSpace(item.MatchValue)
	if item.Name == "" || len(item.Name) > 120 || item.MatchValue == "" || len(item.MatchValue) > 255 || !contains([]string{"allow", "deny"}, item.Decision) || !contains([]string{"client_name", "device_name", "device_id", "platform", "remote_ip"}, item.MatchField) || !contains([]string{"exact", "contains", "prefix", "regex"}, item.MatchOperator) || !contains([]string{"none", "stop_session", "disable_user"}, item.Action) || item.Priority < 0 || item.Priority > 100000 {
		return DeviceRule{}, identity.ErrInvalid
	}
	if item.Decision == "allow" {
		item.Action = "none"
	}
	if item.MatchOperator == "regex" {
		if _, err := regexp.Compile(item.MatchValue); err != nil {
			return DeviceRule{}, identity.ErrInvalid
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DeviceRule{}, err
	}
	defer tx.Rollback(ctx)
	ruleID := uuid.New()
	action := "device_rule.create"
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO device_access_rules(id,instance_id,name,description,decision,match_field,match_operator,match_value,action,observation_mode,enabled,priority,created_by) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13)`, ruleID, item.InstanceID, item.Name, item.Description, item.Decision, item.MatchField, item.MatchOperator, item.MatchValue, item.Action, item.ObservationMode, item.Enabled, item.Priority, actor.Label())
	} else {
		ruleID = *id
		var revision int64
		var builtIn bool
		if err = tx.QueryRow(ctx, `SELECT revision,built_in FROM device_access_rules WHERE id=$1 FOR UPDATE`, ruleID).Scan(&revision, &builtIn); err != nil {
			return DeviceRule{}, notFound(err)
		}
		if revision != expected {
			return DeviceRule{}, identity.ErrConflict
		}
		if builtIn && (item.MatchField != "client_name" || item.Decision != "allow") {
			return DeviceRule{}, identity.ErrForbidden
		}
		_, err = tx.Exec(ctx, `UPDATE device_access_rules SET instance_id=$2,name=$3,description=NULLIF($4,''),decision=$5,match_field=$6,match_operator=$7,match_value=$8,action=$9,observation_mode=$10,enabled=$11,priority=$12,revision=revision+1,updated_at=NOW() WHERE id=$1`, ruleID, item.InstanceID, item.Name, item.Description, item.Decision, item.MatchField, item.MatchOperator, item.MatchValue, item.Action, item.ObservationMode, item.Enabled, item.Priority)
		action = "device_rule.update"
	}
	if err != nil {
		return DeviceRule{}, err
	}
	if err = audit(ctx, tx, actor, action, "device_rule", ruleID.String(), map[string]any{"decision": item.Decision, "observation_mode": item.ObservationMode, "action": item.Action}); err != nil {
		return DeviceRule{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return DeviceRule{}, err
	}
	items, err := s.ListDeviceRules(ctx, item.InstanceID)
	if err != nil {
		return DeviceRule{}, err
	}
	for _, candidate := range items {
		if candidate.ID == ruleID {
			return candidate, nil
		}
	}
	return DeviceRule{}, identity.ErrNotFound
}

func (s *Service) ListRiskRules(ctx context.Context, instanceID *uuid.UUID) ([]RiskRule, error) {
	rows, err := s.db.Query(ctx, `SELECT id,instance_id,code,name,COALESCE(description,''),rule_type,condition,severity,action,observation_mode,enabled,cooldown_seconds,revision,created_by,created_at,updated_at FROM risk_rules WHERE ($1::uuid IS NULL OR instance_id IS NULL OR instance_id=$1) ORDER BY name,id`, uuidQueryValue(instanceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RiskRule
	for rows.Next() {
		var item RiskRule
		var raw []byte
		if err = rows.Scan(&item.ID, &item.InstanceID, &item.Code, &item.Name, &item.Description, &item.RuleType, &raw, &item.Severity, &item.Action, &item.ObservationMode, &item.Enabled, &item.CooldownSeconds, &item.Revision, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Condition = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveRiskRule(ctx context.Context, id *uuid.UUID, item RiskRule, expected int64, actor identity.Actor) (RiskRule, error) {
	item.Code, item.Name = normalize(item.Code), strings.TrimSpace(item.Name)
	if item.Code == "" || len(item.Code) > 80 || item.Name == "" || len(item.Name) > 120 || !contains([]string{"concurrent_streams", "transcoding", "bitrate", "remote_ip", "custom"}, item.RuleType) || !contains([]string{"low", "medium", "high", "critical"}, item.Severity) || !contains([]string{"none", "stop_session", "disable_user"}, item.Action) || item.CooldownSeconds < 0 || item.CooldownSeconds > 604800 || !validRiskCondition(item.RuleType, item.Condition) {
		return RiskRule{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RiskRule{}, err
	}
	defer tx.Rollback(ctx)
	ruleID := uuid.New()
	action := "risk_rule.create"
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO risk_rules(id,instance_id,code,name,description,rule_type,condition,severity,action,observation_mode,enabled,cooldown_seconds,created_by) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13)`, ruleID, item.InstanceID, item.Code, item.Name, item.Description, item.RuleType, jsonBytes(item.Condition), item.Severity, item.Action, item.ObservationMode, item.Enabled, item.CooldownSeconds, actor.Label())
	} else {
		ruleID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM risk_rules WHERE id=$1 FOR UPDATE`, ruleID).Scan(&revision); err != nil {
			return RiskRule{}, notFound(err)
		}
		if revision != expected {
			return RiskRule{}, identity.ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE risk_rules SET instance_id=$2,code=$3,name=$4,description=NULLIF($5,''),rule_type=$6,condition=$7,severity=$8,action=$9,observation_mode=$10,enabled=$11,cooldown_seconds=$12,revision=revision+1,updated_at=NOW() WHERE id=$1`, ruleID, item.InstanceID, item.Code, item.Name, item.Description, item.RuleType, jsonBytes(item.Condition), item.Severity, item.Action, item.ObservationMode, item.Enabled, item.CooldownSeconds)
		action = "risk_rule.update"
	}
	if err != nil {
		return RiskRule{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "risk_rule", ruleID.String(), map[string]any{"observation_mode": item.ObservationMode, "action": item.Action}); err != nil {
		return RiskRule{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RiskRule{}, err
	}
	items, err := s.ListRiskRules(ctx, item.InstanceID)
	if err != nil {
		return RiskRule{}, err
	}
	for _, candidate := range items {
		if candidate.ID == ruleID {
			return candidate, nil
		}
	}
	return RiskRule{}, identity.ErrNotFound
}

func validRiskCondition(kind string, condition map[string]any) bool {
	if condition == nil {
		return false
	}
	switch kind {
	case "concurrent_streams":
		return numberValue(condition["threshold"]) >= 1
	case "transcoding":
		return true
	case "bitrate":
		return numberValue(condition["minimum_bitrate"]) > 0
	case "remote_ip":
		return stringValue(condition["value"]) != "" && validMatchOperator(stringValue(condition["operator"]), stringValue(condition["value"]))
	case "custom":
		return contains([]string{"client_name", "device_name", "device_id", "platform", "remote_ip", "play_method", "item_type"}, stringValue(condition["field"])) && stringValue(condition["value"]) != "" && validMatchOperator(stringValue(condition["operator"]), stringValue(condition["value"]))
	}
	return false
}

func validMatchOperator(operator, value string) bool {
	if !contains([]string{"exact", "contains", "prefix", "regex"}, operator) {
		return false
	}
	if operator == "regex" {
		_, err := regexp.Compile(value)
		return err == nil
	}
	return true
}

func (s *Service) ListRiskEvents(ctx context.Context, instanceID, accountID *uuid.UUID, status, severity string, limit int) ([]RiskEvent, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT e.id,e.instance_id,i.name,e.rule_id,e.device_rule_id,e.playback_session_id,e.binding_id,e.account_id,e.source,e.severity,e.title,e.reason,e.evidence,e.rule_snapshot,e.observation_mode,e.recommended_action,e.status,COALESCE(e.disposition_reason,''),COALESCE(e.disposition_by,''),e.disposition_at,e.created_at,e.updated_at FROM risk_events e JOIN emby_instances i ON i.id=e.instance_id WHERE ($1::uuid IS NULL OR e.instance_id=$1) AND ($2::uuid IS NULL OR e.account_id=$2) AND ($3='' OR e.status=$3) AND ($4='' OR e.severity=$4) ORDER BY e.created_at DESC LIMIT $5`, uuidQueryValue(instanceID), uuidQueryValue(accountID), status, severity, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RiskEvent
	for rows.Next() {
		item, scanErr := scanRiskEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanRiskEvent(row pgx.Row) (RiskEvent, error) {
	var item RiskEvent
	var evidence, snapshot []byte
	err := row.Scan(&item.ID, &item.InstanceID, &item.InstanceName, &item.RuleID, &item.DeviceRuleID, &item.PlaybackSessionID, &item.BindingID, &item.AccountID, &item.Source, &item.Severity, &item.Title, &item.Reason, &evidence, &snapshot, &item.ObservationMode, &item.RecommendedAction, &item.Status, &item.DispositionReason, &item.DispositionBy, &item.DispositionAt, &item.CreatedAt, &item.UpdatedAt)
	item.Evidence, item.RuleSnapshot = decodeJSON(evidence), decodeJSON(snapshot)
	return item, err
}

func (s *Service) GetRiskEvent(ctx context.Context, id uuid.UUID) (RiskEventDetail, error) {
	var detail RiskEventDetail
	row := s.db.QueryRow(ctx, `SELECT e.id,e.instance_id,i.name,e.rule_id,e.device_rule_id,e.playback_session_id,e.binding_id,e.account_id,e.source,e.severity,e.title,e.reason,e.evidence,e.rule_snapshot,e.observation_mode,e.recommended_action,e.status,COALESCE(e.disposition_reason,''),COALESCE(e.disposition_by,''),e.disposition_at,e.created_at,e.updated_at FROM risk_events e JOIN emby_instances i ON i.id=e.instance_id WHERE e.id=$1`, id)
	var err error
	detail.Event, err = scanRiskEvent(row)
	if err != nil {
		return RiskEventDetail{}, notFound(err)
	}
	rows, err := s.db.Query(ctx, `SELECT id,event_id,instance_id,task_id,action_type,status,reason,COALESCE(remote_session_id,''),COALESCE(remote_user_id,''),before_state,after_state,result,attempts,COALESCE(last_error,''),executed_at,reverted_at,created_at,updated_at FROM risk_actions WHERE event_id=$1 ORDER BY created_at,id`, id)
	if err != nil {
		return RiskEventDetail{}, err
	}
	for rows.Next() {
		var item RiskAction
		var before, after, result []byte
		if err = rows.Scan(&item.ID, &item.EventID, &item.InstanceID, &item.TaskID, &item.ActionType, &item.Status, &item.Reason, &item.RemoteSessionID, &item.RemoteUserID, &before, &after, &result, &item.Attempts, &item.LastError, &item.ExecutedAt, &item.RevertedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return RiskEventDetail{}, err
		}
		item.BeforeState, item.AfterState, item.Result = decodeJSON(before), decodeJSON(after), decodeJSON(result)
		detail.Actions = append(detail.Actions, item)
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT id,event_id,event_type,actor,COALESCE(reason,''),details,created_at FROM risk_event_timeline WHERE event_id=$1 ORDER BY id`, id)
	if err != nil {
		return RiskEventDetail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RiskTimelineEntry
		var raw []byte
		if err = rows.Scan(&item.ID, &item.EventID, &item.EventType, &item.Actor, &item.Reason, &raw, &item.CreatedAt); err != nil {
			return RiskEventDetail{}, err
		}
		item.Details = decodeJSON(raw)
		detail.Timeline = append(detail.Timeline, item)
	}
	return detail, rows.Err()
}

func (s *Service) DispositionRiskEvent(ctx context.Context, id uuid.UUID, status, reason string, actor identity.Actor) (RiskEventDetail, error) {
	reason = strings.TrimSpace(reason)
	if !contains([]string{"acknowledged", "resolved"}, status) || reason == "" || len(reason) > 1000 {
		return RiskEventDetail{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RiskEventDetail{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE risk_events SET status=$2,disposition_reason=$3,disposition_by=$4,disposition_at=NOW(),updated_at=NOW() WHERE id=$1 AND status<>'false_positive'`, id, status, reason, actor.Label())
	if err != nil || tag.RowsAffected() != 1 {
		return RiskEventDetail{}, identity.ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) VALUES($1,$2,$3,$4,$5)`, id, status, actor.Label(), reason, jsonBytes(map[string]any{"status": status})); err != nil {
		return RiskEventDetail{}, err
	}
	if err = audit(ctx, tx, actor, "risk_event."+status, "risk_event", id.String(), map[string]any{"reason": reason}); err != nil {
		return RiskEventDetail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RiskEventDetail{}, err
	}
	return s.GetRiskEvent(ctx, id)
}

func (s *Service) MarkRiskFalsePositive(ctx context.Context, id uuid.UUID, reason string, actor identity.Actor) (RiskEventDetail, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1000 {
		return RiskEventDetail{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RiskEventDetail{}, err
	}
	defer tx.Rollback(ctx)
	var instanceID uuid.UUID
	var current string
	if err = tx.QueryRow(ctx, `SELECT instance_id,status FROM risk_events WHERE id=$1 FOR UPDATE`, id).Scan(&instanceID, &current); err != nil {
		return RiskEventDetail{}, notFound(err)
	}
	if current == "false_positive" {
		return RiskEventDetail{}, identity.ErrConflict
	}
	var actionID uuid.UUID
	var actionType, actionStatus string
	var oldTaskID *uuid.UUID
	var actionResultRaw []byte
	revertQueued := false
	err = tx.QueryRow(ctx, `SELECT id,action_type,status,task_id,result FROM risk_actions WHERE event_id=$1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, id).Scan(&actionID, &actionType, &actionStatus, &oldTaskID, &actionResultRaw)
	remoteEffectApplied := boolValue(decodeJSON(actionResultRaw)["action_effect_applied"])
	if err == nil && (actionStatus == "running" || actionStatus == "reverting") {
		return RiskEventDetail{}, identity.ErrConflict
	}
	if err == nil && (actionStatus == "pending" || actionStatus == "failed") && !(actionStatus == "failed" && remoteEffectApplied && actionType == "disable_user") {
		if _, err = tx.Exec(ctx, `UPDATE risk_actions SET status='canceled',last_error='canceled after false-positive review',updated_at=NOW() WHERE id=$1`, actionID); err != nil {
			return RiskEventDetail{}, err
		}
		if oldTaskID != nil {
			if _, err = tx.Exec(ctx, `UPDATE platform_tasks SET status='failed',last_error='canceled after false-positive review',lease_owner=NULL,lease_expires_at=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1 AND status IN ('pending','retry','failed','dead')`, *oldTaskID); err != nil {
				return RiskEventDetail{}, err
			}
		}
	} else if err == nil && (actionStatus == "succeeded" || actionStatus == "failed" && remoteEffectApplied) && actionType == "disable_user" {
		taskID := uuid.New()
		if _, err = tx.Exec(ctx, `UPDATE risk_actions SET status='revert_pending',task_id=$2,updated_at=NOW() WHERE id=$1`, actionID, taskID); err != nil {
			return RiskEventDetail{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'risk.revert',$2,$3,8,$4)`, taskID, "risk-revert:"+actionID.String(), jsonBytes(map[string]any{"instance_id": instanceID.String(), "action_id": actionID.String()}), actor.Label())
		if err != nil {
			return RiskEventDetail{}, err
		}
		revertQueued = true
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return RiskEventDetail{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE risk_events SET status='false_positive',disposition_reason=$2,disposition_by=$3,disposition_at=NOW(),updated_at=NOW() WHERE id=$1`, id, reason, actor.Label()); err != nil {
		return RiskEventDetail{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) VALUES($1,'false_positive',$2,$3,$4)`, id, actor.Label(), reason, jsonBytes(map[string]any{"revert_queued": revertQueued, "action_status": actionStatus})); err != nil {
		return RiskEventDetail{}, err
	}
	if err = audit(ctx, tx, actor, "risk_event.false_positive", "risk_event", id.String(), map[string]any{"reason": reason, "revert_queued": revertQueued, "action_status": actionStatus}); err != nil {
		return RiskEventDetail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RiskEventDetail{}, err
	}
	return s.GetRiskEvent(ctx, id)
}

type playbackObservation struct {
	Session      embyPlaybackSession
	ItemID       string
	ItemName     string
	ItemType     string
	SeriesName   string
	Platform     string
	PlayMethod   string
	RemoteIP     string
	PlaybackKey  string
	Transcoding  bool
	Bitrate      int64
	Position     int64
	Runtime      int64
	Paused       bool
	BindingID    *uuid.UUID
	AccountID    *uuid.UUID
	DeviceRule   *DeviceRule
	DeviceStatus string
}

func (s *Service) IngestPlaybackSnapshot(ctx context.Context, instance EmbyInstance, sessions []embyPlaybackSession, actor identity.Actor) (map[string]any, error) {
	rules, err := s.ListDeviceRules(ctx, &instance.ID)
	if err != nil {
		return nil, err
	}
	riskRules, err := s.ListRiskRules(ctx, &instance.ID)
	if err != nil {
		return nil, err
	}
	historyRetentionDays := 180
	if scanErr := s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key='playback.history_retention_days'`).Scan(&historyRetentionDays); scanErr != nil || historyRetentionDays < 1 {
		historyRetentionDays = 180
	}
	observations := make([]playbackObservation, 0, len(sessions))
	for _, session := range sessions {
		observation := parsePlayback(session)
		var bindingID, accountID uuid.UUID
		if err = s.db.QueryRow(ctx, `SELECT id,account_id FROM emby_account_bindings WHERE instance_id=$1 AND remote_user_id=$2 AND status<>'deleted'`, instance.ID, session.UserID).Scan(&bindingID, &accountID); err == nil {
			observation.BindingID, observation.AccountID = &bindingID, &accountID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		observation.DeviceStatus, observation.DeviceRule = evaluateDeviceRules(rules, observation)
		observations = append(observations, observation)
	}
	concurrent := map[string]int{}
	for _, observation := range observations {
		concurrent[observation.Session.UserID]++
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	seen := make([]string, 0, len(observations))
	eventsCreated, actionsQueued := 0, 0
	for _, observation := range observations {
		seen = append(seen, observation.Session.ID)
		sessionID, isNew, ingestErr := ingestPlaybackTx(ctx, tx, instance.ID, observation)
		if ingestErr != nil {
			return nil, ingestErr
		}
		if observation.DeviceRule != nil && observation.DeviceRule.Decision == "deny" {
			created, queued, createErr := createRiskEventTx(ctx, tx, instance, sessionID, observation, nil, observation.DeviceRule, observation.DeviceRule.ObservationMode, "high", observation.DeviceRule.Action, "设备命中黑名单规则", fmt.Sprintf("设备 %s 使用客户端 %s 命中规则 %s", observation.Session.DeviceName, observation.Session.Client, observation.DeviceRule.Name), map[string]any{"playback_key": observation.PlaybackKey})
			if createErr != nil {
				return nil, createErr
			}
			eventsCreated += boolInt(created)
			actionsQueued += boolInt(queued)
		}
		for index := range riskRules {
			rule := &riskRules[index]
			matched, evidence := evaluateRiskRule(*rule, observation, concurrent[observation.Session.UserID])
			if !matched {
				continue
			}
			observationOnly := rule.ObservationMode || observation.DeviceStatus == "allowed"
			created, queued, createErr := createRiskEventTx(ctx, tx, instance, sessionID, observation, rule, nil, observationOnly, rule.Severity, rule.Action, rule.Name, riskReason(*rule, observation, concurrent[observation.Session.UserID]), evidence)
			if createErr != nil {
				return nil, createErr
			}
			eventsCreated += boolInt(created)
			actionsQueued += boolInt(queued)
		}
		_ = isNew
	}
	if len(seen) == 0 {
		_, err = tx.Exec(ctx, `UPDATE playback_history SET ended_at=COALESCE(ended_at,NOW()) WHERE instance_id=$1 AND ended_at IS NULL`, instance.ID)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM playback_sessions WHERE instance_id=$1`, instance.ID)
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE playback_history h SET ended_at=COALESCE(h.ended_at,NOW()) FROM playback_sessions p WHERE p.instance_id=$1 AND p.remote_session_id<>ALL($2::text[]) AND h.instance_id=p.instance_id AND h.playback_key=p.playback_key AND h.ended_at IS NULL`, instance.ID, seen)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM playback_sessions WHERE instance_id=$1 AND remote_session_id<>ALL($2::text[])`, instance.ID, seen)
		}
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM playback_history WHERE started_at<NOW()-($1::double precision*INTERVAL '1 day')`, historyRetentionDays); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO emby_instance_runtime_health(instance_id,last_success_at) VALUES($1,NOW()) ON CONFLICT(instance_id) DO UPDATE SET consecutive_failures=0,circuit_open_until=NULL,last_success_at=NOW(),last_error=NULL,updated_at=NOW()`, instance.ID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"online_sessions": len(observations), "risk_events_created": eventsCreated, "actions_queued": actionsQueued}, nil
}

func parsePlayback(session embyPlaybackSession) playbackObservation {
	itemID, itemName := stringValue(session.NowPlayingItem["Id"]), stringValue(session.NowPlayingItem["Name"])
	remoteIP := strings.TrimSpace(session.RemoteEndPoint)
	if index := strings.LastIndex(remoteIP, ":"); index > 0 && strings.Count(remoteIP, ":") == 1 {
		remoteIP = remoteIP[:index]
	}
	method := stringValue(session.PlayState["PlayMethod"])
	transcoding := method == "Transcode" || len(session.TranscodingInfo) > 0
	platform := stringValue(session.Raw["DeviceType"])
	if platform == "" {
		platform = stringValue(session.Raw["OperatingSystem"])
	}
	return playbackObservation{Session: session, ItemID: itemID, ItemName: itemName, ItemType: stringValue(session.NowPlayingItem["Type"]), SeriesName: stringValue(session.NowPlayingItem["SeriesName"]), Platform: platform, PlayMethod: method, RemoteIP: remoteIP, PlaybackKey: session.ID + ":" + itemID, Transcoding: transcoding, Bitrate: numberValue(session.TranscodingInfo["Bitrate"]), Position: numberValue(session.PlayState["PositionTicks"]), Runtime: numberValue(session.NowPlayingItem["RunTimeTicks"]), Paused: boolValue(session.PlayState["IsPaused"]), DeviceStatus: "unmatched"}
}

func ingestPlaybackTx(ctx context.Context, tx pgx.Tx, instanceID uuid.UUID, observation playbackObservation) (uuid.UUID, bool, error) {
	ruleID := rulePointer(observation.DeviceRule)
	sessionID := uuid.New()
	if _, err := tx.Exec(ctx, `UPDATE playback_history SET ended_at=COALESCE(ended_at,NOW()) WHERE instance_id=$1 AND playback_key=(SELECT playback_key FROM playback_sessions WHERE instance_id=$1 AND remote_session_id=$2) AND playback_key<>$3 AND ended_at IS NULL`, instanceID, observation.Session.ID, observation.PlaybackKey); err != nil {
		return uuid.Nil, false, err
	}
	err := tx.QueryRow(ctx, `INSERT INTO playback_sessions(id,instance_id,binding_id,account_id,remote_session_id,playback_key,remote_user_id,remote_username,item_id,item_name,item_type,series_name,client_name,device_name,device_id,platform,remote_ip,play_method,transcoding,bitrate,position_ticks,runtime_ticks,paused,device_decision,matched_device_rule_id,raw_snapshot) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),NULLIF($17,''),NULLIF($18,''),$19,NULLIF($20,0),$21,NULLIF($22,0),$23,$24,$25,$26) ON CONFLICT(instance_id,remote_session_id) DO UPDATE SET binding_id=EXCLUDED.binding_id,account_id=EXCLUDED.account_id,playback_key=EXCLUDED.playback_key,remote_user_id=EXCLUDED.remote_user_id,remote_username=EXCLUDED.remote_username,item_id=EXCLUDED.item_id,item_name=EXCLUDED.item_name,item_type=EXCLUDED.item_type,series_name=EXCLUDED.series_name,client_name=EXCLUDED.client_name,device_name=EXCLUDED.device_name,device_id=EXCLUDED.device_id,platform=EXCLUDED.platform,remote_ip=EXCLUDED.remote_ip,play_method=EXCLUDED.play_method,transcoding=EXCLUDED.transcoding,bitrate=EXCLUDED.bitrate,position_ticks=EXCLUDED.position_ticks,runtime_ticks=EXCLUDED.runtime_ticks,paused=EXCLUDED.paused,device_decision=EXCLUDED.device_decision,matched_device_rule_id=EXCLUDED.matched_device_rule_id,raw_snapshot=EXCLUDED.raw_snapshot,last_seen_at=NOW() RETURNING id`, sessionID, instanceID, observation.BindingID, observation.AccountID, observation.Session.ID, observation.PlaybackKey, observation.Session.UserID, observation.Session.UserName, observation.ItemID, observation.ItemName, observation.ItemType, observation.SeriesName, observation.Session.Client, observation.Session.DeviceName, observation.Session.DeviceID, observation.Platform, observation.RemoteIP, observation.PlayMethod, observation.Transcoding, observation.Bitrate, observation.Position, observation.Runtime, observation.Paused, observation.DeviceStatus, ruleID, jsonBytes(observation.Session.Raw)).Scan(&sessionID)
	if err != nil {
		return uuid.Nil, false, err
	}
	historyID := uuid.New()
	isNew := true
	err = tx.QueryRow(ctx, `INSERT INTO playback_history(id,instance_id,binding_id,account_id,remote_session_id,playback_key,remote_user_id,remote_username,item_id,item_name,item_type,series_name,client_name,device_name,device_id,platform,remote_ip,play_method,transcoding,peak_bitrate,max_position_ticks,runtime_ticks,raw_snapshot) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),NULLIF($17,''),NULLIF($18,''),$19,NULLIF($20,0),$21,NULLIF($22,0),$23) ON CONFLICT(instance_id,playback_key) DO NOTHING RETURNING id`, historyID, instanceID, observation.BindingID, observation.AccountID, observation.Session.ID, observation.PlaybackKey, observation.Session.UserID, observation.Session.UserName, observation.ItemID, observation.ItemName, observation.ItemType, observation.SeriesName, observation.Session.Client, observation.Session.DeviceName, observation.Session.DeviceID, observation.Platform, observation.RemoteIP, observation.PlayMethod, observation.Transcoding, observation.Bitrate, observation.Position, observation.Runtime, jsonBytes(observation.Session.Raw)).Scan(&historyID)
	if errors.Is(err, pgx.ErrNoRows) {
		isNew = false
		_, err = tx.Exec(ctx, `UPDATE playback_history SET binding_id=$3,account_id=$4,remote_username=NULLIF($5,''),peak_bitrate=GREATEST(COALESCE(peak_bitrate,0),$6),max_position_ticks=GREATEST(max_position_ticks,$7),runtime_ticks=NULLIF($8,0),last_seen_at=NOW(),ended_at=NULL,raw_snapshot=$9 WHERE instance_id=$1 AND playback_key=$2`, instanceID, observation.PlaybackKey, observation.BindingID, observation.AccountID, observation.Session.UserName, observation.Bitrate, observation.Position, observation.Runtime, jsonBytes(observation.Session.Raw))
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	deviceKey := strings.TrimSpace(observation.Session.DeviceID)
	if deviceKey == "" {
		sum := sha256.Sum256([]byte(strings.Join([]string{observation.Session.UserID, observation.Session.Client, observation.Session.DeviceName}, "\x00")))
		deviceKey = "derived:" + hex.EncodeToString(sum[:8])
	}
	increment := 0
	transcodeIncrement := 0
	if isNew {
		increment = 1
		if observation.Transcoding {
			transcodeIncrement = 1
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO device_profiles(id,instance_id,binding_id,account_id,remote_user_id,device_key,device_id,device_name,client_name,platform,first_ip,last_ip,session_count,transcode_count,maximum_bitrate,access_decision,matched_rule_id) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($11,''),$12,$13,NULLIF($14,0),$15,$16) ON CONFLICT(instance_id,remote_user_id,device_key) DO UPDATE SET binding_id=EXCLUDED.binding_id,account_id=EXCLUDED.account_id,device_id=EXCLUDED.device_id,device_name=EXCLUDED.device_name,client_name=EXCLUDED.client_name,platform=EXCLUDED.platform,last_ip=EXCLUDED.last_ip,session_count=device_profiles.session_count+$12,transcode_count=device_profiles.transcode_count+$13,maximum_bitrate=GREATEST(COALESCE(device_profiles.maximum_bitrate,0),$14),access_decision=EXCLUDED.access_decision,matched_rule_id=EXCLUDED.matched_rule_id,last_seen_at=NOW()`, uuid.New(), instanceID, observation.BindingID, observation.AccountID, observation.Session.UserID, deviceKey, observation.Session.DeviceID, observation.Session.DeviceName, observation.Session.Client, observation.Platform, observation.RemoteIP, increment, transcodeIncrement, observation.Bitrate, observation.DeviceStatus, ruleID)
	return sessionID, isNew, err
}

func evaluateDeviceRules(rules []DeviceRule, observation playbackObservation) (string, *DeviceRule) {
	for index := range rules {
		rule := &rules[index]
		if !rule.Enabled {
			continue
		}
		value := observationField(observation, rule.MatchField)
		if matchValue(value, rule.MatchOperator, rule.MatchValue) {
			if rule.Decision == "allow" {
				return "allowed", rule
			}
			return "denied", rule
		}
	}
	return "unmatched", nil
}

func evaluateRiskRule(rule RiskRule, observation playbackObservation, concurrent int) (bool, map[string]any) {
	evidence := map[string]any{"playback_key": observation.PlaybackKey, "remote_user_id": observation.Session.UserID, "client_name": observation.Session.Client, "device_name": observation.Session.DeviceName, "remote_ip": observation.RemoteIP}
	switch rule.RuleType {
	case "concurrent_streams":
		threshold := int(numberValue(rule.Condition["threshold"]))
		evidence["concurrent_streams"], evidence["threshold"] = concurrent, threshold
		return concurrent >= threshold, evidence
	case "transcoding":
		evidence["transcoding"] = observation.Transcoding
		return observation.Transcoding, evidence
	case "bitrate":
		minimum := numberValue(rule.Condition["minimum_bitrate"])
		evidence["bitrate"], evidence["minimum_bitrate"] = observation.Bitrate, minimum
		return observation.Bitrate >= minimum, evidence
	case "remote_ip":
		return matchValue(observation.RemoteIP, stringValue(rule.Condition["operator"]), stringValue(rule.Condition["value"])), evidence
	case "custom":
		field := stringValue(rule.Condition["field"])
		evidence["field"], evidence["observed_value"] = field, observationField(observation, field)
		return matchValue(observationField(observation, field), stringValue(rule.Condition["operator"]), stringValue(rule.Condition["value"])), evidence
	}
	return false, evidence
}

func createRiskEventTx(ctx context.Context, tx pgx.Tx, instance EmbyInstance, sessionID uuid.UUID, observation playbackObservation, riskRule *RiskRule, deviceRule *DeviceRule, observationOnly bool, severity, recommendedAction, title, reason string, evidence map[string]any) (bool, bool, error) {
	cooldown := 300
	ruleKey, source := "", "risk_rule"
	var riskRuleID, deviceRuleID *uuid.UUID
	var snapshot any
	if riskRule != nil {
		ruleKey, cooldown, riskRuleID, snapshot = riskRule.ID.String(), riskRule.CooldownSeconds, &riskRule.ID, riskRule
	} else if deviceRule != nil {
		ruleKey, deviceRuleID, source, snapshot = deviceRule.ID.String(), &deviceRule.ID, "device_rule", deviceRule
	}
	if cooldown < 1 {
		cooldown = 1
	}
	bucket := time.Now().Unix() / int64(cooldown)
	dedupe := strings.Join([]string{source, ruleKey, instance.ID.String(), observation.PlaybackKey, fmt.Sprint(bucket)}, ":")
	eventID := uuid.New()
	err := tx.QueryRow(ctx, `INSERT INTO risk_events(id,instance_id,rule_id,device_rule_id,playback_session_id,binding_id,account_id,dedupe_key,source,severity,title,reason,evidence,rule_snapshot,observation_mode,recommended_action) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT(dedupe_key) DO NOTHING RETURNING id`, eventID, instance.ID, riskRuleID, deviceRuleID, sessionID, observation.BindingID, observation.AccountID, dedupe, source, severity, title, reason, jsonBytes(evidence), jsonBytes(snapshot), observationOnly, recommendedAction).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) VALUES($1,'detected','system:risk-engine',$2,$3)`, eventID, reason, jsonBytes(map[string]any{"observation_mode": observationOnly, "recommended_action": recommendedAction})); err != nil {
		return false, false, err
	}
	queued := false
	if !observationOnly && recommendedAction != "none" {
		actionID, taskID := uuid.New(), uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO risk_actions(id,event_id,instance_id,task_id,action_type,reason,remote_session_id,remote_user_id) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''))`, actionID, eventID, instance.ID, taskID, recommendedAction, reason, observation.Session.ID, observation.Session.UserID)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'risk.action',$2,$3,8,'system:risk-engine')`, taskID, "risk-action:"+actionID.String(), jsonBytes(map[string]any{"instance_id": instance.ID.String(), "action_id": actionID.String()}))
		}
		if err != nil {
			return false, false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO risk_event_timeline(event_id,event_type,actor,reason,details) VALUES($1,'action_queued','system:risk-engine',$2,$3)`, eventID, reason, jsonBytes(map[string]any{"action_id": actionID, "action_type": recommendedAction, "task_id": taskID})); err != nil {
			return false, false, err
		}
		queued = true
	}
	if err = queueRiskNotificationsTx(ctx, tx, eventID, observation.AccountID, severity, title, reason); err != nil {
		return false, false, err
	}
	return true, queued, nil
}

func queueRiskNotificationsTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, affectedAccountID *uuid.UUID, severity, title, reason string) error {
	var raw []byte
	var notifyAffected bool
	_ = tx.QueryRow(ctx, `SELECT value FROM dynamic_settings WHERE key='risk.telegram_alert_account_ids'`).Scan(&raw)
	_ = tx.QueryRow(ctx, `SELECT (value #>> '{}')::boolean FROM dynamic_settings WHERE key='risk.notify_affected_account'`).Scan(&notifyAffected)
	var configured []string
	_ = json.Unmarshal(raw, &configured)
	recipients := map[uuid.UUID]bool{}
	for _, value := range configured {
		if id, err := uuid.Parse(value); err == nil {
			recipients[id] = true
		}
	}
	if notifyAffected && affectedAccountID != nil {
		recipients[*affectedAccountID] = true
	}
	ids := make([]uuid.UUID, 0, len(recipients))
	for id := range recipients {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, accountID := range ids {
		var telegram bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_identities WHERE account_id=$1 AND kind='telegram' AND NOT disabled)`, accountID).Scan(&telegram); err != nil {
			return err
		}
		if !telegram {
			continue
		}
		_, err := tx.Exec(ctx, `INSERT INTO account_notifications(id,account_id,title,body,channel,metadata) VALUES($1,$2,$3,$4,'telegram',$5)`, uuid.New(), accountID, "["+strings.ToUpper(severity)+"] "+title, reason, jsonBytes(map[string]any{"risk_event_id": eventID.String(), "kind": "risk_alert"}))
		if err != nil {
			return err
		}
	}
	return nil
}

func observationField(observation playbackObservation, field string) string {
	switch field {
	case "client_name":
		return observation.Session.Client
	case "device_name":
		return observation.Session.DeviceName
	case "device_id":
		return observation.Session.DeviceID
	case "platform":
		return observation.Platform
	case "remote_ip":
		return observation.RemoteIP
	case "play_method":
		return observation.PlayMethod
	case "item_type":
		return observation.ItemType
	}
	return ""
}

func matchValue(observed, operator, expected string) bool {
	left, right := strings.ToLower(strings.TrimSpace(observed)), strings.ToLower(strings.TrimSpace(expected))
	switch operator {
	case "exact":
		return left == right
	case "contains":
		return strings.Contains(left, right)
	case "prefix":
		return strings.HasPrefix(left, right)
	case "regex":
		expression, err := regexp.Compile("(?i)" + expected)
		return err == nil && expression.MatchString(observed)
	}
	return false
}

func riskReason(rule RiskRule, observation playbackObservation, concurrent int) string {
	switch rule.RuleType {
	case "concurrent_streams":
		return fmt.Sprintf("用户 %s 同时存在 %d 个播放会话，达到规则阈值", observation.Session.UserName, concurrent)
	case "transcoding":
		return fmt.Sprintf("会话 %s 正在进行转码播放", observation.Session.ID)
	case "bitrate":
		return fmt.Sprintf("会话码率 %d 达到规则阈值", observation.Bitrate)
	default:
		return fmt.Sprintf("播放会话命中风险规则 %s", rule.Name)
	}
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func numberValue(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int:
		return int64(number)
	case int64:
		return number
	case json.Number:
		result, _ := number.Int64()
		return result
	}
	return 0
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func rulePointer(rule *DeviceRule) any {
	if rule == nil {
		return nil
	}
	return rule.ID
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
