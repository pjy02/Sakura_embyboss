package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

type handler func(http.ResponseWriter, *http.Request, identity.Principal)

func registerIdentityRoutes(mux *http.ServeMux, o Options) {
	s := o.Identity
	mux.HandleFunc("POST /api/v3/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
		}
		if !decode(w, r, &body) {
			return
		}
		a, err := s.RegisterLocal(r.Context(), body.Username, body.Password, body.DisplayName, requestActor(r, "anonymous", "registration"))
		respond(w, http.StatusCreated, a, err)
	})
	mux.HandleFunc("POST /api/v3/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if !decode(w, r, &body) {
			return
		}
		result, err := s.AuthenticateLocal(r.Context(), body.Username, body.Password, r.UserAgent(), clientIP(r))
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: o.SessionCookie, Value: result.Token, Path: "/", HttpOnly: true, Secure: o.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt})
		http.SetCookie(w, &http.Cookie{Name: o.SessionCookie + "_csrf", Value: result.CSRFToken, Path: "/", HttpOnly: false, Secure: o.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt})
		respond(w, http.StatusOK, result, nil)
	})
	mux.Handle("GET /api/v3/auth/session", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		a, err := s.GetAccount(r.Context(), *p.AccountID)
		respond(w, http.StatusOK, a, err)
	}))
	mux.Handle("POST /api/v3/auth/logout", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		err := s.RevokeSession(r.Context(), p)
		http.SetCookie(w, &http.Cookie{Name: o.SessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: o.CookieSecure, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: o.SessionCookie + "_csrf", Path: "/", MaxAge: -1, Secure: o.CookieSecure, SameSite: http.SameSiteLaxMode})
		respond(w, http.StatusNoContent, nil, err)
	}))
	mux.Handle("GET /api/v3/me", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		a, err := s.GetAccount(r.Context(), *p.AccountID)
		respond(w, http.StatusOK, a, err)
	}))
	mux.Handle("POST /api/v3/me/telegram/link-requests", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		item, err := s.StartTelegramLink(r.Context(), *p.AccountID, p.Actor)
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/telegram/link-requests/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetLinkRequest(r.Context(), id, *p.AccountID)
		respond(w, http.StatusOK, item, err)
	}))

	mux.Handle("GET /api/v3/admin/accounts", session(o, "accounts.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListAccounts(r.Context(), queryInt(r, "limit", 50))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/accounts/{id}", session(o, "accounts.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetAccount(r.Context(), id)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("PATCH /api/v3/admin/accounts/{id}/lifecycle", session(o, "accounts.lifecycle", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.ChangeLifecycle(r.Context(), id, body.Status, body.Reason, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/accounts/{id}/roles", session(o, "roles.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Roles []string `json:"roles"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AssignRoles(r.Context(), id, body.Roles, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/roles", session(o, "roles.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListRoles(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/permissions", session(o, "roles.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListPermissions(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PUT /api/v3/admin/roles/{code}", session(o, "roles.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveRole(r.Context(), r.PathValue("code"), body.Name, body.Permissions, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/settings", session(o, "settings.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListSettings(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/settings/{key}/history", session(o, "settings.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.SettingHistory(r.Context(), r.PathValue("key"))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PATCH /api/v3/admin/settings/{key}", session(o, "settings.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Value            any    `json:"value"`
			ValueType        string `json:"value_type"`
			ExpectedRevision int64  `json:"expected_revision"`
			Reason           string `json:"reason"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.UpdateSetting(r.Context(), r.PathValue("key"), body.Value, body.ValueType, body.Reason, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/admin/settings/{key}/rollback", session(o, "settings.rollback", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			TargetRevision   int64  `json:"target_revision"`
			ExpectedRevision int64  `json:"expected_revision"`
			Reason           string `json:"reason"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.RollbackSetting(r.Context(), r.PathValue("key"), body.TargetRevision, body.ExpectedRevision, body.Reason, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/credentials", session(o, "credentials.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListCredentials(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("PUT /api/v3/admin/credentials/{name}", session(o, "credentials.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Kind     string `json:"kind"`
			Secret   string `json:"secret"`
			Metadata any    `json:"metadata"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.PutCredential(r.Context(), r.PathValue("name"), body.Kind, body.Secret, body.Metadata, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("DELETE /api/v3/admin/credentials/{name}", session(o, "credentials.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		err := s.DeleteCredential(r.Context(), r.PathValue("name"), actorWithRequest(p.Actor, r))
		respond(w, http.StatusNoContent, nil, err)
	}))
	mux.Handle("GET /api/v3/admin/api-scopes", session(o, "api_clients.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		respond(w, http.StatusOK, map[string]any{"scopes": identity.APIScopeCatalog()}, nil)
	}))
	mux.Handle("GET /api/v3/admin/api-clients", session(o, "api_clients.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListAPIClients(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/api-clients", session(o, "api_clients.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.CreateAPIClient(r.Context(), body.Name, body.Scopes, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("DELETE /api/v3/admin/api-clients/{id}", session(o, "api_clients.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err == nil {
			err = s.RevokeAPIClient(r.Context(), id, actorWithRequest(p.Actor, r))
		}
		respond(w, http.StatusNoContent, nil, err)
	}))
	mux.Handle("GET /api/v3/admin/audit", session(o, "audit.read", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.Audit(r.Context(), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))

	mux.HandleFunc("POST /api/v3/internal/telegram/link-requests/confirm", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		var body struct {
			Code           string `json:"code"`
			TelegramUserID int64  `json:"telegram_user_id"`
			Username       string `json:"username"`
		}
		if !decode(w, r, &body) {
			return
		}
		err := s.ConfirmTelegramLink(r.Context(), body.Code, body.TelegramUserID, body.Username, requestActor(r, "service", "telegram-bot"))
		respond(w, http.StatusNoContent, nil, err)
	})
	mux.HandleFunc("GET /api/v3/internal/credentials/{name}/reveal", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) || r.PathValue("name") != "telegram.bot_token" {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		secret, err := s.RevealCredential(r.Context(), r.PathValue("name"), requestActor(r, "service", "telegram-bot"))
		respond(w, http.StatusOK, map[string]string{"secret": secret}, err)
	})
	mux.Handle("GET /open/v1/system/info", scope(o, "system:read", func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		respond(w, http.StatusOK, map[string]any{"name": "Sakura", "api_version": "v1", "client_id": p.Actor.ID}, nil)
	}))
}

func internalAuthorized(r *http.Request, expected string) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return expected != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func session(o Options, permission string, csrf bool, next handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(o.SessionCookie)
		if err != nil {
			respond(w, 0, nil, identity.ErrInvalidCredentials)
			return
		}
		p, err := o.Identity.AuthenticateSession(r.Context(), cookie.Value)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if permission != "" && !p.HasPermission(permission) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		if csrf && !security.EqualHash(p.CSRFHash, security.HashToken(r.Header.Get("X-CSRF-Token"))) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		p.Actor = actorWithRequest(p.Actor, r)
		next(w, r, p)
	})
}
func scope(o Options, required string, next handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		p, err := o.Identity.AuthenticateAPIClient(r.Context(), token)
		if err != nil || !p.HasScope(required) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		next(w, r, p)
	})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_REQUEST", "message": err.Error()})
		return false
	}
	return true
}
func respond(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		code := http.StatusInternalServerError
		key := "INTERNAL_ERROR"
		message := "internal server error"
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			code, key = http.StatusUnauthorized, "INVALID_CREDENTIALS"
		case errors.Is(err, identity.ErrForbidden):
			code, key = http.StatusForbidden, "FORBIDDEN"
		case errors.Is(err, identity.ErrNotFound):
			code, key = http.StatusNotFound, "NOT_FOUND"
		case errors.Is(err, identity.ErrInvalid):
			code, key = http.StatusBadRequest, "INVALID_REQUEST"
		case errors.Is(err, identity.ErrUsernameTaken), errors.Is(err, identity.ErrConflict):
			code, key = http.StatusConflict, "CONFLICT"
		}
		if code != http.StatusInternalServerError {
			message = err.Error()
		}
		writeJSON(w, code, map[string]string{"code": key, "message": message})
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, value)
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func requestActor(r *http.Request, kind, id string) identity.Actor {
	return identity.Actor{Kind: kind, ID: id, IP: clientIP(r), RequestID: r.Header.Get("X-Request-ID")}
}
func actorWithRequest(a identity.Actor, r *http.Request) identity.Actor {
	a.IP = clientIP(r)
	a.RequestID = r.Header.Get("X-Request-ID")
	return a
}
func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}
