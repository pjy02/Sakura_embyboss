package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func registerPlatformRoutes(mux *http.ServeMux, o Options) {
	s := o.Platform
	mux.HandleFunc("GET /api/v3/membership-plans", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.ListPlans(r.Context(), true)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	})
	mux.Handle("GET /api/v3/me/membership", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		item, err := s.CurrentMembership(r.Context(), *p.AccountID)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/me/invitations/redeem", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Code           string `json:"code"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		key := idempotencyKey(r, body.IdempotencyKey)
		item, err := s.RedeemInvitation(r.Context(), *p.AccountID, body.Code, key, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/me/emby-bindings", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListBindings(r.Context(), p.AccountID, nil, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/emby/provision-requests", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			InstanceID     *uuid.UUID `json:"instance_id"`
			Username       string     `json:"username"`
			InvitationCode string     `json:"invitation_code"`
			IdempotencyKey string     `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.RequestProvisioning(r.Context(), *p.AccountID, body.InstanceID, body.Username, body.InvitationCode, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("GET /api/v3/me/emby/provision-requests/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetProvisioning(r.Context(), *p.AccountID, id)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/me/emby-claims", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Token string `json:"token"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ClaimRemoteUser(r.Context(), *p.AccountID, body.Token, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))

	mux.Handle("GET /api/v3/admin/membership-plans", session(o, "memberships.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListPlans(r.Context(), false)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/membership-plans", session(o, "memberships.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body planBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SavePlan(r.Context(), nil, body.Code, body.Name, body.Description, body.DurationDays, body.Entitlements, body.Enabled, body.IsDefault, body.SortOrder, 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/membership-plans/{id}", session(o, "memberships.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body planBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SavePlan(r.Context(), &id, body.Code, body.Name, body.Description, body.DurationDays, body.Entitlements, body.Enabled, body.IsDefault, body.SortOrder, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/admin/accounts/{id}/membership", session(o, "memberships.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		accountID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			PlanID       uuid.UUID `json:"plan_id"`
			DurationDays int       `json:"duration_days"`
			SourceRef    string    `json:"source_ref"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AssignMembership(r.Context(), accountID, body.PlanID, time.Now(), body.DurationDays, "admin", body.SourceRef, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/invitations", session(o, "invitations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListInvitations(r.Context(), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/invitations", session(o, "invitations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			PlanID    uuid.UUID  `json:"plan_id"`
			Kind      string     `json:"kind"`
			Count     int        `json:"count"`
			MaxUses   int        `json:"max_uses"`
			ExpiresAt *time.Time `json:"expires_at"`
		}
		if !decode(w, r, &body) {
			return
		}
		items, err := s.GenerateInvitations(r.Context(), body.PlanID, body.Kind, body.Count, body.MaxUses, body.ExpiresAt, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, map[string]any{"items": items}, err)
	}))
	mux.Handle("DELETE /api/v3/admin/invitations/{id}", session(o, "invitations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err == nil {
			err = s.RevokeInvitation(r.Context(), id, actorWithRequest(p.Actor, r))
		}
		respond(w, http.StatusNoContent, nil, err)
	}))
	mux.Handle("GET /api/v3/admin/emby/instances", session(o, "emby_instances.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListInstances(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/instances", session(o, "emby_instances.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body instanceBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveInstance(r.Context(), nil, body.Name, body.BaseURL, body.CredentialName, body.Enabled, body.IsDefault, body.VerifyTLS, body.Priority, 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/emby/instances/{id}", session(o, "emby_instances.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body instanceBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveInstance(r.Context(), &id, body.Name, body.BaseURL, body.CredentialName, body.Enabled, body.IsDefault, body.VerifyTLS, body.Priority, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/emby/bindings", session(o, "emby_bindings.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		accountID := queryUUID(r, "account_id")
		instanceID := queryUUID(r, "instance_id")
		items, err := s.ListBindings(r.Context(), accountID, instanceID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/emby/remote-users", session(o, "emby_bindings.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRemoteUsers(r.Context(), queryUUID(r, "instance_id"), r.URL.Query().Get("claim_status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/remote-users/{id}/claim-token", session(o, "emby_bindings.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			ExpiresSeconds int `json:"expires_seconds"`
		}
		if !decode(w, r, &body) {
			return
		}
		token, err := s.GenerateClaimToken(r.Context(), id, time.Duration(body.ExpiresSeconds)*time.Second, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, map[string]string{"token": token}, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/remote-users/{id}/claim", session(o, "emby_bindings.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		remoteID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			AccountID uuid.UUID `json:"account_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AdminClaimRemoteUser(r.Context(), body.AccountID, remoteID, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/instances/{id}/adopt-legacy", session(o, "emby_bindings.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		result, err := s.AdoptLegacyIdentities(r.Context(), id, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, result, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/instances/{id}/tasks", session(o, "emby_sync.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Type           string `json:"type"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.EnqueueInstanceTask(r.Context(), "emby."+strings.TrimPrefix(body.Type, "emby."), id, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("GET /api/v3/admin/emby/tasks", session(o, "emby_sync.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListTasks(r.Context(), queryUUID(r, "instance_id"), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/emby/tasks/{id}/retry", session(o, "emby_sync.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.RetryTask(r.Context(), id, actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("GET /api/v3/admin/emby/instances/{id}/snapshots", session(o, "emby_sync.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListSnapshots(r.Context(), id, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))

	mux.HandleFunc("POST /api/v3/internal/emby/provision-requests", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		var body struct {
			TelegramUserID   int64      `json:"telegram_user_id"`
			TelegramUsername string     `json:"telegram_username"`
			InstanceID       *uuid.UUID `json:"instance_id"`
			Username         string     `json:"username"`
			InvitationCode   string     `json:"invitation_code"`
			IdempotencyKey   string     `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		accountID, err := s.AccountIDByTelegram(r.Context(), body.TelegramUserID)
		if err == nil {
			item, callErr := s.RequestProvisioning(r.Context(), accountID, body.InstanceID, body.Username, body.InvitationCode, idempotencyKey(r, body.IdempotencyKey), requestActor(r, "service", "telegram-bot:"+body.TelegramUsername))
			respond(w, http.StatusAccepted, item, callErr)
			return
		}
		respond(w, 0, nil, err)
	})
	mux.HandleFunc("GET /api/v3/internal/emby/provision-requests/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		id, parseErr := uuid.Parse(r.PathValue("id"))
		telegramID, numberErr := parseInt64(r.URL.Query().Get("telegram_user_id"))
		if parseErr != nil || numberErr != nil {
			respond(w, 0, nil, identity.ErrInvalid)
			return
		}
		accountID, err := s.AccountIDByTelegram(r.Context(), telegramID)
		if err == nil {
			item, callErr := s.GetProvisioning(r.Context(), accountID, id)
			respond(w, http.StatusOK, item, callErr)
			return
		}
		respond(w, 0, nil, err)
	})
}

type planBody struct {
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	DurationDays     int            `json:"duration_days"`
	Entitlements     map[string]any `json:"entitlements"`
	Enabled          bool           `json:"enabled"`
	IsDefault        bool           `json:"is_default"`
	SortOrder        int            `json:"sort_order"`
	ExpectedRevision int64          `json:"expected_revision"`
}
type instanceBody struct {
	Name             string `json:"name"`
	BaseURL          string `json:"base_url"`
	CredentialName   string `json:"credential_name"`
	Enabled          bool   `json:"enabled"`
	IsDefault        bool   `json:"is_default"`
	VerifyTLS        bool   `json:"verify_tls"`
	Priority         int    `json:"priority"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func idempotencyKey(r *http.Request, fallback string) string {
	if value := strings.TrimSpace(r.Header.Get("Idempotency-Key")); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}
func queryUUID(r *http.Request, key string) *uuid.UUID {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &id
}
func parseInt64(value string) (int64, error) {
	var result int64
	_, err := fmt.Sscan(value, &result)
	return result, err
}
