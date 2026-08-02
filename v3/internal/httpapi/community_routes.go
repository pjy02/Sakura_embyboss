package httpapi

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/platform"
)

func registerCommunityRoutes(mux *http.ServeMux, o Options) {
	s := o.Platform
	mux.Handle("GET /api/v3/media/search", session(o, "", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.SearchTMDB(r.Context(), r.URL.Query().Get("q"), queryInt(r, "page", 1))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/media", session(o, "", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListMedia(r.Context(), r.URL.Query().Get("q"), queryInt(r, "limit", 50))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/media/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetMedia(r.Context(), id)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/me/media-requests", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListMediaRequests(r.Context(), p.AccountID, r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/media-requests", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			MediaID uuid.UUID `json:"media_id"`
			Note    string    `json:"note"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.CreateMediaRequest(r.Context(), *p.AccountID, body.MediaID, body.Note, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/media-requests/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetMediaRequest(r.Context(), id, p.AccountID)
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/me/tickets", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListTickets(r.Context(), p.AccountID, r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/tickets", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Subject  string `json:"subject"`
			Category string `json:"category"`
			Priority string `json:"priority"`
			Body     string `json:"body"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.CreateTicket(r.Context(), *p.AccountID, body.Subject, body.Category, body.Priority, body.Body, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/tickets/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetTicket(r.Context(), id, p.AccountID)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/me/tickets/{id}/messages", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListTicketMessages(r.Context(), id, p.AccountID, false)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/tickets/{id}/messages", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Body string `json:"body"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AddTicketMessage(r.Context(), id, p.AccountID, body.Body, false, false, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))

	mux.Handle("GET /api/v3/reviews", session(o, "", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListReviews(r.Context(), queryUUID(r, "media_id"), "approved", false, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/me/reviews", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListAccountReviews(r.Context(), *p.AccountID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/reviews", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			MediaID         uuid.UUID `json:"media_id"`
			Rating          int       `json:"rating"`
			Title           string    `json:"title"`
			Body            string    `json:"body"`
			ContainsSpoiler bool      `json:"contains_spoilers"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SubmitReview(r.Context(), *p.AccountID, body.MediaID, body.Rating, body.Title, body.Body, body.ContainsSpoiler, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/notification-preferences", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.NotificationPreferences(r.Context(), *p.AccountID)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PUT /api/v3/me/notification-preferences/{event}/{channel}", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SetNotificationPreference(r.Context(), *p.AccountID, r.PathValue("event"), r.PathValue("channel"), body.Enabled, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/media/{id}/matches", session(o, "media.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListMediaMatches(r.Context(), id)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/media/{id}/match-tasks", session(o, "media.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.EnqueueMediaMatches(r.Context(), id, idempotencyKey(r, "manual"), actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/media-requests", session(o, "media_requests.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListMediaRequests(r.Context(), nil, r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/media-requests/{id}", session(o, "media_requests.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetMediaRequest(r.Context(), id, nil)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/media-requests/{id}/status", session(o, "media_requests.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Status           string `json:"status"`
			Reason           string `json:"reason"`
			ExpectedRevision int64  `json:"expected_revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.UpdateMediaRequestStatus(r.Context(), id, body.Status, body.Reason, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/admin/media-requests/{id}/moviepilot", session(o, "media_requests.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Resource       map[string]any `json:"resource"`
			IdempotencyKey string         `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SubmitMoviePilot(r.Context(), id, body.Resource, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("GET /api/v3/admin/media-requests/{id}/moviepilot/resources", session(o, "media_requests.write", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.SearchMoviePilot(r.Context(), id)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/moviepilot-jobs", session(o, "media_requests.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListMoviePilotJobs(r.Context(), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))

	mux.Handle("GET /api/v3/admin/tickets", session(o, "tickets.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListTickets(r.Context(), queryUUID(r, "account_id"), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/tickets/{id}", session(o, "tickets.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetTicket(r.Context(), id, nil)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/tickets/{id}/messages", session(o, "tickets.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListTicketMessages(r.Context(), id, nil, true)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/tickets/{id}/messages", session(o, "tickets.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Body     string `json:"body"`
			Internal bool   `json:"internal"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AddTicketMessage(r.Context(), id, p.AccountID, body.Body, body.Internal, true, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/tickets/{id}", session(o, "tickets.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Status           string     `json:"status"`
			Priority         string     `json:"priority"`
			AssignedTo       *uuid.UUID `json:"assigned_to"`
			ExpectedRevision int64      `json:"expected_revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.UpdateTicket(r.Context(), id, body.Status, body.Priority, body.AssignedTo, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/reviews", session(o, "reviews.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListReviews(r.Context(), queryUUID(r, "media_id"), r.URL.Query().Get("status"), true, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PUT /api/v3/admin/reviews/{id}/moderation", session(o, "reviews.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Status           string `json:"status"`
			Reason           string `json:"reason"`
			ExpectedRevision int64  `json:"expected_revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ModerateReview(r.Context(), id, body.Status, body.Reason, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/broadcasts", session(o, "broadcasts.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListBroadcasts(r.Context(), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/broadcasts", session(o, "broadcasts.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Title          string               `json:"title"`
			Body           string               `json:"body"`
			EventKey       string               `json:"event_key"`
			Channel        string               `json:"channel"`
			Target         platform.BatchTarget `json:"target"`
			IdempotencyKey string               `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.CreateBroadcast(r.Context(), body.Title, body.Body, body.EventKey, body.Channel, body.Target, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))

	mux.Handle("GET /api/v3/admin/automation-rules", session(o, "automations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListAutomationRules(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/automation-rules", session(o, "automations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body automationRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveAutomationRule(r.Context(), nil, body.rule(), 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/automation-rules/{id}", session(o, "automations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body automationRuleBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveAutomationRule(r.Context(), &id, body.rule(), body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/automation-executions", session(o, "automations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListAutomationExecutions(r.Context(), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
}

type automationRuleBody struct {
	Code             string           `json:"code"`
	Name             string           `json:"name"`
	Description      string           `json:"description"`
	TriggerEvent     string           `json:"trigger_event"`
	Conditions       map[string]any   `json:"conditions"`
	Actions          []map[string]any `json:"actions"`
	Enabled          bool             `json:"enabled"`
	Priority         int              `json:"priority"`
	ExpectedRevision int64            `json:"expected_revision"`
}

func (b automationRuleBody) rule() platform.AutomationRule {
	return platform.AutomationRule{Code: b.Code, Name: b.Name, Description: b.Description, TriggerEvent: b.TriggerEvent, Conditions: b.Conditions, Actions: b.Actions, Enabled: b.Enabled, Priority: b.Priority}
}
