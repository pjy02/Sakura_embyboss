package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

var productCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,63}$`)
var providerPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,39}$`)

func (s *Service) SaveRechargeProduct(ctx context.Context, id *uuid.UUID, code, name, description string, priceMinor, grantAmount int64, paymentCurrency, walletCurrency string, enabled bool, sortOrder int, expectedRevision int64, actor identity.Actor) (RechargeProduct, error) {
	code = normalize(code)
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	paymentCurrency = strings.ToUpper(strings.TrimSpace(paymentCurrency))
	walletCurrency = normalizeCurrency(walletCurrency)
	if !productCodePattern.MatchString(code) || name == "" || len(name) > 120 || len(description) > 1000 || priceMinor <= 0 || grantAmount <= 0 || !currencyPattern.MatchString(walletCurrency) || !regexp.MustCompile(`^[A-Z]{3,8}$`).MatchString(paymentCurrency) {
		return RechargeProduct{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RechargeProduct{}, err
	}
	defer tx.Rollback(ctx)
	productID := uuid.New()
	action := "recharge_product.create"
	if id != nil {
		productID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM recharge_products WHERE id=$1 FOR UPDATE`, productID).Scan(&revision); err != nil {
			return RechargeProduct{}, notFound(err)
		}
		if revision != expectedRevision {
			return RechargeProduct{}, identity.ErrConflict
		}
		action = "recharge_product.update"
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO recharge_products(id,code,name,description,price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10)`, productID, code, name, description, priceMinor, paymentCurrency, grantAmount, walletCurrency, enabled, sortOrder)
	} else {
		_, err = tx.Exec(ctx, `UPDATE recharge_products SET code=$2,name=$3,description=NULLIF($4,''),price_minor=$5,payment_currency=$6,grant_amount=$7,wallet_currency=$8,enabled=$9,sort_order=$10,revision=revision+1,updated_at=NOW() WHERE id=$1`, productID, code, name, description, priceMinor, paymentCurrency, grantAmount, walletCurrency, enabled, sortOrder)
	}
	if err != nil {
		return RechargeProduct{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "recharge_product", productID.String(), map[string]any{"price_minor": priceMinor, "grant_amount": grantAmount}); err != nil {
		return RechargeProduct{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RechargeProduct{}, err
	}
	return s.GetRechargeProduct(ctx, productID)
}

func scanRechargeProduct(row rowScanner) (RechargeProduct, error) {
	var p RechargeProduct
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.PriceMinor, &p.PaymentCurrency, &p.GrantAmount, &p.WalletCurrency, &p.Enabled, &p.SortOrder, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	return p, notFound(err)
}
func (s *Service) GetRechargeProduct(ctx context.Context, id uuid.UUID) (RechargeProduct, error) {
	return scanRechargeProduct(s.db.QueryRow(ctx, `SELECT id,code,name,COALESCE(description,''),price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order,revision,created_at,updated_at FROM recharge_products WHERE id=$1`, id))
}
func (s *Service) ListRechargeProducts(ctx context.Context, enabledOnly bool) ([]RechargeProduct, error) {
	query := `SELECT id,code,name,COALESCE(description,''),price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order,revision,created_at,updated_at FROM recharge_products`
	if enabledOnly {
		query += ` WHERE enabled`
	}
	query += ` ORDER BY sort_order,code`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RechargeProduct
	for rows.Next() {
		p, scanErr := scanRechargeProduct(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) SaveMembershipProduct(ctx context.Context, id *uuid.UUID, code, name string, planID uuid.UUID, durationDays int, priceAmount int64, walletCurrency string, enabled bool, sortOrder int, expectedRevision int64, actor identity.Actor) (MembershipProduct, error) {
	code = normalize(code)
	name = strings.TrimSpace(name)
	walletCurrency = normalizeCurrency(walletCurrency)
	if !productCodePattern.MatchString(code) || name == "" || len(name) > 120 || durationDays < 1 || durationDays > 3650 || priceAmount <= 0 || !currencyPattern.MatchString(walletCurrency) {
		return MembershipProduct{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MembershipProduct{}, err
	}
	defer tx.Rollback(ctx)
	var planEnabled bool
	if err = tx.QueryRow(ctx, `SELECT enabled FROM membership_plans WHERE id=$1`, planID).Scan(&planEnabled); err != nil || !planEnabled {
		return MembershipProduct{}, identity.ErrNotFound
	}
	productID := uuid.New()
	action := "membership_product.create"
	if id != nil {
		productID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM membership_products WHERE id=$1 FOR UPDATE`, productID).Scan(&revision); err != nil {
			return MembershipProduct{}, notFound(err)
		}
		if revision != expectedRevision {
			return MembershipProduct{}, identity.ErrConflict
		}
		action = "membership_product.update"
	}
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO membership_products(id,code,name,plan_id,duration_days,price_amount,wallet_currency,enabled,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, productID, code, name, planID, durationDays, priceAmount, walletCurrency, enabled, sortOrder)
	} else {
		_, err = tx.Exec(ctx, `UPDATE membership_products SET code=$2,name=$3,plan_id=$4,duration_days=$5,price_amount=$6,wallet_currency=$7,enabled=$8,sort_order=$9,revision=revision+1,updated_at=NOW() WHERE id=$1`, productID, code, name, planID, durationDays, priceAmount, walletCurrency, enabled, sortOrder)
	}
	if err != nil {
		return MembershipProduct{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "membership_product", productID.String(), map[string]any{"price_amount": priceAmount, "plan_id": planID}); err != nil {
		return MembershipProduct{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MembershipProduct{}, err
	}
	return s.GetMembershipProduct(ctx, productID)
}

func scanMembershipProduct(row rowScanner) (MembershipProduct, error) {
	var p MembershipProduct
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.PlanID, &p.PlanCode, &p.PlanName, &p.DurationDays, &p.PriceAmount, &p.WalletCurrency, &p.Enabled, &p.SortOrder, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	return p, notFound(err)
}
func (s *Service) GetMembershipProduct(ctx context.Context, id uuid.UUID) (MembershipProduct, error) {
	return scanMembershipProduct(s.db.QueryRow(ctx, `SELECT m.id,m.code,m.name,m.plan_id,p.code,p.name,m.duration_days,m.price_amount,m.wallet_currency,m.enabled,m.sort_order,m.revision,m.created_at,m.updated_at FROM membership_products m JOIN membership_plans p ON p.id=m.plan_id WHERE m.id=$1`, id))
}
func (s *Service) ListMembershipProducts(ctx context.Context, enabledOnly bool) ([]MembershipProduct, error) {
	query := `SELECT m.id,m.code,m.name,m.plan_id,p.code,p.name,m.duration_days,m.price_amount,m.wallet_currency,m.enabled,m.sort_order,m.revision,m.created_at,m.updated_at FROM membership_products m JOIN membership_plans p ON p.id=m.plan_id`
	if enabledOnly {
		query += ` WHERE m.enabled`
	}
	query += ` ORDER BY m.sort_order,m.code`
	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MembershipProduct
	for rows.Next() {
		p, scanErr := scanMembershipProduct(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) CreateRechargeOrder(ctx context.Context, accountID, productID uuid.UUID, provider, idempotencyKey string, actor identity.Actor) (RechargeOrder, error) {
	provider = normalize(provider)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if !providerPattern.MatchString(provider) || idempotencyKey == "" || len(idempotencyKey) > 160 {
		return RechargeOrder{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RechargeOrder{}, err
	}
	defer tx.Rollback(ctx)
	fullKey := accountID.String() + ":" + idempotencyKey
	var existingID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM recharge_orders WHERE idempotency_key=$1`, fullKey).Scan(&existingID)
	if err == nil {
		return s.rechargeOrderTx(ctx, tx, existingID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RechargeOrder{}, err
	}
	var providerConfigured bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM credentials WHERE name=$1 AND kind='payment_webhook')`, "payment.webhook."+provider).Scan(&providerConfigured); err != nil || !providerConfigured {
		return RechargeOrder{}, identity.ErrNotFound
	}
	product, err := scanRechargeProduct(tx.QueryRow(ctx, `SELECT id,code,name,COALESCE(description,''),price_minor,payment_currency,grant_amount,wallet_currency,enabled,sort_order,revision,created_at,updated_at FROM recharge_products WHERE id=$1 FOR SHARE`, productID))
	if err != nil || !product.Enabled {
		return RechargeOrder{}, identity.ErrNotFound
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, accountID).Scan(&active); err != nil || !active {
		return RechargeOrder{}, identity.ErrForbidden
	}
	ttl := s.dynamicIntTx(ctx, tx, "commerce.order_ttl_minutes", 30)
	if ttl < 5 || ttl > 1440 {
		ttl = 30
	}
	id := uuid.New()
	orderNo := "ORDER-" + time.Now().UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
	_, err = tx.Exec(ctx, `INSERT INTO recharge_orders(id,order_no,idempotency_key,account_id,product_id,provider,price_minor,payment_currency,grant_amount,wallet_currency,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW()+($11::integer*INTERVAL '1 minute'))`, id, orderNo, fullKey, accountID, productID, provider, product.PriceMinor, product.PaymentCurrency, product.GrantAmount, product.WalletCurrency, ttl)
	if err != nil {
		return RechargeOrder{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "recharge_order.create", "recharge_order", id.String(), map[string]any{"provider": provider, "product_id": productID}); err != nil {
		return RechargeOrder{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RechargeOrder{}, err
	}
	return s.GetRechargeOrder(ctx, accountID, id, false)
}

func scanRechargeOrder(row rowScanner) (RechargeOrder, error) {
	var o RechargeOrder
	err := row.Scan(&o.ID, &o.OrderNo, &o.AccountID, &o.ProductID, &o.ProductName, &o.Provider, &o.ExternalOrderID, &o.Status, &o.PriceMinor, &o.PaymentCurrency, &o.GrantAmount, &o.WalletCurrency, &o.PaidAmountMinor, &o.LedgerTransactionID, &o.ExpiresAt, &o.PaidAt, &o.CreatedAt, &o.UpdatedAt)
	return o, notFound(err)
}
func (s *Service) rechargeOrderTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (RechargeOrder, error) {
	return scanRechargeOrder(tx.QueryRow(ctx, `SELECT o.id,o.order_no,o.account_id,o.product_id,p.name,o.provider,COALESCE(o.external_order_id,''),o.status,o.price_minor,o.payment_currency,o.grant_amount,o.wallet_currency,o.paid_amount_minor,o.ledger_transaction_id,o.expires_at,o.paid_at,o.created_at,o.updated_at FROM recharge_orders o JOIN recharge_products p ON p.id=o.product_id WHERE o.id=$1`, id))
}
func (s *Service) GetRechargeOrder(ctx context.Context, accountID, id uuid.UUID, admin bool) (RechargeOrder, error) {
	query := `SELECT o.id,o.order_no,o.account_id,o.product_id,p.name,o.provider,COALESCE(o.external_order_id,''),o.status,o.price_minor,o.payment_currency,o.grant_amount,o.wallet_currency,o.paid_amount_minor,o.ledger_transaction_id,o.expires_at,o.paid_at,o.created_at,o.updated_at FROM recharge_orders o JOIN recharge_products p ON p.id=o.product_id WHERE o.id=$1`
	args := []any{id}
	if !admin {
		query += ` AND o.account_id=$2`
		args = append(args, accountID)
	}
	return scanRechargeOrder(s.db.QueryRow(ctx, query, args...))
}
func (s *Service) ListRechargeOrders(ctx context.Context, accountID *uuid.UUID, limit int) ([]RechargeOrder, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT o.id,o.order_no,o.account_id,o.product_id,p.name,o.provider,COALESCE(o.external_order_id,''),o.status,o.price_minor,o.payment_currency,o.grant_amount,o.wallet_currency,o.paid_amount_minor,o.ledger_transaction_id,o.expires_at,o.paid_at,o.created_at,o.updated_at FROM recharge_orders o JOIN recharge_products p ON p.id=o.product_id WHERE ($1::uuid IS NULL OR o.account_id=$1) ORDER BY o.created_at DESC LIMIT $2`, uuidQueryValue(accountID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RechargeOrder
	for rows.Next() {
		o, scanErr := scanRechargeOrder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Service) AuthorizePaymentProvider(ctx context.Context, provider, token string) bool {
	provider = normalize(provider)
	if !providerPattern.MatchString(provider) || token == "" || s.vault == nil {
		return false
	}
	var ciphertext, nonce []byte
	var version int
	err := s.db.QueryRow(ctx, `SELECT ciphertext,nonce,key_version FROM credentials WHERE name=$1 AND kind='payment_webhook'`, `payment.webhook.`+provider).Scan(&ciphertext, &nonce, &version)
	if err != nil {
		return false
	}
	plain, err := s.vault.Decrypt(ciphertext, nonce, version)
	return err == nil && subtle.ConstantTimeCompare(plain, []byte(token)) == 1
}

func (s *Service) ConfirmRecharge(ctx context.Context, provider, eventID, orderNo, externalOrderID string, amountMinor int64, payload []byte) (RechargeOrder, error) {
	provider = normalize(provider)
	eventID = strings.TrimSpace(eventID)
	orderNo = strings.TrimSpace(orderNo)
	externalOrderID = strings.TrimSpace(externalOrderID)
	if !providerPattern.MatchString(provider) || eventID == "" || len(eventID) > 180 || orderNo == "" || len(orderNo) > 80 || externalOrderID == "" || len(externalOrderID) > 160 || amountMinor <= 0 {
		return RechargeOrder{}, identity.ErrInvalid
	}
	hash := sha256.Sum256(payload)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RechargeOrder{}, err
	}
	defer tx.Rollback(ctx)
	var replayOrderID uuid.UUID
	var replayExternal string
	var replayAmount int64
	err = tx.QueryRow(ctx, `SELECT order_id,external_order_id,amount_minor FROM payment_callback_events WHERE provider=$1 AND event_id=$2`, provider, eventID).Scan(&replayOrderID, &replayExternal, &replayAmount)
	if err == nil {
		replayOrder, loadErr := s.rechargeOrderTx(ctx, tx, replayOrderID)
		if loadErr != nil {
			return RechargeOrder{}, loadErr
		}
		if replayOrder.OrderNo != orderNo || replayExternal != externalOrderID || replayAmount != amountMinor {
			return RechargeOrder{}, identity.ErrConflict
		}
		return replayOrder, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RechargeOrder{}, err
	}
	var orderID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM recharge_orders WHERE order_no=$1 AND provider=$2 FOR UPDATE`, orderNo, provider).Scan(&orderID)
	if err != nil {
		return RechargeOrder{}, notFound(err)
	}
	order, err := s.rechargeOrderTx(ctx, tx, orderID)
	if err != nil {
		return RechargeOrder{}, err
	}
	if order.Status != "pending" {
		if (order.Status == "paid" || order.Status == "partially_refunded" || order.Status == "refunded") && order.ExternalOrderID == externalOrderID && order.PaidAmountMinor != nil && *order.PaidAmountMinor == amountMinor {
			_, err = tx.Exec(ctx, `INSERT INTO payment_callback_events(provider,event_id,order_id,external_order_id,amount_minor,payload_hash,ledger_transaction_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, provider, eventID, order.ID, externalOrderID, amountMinor, hash[:], order.LedgerTransactionID)
			if err != nil {
				return RechargeOrder{}, identity.ErrConflict
			}
			if err = tx.Commit(ctx); err != nil {
				return RechargeOrder{}, err
			}
			return s.GetRechargeOrder(ctx, uuid.Nil, order.ID, true)
		}
		return RechargeOrder{}, identity.ErrConflict
	}
	if time.Now().After(order.ExpiresAt) || amountMinor != order.PriceMinor {
		return RechargeOrder{}, identity.ErrConflict
	}
	wallet, err := s.ensureWalletTx(ctx, tx, order.AccountID, order.WalletCurrency)
	if err != nil {
		return RechargeOrder{}, err
	}
	system, err := systemLedgerAccountTx(ctx, tx, "cash_clearing", order.WalletCurrency)
	if err != nil {
		return RechargeOrder{}, err
	}
	actor := identity.Actor{Kind: "service", ID: "payment:" + provider}
	ledger, err := s.postLedgerTx(ctx, tx, "recharge", order.WalletCurrency, "payment:"+provider+":"+externalOrderID, "recharge_order", order.ID.String(), "Recharge order paid", nil, map[string]any{"provider": provider, "external_order_id": externalOrderID}, actor, []ledgerPosting{{AccountID: system, Side: "debit", Amount: order.GrantAmount}, {AccountID: wallet.LedgerAccountID, Side: "credit", Amount: order.GrantAmount}})
	if err != nil {
		return RechargeOrder{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO payment_callback_events(provider,event_id,order_id,external_order_id,amount_minor,payload_hash,ledger_transaction_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, provider, eventID, order.ID, externalOrderID, amountMinor, hash[:], ledger.ID)
	if err != nil {
		return RechargeOrder{}, identity.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE recharge_orders SET status='paid',external_order_id=$2,paid_amount_minor=$3,ledger_transaction_id=$4,paid_at=NOW(),updated_at=NOW() WHERE id=$1`, order.ID, externalOrderID, amountMinor, ledger.ID)
	if err != nil {
		return RechargeOrder{}, err
	}
	if err = audit(ctx, tx, actor, "recharge_order.paid", "recharge_order", order.ID.String(), map[string]any{"ledger_transaction_id": ledger.ID, "event_id": eventID}); err != nil {
		return RechargeOrder{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RechargeOrder{}, err
	}
	return s.GetRechargeOrder(ctx, uuid.Nil, order.ID, true)
}

func (s *Service) RefundRecharge(ctx context.Context, orderID uuid.UUID, walletAmount, externalAmount int64, reason, idempotencyKey string, actor identity.Actor) (RechargeRefund, error) {
	reason = strings.TrimSpace(reason)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if walletAmount <= 0 || externalAmount <= 0 || reason == "" || len(reason) > 500 || idempotencyKey == "" || len(idempotencyKey) > 160 {
		return RechargeRefund{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RechargeRefund{}, err
	}
	defer tx.Rollback(ctx)
	fullKey := orderID.String() + ":" + idempotencyKey
	var existingID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM recharge_refunds WHERE idempotency_key=$1`, fullKey).Scan(&existingID)
	if err == nil {
		return scanRefund(tx.QueryRow(ctx, refundSelect+` WHERE id=$1`, existingID))
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RechargeRefund{}, err
	}
	var locked uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT id FROM recharge_orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&locked); err != nil {
		return RechargeRefund{}, notFound(err)
	}
	order, err := s.rechargeOrderTx(ctx, tx, orderID)
	if err != nil {
		return RechargeRefund{}, err
	}
	if order.Status != "paid" && order.Status != "partially_refunded" {
		return RechargeRefund{}, identity.ErrConflict
	}
	var usedWallet, usedExternal int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(wallet_amount),0),COALESCE(SUM(external_amount_minor),0) FROM recharge_refunds WHERE order_id=$1`, orderID).Scan(&usedWallet, &usedExternal); err != nil {
		return RechargeRefund{}, err
	}
	if usedWallet+walletAmount > order.GrantAmount || usedExternal+externalAmount > order.PriceMinor {
		return RechargeRefund{}, identity.ErrConflict
	}
	left := new(big.Int).Mul(big.NewInt(walletAmount), big.NewInt(order.PriceMinor))
	right := new(big.Int).Mul(big.NewInt(externalAmount), big.NewInt(order.GrantAmount))
	if left.Cmp(right) != 0 {
		return RechargeRefund{}, identity.ErrInvalid
	}
	wallet, err := s.ensureWalletTx(ctx, tx, order.AccountID, order.WalletCurrency)
	if err != nil {
		return RechargeRefund{}, err
	}
	balance, err := walletBalanceTx(ctx, tx, wallet.LedgerAccountID)
	if err != nil {
		return RechargeRefund{}, err
	}
	if balance < walletAmount {
		return RechargeRefund{}, identity.ErrForbidden
	}
	system, err := systemLedgerAccountTx(ctx, tx, "cash_clearing", order.WalletCurrency)
	if err != nil {
		return RechargeRefund{}, err
	}
	ledger, err := s.postLedgerTx(ctx, tx, "refund", order.WalletCurrency, "refund:"+fullKey, "recharge_order", order.ID.String(), reason, order.LedgerTransactionID, map[string]any{"external_amount_minor": externalAmount}, actor, []ledgerPosting{{AccountID: wallet.LedgerAccountID, Side: "debit", Amount: walletAmount}, {AccountID: system, Side: "credit", Amount: walletAmount}})
	if err != nil {
		return RechargeRefund{}, err
	}
	id := uuid.New()
	refundNo := "REFUND-" + time.Now().UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
	_, err = tx.Exec(ctx, `INSERT INTO recharge_refunds(id,refund_no,order_id,wallet_amount,external_amount_minor,reason,status,ledger_transaction_id,idempotency_key,actor) VALUES($1,$2,$3,$4,$5,$6,'succeeded',$7,$8,$9)`, id, refundNo, order.ID, walletAmount, externalAmount, reason, ledger.ID, fullKey, actor.Label())
	if err != nil {
		return RechargeRefund{}, identity.ErrConflict
	}
	newStatus := "partially_refunded"
	if usedWallet+walletAmount == order.GrantAmount && usedExternal+externalAmount == order.PriceMinor {
		newStatus = "refunded"
	}
	_, err = tx.Exec(ctx, `UPDATE recharge_orders SET status=$2,updated_at=NOW() WHERE id=$1`, order.ID, newStatus)
	if err != nil {
		return RechargeRefund{}, err
	}
	if err = audit(ctx, tx, actor, "recharge_order.refund", "recharge_order", order.ID.String(), map[string]any{"refund_id": id, "ledger_transaction_id": ledger.ID}); err != nil {
		return RechargeRefund{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RechargeRefund{}, err
	}
	return s.GetRefund(ctx, id)
}

const refundSelect = `SELECT id,refund_no,order_id,wallet_amount,external_amount_minor,reason,status,ledger_transaction_id,actor,created_at FROM recharge_refunds`

func scanRefund(row rowScanner) (RechargeRefund, error) {
	var item RechargeRefund
	err := row.Scan(&item.ID, &item.RefundNo, &item.OrderID, &item.WalletAmount, &item.ExternalAmountMinor, &item.Reason, &item.Status, &item.LedgerTransactionID, &item.Actor, &item.CreatedAt)
	return item, notFound(err)
}
func (s *Service) GetRefund(ctx context.Context, id uuid.UUID) (RechargeRefund, error) {
	return scanRefund(s.db.QueryRow(ctx, refundSelect+` WHERE id=$1`, id))
}

func (s *Service) ListRefunds(ctx context.Context, orderID *uuid.UUID, limit int) ([]RechargeRefund, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, refundSelect+` WHERE ($1::uuid IS NULL OR order_id=$1) ORDER BY created_at DESC LIMIT $2`, uuidQueryValue(orderID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RechargeRefund
	for rows.Next() {
		item, scanErr := scanRefund(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) PurchaseMembership(ctx context.Context, accountID, productID uuid.UUID, idempotencyKey string, actor identity.Actor) (MembershipPurchase, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 160 {
		return MembershipPurchase{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MembershipPurchase{}, err
	}
	defer tx.Rollback(ctx)
	fullKey := accountID.String() + ":" + idempotencyKey
	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM membership_purchases WHERE idempotency_key=$1`, fullKey).Scan(&existing)
	if err == nil {
		return s.membershipPurchaseTx(ctx, tx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MembershipPurchase{}, err
	}
	product, err := scanMembershipProduct(tx.QueryRow(ctx, `SELECT m.id,m.code,m.name,m.plan_id,p.code,p.name,m.duration_days,m.price_amount,m.wallet_currency,m.enabled,m.sort_order,m.revision,m.created_at,m.updated_at FROM membership_products m JOIN membership_plans p ON p.id=m.plan_id WHERE m.id=$1 FOR SHARE OF m,p`, productID))
	if err != nil || !product.Enabled {
		return MembershipPurchase{}, identity.ErrNotFound
	}
	wallet, err := s.ensureWalletTx(ctx, tx, accountID, product.WalletCurrency)
	if err != nil {
		return MembershipPurchase{}, err
	}
	if wallet.Status != "active" {
		return MembershipPurchase{}, identity.ErrForbidden
	}
	balance, err := walletBalanceTx(ctx, tx, wallet.LedgerAccountID)
	if err != nil {
		return MembershipPurchase{}, err
	}
	if balance < product.PriceAmount {
		return MembershipPurchase{}, identity.ErrForbidden
	}
	system, err := systemLedgerAccountTx(ctx, tx, "membership_revenue", product.WalletCurrency)
	if err != nil {
		return MembershipPurchase{}, err
	}
	id := uuid.New()
	ledger, err := s.postLedgerTx(ctx, tx, "membership_purchase", product.WalletCurrency, "membership-purchase:"+fullKey, "membership_purchase", id.String(), "Membership purchase", nil, map[string]any{"product_id": product.ID, "plan_id": product.PlanID}, actor, []ledgerPosting{{AccountID: wallet.LedgerAccountID, Side: "debit", Amount: product.PriceAmount}, {AccountID: system, Side: "credit", Amount: product.PriceAmount}})
	if err != nil {
		return MembershipPurchase{}, err
	}
	membership, err := s.assignMembershipTx(ctx, tx, accountID, product.PlanID, time.Now(), product.DurationDays, "wallet_purchase", id.String(), actor)
	if err != nil {
		return MembershipPurchase{}, err
	}
	purchaseNo := "MEMBER-" + time.Now().UTC().Format("20060102150405") + "-" + strings.ToUpper(uuid.NewString()[:8])
	_, err = tx.Exec(ctx, `INSERT INTO membership_purchases(id,purchase_no,account_id,product_id,membership_id,amount,currency,ledger_transaction_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, purchaseNo, accountID, product.ID, membership.ID, product.PriceAmount, product.WalletCurrency, ledger.ID, fullKey)
	if err != nil {
		return MembershipPurchase{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "membership.purchase", "account", accountID.String(), map[string]any{"purchase_id": id, "ledger_transaction_id": ledger.ID}); err != nil {
		return MembershipPurchase{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MembershipPurchase{}, err
	}
	return s.GetMembershipPurchase(ctx, accountID, id)
}

func (s *Service) membershipPurchaseTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (MembershipPurchase, error) {
	var p MembershipPurchase
	var membershipID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id,purchase_no,account_id,product_id,membership_id,amount,currency,ledger_transaction_id,created_at FROM membership_purchases WHERE id=$1`, id).Scan(&p.ID, &p.PurchaseNo, &p.AccountID, &p.ProductID, &membershipID, &p.Amount, &p.Currency, &p.LedgerTransactionID, &p.CreatedAt)
	if err != nil {
		return MembershipPurchase{}, notFound(err)
	}
	p.Membership, err = scanMembership(tx.QueryRow(ctx, `SELECT m.id,m.account_id,m.plan_id,mp.code,mp.name,m.status,m.starts_at,m.expires_at,m.source,mp.entitlements FROM account_memberships m JOIN membership_plans mp ON mp.id=m.plan_id WHERE m.id=$1`, membershipID))
	return p, err
}
func (s *Service) GetMembershipPurchase(ctx context.Context, accountID, id uuid.UUID) (MembershipPurchase, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MembershipPurchase{}, err
	}
	defer tx.Rollback(ctx)
	item, err := s.membershipPurchaseTx(ctx, tx, id)
	if err != nil {
		return MembershipPurchase{}, err
	}
	if accountID != uuid.Nil && item.AccountID != accountID {
		return MembershipPurchase{}, identity.ErrNotFound
	}
	return item, nil
}
