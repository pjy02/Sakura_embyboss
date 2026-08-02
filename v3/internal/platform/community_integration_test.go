package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/migrate"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

func TestStage6MediaCommunityNotificationAndAutomationAcceptance(t *testing.T) {
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
	schema := "community_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
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
	ownerName := "owner_" + suffix
	if err = identities.BootstrapOwner(ctx, ownerName, "Owner-password-123"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := identities.AuthenticateLocal(ctx, ownerName, "Owner-password-123", "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := identities.AuthenticateSession(ctx, ownerSession.Token)
	if err != nil {
		t.Fatal(err)
	}
	first, err := identities.RegisterLocal(ctx, "viewer_"+suffix, "Viewer-password-123", "Viewer", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.RegisterLocal(ctx, "requester_"+suffix, "Requester-password-123", "Requester", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := identities.StartTelegramLink(ctx, first.ID, identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if err = identities.ConfirmTelegramLink(ctx, link.Code, 66554433, "stage6_user", identity.Actor{Kind: "service", ID: "test"}); err != nil {
		t.Fatal(err)
	}
	for name, credential := range map[string][2]string{
		"tmdb.api_token":       {"api_token", "tmdb-secret"},
		"moviepilot.api_token": {"api_token", "moviepilot-secret"},
		"emby.stage6":          {"emby_api_token", "emby-secret"},
	} {
		if _, err = identities.PutCredential(ctx, name, credential[0], credential[1], nil, owner.Actor); err != nil {
			t.Fatal(err)
		}
	}

	var moviePilotSubmits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/3/search/multi":
			if r.Header.Get("Authorization") != "Bearer tmdb-secret" || r.URL.Query().Get("query") != "星际" {
				http.Error(w, "bad tmdb request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 101, "media_type": "movie", "title": "星际远航", "release_date": "2026-01-02", "popularity": 99.0, "vote_average": 8.8},
				map[string]any{"id": 102, "media_type": "tv", "name": "星际新章", "first_air_date": "2026-02-03", "popularity": 80.0, "vote_average": 8.1},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/embyserver/emby/Items":
			if r.Header.Get("X-Emby-Token") != "emby-secret" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			items := []any{}
			if r.URL.Query().Get("AnyProviderIdEquals") == "tmdb.101" {
				items = append(items, map[string]any{"Id": "emby-101", "Name": "星际远航", "Type": "Movie"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": items})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/search/title":
			if r.Header.Get("Authorization") != "Bearer moviepilot-secret" || r.URL.Query().Get("keyword") != "星际新章" {
				http.Error(w, "bad moviepilot search", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{map[string]any{"meta_info": map[string]any{"title": "星际新章"}, "torrent_info": map[string]any{"site": "test", "title": "Star.New.Chapter", "size": 12345}}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/download/add":
			moviePilotSubmits.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("Authorization") != "Bearer moviepilot-secret" || numberValue(body["tmdb_id"]) != 102 || body["torrent_in"] == nil {
				http.Error(w, "bad moviepilot submit", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"download_id": "download-102"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	setStringSetting(t, ctx, pool, "tmdb.api_base_url", upstream.URL)
	setStringSetting(t, ctx, pool, "moviepilot.api_base_url", upstream.URL)

	service := New(pool, vault)
	instance, err := service.SaveInstance(ctx, nil, "Stage 6 Emby", upstream.URL+"/embyserver", "emby.stage6", true, true, true, 10, 0, owner.Actor)
	if err != nil {
		t.Fatal(err)
	}
	media, err := service.SearchTMDB(ctx, "星际", 1)
	if err != nil || len(media) != 2 || media[0].ID == uuid.Nil || media[0].ExternalID != 101 {
		t.Fatalf("TMDB title search must return internal selectable media: %+v %v", media, err)
	}
	byExternalID := map[int64]Media{}
	for _, item := range media {
		byExternalID[item.ExternalID] = item
	}

	rule, err := service.SaveAutomationRule(ctx, nil, AutomationRule{
		Code: "notify_requester", Name: "Notify requester", TriggerEvent: "media_request.created",
		Conditions: map[string]any{"account_id": second.ID.String()},
		Actions:    []map[string]any{{"type": "notify_account", "account_id": "$event.account_id", "event_key": "automation.notification", "title": "求片已登记", "body": "自动化规则已处理"}},
		Enabled:    true, Priority: 10,
	}, 0, owner.Actor)
	if err != nil {
		t.Fatal(err)
	}

	missingRequest, err := service.CreateMediaRequest(ctx, first.ID, byExternalID[102].ID, "需要中文字幕", identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	duplicateRequest, err := service.CreateMediaRequest(ctx, second.ID, byExternalID[102].ID, "同求", identity.Actor{Kind: "account", ID: second.ID.String()})
	if err != nil || !duplicateRequest.Duplicate || duplicateRequest.ID != missingRequest.ID || duplicateRequest.SubscriberCount != 2 {
		t.Fatalf("duplicate request was not merged: %+v %v", duplicateRequest, err)
	}
	availableRequest, err := service.CreateMediaRequest(ctx, second.ID, byExternalID[101].ID, "check library", identity.Actor{Kind: "account", ID: second.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	resources, err := service.SearchMoviePilot(ctx, missingRequest.ID)
	if err != nil || len(resources) != 1 {
		t.Fatalf("MoviePilot search: %+v %v", resources, err)
	}
	job, err := service.SubmitMoviePilot(ctx, missingRequest.ID, resources[0], "submit-102", owner.Actor)
	if err != nil {
		t.Fatal(err)
	}
	replayedJob, err := service.SubmitMoviePilot(ctx, missingRequest.ID, resources[0], "different-key", owner.Actor)
	if err != nil || !replayedJob.Duplicate || replayedJob.ID != job.ID {
		t.Fatalf("duplicate MoviePilot job was not recognized: %+v %v", replayedJob, err)
	}

	ticket, err := service.CreateTicket(ctx, first.ID, "播放问题", "playback", "normal", "无法播放", identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.AddTicketMessage(ctx, ticket.ID, owner.AccountID, "仅管理员可见的诊断信息", true, true, owner.Actor); err != nil {
		t.Fatal(err)
	}
	if _, err = service.AddTicketMessage(ctx, ticket.ID, owner.AccountID, "请重新登录后测试", false, true, owner.Actor); err != nil {
		t.Fatal(err)
	}
	userMessages, err := service.ListTicketMessages(ctx, ticket.ID, &first.ID, true)
	if err != nil || len(userMessages) != 2 {
		t.Fatalf("user ticket view: %+v %v", userMessages, err)
	}
	for _, message := range userMessages {
		if message.Internal || strings.Contains(message.Body, "诊断信息") {
			t.Fatal("internal ticket note leaked to the user")
		}
	}
	adminMessages, err := service.ListTicketMessages(ctx, ticket.ID, nil, true)
	if err != nil || len(adminMessages) != 3 {
		t.Fatalf("admin ticket view: %+v %v", adminMessages, err)
	}
	if _, err = service.AddTicketMessage(ctx, ticket.ID, &first.ID, "try internal", true, false, identity.Actor{Kind: "account", ID: first.ID.String()}); !errors.Is(err, identity.ErrInvalid) {
		t.Fatalf("user internal note was accepted: %v", err)
	}

	delivery, ok, err := service.ClaimTelegramNotification(ctx, "bot-stage6", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim Telegram notification: %+v %v", delivery, err)
	}
	if err = service.CompleteTelegramNotification(ctx, delivery.NotificationID, "bot-stage6", errors.New("temporary Telegram outage")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE account_notifications SET next_delivery_at=NOW() WHERE id=$1`, delivery.NotificationID); err != nil {
		t.Fatal(err)
	}
	retryDelivery, ok, err := service.ClaimTelegramNotification(ctx, "bot-stage6", time.Minute)
	if err != nil || !ok || retryDelivery.NotificationID != delivery.NotificationID {
		t.Fatalf("failed notification was not retried: %+v %v", retryDelivery, err)
	}
	if err = service.CompleteTelegramNotification(ctx, retryDelivery.NotificationID, "bot-stage6", nil); err != nil {
		t.Fatal(err)
	}
	var deliveryStatus string
	var deliveryAttempts int
	if err = pool.QueryRow(ctx, `SELECT delivery_status,delivery_attempts FROM account_notifications WHERE id=$1`, delivery.NotificationID).Scan(&deliveryStatus, &deliveryAttempts); err != nil || deliveryStatus != "sent" || deliveryAttempts != 2 {
		t.Fatalf("notification status=%s attempts=%d err=%v", deliveryStatus, deliveryAttempts, err)
	}

	review, err := service.SubmitReview(ctx, first.ID, byExternalID[101].ID, 9, "值得看", "完整影评内容", false, identity.Actor{Kind: "account", ID: first.ID.String()})
	if err != nil || review.Status != "pending" {
		t.Fatalf("review submit: %+v %v", review, err)
	}
	availableMediaID := byExternalID[101].ID
	if public, listErr := service.ListReviews(ctx, &availableMediaID, "", false, 10); listErr != nil || len(public) != 0 {
		t.Fatalf("pending review became public: %+v %v", public, listErr)
	}
	review, err = service.ModerateReview(ctx, review.ID, "approved", "内容合规", review.Revision, owner.Actor)
	if err != nil || review.Status != "approved" {
		t.Fatalf("review moderation: %+v %v", review, err)
	}

	worker := NewWorker(pool, vault, logger, "stage6-worker", time.Millisecond, time.Minute)
	for index := 0; index < 3; index++ {
		worked, processErr := worker.ProcessNext(ctx)
		if processErr != nil || !worked {
			t.Fatalf("platform task %d: worked=%v err=%v", index, worked, processErr)
		}
	}
	matched, err := service.GetMediaRequest(ctx, availableRequest.ID, nil)
	if err != nil || matched.Status != "completed" || !matched.Media.Available {
		t.Fatalf("Emby media was not matched: %+v %v", matched, err)
	}
	downloaded, err := service.GetMoviePilotJob(ctx, job.ID)
	if err != nil || downloaded.Status != "submitted" || downloaded.ExternalJobID != "download-102" || moviePilotSubmits.Load() != 1 {
		t.Fatalf("MoviePilot job=%+v submits=%d err=%v", downloaded, moviePilotSubmits.Load(), err)
	}
	missingRequest, err = service.GetMediaRequest(ctx, missingRequest.ID, nil)
	if err != nil || missingRequest.Status != "downloading" {
		t.Fatalf("request did not follow MoviePilot state: %+v %v", missingRequest, err)
	}

	for index := 0; index < 20; index++ {
		worked, processErr := service.ProcessNextAutomation(ctx, "stage6-worker", time.Minute)
		if processErr != nil {
			t.Fatal(processErr)
		}
		if !worked {
			break
		}
	}
	executions, err := service.ListAutomationExecutions(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	var succeeded bool
	for _, execution := range executions {
		if execution.RuleID == rule.ID && execution.Status == "succeeded" {
			succeeded = true
		}
	}
	if !succeeded {
		t.Fatalf("automation rule did not execute: %+v", executions)
	}

	if _, err = service.SetNotificationPreference(ctx, second.ID, "broadcast.general", "in_app", false, identity.Actor{Kind: "account", ID: second.ID.String()}); err != nil {
		t.Fatal(err)
	}
	broadcast, err := service.CreateBroadcast(ctx, "维护通知", "今晚维护", "broadcast.general", "in_app", BatchTarget{AccountIDs: []uuid.UUID{second.ID}}, "stage6-broadcast", owner.Actor)
	if err != nil {
		t.Fatal(err)
	}
	if worked, processErr := service.ProcessNextBatch(ctx, "stage6-worker", time.Minute); processErr != nil || !worked {
		t.Fatalf("broadcast process: %v %v", worked, processErr)
	}
	var broadcastNotifications int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM account_notifications WHERE batch_operation_id=$1`, broadcast.BatchOperation.ID).Scan(&broadcastNotifications); err != nil || broadcastNotifications != 0 {
		t.Fatalf("disabled broadcast preference produced %d notifications: %v", broadcastNotifications, err)
	}
	items, err := service.ListBatchItems(ctx, broadcast.BatchOperation.ID, "", 10)
	if err != nil || len(items) != 1 || items[0].Result["notification_skipped"] != true {
		t.Fatalf("broadcast preference skip was not recorded: %+v %v", items, err)
	}

	matches, err := service.ListMediaMatches(ctx, byExternalID[101].ID)
	if err != nil || len(matches) != 1 || matches[0].InstanceID != instance.ID || matches[0].Status != "matched" {
		t.Fatalf("media match state: %+v %v", matches, err)
	}
}

func setStringSetting(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key, value string) {
	t.Helper()
	raw, _ := json.Marshal(value)
	if _, err := pool.Exec(ctx, `UPDATE dynamic_settings SET value=$2::jsonb,revision=revision+1,updated_at=NOW() WHERE key=$1`, key, string(raw)); err != nil {
		t.Fatal(err)
	}
}
