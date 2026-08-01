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
	"regexp"
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

type fakeEmby struct {
	mu            sync.Mutex
	users         map[string]map[string]any
	creates       int
	failPasswords int
}

func (f *fakeEmby) handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Emby-Token") != "emby-secret" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/emby/System/Info":
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "fake-server", "ServerName": "Fake", "Version": "4.9.5"})
	case r.Method == http.MethodGet && r.URL.Path == "/emby/Users":
		items := make([]map[string]any, 0, len(f.users))
		for _, user := range f.users {
			items = append(items, user)
		}
		_ = json.NewEncoder(w).Encode(items)
	case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/New":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.creates++
		id := "created-" + uuid.NewString()
		user := map[string]any{"Id": id, "Name": body["Name"], "Policy": map[string]any{"IsDisabled": false}}
		f.users[id] = user
		_ = json.NewEncoder(w).Encode(user)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/Password"):
		if f.failPasswords > 0 {
			f.failPasswords--
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/Policy"):
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-2]
		var policy map[string]any
		_ = json.NewDecoder(r.Body).Decode(&policy)
		if user := f.users[id]; user != nil {
			user["Policy"] = policy
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func TestStage3ProvisionRetryMultiInstanceImportAndClaim(t *testing.T) {
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
	schema := "platform_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
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
	identityService := identity.New(pool, time.Hour, vault)
	ownerName := "owner_" + schema[len(schema)-8:]
	if err = identityService.BootstrapOwner(ctx, ownerName, "Owner-password-123"); err != nil {
		t.Fatal(err)
	}
	ownerSession, err := identityService.AuthenticateLocal(ctx, ownerName, "Owner-password-123", "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := identityService.AuthenticateSession(ctx, ownerSession.Token)
	if err != nil {
		t.Fatal(err)
	}
	actor := owner.Actor
	account, err := identityService.RegisterLocal(ctx, "member_"+schema[len(schema)-8:], "Member-password-123", "Member", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	claimAccount, err := identityService.RegisterLocal(ctx, "claim_"+schema[len(schema)-8:], "Claim-password-123", "Claim", identity.Actor{Kind: "anonymous", ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identityService.PutCredential(ctx, "emby.test", "emby_api_token", "emby-secret", nil, actor); err != nil {
		t.Fatal(err)
	}
	service := New(pool, vault)
	plans, err := service.ListPlans(ctx, true)
	if err != nil || len(plans) == 0 {
		t.Fatalf("plans: %v", err)
	}
	if _, err = service.AssignMembership(ctx, account.ID, plans[0].ID, time.Now(), 90, "test", "", actor); err != nil {
		t.Fatal(err)
	}
	invitations, err := service.GenerateInvitations(ctx, plans[0].ID, "registration", 1, 1, nil, actor)
	if err != nil || len(invitations) != 1 || !regexp.MustCompile(`^[A-Z0-9]+-[0-9]+-Register_[A-Za-z0-9]+$`).MatchString(invitations[0].Code) {
		t.Fatalf("TG-compatible invitation: %+v %v", invitations, err)
	}
	if _, err = service.RedeemInvitation(ctx, claimAccount.ID, invitations[0].Code, "claim-membership", identity.Actor{Kind: "account", ID: claimAccount.ID.String()}); err != nil {
		t.Fatal(err)
	}

	fake := &fakeEmby{users: map[string]map[string]any{"legacy-1": {"Id": "legacy-1", "Name": "legacy_user", "Policy": map[string]any{"IsDisabled": false}}}, failPasswords: 1}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()
	instance, err := service.SaveInstance(ctx, nil, "Primary", server.URL, "emby.test", true, true, true, 10, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	request, err := service.RequestProvisioning(ctx, account.ID, &instance.ID, "new_user", "", "web-request-1", actor)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.RequestProvisioning(ctx, account.ID, &instance.ID, "new_user", "", "web-request-1", actor)
	if err != nil || replay.Task.ID != request.Task.ID {
		t.Fatalf("idempotent replay: %+v %v", replay, err)
	}
	worker := NewWorker(pool, vault, logger, "test-worker", time.Millisecond, time.Minute)
	worked, err := worker.ProcessNext(ctx)
	if !worked || err != nil {
		t.Fatalf("first process: worked=%v err=%v", worked, err)
	}
	first, _ := service.GetTask(ctx, request.Task.ID)
	if first.Status != "retry" {
		t.Fatalf("expected retry, got %s", first.Status)
	}
	_, _ = pool.Exec(ctx, `UPDATE platform_tasks SET available_at=NOW() WHERE id=$1`, request.Task.ID)
	worked, err = worker.ProcessNext(ctx)
	if !worked || err != nil {
		t.Fatalf("retry process: worked=%v err=%v", worked, err)
	}
	completed, err := service.GetProvisioning(ctx, account.ID, request.Task.ID)
	if err != nil || completed.Task.Status != "succeeded" || completed.GeneratedPassword == "" {
		t.Fatalf("completed: %+v %v", completed, err)
	}
	fake.mu.Lock()
	creates := fake.creates
	fake.mu.Unlock()
	if creates != 1 {
		t.Fatalf("external retry created %d users", creates)
	}

	secondFake := &fakeEmby{users: map[string]map[string]any{}}
	secondServer := httptest.NewServer(http.HandlerFunc(secondFake.handler))
	defer secondServer.Close()
	second, err := service.SaveInstance(ctx, nil, "Secondary", secondServer.URL, "emby.test", true, false, true, 20, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := service.RequestProvisioning(ctx, account.ID, &second.ID, "new_user_2", "", "web-request-2", actor)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err = worker.ProcessNext(ctx); !worked || err != nil {
		t.Fatalf("second instance process: %v %v", worked, err)
	}
	secondDone, _ := service.GetProvisioning(ctx, account.ID, secondRequest.Task.ID)
	if secondDone.Task.Status != "succeeded" {
		t.Fatalf("second instance status %s", secondDone.Task.Status)
	}
	bindings, err := service.ListBindings(ctx, &account.ID, nil, 10)
	if err != nil || len(bindings) != 2 {
		t.Fatalf("multi instance bindings=%d err=%v", len(bindings), err)
	}

	importTask, err := service.EnqueueInstanceTask(ctx, "emby.import", instance.ID, "manual-import-1", actor)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err = worker.ProcessNext(ctx); !worked || err != nil {
		t.Fatalf("import process: %v %v", worked, err)
	}
	importDone, _ := service.GetTask(ctx, importTask.ID)
	if importDone.Status != "succeeded" {
		t.Fatalf("import status %s", importDone.Status)
	}
	remote, err := service.ListRemoteUsers(ctx, &instance.ID, "unclaimed", 10)
	if err != nil || len(remote) != 1 || remote[0].RemoteUserID != "legacy-1" {
		t.Fatalf("remote import: %+v %v", remote, err)
	}
	token, err := service.GenerateClaimToken(ctx, remote[0].ID, time.Hour, actor)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.ClaimRemoteUser(ctx, claimAccount.ID, token, identity.Actor{Kind: "account", ID: claimAccount.ID.String()})
	if err != nil || claimed.RemoteUserID != "legacy-1" {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	snapshots, err := service.ListSnapshots(ctx, instance.ID, 10)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("snapshots: %v %v", snapshots, err)
	}
}
