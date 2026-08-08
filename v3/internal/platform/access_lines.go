package platform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func entitlementDigest(code string) [32]byte {
	return sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
}

func (s *Service) GenerateEntitlementCodes(ctx context.Context, instanceID *uuid.UUID, resourceKind, resourceKey string, durationDays, count int, metadata map[string]any, actor identity.Actor) ([]EntitlementCode, error) {
	resourceKind, resourceKey = normalize(resourceKind), strings.TrimSpace(resourceKey)
	if resourceKind != "emby_library" || resourceKey == "" || len(resourceKey) > 160 || durationDays < 1 || durationDays > 3650 || count < 1 || count > 200 {
		return nil, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if instanceID != nil {
		var enabled bool
		if err = tx.QueryRow(ctx, `SELECT enabled FROM emby_instances WHERE id=$1`, *instanceID).Scan(&enabled); err != nil {
			return nil, notFound(err)
		}
	}
	items := make([]EntitlementCode, 0, count)
	for index := 0; index < count; index++ {
		raw, randomErr := randomAlphaNumeric(24)
		if randomErr != nil {
			return nil, randomErr
		}
		code := "ENT-" + strings.ToUpper(raw[:8]) + "-" + strings.ToUpper(raw[8:])
		digest := entitlementDigest(code)
		item := EntitlementCode{ID: uuid.New(), Code: code, CodeHint: code[:8] + "…" + code[len(code)-4:], InstanceID: instanceID, ResourceKind: resourceKind, ResourceKey: resourceKey, DurationDays: durationDays, Status: "available", IssuedBy: actor.Label(), Metadata: metadata}
		_, err = tx.Exec(ctx, `INSERT INTO entitlement_codes(id,code_hash,code_prefix,code_hint,instance_id,resource_kind,resource_key,duration_days,issued_by,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, item.ID, digest[:], code[:8], item.CodeHint, uuidQueryValue(instanceID), resourceKind, resourceKey, durationDays, item.IssuedBy, jsonBytes(metadata))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = audit(ctx, tx, actor, "entitlement_code.generate", "entitlement_code", "", map[string]any{"count": count, "instance_id": instanceID, "resource_kind": resourceKind, "resource_key": resourceKey, "duration_days": durationDays}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) ListEntitlementCodes(ctx context.Context, status string, limit int) ([]EntitlementCode, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,code_hint,instance_id,resource_kind,resource_key,duration_days,status,issued_by,metadata,created_at,updated_at FROM entitlement_codes WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT $2`, normalize(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EntitlementCode
	for rows.Next() {
		var item EntitlementCode
		var raw []byte
		if err = rows.Scan(&item.ID, &item.CodeHint, &item.InstanceID, &item.ResourceKind, &item.ResourceKey, &item.DurationDays, &item.Status, &item.IssuedBy, &raw, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListAccountEntitlements(ctx context.Context, accountID *uuid.UUID, status string, limit int) ([]AccountEntitlement, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,account_id,instance_id,binding_id,resource_kind,resource_key,status,source_code_id,starts_at,expires_at,metadata,created_at,updated_at FROM account_entitlements WHERE ($1::uuid IS NULL OR account_id=$1) AND ($2='' OR status=$2) ORDER BY expires_at DESC LIMIT $3`, uuidQueryValue(accountID), normalize(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountEntitlement
	for rows.Next() {
		var item AccountEntitlement
		var raw []byte
		if err = rows.Scan(&item.ID, &item.AccountID, &item.InstanceID, &item.BindingID, &item.ResourceKind, &item.ResourceKey, &item.Status, &item.SourceCodeID, &item.StartsAt, &item.ExpiresAt, &raw, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) RedeemEntitlementCode(ctx context.Context, accountID uuid.UUID, code string, actor identity.Actor) (AccountEntitlement, error) {
	digest := entitlementDigest(code)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountEntitlement{}, err
	}
	defer tx.Rollback(ctx)
	var codeID uuid.UUID
	var instanceID *uuid.UUID
	var kind, key, status string
	var days int
	if err = tx.QueryRow(ctx, `SELECT id,instance_id,resource_kind,resource_key,duration_days,status FROM entitlement_codes WHERE code_hash=$1 FOR UPDATE`, digest[:]).Scan(&codeID, &instanceID, &kind, &key, &days, &status); err != nil {
		return AccountEntitlement{}, notFound(err)
	}
	if status != "available" {
		return AccountEntitlement{}, identity.ErrConflict
	}
	var bindingID *uuid.UUID
	if instanceID != nil {
		var id uuid.UUID
		if scanErr := tx.QueryRow(ctx, `SELECT id FROM emby_account_bindings WHERE account_id=$1 AND instance_id=$2 AND status<>'deleted' ORDER BY is_primary DESC,created_at LIMIT 1`, accountID, *instanceID).Scan(&id); scanErr == nil {
			bindingID = &id
		} else if !errors.Is(scanErr, pgx.ErrNoRows) {
			return AccountEntitlement{}, scanErr
		}
	} else {
		var id, selectedInstance uuid.UUID
		if scanErr := tx.QueryRow(ctx, `SELECT id,instance_id FROM emby_account_bindings WHERE account_id=$1 AND status<>'deleted' ORDER BY is_primary DESC,created_at LIMIT 1`, accountID).Scan(&id, &selectedInstance); scanErr == nil {
			bindingID, instanceID = &id, &selectedInstance
		} else if !errors.Is(scanErr, pgx.ErrNoRows) {
			return AccountEntitlement{}, scanErr
		}
	}
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO account_entitlements(id,account_id,instance_id,binding_id,resource_kind,resource_key,source_code_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,NOW()+($8::double precision*INTERVAL '1 day')) ON CONFLICT(account_id,instance_id,resource_kind,resource_key) DO UPDATE SET status='active',binding_id=COALESCE(EXCLUDED.binding_id,account_entitlements.binding_id),source_code_id=EXCLUDED.source_code_id,expires_at=GREATEST(account_entitlements.expires_at,NOW())+($8::double precision*INTERVAL '1 day'),updated_at=NOW() RETURNING id`, id, accountID, uuidQueryValue(instanceID), uuidQueryValue(bindingID), kind, key, codeID, days).Scan(&id)
	if err != nil {
		return AccountEntitlement{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE entitlement_codes SET status='redeemed',reserved_by=$2,updated_at=NOW() WHERE id=$1`, codeID, accountID); err != nil {
		return AccountEntitlement{}, err
	}
	if bindingID != nil {
		if err = enqueueEntitlementSyncTx(ctx, tx, *bindingID, "redeem:"+codeID.String(), actor.Label()); err != nil {
			return AccountEntitlement{}, err
		}
	}
	if err = audit(ctx, tx, actor, "entitlement.redeem", "account_entitlement", id.String(), map[string]any{"code_id": codeID, "resource_key": key, "instance_id": instanceID}); err != nil {
		return AccountEntitlement{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountEntitlement{}, err
	}
	return s.GetAccountEntitlement(ctx, id)
}

func (s *Service) GrantEntitlement(ctx context.Context, accountID uuid.UUID, instanceID *uuid.UUID, resourceKind, resourceKey string, durationDays int, actor identity.Actor) (AccountEntitlement, error) {
	resourceKind, resourceKey = normalize(resourceKind), strings.TrimSpace(resourceKey)
	if resourceKind != "emby_library" || resourceKey == "" || durationDays < 1 || durationDays > 3650 {
		return AccountEntitlement{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountEntitlement{}, err
	}
	defer tx.Rollback(ctx)
	var bindingID *uuid.UUID
	if instanceID != nil {
		var id uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT id FROM emby_account_bindings WHERE account_id=$1 AND instance_id=$2 AND status<>'deleted' ORDER BY is_primary DESC,created_at LIMIT 1`, accountID, *instanceID).Scan(&id); err != nil {
			return AccountEntitlement{}, notFound(err)
		}
		bindingID = &id
	} else {
		var id, selectedInstance uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT id,instance_id FROM emby_account_bindings WHERE account_id=$1 AND status<>'deleted' ORDER BY is_primary DESC,created_at LIMIT 1`, accountID).Scan(&id, &selectedInstance); err != nil {
			return AccountEntitlement{}, notFound(err)
		}
		bindingID, instanceID = &id, &selectedInstance
	}
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO account_entitlements(id,account_id,instance_id,binding_id,resource_kind,resource_key,expires_at) VALUES($1,$2,$3,$4,$5,$6,NOW()+($7::double precision*INTERVAL '1 day')) ON CONFLICT(account_id,instance_id,resource_kind,resource_key) DO UPDATE SET status='active',binding_id=EXCLUDED.binding_id,expires_at=GREATEST(account_entitlements.expires_at,NOW())+($7::double precision*INTERVAL '1 day'),updated_at=NOW() RETURNING id`, id, accountID, uuidQueryValue(instanceID), uuidQueryValue(bindingID), resourceKind, resourceKey, durationDays).Scan(&id)
	if err != nil {
		return AccountEntitlement{}, err
	}
	if bindingID != nil {
		if err = enqueueEntitlementSyncTx(ctx, tx, *bindingID, "grant:"+id.String()+":"+fmt.Sprint(time.Now().UnixNano()), actor.Label()); err != nil {
			return AccountEntitlement{}, err
		}
	}
	if err = audit(ctx, tx, actor, "entitlement.grant", "account_entitlement", id.String(), map[string]any{"account_id": accountID, "instance_id": instanceID, "resource_key": resourceKey, "duration_days": durationDays}); err != nil {
		return AccountEntitlement{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountEntitlement{}, err
	}
	return s.GetAccountEntitlement(ctx, id)
}

func (s *Service) RevokeEntitlement(ctx context.Context, id uuid.UUID, reason string, actor identity.Actor) (AccountEntitlement, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 500 {
		return AccountEntitlement{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AccountEntitlement{}, err
	}
	defer tx.Rollback(ctx)
	var bindingID *uuid.UUID
	if err = tx.QueryRow(ctx, `UPDATE account_entitlements SET status='revoked',metadata=metadata||jsonb_build_object('revocation_reason',$2::text),updated_at=NOW() WHERE id=$1 AND status<>'revoked' RETURNING binding_id`, id, reason).Scan(&bindingID); err != nil {
		return AccountEntitlement{}, notFound(err)
	}
	if bindingID != nil {
		if err = enqueueEntitlementSyncTx(ctx, tx, *bindingID, "revoke:"+id.String(), actor.Label()); err != nil {
			return AccountEntitlement{}, err
		}
	}
	if err = audit(ctx, tx, actor, "entitlement.revoke", "account_entitlement", id.String(), map[string]any{"reason": reason}); err != nil {
		return AccountEntitlement{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountEntitlement{}, err
	}
	return s.GetAccountEntitlement(ctx, id)
}

func enqueueEntitlementSyncTx(ctx context.Context, tx pgx.Tx, bindingID uuid.UUID, key, createdBy string) error {
	var instanceID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT instance_id FROM emby_account_bindings WHERE id=$1`, bindingID).Scan(&instanceID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by) VALUES($1,'entitlement.sync',$2,$3,$4) ON CONFLICT(idempotency_key) DO NOTHING`, uuid.New(), "entitlement-sync:"+key, jsonBytes(map[string]any{"instance_id": instanceID.String(), "binding_id": bindingID.String()}), createdBy)
	return err
}

func (s *Service) GetAccountEntitlement(ctx context.Context, id uuid.UUID) (AccountEntitlement, error) {
	items, err := s.ListAccountEntitlements(ctx, nil, "", 500)
	if err != nil {
		return AccountEntitlement{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return AccountEntitlement{}, identity.ErrNotFound
}

func (s *Service) ListLines(ctx context.Context, publicOnly bool) ([]LineEndpoint, error) {
	where := ""
	if publicOnly {
		where = "WHERE enabled AND NOT maintenance"
	}
	rows, err := s.db.Query(ctx, `SELECT id,name,base_url,COALESCE(region,''),COALESCE(carrier,''),audience,weight,sort_order,enabled,maintenance,revision,last_status,last_latency_ms,COALESCE(last_error,''),last_checked_at,metadata,created_at,updated_at FROM line_endpoints `+where+` ORDER BY sort_order,weight DESC,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LineEndpoint
	for rows.Next() {
		var item LineEndpoint
		var raw []byte
		if err = rows.Scan(&item.ID, &item.Name, &item.BaseURL, &item.Region, &item.Carrier, &item.Audience, &item.Weight, &item.SortOrder, &item.Enabled, &item.Maintenance, &item.Revision, &item.LastStatus, &item.LastLatencyMS, &item.LastError, &item.LastCheckedAt, &raw, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Metadata = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListAvailableLines(ctx context.Context, accountID uuid.UUID) ([]LineEndpoint, error) {
	var member bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_memberships WHERE account_id=$1 AND status IN ('active','grace') AND expires_at>NOW())`, accountID).Scan(&member); err != nil {
		return nil, err
	}
	items, err := s.ListLines(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]LineEndpoint, 0, len(items))
	for _, item := range items {
		if item.Audience == "all" || item.Audience == "member" && member {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Service) SaveLine(ctx context.Context, id *uuid.UUID, name, baseURL, region, carrier, audience string, weight, sortOrder int, enabled, maintenance bool, expected int64, actor identity.Actor) (LineEndpoint, error) {
	name, baseURL, region, carrier, audience = strings.TrimSpace(name), strings.TrimRight(strings.TrimSpace(baseURL), "/"), strings.TrimSpace(region), strings.TrimSpace(carrier), normalize(audience)
	parsed, err := url.Parse(baseURL)
	if err != nil || name == "" || len(name) > 120 || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || !contains([]string{"all", "member", "admin"}, audience) || weight < 0 || weight > 100000 {
		return LineEndpoint{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LineEndpoint{}, err
	}
	defer tx.Rollback(ctx)
	lineID := uuid.New()
	action := "line.create"
	if id != nil {
		lineID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM line_endpoints WHERE id=$1 FOR UPDATE`, lineID).Scan(&revision); err != nil {
			return LineEndpoint{}, notFound(err)
		}
		if revision != expected {
			return LineEndpoint{}, identity.ErrConflict
		}
		action = "line.update"
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO line_endpoints(id,name,base_url,region,carrier,audience,weight,sort_order,enabled,maintenance) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10)`, lineID, name, baseURL, region, carrier, audience, weight, sortOrder, enabled, maintenance)
	} else {
		_, err = tx.Exec(ctx, `UPDATE line_endpoints SET name=$2,base_url=$3,region=NULLIF($4,''),carrier=NULLIF($5,''),audience=$6,weight=$7,sort_order=$8,enabled=$9,maintenance=$10,revision=revision+1,updated_at=NOW() WHERE id=$1`, lineID, name, baseURL, region, carrier, audience, weight, sortOrder, enabled, maintenance)
	}
	if err != nil {
		return LineEndpoint{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "line_endpoint", lineID.String(), map[string]any{"name": name, "base_url": baseURL, "enabled": enabled, "maintenance": maintenance}); err != nil {
		return LineEndpoint{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LineEndpoint{}, err
	}
	return s.GetLine(ctx, lineID)
}

func (s *Service) GetLine(ctx context.Context, id uuid.UUID) (LineEndpoint, error) {
	items, err := s.ListLines(ctx, false)
	if err != nil {
		return LineEndpoint{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return LineEndpoint{}, identity.ErrNotFound
}

func (s *Service) ProbeLine(ctx context.Context, id uuid.UUID, actor identity.Actor) (LineProbeSample, error) {
	line, err := s.GetLine(ctx, id)
	if err != nil {
		return LineProbeSample{}, err
	}
	timeout := s.dynamicInt(ctx, "lines.probe_timeout_seconds", 10)
	if timeout < 1 || timeout > 60 {
		timeout = 10
	}
	endpoint := lineProbeEndpoint(line.BaseURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return LineProbeSample{}, identity.ErrInvalid
	}
	started := time.Now()
	response, probeErr := (&http.Client{Timeout: time.Duration(timeout) * time.Second}).Do(request)
	latency := int(time.Since(started).Milliseconds())
	status := "healthy"
	var httpStatus *int
	message := ""
	if response != nil {
		code := response.StatusCode
		httpStatus = &code
		response.Body.Close()
	}
	if probeErr != nil || response == nil || response.StatusCode < 200 || response.StatusCode >= 400 {
		status = "unhealthy"
		if probeErr != nil {
			message = truncateError(probeErr)
		} else {
			message = fmt.Sprintf("HTTP %d", response.StatusCode)
		}
	} else if latency > 2000 {
		status = "degraded"
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LineProbeSample{}, err
	}
	defer tx.Rollback(ctx)
	var sample LineProbeSample
	sample.LineID = id
	sample.Status = status
	sample.HTTPStatus = httpStatus
	sample.LatencyMS = &latency
	sample.ErrorMessage = message
	sample.CheckedBy = actor.Label()
	err = tx.QueryRow(ctx, `INSERT INTO line_probe_samples(line_id,status,http_status,latency_ms,error_message,checked_by) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) RETURNING id,checked_at`, id, status, httpStatus, latency, message, sample.CheckedBy).Scan(&sample.ID, &sample.CheckedAt)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE line_endpoints SET last_status=$2,last_latency_ms=$3,last_error=NULLIF($4,''),last_checked_at=NOW(),updated_at=NOW() WHERE id=$1`, id, status, latency, message)
	}
	if err == nil {
		err = audit(ctx, tx, actor, "line.probe", "line_endpoint", id.String(), map[string]any{"status": status, "latency_ms": latency, "http_status": httpStatus})
	}
	if err != nil {
		return LineProbeSample{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return LineProbeSample{}, err
	}
	return sample, nil
}

func lineProbeEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/emby") {
		return baseURL + "/System/Info/Public"
	}
	return baseURL + "/emby/System/Info/Public"
}

func (s *Service) dynamicInt(ctx context.Context, key string, fallback int) int {
	var value int
	if s.db.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM dynamic_settings WHERE key=$1`, key).Scan(&value) != nil {
		return fallback
	}
	return value
}
