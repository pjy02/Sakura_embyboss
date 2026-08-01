package platform

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

var planCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)

func (s *Service) ListPlans(ctx context.Context, enabledOnly bool) ([]MembershipPlan, error) {
	query := `SELECT id,code,name,COALESCE(description,''),duration_days,entitlements,enabled,is_default,sort_order,revision,created_at,updated_at FROM membership_plans`
	if enabledOnly {
		query += ` WHERE enabled`
	}
	query += ` ORDER BY sort_order,code`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MembershipPlan
	for rows.Next() {
		var item MembershipPlan
		var raw []byte
		if err = rows.Scan(&item.ID, &item.Code, &item.Name, &item.Description, &item.DurationDays, &raw, &item.Enabled, &item.IsDefault, &item.SortOrder, &item.Revision, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Entitlements = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SavePlan(ctx context.Context, id *uuid.UUID, code, name, description string, durationDays int, entitlements map[string]any, enabled, isDefault bool, sortOrder int, expectedRevision int64, actor identity.Actor) (MembershipPlan, error) {
	code = normalize(code)
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if !planCodePattern.MatchString(code) || name == "" || len(name) > 100 || len(description) > 1000 || durationDays < 1 || durationDays > 3650 || !validEntitlements(entitlements) {
		return MembershipPlan{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MembershipPlan{}, err
	}
	defer tx.Rollback(ctx)
	planID := uuid.New()
	action := "membership_plan.create"
	if id != nil {
		planID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM membership_plans WHERE id=$1 FOR UPDATE`, planID).Scan(&revision); err != nil {
			return MembershipPlan{}, notFound(err)
		}
		if expectedRevision != revision {
			return MembershipPlan{}, identity.ErrConflict
		}
		action = "membership_plan.update"
	}
	if isDefault {
		if _, err = tx.Exec(ctx, `UPDATE membership_plans SET is_default=FALSE,updated_at=NOW() WHERE is_default AND id<>$1`, planID); err != nil {
			return MembershipPlan{}, err
		}
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO membership_plans(id,code,name,description,duration_days,entitlements,enabled,is_default,sort_order) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9)`, planID, code, name, description, durationDays, jsonBytes(entitlements), enabled, isDefault, sortOrder)
	} else {
		_, err = tx.Exec(ctx, `UPDATE membership_plans SET code=$2,name=$3,description=NULLIF($4,''),duration_days=$5,entitlements=$6,enabled=$7,is_default=$8,sort_order=$9,revision=revision+1,updated_at=NOW() WHERE id=$1`, planID, code, name, description, durationDays, jsonBytes(entitlements), enabled, isDefault, sortOrder)
	}
	if err != nil {
		return MembershipPlan{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "membership_plan", planID.String(), map[string]any{"code": code, "duration_days": durationDays}); err != nil {
		return MembershipPlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MembershipPlan{}, err
	}
	return s.getPlan(ctx, planID)
}

func validEntitlements(value map[string]any) bool {
	if value == nil {
		return false
	}
	maximum, ok := value["max_emby_accounts"]
	if !ok {
		return false
	}
	number, ok := maximum.(float64)
	if !ok {
		switch integer := maximum.(type) {
		case int:
			number, ok = float64(integer), true
		case int64:
			number, ok = float64(integer), true
		}
	}
	return ok && number >= 1 && number <= 100 && number == float64(int(number))
}

func (s *Service) getPlan(ctx context.Context, id uuid.UUID) (MembershipPlan, error) {
	var item MembershipPlan
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT id,code,name,COALESCE(description,''),duration_days,entitlements,enabled,is_default,sort_order,revision,created_at,updated_at FROM membership_plans WHERE id=$1`, id).Scan(&item.ID, &item.Code, &item.Name, &item.Description, &item.DurationDays, &raw, &item.Enabled, &item.IsDefault, &item.SortOrder, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return MembershipPlan{}, notFound(err)
	}
	item.Entitlements = decodeJSON(raw)
	return item, nil
}

func (s *Service) CurrentMembership(ctx context.Context, accountID uuid.UUID) (Membership, error) {
	return scanMembership(s.db.QueryRow(ctx, `SELECT m.id,m.account_id,m.plan_id,p.code,p.name,m.status,m.starts_at,m.expires_at,m.source,p.entitlements FROM account_memberships m JOIN membership_plans p ON p.id=m.plan_id WHERE m.account_id=$1 AND m.status IN ('active','grace') ORDER BY m.expires_at DESC LIMIT 1`, accountID))
}

type rowScanner interface{ Scan(...any) error }

func scanMembership(row rowScanner) (Membership, error) {
	var item Membership
	var raw []byte
	if err := row.Scan(&item.ID, &item.AccountID, &item.PlanID, &item.PlanCode, &item.PlanName, &item.Status, &item.StartsAt, &item.ExpiresAt, &item.Source, &raw); err != nil {
		return Membership{}, notFound(err)
	}
	item.Benefits = decodeJSON(raw)
	return item, nil
}

func (s *Service) AssignMembership(ctx context.Context, accountID, planID uuid.UUID, startsAt time.Time, durationDays int, source, sourceRef string, actor identity.Actor) (Membership, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Membership{}, err
	}
	defer tx.Rollback(ctx)
	membership, err := s.assignMembershipTx(ctx, tx, accountID, planID, startsAt, durationDays, source, sourceRef, actor)
	if err != nil {
		return Membership{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Membership{}, err
	}
	return membership, nil
}

func (s *Service) assignMembershipTx(ctx context.Context, tx pgx.Tx, accountID, planID uuid.UUID, startsAt time.Time, durationDays int, source, sourceRef string, actor identity.Actor) (Membership, error) {
	var plan MembershipPlan
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT id,code,name,duration_days,entitlements,enabled FROM membership_plans WHERE id=$1 FOR SHARE`, planID).Scan(&plan.ID, &plan.Code, &plan.Name, &plan.DurationDays, &raw, &plan.Enabled); err != nil {
		return Membership{}, notFound(err)
	}
	if !plan.Enabled {
		return Membership{}, identity.ErrForbidden
	}
	if durationDays <= 0 {
		durationDays = plan.DurationDays
	}
	if startsAt.IsZero() {
		startsAt = time.Now()
	}
	expiryBase := startsAt
	var currentExpiry time.Time
	err := tx.QueryRow(ctx, `SELECT expires_at FROM account_memberships WHERE account_id=$1 AND status IN ('active','grace') FOR UPDATE`, accountID).Scan(&currentExpiry)
	if err == nil && currentExpiry.After(expiryBase) {
		expiryBase = currentExpiry
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_memberships SET status='expired',updated_at=NOW() WHERE account_id=$1 AND status IN ('active','grace')`, accountID); err != nil {
		return Membership{}, err
	}
	id := uuid.New()
	expires := expiryBase.Add(time.Duration(durationDays) * 24 * time.Hour)
	if _, err = tx.Exec(ctx, `INSERT INTO account_memberships(id,account_id,plan_id,status,starts_at,expires_at,source,source_ref,created_by) VALUES($1,$2,$3,'active',$4,$5,$6,NULLIF($7,''),$8)`, id, accountID, planID, startsAt, expires, source, sourceRef, actor.Label()); err != nil {
		return Membership{}, err
	}
	if err = audit(ctx, tx, actor, "membership.assign", "account", accountID.String(), map[string]any{"membership_id": id, "plan_id": planID, "expires_at": expires, "source": source}); err != nil {
		return Membership{}, err
	}
	plan.Entitlements = decodeJSON(raw)
	return Membership{ID: id, AccountID: accountID, PlanID: planID, PlanCode: plan.Code, PlanName: plan.Name, Status: "active", StartsAt: startsAt, ExpiresAt: expires, Source: source, Benefits: plan.Entitlements}, nil
}

func (s *Service) GenerateInvitations(ctx context.Context, planID uuid.UUID, kind string, count, maxUses int, expiresAt *time.Time, actor identity.Actor) ([]Invitation, error) {
	if kind != "registration" && kind != "renewal" || count < 1 || count > 500 || maxUses < 1 || maxUses > 10000 {
		return nil, identity.ErrInvalid
	}
	plan, err := s.getPlan(ctx, planID)
	if err != nil || !plan.Enabled {
		return nil, identity.ErrNotFound
	}
	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, identity.ErrInvalid
	}
	prefix := "SAKURA"
	_ = s.db.QueryRow(ctx, `SELECT value #>> '{}' FROM dynamic_settings WHERE key='site.code_prefix'`).Scan(&prefix)
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if !regexp.MustCompile(`^[A-Z0-9]{2,16}$`).MatchString(prefix) {
		return nil, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	items := make([]Invitation, 0, count)
	for index := 0; index < count; index++ {
		suffix, randomErr := randomAlphaNumeric(10)
		if randomErr != nil {
			return nil, randomErr
		}
		marker := "Register"
		if kind == "renewal" {
			marker = "Renew"
		}
		code := fmt.Sprintf("%s-%d-%s_%s", prefix, plan.DurationDays, marker, suffix)
		id := uuid.New()
		hint := suffix[len(suffix)-6:]
		_, err = tx.Exec(ctx, `INSERT INTO invitation_codes(id,code_hash,code_prefix,code_hint,kind,plan_id,max_uses,expires_at,issued_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, security.HashToken(code), prefix, hint, kind, planID, maxUses, expiresAt, actor.Label())
		if err != nil {
			return nil, err
		}
		items = append(items, Invitation{ID: id, Code: code, CodeHint: hint, Kind: kind, PlanID: planID, PlanCode: plan.Code, MaxUses: maxUses, Status: "active", ExpiresAt: expiresAt, CreatedAt: time.Now()})
	}
	if err = audit(ctx, tx, actor, "invitation.generate", "invitation", "", map[string]any{"plan_id": planID, "kind": kind, "count": count, "max_uses": maxUses}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func randomAlphaNumeric(length int) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = alphabet[int(random[index])%len(alphabet)]
	}
	return string(buffer), nil
}

func (s *Service) ListInvitations(ctx context.Context, limit int) ([]Invitation, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT i.id,i.code_hint,i.kind,i.plan_id,p.code,i.max_uses,i.used_count,CASE WHEN i.status='active' AND i.expires_at<NOW() THEN 'expired' ELSE i.status END,i.expires_at,i.created_at FROM invitation_codes i JOIN membership_plans p ON p.id=i.plan_id ORDER BY i.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invitation
	for rows.Next() {
		var item Invitation
		if err = rows.Scan(&item.ID, &item.CodeHint, &item.Kind, &item.PlanID, &item.PlanCode, &item.MaxUses, &item.UsedCount, &item.Status, &item.ExpiresAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) RevokeInvitation(ctx context.Context, id uuid.UUID, actor identity.Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE invitation_codes SET status='revoked',updated_at=NOW() WHERE id=$1 AND status='active'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "invitation.revoke", "invitation", id.String(), nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RedeemInvitation(ctx context.Context, accountID uuid.UUID, code, idempotencyKey string, actor identity.Actor) (Membership, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Membership{}, err
	}
	defer tx.Rollback(ctx)
	membership, _, err := s.redeemInvitationTx(ctx, tx, accountID, code, idempotencyKey, actor)
	if err != nil {
		return Membership{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Membership{}, err
	}
	return membership, nil
}

func (s *Service) redeemInvitationTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, code, idempotencyKey string, actor identity.Actor) (Membership, *uuid.UUID, error) {
	code = strings.TrimSpace(code)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if code == "" || len(code) > 100 || idempotencyKey == "" || len(idempotencyKey) > 160 {
		return Membership{}, nil, identity.ErrInvalid
	}
	var invitationID, planID uuid.UUID
	var kind, status string
	var maxUses, usedCount int
	var expiresAt *time.Time
	err := tx.QueryRow(ctx, `SELECT id,plan_id,kind,status,max_uses,used_count,expires_at FROM invitation_codes WHERE code_hash=$1 FOR UPDATE`, security.HashToken(code)).Scan(&invitationID, &planID, &kind, &status, &maxUses, &usedCount, &expiresAt)
	if err != nil {
		return Membership{}, nil, identity.ErrNotFound
	}
	var replayMembershipID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT membership_id FROM invitation_redemptions WHERE idempotency_key=$1 AND account_id=$2 AND invitation_id=$3`, idempotencyKey, accountID, invitationID).Scan(&replayMembershipID)
	if err == nil {
		membership, replayErr := scanMembership(tx.QueryRow(ctx, `SELECT m.id,m.account_id,m.plan_id,p.code,p.name,m.status,m.starts_at,m.expires_at,m.source,p.entitlements FROM account_memberships m JOIN membership_plans p ON p.id=m.plan_id WHERE m.id=$1`, replayMembershipID))
		return membership, &invitationID, replayErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, nil, err
	}
	if status != "active" || usedCount >= maxUses || expiresAt != nil && expiresAt.Before(time.Now()) {
		return Membership{}, nil, identity.ErrConflict
	}
	membership, err := s.assignMembershipTx(ctx, tx, accountID, planID, time.Now(), 0, "invitation_"+kind, invitationID.String(), actor)
	if err != nil {
		return Membership{}, nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO invitation_redemptions(id,invitation_id,account_id,membership_id,idempotency_key) VALUES($1,$2,$3,$4,$5)`, uuid.New(), invitationID, accountID, membership.ID, idempotencyKey)
	if err != nil {
		return Membership{}, nil, identity.ErrConflict
	}
	newStatus := "active"
	if usedCount+1 >= maxUses {
		newStatus = "used"
	}
	if _, err = tx.Exec(ctx, `UPDATE invitation_codes SET used_count=used_count+1,status=$2,updated_at=NOW() WHERE id=$1`, invitationID, newStatus); err != nil {
		return Membership{}, nil, err
	}
	if err = audit(ctx, tx, actor, "invitation.redeem", "invitation", invitationID.String(), map[string]any{"account_id": accountID, "membership_id": membership.ID}); err != nil {
		return Membership{}, nil, err
	}
	return membership, &invitationID, nil
}
