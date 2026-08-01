package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/platform"
)

func registerRiskRoutes(mux *http.ServeMux, o Options) {
	s := o.Platform
	mux.Handle("GET /api/v3/me/playback/online", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListPlaybackSessions(r.Context(), queryUUID(r, "instance_id"), p.AccountID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/me/playback/history", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListPlaybackHistory(r.Context(), queryUUID(r, "instance_id"), p.AccountID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/me/devices", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListDevices(r.Context(), queryUUID(r, "instance_id"), p.AccountID, r.URL.Query().Get("decision"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))

	mux.Handle("GET /api/v3/admin/playback/online", session(o, "playback.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListPlaybackSessions(r.Context(), queryUUID(r, "instance_id"), queryUUID(r, "account_id"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/playback/history", session(o, "playback.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListPlaybackHistory(r.Context(), queryUUID(r, "instance_id"), queryUUID(r, "account_id"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/devices", session(o, "devices.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListDevices(r.Context(), queryUUID(r, "instance_id"), queryUUID(r, "account_id"), r.URL.Query().Get("decision"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/device-rules", session(o, "devices.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListDeviceRules(r.Context(), queryUUID(r, "instance_id"))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/device-rules", session(o, "devices.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body deviceRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveDeviceRule(r.Context(), nil, body.rule(), 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/device-rules/{id}", session(o, "devices.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body deviceRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveDeviceRule(r.Context(), &id, body.rule(), body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/risk-rules", session(o, "risk.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRiskRules(r.Context(), queryUUID(r, "instance_id"))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/risk-rules", session(o, "risk.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body riskRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveRiskRule(r.Context(), nil, body.rule(), 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/risk-rules/{id}", session(o, "risk.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body riskRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveRiskRule(r.Context(), &id, body.rule(), body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/risk-events", session(o, "risk.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRiskEvents(r.Context(), queryUUID(r, "instance_id"), queryUUID(r, "account_id"), r.URL.Query().Get("status"), r.URL.Query().Get("severity"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/risk-events/{id}", session(o, "risk.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetRiskEvent(r.Context(), id)
		respond(w, http.StatusOK, item, err)
	}))
	for _, status := range []string{"acknowledged", "resolved"} {
		status := status
		mux.Handle("POST /api/v3/admin/risk-events/{id}/"+status, session(o, "risk.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
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
			item, err := s.DispositionRiskEvent(r.Context(), id, status, body.Reason, actorWithRequest(p.Actor, r))
			respond(w, http.StatusOK, item, err)
		}))
	}
	mux.Handle("POST /api/v3/admin/risk-events/{id}/false-positive", session(o, "risk.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
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
		item, err := s.MarkRiskFalsePositive(r.Context(), id, body.Reason, actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
}

type deviceRuleBody struct {
	InstanceID       *uuid.UUID `json:"instance_id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Decision         string     `json:"decision"`
	MatchField       string     `json:"match_field"`
	MatchOperator    string     `json:"match_operator"`
	MatchValue       string     `json:"match_value"`
	Action           string     `json:"action"`
	ObservationMode  bool       `json:"observation_mode"`
	Enabled          bool       `json:"enabled"`
	Priority         int        `json:"priority"`
	ExpectedRevision int64      `json:"expected_revision"`
}

func (b deviceRuleBody) rule() platform.DeviceRule {
	return platform.DeviceRule{InstanceID: b.InstanceID, Name: b.Name, Description: b.Description, Decision: b.Decision, MatchField: b.MatchField, MatchOperator: b.MatchOperator, MatchValue: b.MatchValue, Action: b.Action, ObservationMode: b.ObservationMode, Enabled: b.Enabled, Priority: b.Priority}
}

type riskRuleBody struct {
	InstanceID       *uuid.UUID     `json:"instance_id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	RuleType         string         `json:"rule_type"`
	Condition        map[string]any `json:"condition"`
	Severity         string         `json:"severity"`
	Action           string         `json:"action"`
	ObservationMode  bool           `json:"observation_mode"`
	Enabled          bool           `json:"enabled"`
	CooldownSeconds  int            `json:"cooldown_seconds"`
	ExpectedRevision int64          `json:"expected_revision"`
}

func (b riskRuleBody) rule() platform.RiskRule {
	return platform.RiskRule{InstanceID: b.InstanceID, Code: b.Code, Name: b.Name, Description: b.Description, RuleType: b.RuleType, Condition: b.Condition, Severity: b.Severity, Action: b.Action, ObservationMode: b.ObservationMode, Enabled: b.Enabled, CooldownSeconds: b.CooldownSeconds}
}
