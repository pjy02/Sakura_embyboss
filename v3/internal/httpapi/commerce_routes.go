package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/platform"
)

func registerCommerceRoutes(mux *http.ServeMux, o Options) {
	s := o.Platform
	mux.HandleFunc("GET /api/v3/recharge-products", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.ListRechargeProducts(r.Context(), true)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	})
	mux.HandleFunc("GET /api/v3/membership-products", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.ListMembershipProducts(r.Context(), true)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	})
	mux.Handle("GET /api/v3/me/wallet", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		item, err := s.Wallet(r.Context(), *p.AccountID, r.URL.Query().Get("currency"))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/me/wallet/ledger", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListLedger(r.Context(), *p.AccountID, r.URL.Query().Get("currency"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/me/recharge-orders", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListRechargeOrders(r.Context(), p.AccountID, queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/recharge-orders", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			ProductID      uuid.UUID `json:"product_id"`
			Provider       string    `json:"provider"`
			IdempotencyKey string    `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.CreateRechargeOrder(r.Context(), *p.AccountID, body.ProductID, body.Provider, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/recharge-orders/{id}", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetRechargeOrder(r.Context(), *p.AccountID, id, false)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("POST /api/v3/me/membership-purchases", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			ProductID      uuid.UUID `json:"product_id"`
			IdempotencyKey string    `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.PurchaseMembership(r.Context(), *p.AccountID, body.ProductID, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/me/notifications", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		items, err := s.ListNotifications(r.Context(), *p.AccountID, r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/me/notifications/{id}/read", session(o, "", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err == nil {
			err = s.MarkNotificationRead(r.Context(), *p.AccountID, id)
		}
		respond(w, http.StatusNoContent, nil, err)
	}))

	mux.HandleFunc("POST /api/v3/internal/payments/{provider}/callback", func(w http.ResponseWriter, r *http.Request) {
		provider := r.PathValue("provider")
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !s.AuthorizePaymentProvider(r.Context(), provider, token) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			respond(w, 0, nil, identity.ErrInvalid)
			return
		}
		var body struct {
			EventID         string `json:"event_id"`
			OrderNo         string `json:"order_no"`
			ExternalOrderID string `json:"external_order_id"`
			AmountMinor     int64  `json:"amount_minor"`
		}
		if json.Unmarshal(raw, &body) != nil {
			respond(w, 0, nil, identity.ErrInvalid)
			return
		}
		item, err := s.ConfirmRecharge(r.Context(), provider, body.EventID, body.OrderNo, body.ExternalOrderID, body.AmountMinor, raw)
		respond(w, http.StatusOK, item, err)
	})
	mux.HandleFunc("GET /api/v3/internal/notifications/telegram/next", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		item, ok, err := s.ClaimTelegramNotification(r.Context(), "telegram-bot", 45*time.Second)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if !ok {
			respond(w, http.StatusNoContent, nil, nil)
			return
		}
		respond(w, http.StatusOK, item, nil)
	})
	mux.HandleFunc("POST /api/v3/internal/notifications/telegram/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		if !internalAuthorized(r, o.InternalBotToken) {
			respond(w, 0, nil, identity.ErrForbidden)
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Error string `json:"error"`
		}
		if !decode(w, r, &body) {
			return
		}
		var deliveryErr error
		if strings.TrimSpace(body.Error) != "" {
			deliveryErr = errors.New(body.Error)
		}
		err = s.CompleteTelegramNotification(r.Context(), id, "telegram-bot", deliveryErr)
		respond(w, http.StatusNoContent, nil, err)
	})

	mux.Handle("GET /api/v3/admin/recharge-products", session(o, "commerce.products.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRechargeProducts(r.Context(), false)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/recharge-products", session(o, "commerce.products.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body rechargeProductBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveRechargeProduct(r.Context(), nil, body.Code, body.Name, body.Description, body.PriceMinor, body.GrantAmount, body.PaymentCurrency, body.WalletCurrency, body.Enabled, body.SortOrder, 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/recharge-products/{id}", session(o, "commerce.products.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body rechargeProductBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveRechargeProduct(r.Context(), &id, body.Code, body.Name, body.Description, body.PriceMinor, body.GrantAmount, body.PaymentCurrency, body.WalletCurrency, body.Enabled, body.SortOrder, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/membership-products", session(o, "commerce.products.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListMembershipProducts(r.Context(), false)
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/membership-products", session(o, "commerce.products.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body membershipProductBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveMembershipProduct(r.Context(), nil, body.Code, body.Name, body.PlanID, body.DurationDays, body.PriceAmount, body.WalletCurrency, body.Enabled, body.SortOrder, 0, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/membership-products/{id}", session(o, "commerce.products.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body membershipProductBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveMembershipProduct(r.Context(), &id, body.Code, body.Name, body.PlanID, body.DurationDays, body.PriceAmount, body.WalletCurrency, body.Enabled, body.SortOrder, body.ExpectedRevision, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/recharge-orders", session(o, "commerce.orders.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRechargeOrders(r.Context(), queryUUID(r, "account_id"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/recharge-refunds", session(o, "commerce.orders.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListRefunds(r.Context(), queryUUID(r, "order_id"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/recharge-orders/{id}/refunds", session(o, "commerce.orders.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			WalletAmount        int64  `json:"wallet_amount"`
			ExternalAmountMinor int64  `json:"external_amount_minor"`
			Reason              string `json:"reason"`
			IdempotencyKey      string `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.RefundRecharge(r.Context(), id, body.WalletAmount, body.ExternalAmountMinor, body.Reason, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("POST /api/v3/admin/accounts/{id}/wallet-adjustments", session(o, "wallets.adjust", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		accountID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body struct {
			Amount         int64  `json:"amount"`
			Currency       string `json:"currency"`
			Reason         string `json:"reason"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if !decode(w, r, &body) {
			return
		}
		item, err := s.AdjustWallet(r.Context(), accountID, body.Amount, body.Currency, body.Reason, idempotencyKey(r, body.IdempotencyKey), actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("GET /api/v3/admin/accounts/{id}/wallet", session(o, "wallets.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		accountID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.Wallet(r.Context(), accountID, r.URL.Query().Get("currency"))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/accounts/{id}/wallet/ledger", session(o, "wallets.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		accountID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListLedger(r.Context(), accountID, r.URL.Query().Get("currency"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("GET /api/v3/admin/account-tags", session(o, "account_tags.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListTags(r.Context())
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/account-tags", session(o, "account_tags.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body tagBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveTag(r.Context(), nil, body.Code, body.Name, body.Color, body.Description, actorWithRequest(p.Actor, r))
		respond(w, http.StatusCreated, item, err)
	}))
	mux.Handle("PUT /api/v3/admin/account-tags/{id}", session(o, "account_tags.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		var body tagBody
		if !decode(w, r, &body) {
			return
		}
		item, err := s.SaveTag(r.Context(), &id, body.Code, body.Name, body.Color, body.Description, actorWithRequest(p.Actor, r))
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/batch-operations", session(o, "batch_operations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		items, err := s.ListBatches(r.Context(), r.URL.Query().Get("status"), queryInt(r, "limit", 100))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	mux.Handle("POST /api/v3/admin/batch-operations", session(o, "batch_operations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		var body struct {
			OperationType  string               `json:"operation_type"`
			Target         platform.BatchTarget `json:"target"`
			Payload        map[string]any       `json:"payload"`
			IdempotencyKey string               `json:"idempotency_key"`
			MaxAttempts    int                  `json:"max_attempts"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.MaxAttempts == 0 {
			body.MaxAttempts = 3
		}
		item, err := s.CreateBatch(r.Context(), body.OperationType, body.Target, body.Payload, idempotencyKey(r, body.IdempotencyKey), body.MaxAttempts, actorWithRequest(p.Actor, r))
		respond(w, http.StatusAccepted, item, err)
	}))
	mux.Handle("GET /api/v3/admin/batch-operations/{id}", session(o, "batch_operations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		item, err := s.GetBatch(r.Context(), id)
		respond(w, http.StatusOK, item, err)
	}))
	mux.Handle("GET /api/v3/admin/batch-operations/{id}/items", session(o, "batch_operations.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			respond(w, 0, nil, identity.ErrNotFound)
			return
		}
		items, err := s.ListBatchItems(r.Context(), id, r.URL.Query().Get("status"), queryInt(r, "limit", 200))
		respond(w, http.StatusOK, map[string]any{"items": items}, err)
	}))
	for _, action := range []string{"pause", "resume", "retry", "cancel"} {
		action := action
		mux.Handle("POST /api/v3/admin/batch-operations/{id}/"+action, session(o, "batch_operations.write", true, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
			id, err := uuid.Parse(r.PathValue("id"))
			if err != nil {
				respond(w, 0, nil, identity.ErrNotFound)
				return
			}
			var item platform.BatchOperation
			switch action {
			case "pause":
				item, err = s.PauseBatch(r.Context(), id, actorWithRequest(p.Actor, r))
			case "resume":
				item, err = s.ResumeBatch(r.Context(), id, actorWithRequest(p.Actor, r))
			case "retry":
				item, err = s.RetryBatch(r.Context(), id, actorWithRequest(p.Actor, r))
			case "cancel":
				item, err = s.CancelBatch(r.Context(), id, actorWithRequest(p.Actor, r))
			}
			respond(w, http.StatusOK, item, err)
		}))
	}
}

type rechargeProductBody struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	PriceMinor       int64  `json:"price_minor"`
	PaymentCurrency  string `json:"payment_currency"`
	GrantAmount      int64  `json:"grant_amount"`
	WalletCurrency   string `json:"wallet_currency"`
	Enabled          bool   `json:"enabled"`
	SortOrder        int    `json:"sort_order"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type membershipProductBody struct {
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	PlanID           uuid.UUID `json:"plan_id"`
	DurationDays     int       `json:"duration_days"`
	PriceAmount      int64     `json:"price_amount"`
	WalletCurrency   string    `json:"wallet_currency"`
	Enabled          bool      `json:"enabled"`
	SortOrder        int       `json:"sort_order"`
	ExpectedRevision int64     `json:"expected_revision"`
}
type tagBody struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}
