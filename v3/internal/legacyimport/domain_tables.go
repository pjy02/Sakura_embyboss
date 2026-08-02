package legacyimport

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

// TableImportReport makes every previously-uncovered v2 table visible in both
// dry-run and apply reports.  Archived is intentional preservation (for
// example an in-flight task); Deferred requires an operator decision and is a
// reconciliation blocker.
type TableImportReport struct {
	Table       string `json:"table"`
	SourceRows  int    `json:"source_rows"`
	Transformed int    `json:"transformed_rows"`
	Archived    int    `json:"archived_rows"`
	Deferred    int    `json:"deferred_rows"`
}

type adapterTableSpec struct {
	name string
	key  string
}

var adapterTableSpecs = []adapterTableSpec{
	// Financial and account-governance history is mandatory and must be
	// represented in an active v3 ledger/audit domain.
	{"point_transactions", "id"},
	{"billing_entries", "id"},
	{"account_lifecycle_events", "id"},
	{"emby2", "embyid"},
	{"partition_codes", "code"},
	{"partition_grants", "id"},
	{"line_endpoints", "id"},
	{"playback_sessions", "id"},
	{"known_devices", "device_key"},
	{"device_client_rules", "id"},
	{"security_events", "id"},
	{"risk_rules", "id"},
	{"media_requests", "id"},
	{"request_records", "download_id"},
	{"media_reviews", "id"},
	{"review_reactions", "id"},
	{"review_reports", "id"},
	{"automation_rules", "id"},
	{"operation_tasks", "id"},
	{"config_revisions", "id"},
	{"api_clients", "id"},
	// Low-value runtime history is intentionally retained only in the unified
	// migration archive. It is never replayed into v3 workers.
	{"idempotency_records", "id"},
	{"job_runs", "id"},
	{"system_events", "id"},
	{"automation_runs", "id"},
	{"line_health_samples", "id"},
	{"service_probes", "id"},
	{"alert_deliveries", "id"},
}

type adapterRow struct {
	Table string
	Key   string
	Data  map[string]any
}

type adapterResult struct {
	disposition string
	targetTable string
	targetKey   string
	detail      string
}

func (i *Importer) readAdapterTables(ctx context.Context) (map[string][]adapterRow, error) {
	result := make(map[string][]adapterRow, len(adapterTableSpecs))
	for _, spec := range adapterTableSpecs {
		exists, err := i.tableExists(ctx, spec.name)
		if err != nil {
			return nil, err
		}
		if !exists {
			result[spec.name] = nil
			continue
		}
		rows, err := i.source.QueryContext(ctx, "SELECT * FROM `"+spec.name+"`")
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", spec.name, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, err
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s: %w", spec.name, err)
			}
			data := make(map[string]any, len(columns))
			for index, column := range columns {
				data[column] = normalizeSourceValue(values[index])
			}
			key := valueString(data, spec.key)
			if key == "" {
				rows.Close()
				return nil, fmt.Errorf("%s row has empty migration key %s", spec.name, spec.key)
			}
			result[spec.name] = append(result[spec.name], adapterRow{Table: spec.name, Key: key, Data: data})
		}
		if err = rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func normalizeSourceValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return typed
	}
}

func summarizeAdapterRows(rows map[string][]adapterRow) []TableImportReport {
	reports := make([]TableImportReport, 0, len(adapterTableSpecs))
	for _, spec := range adapterTableSpecs {
		reports = append(reports, TableImportReport{Table: spec.name, SourceRows: len(rows[spec.name])})
	}
	return reports
}

func (i *Importer) importAdapterTables(ctx context.Context, tx pgTx, runID uuid.UUID, rows map[string][]adapterRow, tgMap map[int64]uuid.UUID, idMap map[string]uuid.UUID, instanceMap map[string]uuid.UUID) ([]TableImportReport, error) {
	reports := summarizeAdapterRows(rows)
	reportByTable := map[string]*TableImportReport{}
	for index := range reports {
		reportByTable[reports[index].Table] = &reports[index]
	}
	defaultInstance, _ := findDefaultInstance(ctx, tx, instanceMap)
	for _, spec := range adapterTableSpecs {
		for _, row := range rows[spec.name] {
			result, err := transformAdapterRow(ctx, tx, row, tgMap, idMap, defaultInstance)
			if err != nil {
				return nil, fmt.Errorf("transform %s[%s]: %w", row.Table, row.Key, err)
			}
			if err = archiveAdapterRow(ctx, tx, runID, row, result); err != nil {
				return nil, err
			}
			switch result.disposition {
			case "transformed":
				reportByTable[row.Table].Transformed++
			case "archived":
				reportByTable[row.Table].Archived++
			default:
				reportByTable[row.Table].Deferred++
			}
		}
	}
	return reports, nil
}

func archiveAdapterRow(ctx context.Context, tx pgTx, runID uuid.UUID, row adapterRow, result adapterResult) error {
	payload, err := json.Marshal(row.Data)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	id := deterministicUUID("sakura-v2-archive:" + row.Table + ":" + row.Key)
	_, err = tx.Exec(ctx, `INSERT INTO migration_archive_records(id,source_table,source_key,source_payload,payload_sha256,disposition,target_table,target_key,detail,import_run_id)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10)
		ON CONFLICT(source_table,source_key) DO UPDATE SET source_payload=EXCLUDED.source_payload,payload_sha256=EXCLUDED.payload_sha256,disposition=EXCLUDED.disposition,target_table=EXCLUDED.target_table,target_key=EXCLUDED.target_key,detail=EXCLUDED.detail,import_run_id=EXCLUDED.import_run_id,imported_at=NOW()`, id, row.Table, row.Key, payload, hex.EncodeToString(digest[:]), result.disposition, result.targetTable, result.targetKey, result.detail, runID)
	return err
}

func transformAdapterRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, idMap map[string]uuid.UUID, defaultInstance uuid.UUID) (adapterResult, error) {
	switch row.Table {
	case "point_transactions":
		return importPointTransactionRow(ctx, tx, row)
	case "billing_entries":
		return importBillingEntryRow(ctx, tx, row)
	case "account_lifecycle_events":
		return importAccountLifecycleRow(ctx, tx, row, tgMap, idMap)
	case "emby2":
		return importEmby2Row(ctx, tx, row, defaultInstance)
	case "partition_codes":
		return importPartitionCodeRow(ctx, tx, row, tgMap)
	case "partition_grants":
		return importPartitionGrantRow(ctx, tx, row, tgMap, defaultInstance)
	case "line_endpoints":
		return importLineEndpointRow(ctx, tx, row)
	case "playback_sessions":
		return importPlaybackRow(ctx, tx, row, tgMap, defaultInstance)
	case "known_devices":
		return importKnownDeviceRow(ctx, tx, row, tgMap, defaultInstance)
	case "device_client_rules":
		return importDeviceRuleRow(ctx, tx, row)
	case "security_events":
		return importSecurityEventRow(ctx, tx, row, tgMap, defaultInstance)
	case "risk_rules":
		return importRiskRuleRow(ctx, tx, row)
	case "media_requests", "request_records":
		return importMediaRequestRow(ctx, tx, row, tgMap)
	case "media_reviews":
		return importMediaReviewRow(ctx, tx, row, tgMap)
	case "review_reactions":
		return importReviewReactionRow(ctx, tx, row, tgMap)
	case "review_reports":
		return importReviewReportRow(ctx, tx, row, tgMap)
	case "automation_rules":
		return importAutomationRuleRow(ctx, tx, row)
	case "operation_tasks":
		status := strings.ToLower(valueString(row.Data, "status"))
		if status == "pending" || status == "running" || status == "retrying" || status == "leased" {
			return deferred("v2 task must finish or be canceled before cutover"), nil
		}
		return adapterResult{disposition: "archived", detail: "terminal v2 task is preserved but never resumed in v3"}, nil
	case "config_revisions":
		return importConfigRevisionRow(ctx, tx, row)
	case "api_clients":
		return importAPIClientRow(ctx, tx, row, idMap)
	case "idempotency_records", "job_runs", "system_events", "automation_runs", "line_health_samples", "service_probes", "alert_deliveries":
		return adapterResult{disposition: "archived", detail: "low-value v2 runtime history is retained only in migration_archive_records"}, nil
	default:
		return adapterResult{disposition: "deferred", detail: "adapter is not registered"}, nil
	}
}

func importPointTransactionRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	// Unified v2 already mirrors each point transaction into
	// account_ledger_entries. That canonical table is imported first, so reuse
	// its balanced v3 transaction instead of posting the amount twice.
	var transactionID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM ledger_transactions WHERE metadata->>'legacy_source_transaction_id'=$1 LIMIT 1`, row.Key).Scan(&transactionID)
	if err == nil {
		return transformed("ledger_transactions", transactionID.String()), nil
	}
	if err != pgx.ErrNoRows {
		return adapterResult{}, err
	}
	if valueInt64(row.Data, "amount") == 0 {
		requestID := "legacy-v2-point-zero-" + row.Key
		_, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,details,created_at)
			SELECT $1,$2,'legacy.point.zero','wallet',$3,$4,$5,$6 WHERE NOT EXISTS(SELECT 1 FROM audit_logs WHERE request_id=$4)`, fallback(valueString(row.Data, "actor_kind"), "system"), fallback(valueString(row.Data, "actor_id"), "legacy-import"), valueString(row.Data, "account_id"), requestID, jsonBytes(mergeMetadata(row.Data, map[string]any{"source": "v2-point-transaction"})), fallbackTime(row.Data, "created_at"))
		return transformed("audit_logs", requestID), err
	}
	return deferred("canonical account_ledger_entries mapping is missing; refusing to double-post points"), nil
}

func importBillingEntryRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	requestID := "legacy-v2-billing-" + row.Key
	details := mergeMetadata(row.Data, map[string]any{"source": "v2-billing-entry"})
	_, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,details,created_at)
		SELECT $1,$2,$3,'billing_entry',NULLIF($4,''),$5,$6,$7 WHERE NOT EXISTS(SELECT 1 FROM audit_logs WHERE request_id=$5)`, fallback(valueString(row.Data, "actor_kind"), "system"), fallback(valueString(row.Data, "actor_id"), "legacy-import"), "billing."+fallback(valueString(row.Data, "entry_type"), "event"), valueString(row.Data, "order_id"), requestID, jsonBytes(details), fallbackTime(row.Data, "created_at"))
	return transformed("audit_logs", requestID), err
}

func importAccountLifecycleRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, idMap map[string]uuid.UUID) (adapterResult, error) {
	accountID, ok := idMap[valueString(row.Data, "account_id")]
	if !ok {
		accountID, ok = tgMap[valueInt64(row.Data, "tg")]
	}
	if !ok {
		return deferred("lifecycle account mapping is missing"), nil
	}
	action := strings.ToLower(valueString(row.Data, "action"))
	fromStatus, toStatus := lifecycleStatuses(action)
	detail := validJSONOr(row.Data["detail_json"], map[string]any{})
	reason := "Imported v2 lifecycle action: " + fallback(action, "unknown")
	requestID := "legacy-v2-lifecycle-" + row.Key
	if _, err := tx.Exec(ctx, `INSERT INTO account_lifecycle_events(account_id,from_status,to_status,reason,actor,created_at,legacy_source_key) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(legacy_source_key) DO NOTHING`, accountID, fromStatus, toStatus, reason, legacyActorPair(row.Data, "actor_kind", "actor_id"), fallbackTime(row.Data, "created_at"), row.Key); err != nil {
		return adapterResult{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(actor_kind,actor_id,action,resource_type,resource_id,request_id,details,created_at)
		SELECT $1,$2,$3,'account',$4,$5,$6,$7 WHERE NOT EXISTS(SELECT 1 FROM audit_logs WHERE request_id=$5)`, fallback(valueString(row.Data, "actor_kind"), "system"), fallback(valueString(row.Data, "actor_id"), "legacy-import"), "account.lifecycle."+fallback(action, "unknown"), accountID.String(), requestID, detail, fallbackTime(row.Data, "created_at")); err != nil {
		return adapterResult{}, err
	}
	return transformed("account_lifecycle_events", requestID), nil
}

func lifecycleStatuses(action string) (string, string) {
	switch action {
	case "activate", "restore", "resume", "unban", "unsuspend":
		return "suspended", "active"
	case "ban", "banned":
		return "active", "banned"
	case "delete", "deleted":
		return "active", "deleted"
	case "expire", "expired", "suspend", "suspended", "disable":
		return "active", "suspended"
	default:
		return "active", "active"
	}
}

func findDefaultInstance(ctx context.Context, tx pgTx, known map[string]uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM emby_instances WHERE enabled ORDER BY is_default DESC,priority,id LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return uuid.Nil, err
	}
	for _, value := range known {
		return value, nil
	}
	return uuid.Nil, nil
}

func importEmby2Row(ctx context.Context, tx pgTx, row adapterRow, instanceID uuid.UUID) (adapterResult, error) {
	if instanceID == uuid.Nil {
		return deferred("Emby instance is required before importing unclaimed emby2 users"), nil
	}
	remoteID, name := valueString(row.Data, "embyid"), valueString(row.Data, "name")
	if remoteID == "" {
		return deferred("remote Emby user id is empty"), nil
	}
	if name == "" {
		name = remoteID
	}
	id := deterministicUUID("sakura-v2-emby2:" + remoteID)
	snapshot := mergeMetadata(row.Data, map[string]any{"source": "v2-emby2"})
	_, err := tx.Exec(ctx, `INSERT INTO remote_emby_users(id,instance_id,remote_user_id,username,username_normalized,claim_status,snapshot,last_seen_at)
		VALUES($1,$2,$3,$4,$5,'unclaimed',$6,NOW()) ON CONFLICT(instance_id,remote_user_id) DO UPDATE SET username=EXCLUDED.username,username_normalized=EXCLUDED.username_normalized,snapshot=EXCLUDED.snapshot,last_seen_at=NOW()`, id, instanceID, remoteID, name, strings.ToLower(name), jsonBytes(snapshot))
	return transformed("remote_emby_users", id.String()), err
}

func importPartitionCodeRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID) (adapterResult, error) {
	code := valueString(row.Data, "code")
	if code == "" {
		return deferred("partition code is empty"), nil
	}
	id := deterministicUUID("sakura-v2-partition-code:" + code)
	status := normalizeEntitlementCodeStatus(valueString(row.Data, "status"))
	var reserved any
	if account, ok := tgMap[valueInt64(row.Data, "reserved_by")]; ok {
		reserved = account
	}
	hint := code
	if len(hint) > 16 {
		hint = hint[len(hint)-16:]
	}
	prefix := code
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	_, err := tx.Exec(ctx, `INSERT INTO entitlement_codes(id,code_hash,code_prefix,code_hint,resource_key,duration_days,status,reserved_by,reservation_token,reserved_at,issued_by,metadata,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13) ON CONFLICT(code_hash) DO NOTHING`, id, security.HashToken(code), prefix, hint, fallback(valueString(row.Data, "partition"), "default"), clampInt(valueInt(row.Data, "duration_days"), 1, 3650, 1), status, reserved, valueString(row.Data, "reservation_token"), nullableTime(row.Data, "reserved_at"), legacyActor(row.Data, "created_by"), jsonBytes(mergeMetadata(row.Data, map[string]any{"source": "v2-partition-code"})), fallbackTime(row.Data, "created_at"))
	return transformed("entitlement_codes", id.String()), err
}

func importPartitionGrantRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, instanceID uuid.UUID) (adapterResult, error) {
	accountID, ok := tgMap[valueInt64(row.Data, "tg")]
	if !ok {
		return deferred("Telegram account mapping is missing"), nil
	}
	if instanceID == uuid.Nil {
		return deferred("Emby instance is required for partition grant"), nil
	}
	partition := fallback(valueString(row.Data, "partition"), "default")
	id := deterministicUUID("sakura-v2-partition-grant:" + row.Key)
	var bindingID any
	remoteID := valueString(row.Data, "embyid")
	if remoteID != "" {
		var found uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM emby_account_bindings WHERE instance_id=$1 AND remote_user_id=$2 AND status<>'deleted' LIMIT 1`, instanceID, remoteID).Scan(&found); err == nil {
			bindingID = found
		} else if err != pgx.ErrNoRows {
			return adapterResult{}, err
		}
	}
	var codeID any
	if code := valueString(row.Data, "code"); code != "" {
		candidate := deterministicUUID("sakura-v2-partition-code:" + code)
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM entitlement_codes WHERE id=$1)`, candidate).Scan(&exists); err != nil {
			return adapterResult{}, err
		}
		if exists {
			codeID = candidate
		}
	}
	expires := fallbackTime(row.Data, "expires_at")
	status := normalizeEntitlementStatus(valueString(row.Data, "status"), expires)
	_, err := tx.Exec(ctx, `INSERT INTO account_entitlements(id,account_id,instance_id,binding_id,resource_key,status,source_code_id,starts_at,expires_at,metadata,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$8,$11) ON CONFLICT(account_id,instance_id,resource_kind,resource_key) DO UPDATE SET expires_at=GREATEST(account_entitlements.expires_at,EXCLUDED.expires_at),status=EXCLUDED.status,binding_id=COALESCE(EXCLUDED.binding_id,account_entitlements.binding_id),updated_at=EXCLUDED.updated_at`, id, accountID, instanceID, bindingID, partition, status, codeID, fallbackTime(row.Data, "created_at"), expires, jsonBytes(mergeMetadata(row.Data, map[string]any{"source": "v2-partition-grant"})), fallbackTime(row.Data, "updated_at"))
	return transformed("account_entitlements", id.String()), err
}

func importLineEndpointRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	id := deterministicUUID("sakura-v2-line:" + row.Key)
	baseURL := valueString(row.Data, "base_url")
	if baseURL == "" {
		return deferred("line base_url is empty"), nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO line_endpoints(id,name,base_url,region,carrier,audience,weight,sort_order,enabled,maintenance,revision,last_status,last_latency_ms,last_error,last_checked_at,metadata,created_at,updated_at)
		VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16,$17,$18) ON CONFLICT(base_url) DO NOTHING`, id, fallback(valueString(row.Data, "name"), baseURL), baseURL, valueString(row.Data, "region"), valueString(row.Data, "carrier"), fallback(valueString(row.Data, "audience"), "all"), clampInt(valueInt(row.Data, "weight"), 0, 100000, 100), valueInt(row.Data, "sort_order"), valueBoolDefault(row.Data, "enabled", true), valueBool(row.Data, "maintenance"), max(valueInt(row.Data, "revision"), 1), fallback(valueString(row.Data, "last_status"), "unknown"), nullableInt(row.Data, "last_latency_ms"), valueString(row.Data, "last_error"), nullableTime(row.Data, "last_checked_at"), jsonBytes(mergeMetadata(row.Data, map[string]any{"source": "v2-line"})), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT id FROM line_endpoints WHERE base_url=$1`, baseURL).Scan(&id)
	}
	return transformed("line_endpoints", id.String()), err
}

func importPlaybackRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, instanceID uuid.UUID) (adapterResult, error) {
	if instanceID == uuid.Nil {
		return deferred("Emby instance is required for playback history"), nil
	}
	// Live state is rebuilt from Emby after cutover; only completed sessions are
	// materialized as history.  The raw row is still archived either way.
	ended := nullableTime(row.Data, "ended_at")
	if ended == nil {
		return adapterResult{disposition: "archived", detail: "active playback session is rebuilt from Emby"}, nil
	}
	remoteSession := fallback(valueString(row.Data, "session_id"), row.Key)
	itemID := fallback(valueString(row.Data, "item_id"), "legacy-unknown")
	itemName := fallback(valueString(row.Data, "item_name"), itemID)
	remoteUserID := valueString(row.Data, "emby_user_id")
	accountID, bindingID, err := legacyPlaybackOwner(ctx, tx, tgMap, row.Data, instanceID, remoteUserID)
	if err != nil {
		return adapterResult{}, err
	}
	id := deterministicUUID("sakura-v2-playback:" + row.Key)
	playbackKey := "legacy:" + row.Key
	_, err = tx.Exec(ctx, `INSERT INTO playback_history(id,instance_id,binding_id,account_id,remote_session_id,playback_key,remote_user_id,remote_username,item_id,item_name,item_type,series_name,client_name,device_name,device_id,remote_ip,transcoding,max_position_ticks,runtime_ticks,started_at,last_seen_at,ended_at,raw_snapshot)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17,$18,$19,$20,$21,$22,$23) ON CONFLICT(instance_id,playback_key) DO NOTHING`, id, instanceID, bindingID, accountID, remoteSession, playbackKey, remoteUserID, valueString(row.Data, "emby_user_name"), itemID, itemName, valueString(row.Data, "item_type"), valueString(row.Data, "series_name"), valueString(row.Data, "client_name"), valueString(row.Data, "device_name"), valueString(row.Data, "device_key"), valueString(row.Data, "remote_address"), valueBool(row.Data, "is_transcoding"), valueInt64(row.Data, "position_ticks"), valueInt64(row.Data, "runtime_ticks"), fallbackTime(row.Data, "started_at"), fallbackTime(row.Data, "last_seen_at"), ended, jsonBytes(row.Data))
	return transformed("playback_history", id.String()), err
}

func importKnownDeviceRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, instanceID uuid.UUID) (adapterResult, error) {
	if instanceID == uuid.Nil {
		return deferred("Emby instance is required for device profile"), nil
	}
	remoteUserID := valueString(row.Data, "emby_user_id")
	accountID, bindingID, err := legacyPlaybackOwner(ctx, tx, tgMap, row.Data, instanceID, remoteUserID)
	if err != nil {
		return adapterResult{}, err
	}
	decision := "unmatched"
	if valueBool(row.Data, "banned") {
		decision = "denied"
	} else if valueBool(row.Data, "trusted") {
		decision = "allowed"
	}
	id := deterministicUUID("sakura-v2-device:" + row.Key)
	metadata := mergeMetadata(row.Data, map[string]any{"source": "v2-known-device", "trusted": valueBool(row.Data, "trusted"), "banned": valueBool(row.Data, "banned"), "notes": valueString(row.Data, "notes")})
	_, err = tx.Exec(ctx, `INSERT INTO device_profiles(id,instance_id,binding_id,account_id,remote_user_id,device_key,device_id,device_name,client_name,first_ip,last_ip,session_count,access_decision,first_seen_at,last_seen_at,metadata)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($9,''),$10,$11,$12,$13,$14) ON CONFLICT(instance_id,remote_user_id,device_key) DO UPDATE SET binding_id=COALESCE(EXCLUDED.binding_id,device_profiles.binding_id),account_id=COALESCE(EXCLUDED.account_id,device_profiles.account_id),device_name=EXCLUDED.device_name,client_name=EXCLUDED.client_name,last_ip=EXCLUDED.last_ip,session_count=GREATEST(device_profiles.session_count,EXCLUDED.session_count),access_decision=EXCLUDED.access_decision,last_seen_at=GREATEST(device_profiles.last_seen_at,EXCLUDED.last_seen_at),metadata=EXCLUDED.metadata`, id, instanceID, bindingID, accountID, remoteUserID, row.Key, valueString(row.Data, "device_name"), valueString(row.Data, "client_name"), valueString(row.Data, "last_ip"), valueInt64(row.Data, "playback_count"), decision, fallbackTime(row.Data, "first_seen_at"), fallbackTime(row.Data, "last_seen_at"), jsonBytes(metadata))
	return transformed("device_profiles", id.String()), err
}

func legacyPlaybackOwner(ctx context.Context, tx pgTx, tgMap map[int64]uuid.UUID, data map[string]any, instanceID uuid.UUID, remoteUserID string) (any, any, error) {
	var account any
	if mapped, ok := tgMap[valueInt64(data, "tg")]; ok {
		account = mapped
	}
	var binding any
	if remoteUserID != "" {
		var bindingID, accountID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id,account_id FROM emby_account_bindings WHERE instance_id=$1 AND remote_user_id=$2 AND status<>'deleted' LIMIT 1`, instanceID, remoteUserID).Scan(&bindingID, &accountID)
		if err == nil {
			binding, account = bindingID, accountID
		} else if err != pgx.ErrNoRows {
			return nil, nil, err
		}
	}
	return account, binding, nil
}

func importDeviceRuleRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	id := deterministicUUID("sakura-v2-device-rule:" + row.Key)
	action := strings.ToLower(valueString(row.Data, "action"))
	decision, enforcement := "allow", "none"
	if action == "deny" || action == "block" || action == "blacklist" {
		decision, enforcement = "deny", "stop_session"
	}
	operator := normalizeMatchOperator(valueString(row.Data, "match_type"))
	_, err := tx.Exec(ctx, `INSERT INTO device_access_rules(id,name,description,decision,match_field,match_operator,match_value,action,observation_mode,enabled,built_in,priority,revision,created_by,created_at,updated_at)
		VALUES($1,$2,NULLIF($3,''),$4,'client_name',$5,$6,$7,FALSE,$8,$9,$10,$11,'system:legacy-import',$12,$13) ON CONFLICT(id) DO NOTHING`, id, fallback(valueString(row.Data, "name"), "Legacy device rule "+row.Key), valueString(row.Data, "notes"), decision, operator, valueString(row.Data, "pattern"), enforcement, valueBoolDefault(row.Data, "enabled", true), valueBool(row.Data, "built_in"), clampInt(valueInt(row.Data, "priority"), 0, 100000, 100), max(valueInt(row.Data, "revision"), 1), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	return transformed("device_access_rules", id.String()), err
}

func importSecurityEventRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID, instanceID uuid.UUID) (adapterResult, error) {
	if instanceID == uuid.Nil {
		return deferred("Emby instance is required for risk event"), nil
	}
	id := deterministicUUID("sakura-v2-security-event:" + row.Key)
	var account any
	if strings.EqualFold(valueString(row.Data, "subject_kind"), "telegram") || strings.EqualFold(valueString(row.Data, "subject_kind"), "account") {
		if mapped, ok := tgMap[parseInt64(valueString(row.Data, "subject_id"))]; ok {
			account = mapped
		}
	}
	status := normalizeRiskEventStatus(valueString(row.Data, "status"))
	severity := normalizeSeverity(valueString(row.Data, "severity"))
	detail := fallback(valueString(row.Data, "detail_json"), "{}")
	evidence := map[string]any{"source": "v2-security-event", "legacy": row.Data}
	if json.Valid([]byte(detail)) {
		var parsed any
		_ = json.Unmarshal([]byte(detail), &parsed)
		evidence["detail"] = parsed
	}
	title := fallback(valueString(row.Data, "event_type"), "Legacy security event")
	reason := title
	if ip := valueString(row.Data, "ip_address"); ip != "" {
		reason += " from " + ip
	}
	_, err := tx.Exec(ctx, `INSERT INTO risk_events(id,instance_id,account_id,dedupe_key,source,severity,title,reason,evidence,observation_mode,recommended_action,status,disposition_reason,disposition_by,disposition_at,created_at,updated_at)
		VALUES($1,$2,$3,$4,'manual',$5,$6,$7,$8,TRUE,'none',$9,NULLIF($10,''),NULLIF($11,''),$12,$13,$14) ON CONFLICT(dedupe_key) DO NOTHING`, id, instanceID, account, "v2-security-event:"+row.Key, severity, title, reason, jsonBytes(evidence), status, valueString(row.Data, "resolution_note"), legacyActor(row.Data, "resolved_by"), nullableTime(row.Data, "resolved_at"), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	return transformed("risk_events", id.String()), err
}

func importRiskRuleRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	id := deterministicUUID("sakura-v2-risk-rule:" + row.Key)
	code := "v2-risk-" + safeCode(row.Key, 60)
	condition := map[string]any{"event_pattern": valueString(row.Data, "event_pattern"), "threshold_count": clampInt(valueInt(row.Data, "threshold_count"), 1, 100000, 1), "window_minutes": clampInt(valueInt(row.Data, "window_minutes"), 1, 10080, 10), "telegram_alert": valueBoolDefault(row.Data, "telegram_alert", true)}
	_, err := tx.Exec(ctx, `INSERT INTO risk_rules(id,code,name,description,rule_type,condition,severity,action,observation_mode,enabled,cooldown_seconds,revision,created_by,created_at,updated_at)
		VALUES($1,$2,$3,'Imported from v2 risk rule','custom',$4,$5,'none',TRUE,$6,$7,$8,'system:legacy-import',$9,$10) ON CONFLICT(id) DO NOTHING`, id, code, fallback(valueString(row.Data, "name"), code), jsonBytes(condition), normalizeSeverity(valueString(row.Data, "severity")), valueBoolDefault(row.Data, "enabled", true), clampInt(valueInt(row.Data, "cooldown_minutes")*60, 0, 604800, 1800), max(valueInt(row.Data, "revision"), 1), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	return transformed("risk_rules", id.String()), err
}

func importMediaRequestRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID) (adapterResult, error) {
	accountID, ok := tgMap[valueInt64(row.Data, "tg")]
	if !ok {
		return deferred("request owner account mapping is missing"), nil
	}
	title := valueString(row.Data, "title")
	mediaType := valueString(row.Data, "media_type")
	status := valueString(row.Data, "status")
	note := valueString(row.Data, "description")
	requestNo := valueString(row.Data, "request_no")
	downloadID := valueString(row.Data, "download_id")
	if row.Table == "request_records" {
		title = valueString(row.Data, "request_name")
		mediaType = "movie"
		status = valueString(row.Data, "download_state")
		note = valueString(row.Data, "detail")
		requestNo = "V2MP-" + safeCode(row.Key, 32)
		downloadID = row.Key
	}
	title = fallback(title, "Legacy media request "+row.Key)
	mediaID, err := ensureLegacyMedia(ctx, tx, row.Table+":"+row.Key, title, mediaType, valueInt(row.Data, "year"), row.Data)
	if err != nil {
		return adapterResult{}, err
	}
	requestID := parseOrDeterministicUUID(valueString(row.Data, "id"), "sakura-v2-media-request:"+row.Table+":")
	if row.Table == "request_records" {
		requestID = deterministicUUID("sakura-v2-request-record:" + row.Key)
	}
	requestNo = fallback(requestNo, "V2REQ-"+safeCode(row.Key, 30))
	targetStatus := normalizeMediaRequestStatus(status)
	priority := normalizeRequestPriority(row.Data["priority"])
	_, err = tx.Exec(ctx, `INSERT INTO media_requests(id,request_no,media_id,requested_by,status,priority,note,subscriber_count,resolution_reason,resolved_by,resolved_at,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),1,NULLIF($8,''),NULLIF($9,''),$10,$11,$12)
		ON CONFLICT DO NOTHING`, requestID, requestNo, mediaID, accountID, targetStatus, priority, note, valueString(row.Data, "admin_note"), legacyActor(row.Data, "reviewed_by"), terminalTime(row.Data, targetStatus), fallbackTimeAny(row.Data, []string{"created_at", "create_at"}), fallbackTimeAny(row.Data, []string{"updated_at", "update_at"}))
	if err != nil {
		return adapterResult{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM media_requests WHERE id=$1 OR request_no=$2 OR (media_id=$3 AND status IN ('requested','approved','queued','downloading')) ORDER BY CASE WHEN id=$1 THEN 0 WHEN request_no=$2 THEN 1 ELSE 2 END LIMIT 1`, requestID, requestNo, mediaID).Scan(&requestID); err != nil {
		return adapterResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_request_subscriptions(request_id,account_id,note,created_at) VALUES($1,$2,NULLIF($3,''),$4) ON CONFLICT DO NOTHING`, requestID, accountID, note, fallbackTimeAny(row.Data, []string{"created_at", "create_at"}))
	if err != nil {
		return adapterResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,to_status,actor,reason,details,created_at)
		SELECT $1,'legacy_import',$2,'system:legacy-import','Imported from v2',$3,$4 WHERE NOT EXISTS(SELECT 1 FROM media_request_events WHERE request_id=$1 AND event_type='legacy_import' AND details->>'source_table'=$5)`, requestID, targetStatus, jsonBytes(map[string]any{"source_table": row.Table, "source_key": row.Key, "download_id": downloadID}), fallbackTimeAny(row.Data, []string{"created_at", "create_at"}), row.Table)
	if err != nil {
		return adapterResult{}, err
	}
	if downloadID != "" {
		jobID := deterministicUUID("sakura-v2-moviepilot:" + downloadID)
		jobStatus := normalizeMoviePilotStatus(status)
		_, err = tx.Exec(ctx, `INSERT INTO moviepilot_jobs(id,media_id,request_id,idempotency_key,status,external_job_id,payload,result,attempts,created_by,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,0,'system:legacy-import',$9,$10) ON CONFLICT DO NOTHING`, jobID, mediaID, requestID, "legacy-moviepilot:"+downloadID, jobStatus, downloadID, jsonBytes(map[string]any{"source": row.Table}), jsonBytes(row.Data), fallbackTimeAny(row.Data, []string{"created_at", "create_at"}), fallbackTimeAny(row.Data, []string{"updated_at", "update_at"}))
		if err != nil {
			return adapterResult{}, err
		}
	}
	return transformed("media_requests", requestID.String()), nil
}

func ensureLegacyMedia(ctx context.Context, tx pgTx, key, title, mediaType string, year int, payload map[string]any) (uuid.UUID, error) {
	mediaType = normalizeMediaType(mediaType)
	externalID := stablePositiveInt64(key)
	id := deterministicUUID("sakura-v2-media:" + key)
	var release any
	if year >= 1800 && year <= 9999 {
		release = fmt.Sprintf("%04d-01-01", year)
	}
	_, err := tx.Exec(ctx, `INSERT INTO media_catalog(id,provider,external_id,media_type,title,release_date,metadata,last_refreshed_at,created_at,updated_at)
		VALUES($1,'legacy-v2',$2,$3,$4,$5,$6,NOW(),NOW(),NOW()) ON CONFLICT(provider,external_id,media_type) DO NOTHING`, id, externalID, mediaType, title, release, jsonBytes(mergeMetadata(payload, map[string]any{"source": "v2"})))
	if err != nil {
		return uuid.Nil, err
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM media_catalog WHERE provider='legacy-v2' AND external_id=$1 AND media_type=$2`, externalID, mediaType).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func importMediaReviewRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID) (adapterResult, error) {
	accountID, ok := tgMap[valueInt64(row.Data, "tg")]
	if !ok {
		return deferred("review author account mapping is missing"), nil
	}
	mediaKey := fallback(valueString(row.Data, "media_key"), row.Key)
	mediaID, err := ensureLegacyMedia(ctx, tx, "review:"+mediaKey, fallback(valueString(row.Data, "media_title"), mediaKey), "movie", valueInt(row.Data, "media_year"), row.Data)
	if err != nil {
		return adapterResult{}, err
	}
	id := parseOrDeterministicUUID(valueString(row.Data, "id"), "sakura-v2-review:")
	status := normalizeReviewStatus(valueString(row.Data, "status"))
	_, err = tx.Exec(ctx, `INSERT INTO media_reviews(id,media_id,account_id,rating,body,contains_spoilers,status,moderation_reason,moderated_by,moderated_at,revision,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,1,$11,$12) ON CONFLICT DO NOTHING`, id, mediaID, accountID, clampInt(valueInt(row.Data, "rating"), 1, 10, 1), fallback(valueString(row.Data, "content"), "Legacy review"), valueBool(row.Data, "spoiler"), status, valueString(row.Data, "admin_note"), legacyActor(row.Data, "moderated_by"), nullableTime(row.Data, "moderated_at"), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT id FROM media_reviews WHERE id=$1 OR (media_id=$2 AND account_id=$3) LIMIT 1`, id, mediaID, accountID).Scan(&id)
	}
	return transformed("media_reviews", id.String()), err
}

func importReviewReactionRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID) (adapterResult, error) {
	accountID, ok := tgMap[valueInt64(row.Data, "tg")]
	if !ok {
		return deferred("reaction account mapping is missing"), nil
	}
	reviewID := parseOrDeterministicUUID(valueString(row.Data, "review_id"), "sakura-v2-review:")
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_reviews WHERE id=$1)`, reviewID).Scan(&exists); err != nil {
		return adapterResult{}, err
	}
	if !exists {
		return deferred("reaction review mapping is missing"), nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO review_reactions(review_id,account_id,reaction,created_at) VALUES($1,$2,'like',$3) ON CONFLICT DO NOTHING`, reviewID, accountID, fallbackTime(row.Data, "created_at"))
	return transformed("review_reactions", reviewID.String()+":"+accountID.String()), err
}

func importReviewReportRow(ctx context.Context, tx pgTx, row adapterRow, tgMap map[int64]uuid.UUID) (adapterResult, error) {
	accountID, ok := tgMap[valueInt64(row.Data, "tg")]
	if !ok {
		return deferred("reporter account mapping is missing"), nil
	}
	reviewID := parseOrDeterministicUUID(valueString(row.Data, "review_id"), "sakura-v2-review:")
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_reviews WHERE id=$1)`, reviewID).Scan(&exists); err != nil {
		return adapterResult{}, err
	}
	if !exists {
		return deferred("reported review mapping is missing"), nil
	}
	id := deterministicUUID("sakura-v2-review-report:" + row.Key)
	_, err := tx.Exec(ctx, `INSERT INTO review_reports(id,review_id,reporter_account_id,reason,detail,created_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) ON CONFLICT(review_id,reporter_account_id) DO NOTHING`, id, reviewID, accountID, fallback(valueString(row.Data, "reason"), "other"), valueString(row.Data, "detail"), fallbackTime(row.Data, "created_at"))
	return transformed("review_reports", id.String()), err
}

func importAutomationRuleRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	id := parseOrDeterministicUUID(valueString(row.Data, "id"), "sakura-v2-automation-rule:")
	conditions := validJSONOr(row.Data["conditions_json"], map[string]any{})
	actions := validJSONOr(row.Data["actions_json"], []any{})
	triggerEvent := fallback(valueString(row.Data, "trigger_value"), valueString(row.Data, "trigger_type"))
	triggerEvent = fallback(triggerEvent, "legacy.event")
	code := "v2-auto-" + safeCode(row.Key, 65)
	_, err := tx.Exec(ctx, `INSERT INTO automation_rules(id,code,name,description,trigger_event,conditions,actions,enabled,priority,revision,created_by,created_at,updated_at)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,FALSE,100,$8,'system:legacy-import',$9,$10) ON CONFLICT DO NOTHING`, id, code, fallback(valueString(row.Data, "name"), code), valueString(row.Data, "description"), triggerEvent, conditions, actions, max(valueInt(row.Data, "revision"), 1), fallbackTime(row.Data, "created_at"), fallbackTime(row.Data, "updated_at"))
	return transformed("automation_rules", id.String()), err
}

func importConfigRevisionRow(ctx context.Context, tx pgTx, row adapterRow) (adapterResult, error) {
	key := valueString(row.Data, "setting_key")
	if key == "" {
		return deferred("setting key is empty"), nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dynamic_settings WHERE key=$1)`, key).Scan(&exists); err != nil {
		return adapterResult{}, err
	}
	if !exists {
		return deferred("setting no longer exists in v3"), nil
	}
	raw := []byte(fallback(valueString(row.Data, "new_value_json"), "null"))
	if !json.Valid(raw) {
		raw, _ = json.Marshal(string(raw))
	}
	revision := max(valueInt(row.Data, "revision"), 1)
	valueType := normalizeSettingType("", raw)
	_, err := tx.Exec(ctx, `INSERT INTO setting_revisions(setting_key,revision,value,value_type,actor,reason,created_at) VALUES($1,$2,$3,$4,$5,'Imported v2 revision history',$6) ON CONFLICT(setting_key,revision) DO NOTHING`, key, revision, raw, valueType, legacyActorPair(row.Data, "actor_kind", "actor_id"), fallbackTime(row.Data, "created_at"))
	return transformed("setting_revisions", key+":"+strconv.Itoa(revision)), err
}

func importAPIClientRow(ctx context.Context, tx pgTx, row adapterRow, idMap map[string]uuid.UUID) (adapterResult, error) {
	id := parseOrDeterministicUUID(valueString(row.Data, "id"), "sakura-v2-api-client:")
	prefix := fallback(valueString(row.Data, "key_prefix"), "retired")
	if len(prefix) > 20 {
		prefix = prefix[:20]
	}
	scopes := stringSlice(validJSONOr(row.Data["scopes_json"], []any{}))
	var createdBy any
	if mapped, ok := idMap[valueString(row.Data, "created_by")]; ok {
		createdBy = mapped
	}
	retiredHash := security.HashToken("retired-v2-api-client:" + row.Key + ":" + valueString(row.Data, "key_hash"))
	_, err := tx.Exec(ctx, `INSERT INTO api_clients(id,name,token_prefix,token_hash,scopes,active,expires_at,last_used_at,created_by,created_at,revoked_at)
		VALUES($1,$2,$3,$4,$5,FALSE,$6,$7,$8,$9,NOW()) ON CONFLICT(id) DO NOTHING`, id, fallback(valueString(row.Data, "name"), "Legacy API client "+row.Key), prefix, retiredHash, scopes, nullableTime(row.Data, "expires_at"), nullableTime(row.Data, "last_used_at"), createdBy, fallbackTime(row.Data, "created_at"))
	return transformed("api_clients", id.String()), err
}

func transformed(table, key string) adapterResult {
	return adapterResult{disposition: "transformed", targetTable: table, targetKey: key}
}

func deferred(detail string) adapterResult {
	return adapterResult{disposition: "deferred", detail: detail}
}

func valueString(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func valueInt(data map[string]any, key string) int {
	return int(valueInt64(data, key))
}

func valueInt64(data map[string]any, key string) int64 {
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int32:
		return int64(typed)
	case int:
		return int64(typed)
	case uint64:
		if typed <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	case float64:
		return int64(typed)
	case []byte:
		parsed, _ := strconv.ParseInt(string(typed), 10, 64)
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	}
	return 0
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func valueBool(data map[string]any, key string) bool {
	return valueBoolDefault(data, key, false)
}

func valueBoolDefault(data map[string]any, key string, fallbackValue bool) bool {
	value, ok := data[key]
	if !ok || value == nil {
		return fallbackValue
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case int64:
		return typed != 0
	case uint64:
		return typed != 0
	case []byte:
		parsed, err := strconv.ParseBool(string(typed))
		if err == nil {
			return parsed
		}
		return string(typed) == "1"
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
		return typed == "1"
	}
	return fallbackValue
}

func sourceTime(data map[string]any, key string) (time.Time, bool) {
	value, ok := data[key]
	if !ok || value == nil {
		return time.Time{}, false
	}
	if typed, ok := value.(time.Time); ok {
		return typed, true
	}
	text := valueString(data, key)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func fallbackTime(data map[string]any, key string) time.Time {
	if value, ok := sourceTime(data, key); ok {
		return value
	}
	return time.Unix(0, 0).UTC()
}

func fallbackTimeAny(data map[string]any, keys []string) time.Time {
	for _, key := range keys {
		if value, ok := sourceTime(data, key); ok {
			return value
		}
	}
	return time.Unix(0, 0).UTC()
}

func nullableTime(data map[string]any, key string) any {
	if value, ok := sourceTime(data, key); ok {
		return value
	}
	return nil
}

func nullableInt(data map[string]any, key string) any {
	if _, ok := data[key]; !ok || data[key] == nil || valueString(data, key) == "" {
		return nil
	}
	return valueInt(data, key)
}

func clampInt(value, minimum, maximum, fallbackValue int) int {
	if value == 0 && fallbackValue != 0 {
		value = fallbackValue
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func mergeMetadata(source map[string]any, additions map[string]any) map[string]any {
	result := make(map[string]any, len(source)+len(additions))
	for key, value := range source {
		result[key] = value
	}
	for key, value := range additions {
		result[key] = value
	}
	return result
}

func jsonBytes(value any) []byte {
	body, _ := json.Marshal(value)
	return body
}

func normalizeEntitlementCodeStatus(value string) string {
	switch strings.ToLower(value) {
	case "reserved":
		return "reserved"
	case "used", "redeemed":
		return "redeemed"
	case "expired":
		return "expired"
	case "revoked", "disabled":
		return "revoked"
	default:
		return "available"
	}
}

func normalizeEntitlementStatus(value string, expires time.Time) string {
	if expires.Before(time.Now()) || strings.EqualFold(value, "expired") {
		return "expired"
	}
	if strings.EqualFold(value, "revoked") || strings.EqualFold(value, "disabled") {
		return "revoked"
	}
	return "active"
}

func normalizeMatchOperator(value string) string {
	switch strings.ToLower(value) {
	case "exact", "equals":
		return "exact"
	case "prefix", "startswith":
		return "prefix"
	case "regex", "regexp":
		return "regex"
	default:
		return "contains"
	}
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(value) {
	case "critical", "fatal":
		return "critical"
	case "high", "error":
		return "high"
	case "medium", "warning", "warn":
		return "medium"
	default:
		return "low"
	}
}

func normalizeRiskEventStatus(value string) string {
	switch strings.ToLower(value) {
	case "acknowledged", "assigned", "processing":
		return "acknowledged"
	case "resolved", "closed":
		return "resolved"
	case "false_positive", "dismissed":
		return "false_positive"
	default:
		return "open"
	}
}

func normalizeMediaType(value string) string {
	switch strings.ToLower(value) {
	case "tv", "series", "show", "tvshow":
		return "tv"
	default:
		return "movie"
	}
}

func normalizeMediaRequestStatus(value string) string {
	switch strings.ToLower(value) {
	case "approved":
		return "approved"
	case "queued", "pending_download":
		return "queued"
	case "downloading", "transfer", "transferring":
		return "downloading"
	case "completed", "success", "transferred":
		return "completed"
	case "rejected", "failed":
		return "rejected"
	case "canceled", "cancelled":
		return "canceled"
	default:
		return "requested"
	}
}

func normalizeMoviePilotStatus(value string) string {
	switch normalizeMediaRequestStatus(value) {
	case "downloading":
		return "downloading"
	case "completed":
		return "completed"
	case "canceled":
		return "canceled"
	case "rejected":
		return "failed"
	default:
		return "submitted"
	}
}

func normalizeRequestPriority(value any) int {
	text := strings.ToLower(fmt.Sprint(value))
	switch text {
	case "urgent":
		return 900
	case "high":
		return 700
	case "low":
		return 100
	case "normal", "", "<nil>":
		return 500
	}
	parsed, err := strconv.Atoi(text)
	if err != nil {
		return 500
	}
	return clampInt(parsed, 0, 1000, 500)
}

func terminalTime(data map[string]any, status string) any {
	if status != "completed" && status != "rejected" && status != "canceled" {
		return nil
	}
	for _, key := range []string{"completed_at", "canceled_at", "reviewed_at", "updated_at", "update_at"} {
		if value, ok := sourceTime(data, key); ok {
			return value
		}
	}
	return time.Unix(0, 0).UTC()
}

func normalizeReviewStatus(value string) string {
	switch strings.ToLower(value) {
	case "approved", "published":
		return "approved"
	case "rejected":
		return "rejected"
	case "hidden":
		return "hidden"
	default:
		return "pending"
	}
}

func stablePositiveInt64(value string) int64 {
	digest := sha256.Sum256([]byte(value))
	return int64(binary.BigEndian.Uint64(digest[:8]) & 0x7fffffffffffffff)
}

func safeCode(value string, limit int) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		result = hex.EncodeToString(security.HashToken(value))[:16]
	}
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func validJSONOr(value any, fallbackValue any) []byte {
	text := ""
	switch typed := value.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	}
	if json.Valid([]byte(text)) {
		return []byte(text)
	}
	return jsonBytes(fallbackValue)
}

func stringSlice(raw []byte) []string {
	var values []string
	if json.Unmarshal(raw, &values) == nil {
		return values
	}
	return []string{}
}

func legacyActor(data map[string]any, key string) string {
	value := valueString(data, key)
	if value == "" || value == "0" {
		return "system:legacy-import"
	}
	return "legacy:" + value
}

func legacyActorPair(data map[string]any, kindKey, idKey string) string {
	kind, id := valueString(data, kindKey), valueString(data, idKey)
	if kind == "" {
		kind = "system"
	}
	if id == "" {
		id = "legacy-import"
	}
	return kind + ":" + id
}
