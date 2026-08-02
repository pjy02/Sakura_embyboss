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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/migrate"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

func TestRuntimeCompletionEntitlementsLinesReviewsFavoritesAndProbes(t *testing.T) {
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
	schema := "completion_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
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
	ids := identity.New(pool, time.Hour, vault)
	ownerName := "owner_" + schema[len(schema)-8:]
	if err = ids.BootstrapOwner(ctx, ownerName, "Owner-password-123"); err != nil {
		t.Fatal(err)
	}
	session, err := ids.AuthenticateLocal(ctx, ownerName, "Owner-password-123", "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := ids.AuthenticateSession(ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	actor := owner.Actor
	member, err := ids.RegisterLocal(ctx, "member_"+schema[len(schema)-8:], "Member-password-123", "Member", identity.Actor{Kind: "test", ID: "register"})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := ids.RegisterLocal(ctx, "reporter_"+schema[len(schema)-8:], "Reporter-password-123", "Reporter", identity.Actor{Kind: "test", ID: "register"})
	if err != nil {
		t.Fatal(err)
	}

	type fakeState struct {
		mu       sync.Mutex
		policy   map[string]any
		favorite bool
	}
	state := &fakeState{policy: map[string]any{"IsDisabled": false, "EnableAllFolders": false, "EnabledFolders": []any{"manual-library"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/emby/") && r.Header.Get("X-Emby-Token") != "emby-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/emby/System/Info":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "server-1", "ServerName": "Completion", "Version": "4.9.5"})
		case r.Method == http.MethodGet && r.URL.Path == "/emby/System/Info/Public":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "server-1", "ServerName": "Completion", "Version": "4.9.5"})
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "remote-1", "Name": "member", "Policy": state.policy}})
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/remote-1/Policy":
			_ = json.NewDecoder(r.Body).Decode(&state.policy)
			w.WriteHeader(http.StatusNoContent)
		case (r.Method == http.MethodPost || r.Method == http.MethodDelete) && r.URL.Path == "/emby/Users/remote-1/FavoriteItems/item-1":
			state.favorite = r.Method == http.MethodPost
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/remote-1/Items":
			items := []any{}
			if state.favorite {
				items = append(items, map[string]any{"Id": "item-1", "Name": "Movie One", "Type": "Movie", "ImageTags": map[string]any{"Primary": "tag-1"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Items": items})
		case r.URL.Path == "/3/configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"images": map[string]any{"base_url": "https://image.tmdb.org"}})
		case r.URL.Path == "/moviepilot/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "version": "2"})
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": 42, "username": "sakura_test_bot"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, credential := range []struct{ name, kind, secret string }{{"emby.test", "emby_api_token", "emby-secret"}, {"tmdb.api_token", "api_token", "tmdb-secret"}, {"moviepilot.api_token", "api_token", "moviepilot-secret"}, {"telegram.bot_token", "telegram_bot_token", "telegram-secret"}} {
		if _, err = ids.PutCredential(ctx, credential.name, credential.kind, credential.secret, nil, actor); err != nil {
			t.Fatal(err)
		}
	}
	service := New(pool, vault)
	instance, err := service.SaveInstance(ctx, nil, "Completion", server.URL, "emby.test", true, true, true, 10, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	bindingID := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO emby_account_bindings(id,account_id,instance_id,remote_user_id,remote_username,status,origin,is_primary) VALUES($1,$2,$3,'remote-1','member','active','remote_import',TRUE)`, bindingID, member.ID, instance.ID); err != nil {
		t.Fatal(err)
	}

	codes, err := service.GenerateEntitlementCodes(ctx, &instance.ID, "emby_library", "library-1", 30, 1, map[string]any{"source": "test"}, actor)
	if err != nil || len(codes) != 1 || codes[0].Code == "" {
		t.Fatalf("generate codes: %+v %v", codes, err)
	}
	grant, err := service.RedeemEntitlementCode(ctx, member.ID, codes[0].Code, identity.Actor{Kind: "account", ID: member.ID.String()})
	if err != nil || grant.BindingID == nil {
		t.Fatalf("redeem entitlement: %+v %v", grant, err)
	}
	worker := NewWorker(pool, vault, logger, "completion-worker", time.Millisecond, time.Minute)
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("entitlement worker: %v %v", worked, processErr)
	}
	state.mu.Lock()
	folders := policyFolders(state.policy["EnabledFolders"])
	all := state.policy["EnableAllFolders"]
	state.mu.Unlock()
	if all != false || strings.Join(folders, ",") != "library-1,manual-library" {
		t.Fatalf("entitlement policy not applied: %#v", state.policy)
	}
	if _, err = service.RevokeEntitlement(ctx, grant.ID, "completion test", actor); err != nil {
		t.Fatal(err)
	}
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("entitlement revoke worker: %v %v", worked, processErr)
	}
	state.mu.Lock()
	folders = policyFolders(state.policy["EnabledFolders"])
	state.mu.Unlock()
	if strings.Join(folders, ",") != "manual-library" {
		t.Fatalf("entitlement sync removed unmanaged folders: %#v", state.policy)
	}

	line, err := service.SaveLine(ctx, nil, "Primary line", server.URL, "test", "local", "all", 100, 0, true, false, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	sample, err := service.ProbeLine(ctx, line.ID, actor)
	if err != nil || sample.Status == "unhealthy" {
		t.Fatalf("line probe: %+v %v", sample, err)
	}
	mediaID := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO media_catalog(id,external_id,media_type,title) VALUES($1,100,'movie','Movie One')`, mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dynamic_settings SET value='false'::jsonb WHERE key='reviews.require_moderation'`); err != nil {
		t.Fatal(err)
	}
	review, err := service.SubmitReview(ctx, member.ID, mediaID, 9, "Good", "A useful review", false, identity.Actor{Kind: "account", ID: member.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	review, err = service.SetReviewLike(ctx, review.ID, reporter.ID, true, identity.Actor{Kind: "account", ID: reporter.ID.String()})
	if err != nil || review.LikeCount != 1 {
		t.Fatalf("review like: %+v %v", review, err)
	}
	report, err := service.ReportReview(ctx, review.ID, reporter.ID, "other", "moderator check", identity.Actor{Kind: "account", ID: reporter.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ResolveReviewReport(ctx, report.ID, "dismissed", "not a violation", actor); err != nil {
		t.Fatal(err)
	}

	if _, err = service.SetFavorite(ctx, member.ID, bindingID, "item-1", "Movie One", "Movie", &mediaID, true, identity.Actor{Kind: "account", ID: member.ID.String()}); err != nil {
		t.Fatal(err)
	}
	if worked, processErr := worker.ProcessNext(ctx); !worked || processErr != nil {
		t.Fatalf("favorite worker: %v %v", worked, processErr)
	}
	state.mu.Lock()
	favorite := state.favorite
	state.mu.Unlock()
	if !favorite {
		t.Fatal("favorite was not sent to Emby")
	}

	settings := map[string]string{"tmdb.api_base_url": server.URL, "moviepilot.api_base_url": server.URL, "moviepilot.health_path": "/moviepilot/health", "telegram.api_base_url": server.URL}
	for key, value := range settings {
		encoded, _ := json.Marshal(value)
		if _, err = pool.Exec(ctx, `INSERT INTO dynamic_settings(key,value,value_type,updated_by) VALUES($1,$2,'string','test') ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`, key, encoded); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"emby", "tmdb", "moviepilot", "telegram"} {
		var target *uuid.UUID
		if kind == "emby" {
			target = &instance.ID
		}
		probe, probeErr := service.ProbeIntegration(ctx, kind, target, actor)
		if probeErr != nil || probe.Status != "healthy" {
			t.Fatalf("%s probe: %+v %v", kind, probe, probeErr)
		}
	}
}
