package platform

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/migrate"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

func TestStage4LedgerCommerceAndBatchAcceptance(t *testing.T) {
	databaseURL := os.Getenv("SAKURA_V3_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SAKURA_V3_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "commerce_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	}()
	parsed, _ := url.Parse(databaseURL)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	testURL := parsed.String()
	if err = migrate.New(testURL, logger).Run(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	vault, _ := security.NewVault("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	identities := identity.New(pool, time.Hour, vault)
	suffix := schema[len(schema)-8:]
	if err = identities.BootstrapOwner(ctx, "owner_"+suffix, "Owner-password-123"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := identities.AuthenticateLocal(ctx, "owner_"+suffix, "Owner-password-123", "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := identities.AuthenticateSession(ctx, ownerSession.Token)
	if err != nil {
		t.Fatal(err)
	}
	actor := principal.Actor
	first, err := identities.RegisterLocal(ctx, "buyer_"+suffix, "Buyer-password-123", "Buyer", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.RegisterLocal(ctx, "second_"+suffix, "Second-password-123", "Second", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identities.PutCredential(ctx, "payment.webhook.testpay", "payment_webhook", "webhook-secret", nil, actor); err != nil {
		t.Fatal(err)
	}
	service := New(pool, vault)
	if !service.AuthorizePaymentProvider(ctx, "testpay", "webhook-secret") || service.AuthorizePaymentProvider(ctx, "testpay", "wrong") {
		t.Fatal("payment provider credential authorization failed")
	}

	recharge, err := service.SaveRechargeProduct(ctx, nil, "points_1000", "1000 Points", "", 1000, 1000, "CNY", "POINTS", true, 10, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.CreateRechargeOrder(ctx, first.ID, recharge.ID, "testpay", "create-order-1", identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	paid, err := service.ConfirmRecharge(ctx, "testpay", "event-1", order.OrderNo, "external-1", 1000, []byte(`{"event":"event-1"}`))
	if err != nil || paid.Status != "paid" {
		t.Fatalf("payment: %+v %v", paid, err)
	}
	replayed, err := service.ConfirmRecharge(ctx, "testpay", "event-1", order.OrderNo, "external-1", 1000, []byte(`{"event":"event-1"}`))
	if err != nil || replayed.LedgerTransactionID == nil || *replayed.LedgerTransactionID != *paid.LedgerTransactionID {
		t.Fatalf("callback replay: %+v %v", replayed, err)
	}
	if _, err = service.ConfirmRecharge(ctx, "testpay", "event-2", order.OrderNo, "external-1", 1000, []byte(`{"event":"event-2"}`)); err != nil {
		t.Fatal(err)
	}
	var rechargeTransactions, callbackEvents int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM ledger_transactions WHERE kind='recharge'`).Scan(&rechargeTransactions); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM payment_callback_events WHERE order_id=$1`, order.ID).Scan(&callbackEvents); err != nil {
		t.Fatal(err)
	}
	if rechargeTransactions != 1 || callbackEvents != 2 {
		t.Fatalf("recharge transactions=%d callback events=%d", rechargeTransactions, callbackEvents)
	}

	adjustment, err := service.AdjustWallet(ctx, first.ID, 200, "POINTS", "service compensation", "adjust-1", actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.LedgerInvariant(ctx, adjustment.ID); err != nil {
		t.Fatal(err)
	}
	plans, err := service.ListPlans(ctx, true)
	if err != nil || len(plans) == 0 {
		t.Fatal("default membership plan missing")
	}
	membershipProduct, err := service.SaveMembershipProduct(ctx, nil, "member_30", "30 Day Membership", plans[0].ID, 30, 300, "POINTS", true, 10, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	purchase, err := service.PurchaseMembership(ctx, first.ID, membershipProduct.ID, "purchase-1", identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	purchaseReplay, err := service.PurchaseMembership(ctx, first.ID, membershipProduct.ID, "purchase-1", identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil || purchaseReplay.LedgerTransactionID != purchase.LedgerTransactionID {
		t.Fatalf("purchase replay: %+v %v", purchaseReplay, err)
	}
	refund, err := service.RefundRecharge(ctx, order.ID, 100, 100, "partial refund", "refund-1", actor)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.LedgerInvariant(ctx, refund.LedgerTransactionID); err != nil {
		t.Fatal(err)
	}
	wallet, err := service.Wallet(ctx, first.ID, "POINTS")
	if err != nil || wallet.Balance != 800 {
		t.Fatalf("wallet=%+v err=%v", wallet, err)
	}

	var unbalanced int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM (SELECT transaction_id,SUM(amount) FILTER(WHERE side='debit') debit,SUM(amount) FILTER(WHERE side='credit') credit FROM ledger_entries GROUP BY transaction_id HAVING SUM(amount) FILTER(WHERE side='debit')<>SUM(amount) FILTER(WHERE side='credit')) x`).Scan(&unbalanced); err != nil || unbalanced != 0 {
		t.Fatalf("unbalanced=%d err=%v", unbalanced, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE ledger_entries SET amount=amount+1 WHERE transaction_id=$1`, adjustment.ID); err == nil {
		t.Fatal("immutable ledger entry accepted an update")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO ledger_transactions(id,transaction_no,idempotency_key,kind,currency,status,actor,posted_at) VALUES($1,$2,$3,'admin_adjustment','POINTS','posted','test',NOW())`, uuid.New(), "INVALID-"+uuid.NewString(), "invalid-"+uuid.NewString()); err == nil {
		t.Fatal("ledger accepted a directly posted transaction")
	}
	var balanceColumn int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='wallets' AND column_name='balance'`).Scan(&balanceColumn); err != nil || balanceColumn != 0 {
		t.Fatal("wallets must not contain an editable balance column")
	}

	tag, err := service.SaveTag(ctx, nil, "vip", "VIP", "#7c3aed", "High value user", actor)
	if err != nil {
		t.Fatal(err)
	}
	tagBatch, err := service.CreateBatch(ctx, "tag_add", BatchTarget{AccountIDs: []uuid.UUID{first.ID, second.ID}}, map[string]any{"tag_id": tag.ID.String()}, "batch-tag-1", 3, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.PauseBatch(ctx, tagBatch.ID, actor); err != nil {
		t.Fatal(err)
	}
	worked, err := service.ProcessNextBatch(ctx, "test-worker", time.Minute)
	if err != nil || worked {
		t.Fatalf("paused batch worked=%v err=%v", worked, err)
	}
	if _, err = service.ResumeBatch(ctx, tagBatch.ID, actor); err != nil {
		t.Fatal(err)
	}
	if worked, err = service.ProcessNextBatch(ctx, "test-worker", time.Minute); err != nil || !worked {
		t.Fatalf("tag batch worked=%v err=%v", worked, err)
	}
	completed, err := service.GetBatch(ctx, tagBatch.ID)
	if err != nil || completed.Status != "succeeded" || completed.SucceededCount != 2 {
		t.Fatalf("tag batch=%+v err=%v", completed, err)
	}
	var assignments int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM account_tag_assignments WHERE tag_id=$1`, tag.ID).Scan(&assignments); err != nil || assignments != 2 {
		t.Fatalf("tag assignments=%d err=%v", assignments, err)
	}

	notificationBatch, err := service.CreateBatch(ctx, "notification", BatchTarget{AccountIDs: []uuid.UUID{first.ID, second.ID}}, map[string]any{"title": "Maintenance", "body": "Service window", "channel": "in_app"}, "batch-notify-1", 3, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ProcessNextBatch(ctx, "test-worker", time.Minute); err != nil {
		t.Fatal(err)
	}
	notificationDone, err := service.GetBatch(ctx, notificationBatch.ID)
	if err != nil || notificationDone.Status != "succeeded" {
		t.Fatalf("notification batch=%+v err=%v", notificationDone, err)
	}
	notifications, err := service.ListNotifications(ctx, second.ID, "unread", 10)
	if err != nil || len(notifications) != 1 {
		t.Fatalf("notifications=%+v err=%v", notifications, err)
	}
	link, err := identities.StartTelegramLink(ctx, second.ID, identity.Actor{Kind: "account", ID: second.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if err = identities.ConfirmTelegramLink(ctx, link.Code, 99887766, "batch_user", identity.Actor{Kind: "service", ID: "test"}); err != nil {
		t.Fatal(err)
	}
	telegramBatch, err := service.CreateBatch(ctx, "notification", BatchTarget{AccountIDs: []uuid.UUID{second.ID}}, map[string]any{"title": "Telegram Notice", "body": "Delivered by Bot", "channel": "telegram"}, "batch-telegram-1", 3, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ProcessNextBatch(ctx, "test-worker", time.Minute); err != nil {
		t.Fatal(err)
	}
	delivery, ok, err := service.ClaimTelegramNotification(ctx, "telegram-bot", time.Minute)
	if err != nil || !ok || delivery.TelegramUserID != 99887766 {
		t.Fatalf("delivery=%+v ok=%v err=%v", delivery, ok, err)
	}
	if err = service.CompleteTelegramNotification(ctx, delivery.NotificationID, "telegram-bot", nil); err != nil {
		t.Fatal(err)
	}
	telegramDone, err := service.GetBatch(ctx, telegramBatch.ID)
	if err != nil || telegramDone.Status != "succeeded" {
		t.Fatalf("telegram batch=%+v err=%v", telegramDone, err)
	}

	membershipBatch, err := service.CreateBatch(ctx, "membership_adjust", BatchTarget{AccountIDs: []uuid.UUID{second.ID}}, map[string]any{"plan_id": plans[0].ID.String(), "duration_days": 15}, "batch-membership-1", 3, actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE membership_plans SET enabled=FALSE WHERE id=$1`, plans[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ProcessNextBatch(ctx, "test-worker", time.Minute); err != nil {
		t.Fatal(err)
	}
	failed, err := service.GetBatch(ctx, membershipBatch.ID)
	if err != nil || failed.Status != "failed" {
		t.Fatalf("failed batch=%+v err=%v", failed, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE membership_plans SET enabled=TRUE WHERE id=$1`, plans[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RetryBatch(ctx, membershipBatch.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ProcessNextBatch(ctx, "test-worker", time.Minute); err != nil {
		t.Fatal(err)
	}
	retried, err := service.GetBatch(ctx, membershipBatch.ID)
	if err != nil || retried.Status != "succeeded" {
		t.Fatalf("retried batch=%+v err=%v", retried, err)
	}
	if _, err = service.CurrentMembership(ctx, second.ID); err != nil {
		t.Fatalf("batch membership missing: %v", err)
	}
	var audits int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'batch.%'`).Scan(&audits); err != nil || audits < 6 {
		t.Fatalf("batch audits=%d err=%v", audits, err)
	}
}
