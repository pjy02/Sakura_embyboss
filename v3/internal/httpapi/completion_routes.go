package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func registerCompletionRoutes(mux *http.ServeMux, o Options) {
	s := o.Platform
	mux.Handle("GET /api/v3/me/entitlements", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListAccountEntitlements(r.Context(), p.AccountID, r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/entitlements/redeem", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.RedeemEntitlementCode(r.Context(), *p.AccountID, body.Code, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/lines", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListAvailableLines(r.Context(), *p.AccountID)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/me/favorites", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListFavorites(r.Context(), p.AccountID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/favorites", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			BindingID    uuid.UUID  `json:"binding_id"`
			RemoteItemID string     `json:"remote_item_id"`
			Title        string     `json:"title"`
			MediaType    string     `json:"media_type"`
			MediaID      *uuid.UUID `json:"media_id"`
			Favorite     bool       `json:"favorite"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SetFavorite(r.Context(), *p.AccountID, body.BindingID, body.RemoteItemID, body.Title, body.MediaType, body.MediaID, body.Favorite, actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("POST /api/v3/me/favorites/sync", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			BindingID uuid.UUID `json:"binding_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.EnqueueFavoriteSync(r.Context(), *p.AccountID, body.BindingID, actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("PUT /api/v3/reviews/{id}/like", session(o, "reviews.interact", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Liked bool `json:"liked"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SetReviewLike(r.Context(), id, *p.AccountID, body.Liked, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/reviews/{id}/reports", session(o, "reviews.interact", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Reason string `json:"reason"`
			Detail string `json:"detail"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ReportReview(r.Context(), id, *p.AccountID, body.Reason, body.Detail, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))

	mux.Handle("GET /api/v3/admin/entitlement-codes", session(o, "entitlements.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListEntitlementCodes(r.Context(), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/entitlement-codes", session(o, "entitlements.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			InstanceID   *uuid.UUID     `json:"instance_id"`
			ResourceKind string         `json:"resource_kind"`
			ResourceKey  string         `json:"resource_key"`
			DurationDays int            `json:"duration_days"`
			Count        int            `json:"count"`
			Metadata     map[string]any `json:"metadata"`
		}
		if !decode(w, r, &body) {
			return
		}
		items, err := s.GenerateEntitlementCodes(r.Context(), body.InstanceID, body.ResourceKind, body.ResourceKey, body.DurationDays, body.Count, body.Metadata, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/entitlements", session(o, "entitlements.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListAccountEntitlements(r.Context(), queryUUID(r, "account_id"), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/entitlements", session(o, "entitlements.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			AccountID    uuid.UUID  `json:"account_id"`
			InstanceID   *uuid.UUID `json:"instance_id"`
			ResourceKind string     `json:"resource_kind"`
			ResourceKey  string     `json:"resource_key"`
			DurationDays int        `json:"duration_days"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.GrantEntitlement(r.Context(), body.AccountID, body.InstanceID, body.ResourceKind, body.ResourceKey, body.DurationDays, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("POST /api/v3/admin/entitlements/{id}/revoke", session(o, "entitlements.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.RevokeEntitlement(r.Context(), id, body.Reason, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/lines", session(o, "lines.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListLines(r.Context(), false)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/lines", session(o, "lines.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body lineBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveLine(r.Context(), nil, body.Name, body.BaseURL, body.Region, body.Carrier, body.Audience, body.Weight, body.SortOrder, body.Enabled, body.Maintenance, 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/lines/{id}", session(o, "lines.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body lineBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveLine(r.Context(), &id, body.Name, body.BaseURL, body.Region, body.Carrier, body.Audience, body.Weight, body.SortOrder, body.Enabled, body.Maintenance, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/admin/lines/{id}/probe", session(o, "lines.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.ProbeLine(r.Context(), id, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/review-reports", session(o, "reviews.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListReviewReports(r.Context(), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PUT /api/v3/admin/review-reports/{id}", session(o, "reviews.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Status     string `json:"status"`
			Resolution string `json:"resolution"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ResolveReviewReport(r.Context(), id, body.Status, body.Resolution, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/favorites", session(o, "emby_bindings.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListFavorites(r.Context(), queryUUID(r, "account_id"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))

	mux.Handle("GET /api/v3/admin/integration-probes", session(o, "integrations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListIntegrationProbes(r.Context(), r.URL.Query().Get("integration"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/integrations/{integration}/probe", session(o, "integrations.probe", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			InstanceID *uuid.UUID `json:"instance_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ProbeIntegration(r.Context(), r.PathValue("integration"), body.InstanceID, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
}

type lineBody struct {
	Name             string `json:"name"`
	BaseURL          string `json:"base_url"`
	Region           string `json:"region"`
	Carrier          string `json:"carrier"`
	Audience         string `json:"audience"`
	Weight           int    `json:"weight"`
	SortOrder        int    `json:"sort_order"`
	Enabled          bool   `json:"enabled"`
	Maintenance      bool   `json:"maintenance"`
	ExpectedRevision int64  `json:"expected_revision"`
}
