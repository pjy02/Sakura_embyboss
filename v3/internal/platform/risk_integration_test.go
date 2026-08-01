package platform

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestStage5PlaybackDeviceRiskAcceptance(t *testing.T) {
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
	schema := "risk_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
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
	account, err := identities.RegisterLocal(ctx, "viewer_"+suffix, "Viewer-password-123", "Viewer", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	link, err := identities.StartTelegramLink(ctx, account.ID, identity.Actor{Kind: "account", ID: account.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if err = identities.ConfirmTelegramLink(ctx, link.Code, 55667788, "risk_viewer", identity.Actor{Kind: "service", ID: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err = identities.PutCredential(ctx, "emby.risk-test", "emby_api_token", "emby-secret", nil, actor); err != nil {
		t.Fatal(err)
	}

	goodSessionCounter := 0
	remoteDisabled := false
	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "emby-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Sessions":
			goodSessionCounter++
			_ = json.NewEncoder(w).Encode([]any{playbackFixture("worker-session", "worker-item", "Emby Web")})
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "remote-1", "Name": "viewer", "Policy": map[string]any{"IsDisabled": remoteDisabled}}})
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/remote-1/Policy":
			var policy map[string]any
			if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			remoteDisabled, _ = policy["IsDisabled"].(bool)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer goodServer.Close()
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer badServer.Close()

	service := New(pool, vault)
	good, err := service.SaveInstance(ctx, nil, "good", goodServer.URL, "emby.risk-test", true, true, true, 10, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := service.SaveInstance(ctx, nil, "bad", badServer.URL, "emby.risk-test", true, false, true, 20, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	bindingID := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO emby_account_bindings(id,account_id,instance_id,remote_user_id,remote_username,status,origin,is_primary,remote_disabled,remote_snapshot,claimed_at,last_synced_at) VALUES($1,$2,$3,'remote-1','viewer','active','test',TRUE,FALSE,'{}',NOW(),NOW())`, bindingID, account.ID, good.ID); err != nil {
		t.Fatal(err)
	}
	deny, err := service.SaveDeviceRule(ctx, nil, DeviceRule{Name: "Block BadPlayer", Decision: "deny", MatchField: "client_name", MatchOperator: "exact", MatchValue: "BadPlayer", Action: "disable_user", Enabled: true, Priority: 20}, 0, actor)
	if err != nil || deny.ID == uuid.Nil {
		t.Fatalf("deny rule: %+v %v", deny, err)
	}
	observe, err := service.SaveRiskRule(ctx, nil, RiskRule{Code: "observe-concurrency", Name: "Observe concurrency", RuleType: "concurrent_streams", Condition: map[string]any{"threshold": 1}, Severity: "medium", Action: "disable_user", ObservationMode: true, Enabled: true, CooldownSeconds: 300}, 0, actor)
	if err != nil || !observe.ObservationMode {
		t.Fatalf("risk rule: %+v %v", observe, err)
	}
	snapshot := []embyPlaybackSession{decodePlaybackFixture(playbackFixture("session-1", "item-1", "BadPlayer"))}
	result, err := service.IngestPlaybackSnapshot(ctx, good, snapshot, identity.Actor{Kind: "system", ID: "test-worker"})
	if err != nil || result["risk_events_created"] != 2 || result["actions_queued"] != 1 {
		t.Fatalf("ingest result=%+v err=%v", result, err)
	}
	if _, err = service.IngestPlaybackSnapshot(ctx, good, snapshot, identity.Actor{Kind: "system", ID: "test-worker"}); err != nil {
		t.Fatal(err)
	}
	var events, actions, alerts int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM risk_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM risk_actions`).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM account_notifications WHERE channel='telegram' AND metadata->>'kind'='risk_alert'`).Scan(&alerts); err != nil {
		t.Fatal(err)
	}
	if events != 2 || actions != 1 || alerts != 2 {
		t.Fatalf("events=%d actions=%d alerts=%d", events, actions, alerts)
	}
	devices, err := service.ListDevices(ctx, &good.ID, &account.ID, "denied", 10)
	if err != nil || len(devices) != 1 || devices[0].SessionCount != 1 {
		t.Fatalf("devices=%+v err=%v", devices, err)
	}
	history, err := service.ListPlaybackHistory(ctx, &good.ID, &account.ID, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	riskEvents, err := service.ListRiskEvents(ctx, &good.ID, &account.ID, "", "", 10)
	if err != nil || len(riskEvents) != 2 {
		t.Fatalf("risk events=%+v err=%v", riskEvents, err)
	}
	var enforced RiskEvent
	for _, event := range riskEvents {
		if !event.ObservationMode {
			enforced = event
		}
	}
	if enforced.ID == uuid.Nil {
		t.Fatal("enforced event missing")
	}
	falsePositive, err := service.MarkRiskFalsePositive(ctx, enforced.ID, "confirmed shared household device", actor)
	if err != nil || falsePositive.Event.Status != "false_positive" || len(falsePositive.Actions) != 1 || falsePositive.Actions[0].Status != "canceled" {
		t.Fatalf("false positive=%+v err=%v", falsePositive, err)
	}

	if _, err = service.SaveDeviceRule(ctx, nil, DeviceRule{Name: "Allow BadPlayer for migration", Decision: "allow", MatchField: "client_name", MatchOperator: "exact", MatchValue: "BadPlayer", Action: "none", Enabled: true, Priority: 1}, 0, actor); err != nil {
		t.Fatal(err)
	}
	allowedSnapshot := []embyPlaybackSession{decodePlaybackFixture(playbackFixture("session-2", "item-2", "BadPlayer"))}
	if _, err = service.IngestPlaybackSnapshot(ctx, good, allowedSnapshot, identity.Actor{Kind: "system", ID: "test-worker"}); err != nil {
		t.Fatal(err)
	}
	online, err := service.ListPlaybackSessions(ctx, &good.ID, &account.ID, 10)
	if err != nil || len(online) != 1 || online[0].DeviceDecision != "allowed" {
		t.Fatalf("allowlist online=%+v err=%v", online, err)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM risk_actions`).Scan(&actions); err != nil || actions != 1 {
		t.Fatalf("allowlisted playback created an action: %d %v", actions, err)
	}
	if _, err = service.SaveDeviceRule(ctx, nil, DeviceRule{Name: "Enforce blocked client", Decision: "deny", MatchField: "client_name", MatchOperator: "exact", MatchValue: "EnforcePlayer", Action: "disable_user", Enabled: true, Priority: 2}, 0, actor); err != nil {
		t.Fatal(err)
	}
	enforcedSnapshot := []embyPlaybackSession{decodePlaybackFixture(playbackFixture("session-3", "item-3", "EnforcePlayer"))}
	if _, err = service.IngestPlaybackSnapshot(ctx, good, enforcedSnapshot, identity.Actor{Kind: "system", ID: "test-worker"}); err != nil {
		t.Fatal(err)
	}
	enforcedEvents, err := service.ListRiskEvents(ctx, &good.ID, &account.ID, "open", "high", 10)
	if err != nil || len(enforcedEvents) != 1 {
		t.Fatalf("enforced events=%+v err=%v", enforcedEvents, err)
	}
	worker := NewWorker(pool, vault, logger, "risk-test-worker", time.Millisecond, time.Minute)
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("automatic action worked=%v err=%v", worked, processErr)
	}
	actionDone, err := service.GetRiskEvent(ctx, enforcedEvents[0].ID)
	if err != nil || len(actionDone.Actions) != 1 || actionDone.Actions[0].Status != "succeeded" || !remoteDisabled {
		t.Fatalf("automatic action=%+v disabled=%v err=%v", actionDone, remoteDisabled, err)
	}
	revertQueued, err := service.MarkRiskFalsePositive(ctx, enforcedEvents[0].ID, "operator verified this device", actor)
	if err != nil || revertQueued.Actions[0].Status != "revert_pending" {
		t.Fatalf("revert queue=%+v err=%v", revertQueued, err)
	}
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("revert action worked=%v err=%v", worked, processErr)
	}
	reverted, err := service.GetRiskEvent(ctx, enforcedEvents[0].ID)
	if err != nil || reverted.Actions[0].Status != "reverted" || remoteDisabled {
		t.Fatalf("reverted=%+v disabled=%v err=%v", reverted, remoteDisabled, err)
	}

	badTask, err := service.EnqueueInstanceTask(ctx, "emby.playback_sync", bad.ID, "isolation-bad", actor)
	if err != nil {
		t.Fatal(err)
	}
	goodTask, err := service.EnqueueInstanceTask(ctx, "emby.playback_sync", good.ID, "isolation-good", actor)
	if err != nil {
		t.Fatal(err)
	}
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("bad instance task cycle worked=%v err=%v", worked, processErr)
	}
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("good instance task cycle worked=%v err=%v", worked, processErr)
	}
	badState, err := service.GetTask(ctx, badTask.ID)
	if err != nil || badState.Status != "retry" {
		t.Fatalf("bad task=%+v err=%v", badState, err)
	}
	goodState, err := service.GetTask(ctx, goodTask.ID)
	if err != nil || goodState.Status != "succeeded" || goodSessionCounter != 1 {
		t.Fatalf("good task=%+v calls=%d err=%v", goodState, goodSessionCounter, err)
	}
}

func playbackFixture(sessionID, itemID, client string) map[string]any {
	return map[string]any{
		"Id": sessionID, "UserId": "remote-1", "UserName": "viewer", "Client": client,
		"DeviceName": "Living Room", "DeviceId": "device-1", "RemoteEndPoint": "203.0.113.8:443", "DeviceType": "tvOS",
		"NowPlayingItem": map[string]any{"Id": itemID, "Name": "Movie", "Type": "Movie", "RunTimeTicks": 7_200_000_000},
		"PlayState":      map[string]any{"PlayMethod": "DirectPlay", "PositionTicks": 100_000_000, "IsPaused": false},
	}
}

func decodePlaybackFixture(raw map[string]any) embyPlaybackSession {
	body, _ := json.Marshal(raw)
	var session embyPlaybackSession
	_ = json.Unmarshal(body, &session)
	session.Raw = raw
	return session
}
