package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/platform"
)

type botActionRequest struct {
	TelegramUserID int64             `json:"telegram_user_id"`
	Action         string            `json:"action"`
	Arguments      map[string]string `json:"arguments"`
	IdempotencyKey string            `json:"idempotency_key"`
}

func registerBotRoutes(mux *http.ServeMux, o Options) {
	mux.HandleFunc("POST /api/v3/internal/bot/actions", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		var body botActionRequest
		if !decode(w, r, &body) {
			return
		}
		principal, err := o.Identity.AuthenticateTelegram(r.Context(), body.TelegramUserID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		principal.Actor = actorWithRequest(principal.Actor, r)
		value, err := executeBotAction(r, o, principal, body)
		respond(w, http.StatusOK, value, err)
	})
}

func executeBotAction(r *http.Request, o Options, p identity.Principal, request botActionRequest) (any, error) {
	ctx := r.Context()
	accountID := *p.AccountID
	action := strings.ToLower(strings.TrimSpace(request.Action))
	arg := func(key string) string { return strings.TrimSpace(request.Arguments[key]) }
	switch action {
	case "context":
		account, err := o.Identity.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		permissions := make([]string, 0, len(p.Permissions))
		for permission := range p.Permissions {
			permissions = append(permissions, permission)
		}
		return map[string]any{"account": account, "permissions": permissions}, nil
	case "dashboard":
		membership, membershipErr := o.Platform.CurrentMembership(ctx, accountID)
		wallet, walletErr := o.Platform.Wallet(ctx, accountID, "POINTS")
		bindings, bindingsErr := o.Platform.ListBindings(ctx, &accountID, nil, 20)
		if membershipErr != nil && !errors.Is(membershipErr, identity.ErrNotFound) {
			return nil, membershipErr
		}
		if walletErr != nil {
			return nil, walletErr
		}
		if bindingsErr != nil {
			return nil, bindingsErr
		}
		return map[string]any{"membership": membership, "wallet": wallet, "emby_bindings": bindings}, nil
	case "media_search":
		return o.Platform.SearchTMDB(ctx, arg("query"), 1)
	case "media_request":
		mediaID, err := uuid.Parse(arg("media_id"))
		if err != nil {
			return nil, identity.ErrInvalid
		}
		return o.Platform.CreateMediaRequest(ctx, accountID, mediaID, arg("note"), p.Actor)
	case "requests":
		return o.Platform.ListMediaRequests(ctx, &accountID, "", 20)
	case "tickets":
		return o.Platform.ListTickets(ctx, &accountID, "", 20)
	case "ticket_create":
		return o.Platform.CreateTicket(ctx, accountID, arg("subject"), stringOrDefault(arg("category"), "general"), stringOrDefault(arg("priority"), "normal"), arg("body"), p.Actor)
	case "notifications":
		return o.Platform.ListNotifications(ctx, accountID, "unread", 20)
	case "entitlements":
		return o.Platform.ListAccountEntitlements(ctx, &accountID, "", 20)
	case "lines":
		return o.Platform.ListAvailableLines(ctx, accountID)
	case "entitlement_redeem":
		return o.Platform.RedeemEntitlementCode(ctx, accountID, arg("code"), p.Actor)
	case "favorites":
		return o.Platform.ListFavorites(ctx, &accountID, 20)
	case "favorite_sync":
		bindingID, err := uuid.Parse(arg("binding_id"))
		if err != nil {
			return nil, identity.ErrInvalid
		}
		return o.Platform.EnqueueFavoriteSync(ctx, accountID, bindingID, p.Actor)
	case "review_like":
		reviewID, err := uuid.Parse(arg("review_id"))
		if err != nil {
			return nil, identity.ErrInvalid
		}
		return o.Platform.SetReviewLike(ctx, reviewID, accountID, arg("liked") != "false", p.Actor)
	case "review_report":
		reviewID, err := uuid.Parse(arg("review_id"))
		if err != nil {
			return nil, identity.ErrInvalid
		}
		return o.Platform.ReportReview(ctx, reviewID, accountID, arg("reason"), arg("detail"), p.Actor)
	case "admin_dashboard":
		if !p.HasPermission("dashboard.read") {
			return nil, identity.ErrForbidden
		}
		return o.Platform.AdminRealtime(ctx)
	case "admin_accounts":
		if !p.HasPermission("accounts.read") {
			return nil, identity.ErrForbidden
		}
		return o.Identity.ListAccounts(ctx, 20)
	case "admin_tasks":
		if !p.HasPermission("emby_sync.read") {
			return nil, identity.ErrForbidden
		}
		return o.Platform.ListTasks(ctx, nil, "", 20)
	case "admin_risks":
		if !p.HasPermission("risk.read") {
			return nil, identity.ErrForbidden
		}
		return o.Platform.ListRiskEvents(ctx, nil, nil, "", "", 20)
	case "admin_broadcast":
		if !p.HasPermission("broadcasts.write") {
			return nil, identity.ErrForbidden
		}
		return o.Platform.CreateBroadcast(ctx, arg("title"), arg("body"), "broadcast.general", stringOrDefault(arg("channel"), "telegram"), platform.BatchTarget{Status: "active"}, request.IdempotencyKey, p.Actor)
	default:
		return nil, identity.ErrInvalid
	}
}

func stringOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
