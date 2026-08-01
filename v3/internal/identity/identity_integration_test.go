package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pjy02/Sakura_embyboss/v3/internal/httpapi"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
	"github.com/pjy02/Sakura_embyboss/v3/internal/migrate"
	"github.com/pjy02/Sakura_embyboss/v3/internal/security"
)

func TestStage2AcceptanceFlow(t *testing.T) {
	databaseURL := os.Getenv("SAKURA_V3_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SAKURA_V3_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	suffix := uuid.NewString()[:8]
	schema := "identity_test_" + suffix
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect for test schema: %v", err)
	}
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatalf("create test schema: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	}()
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	testDatabaseURL := parsedURL.String()
	if err = migrate.New(testDatabaseURL, logger).Run(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, testDatabaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	vault, err := security.NewVault("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	service := identity.New(pool, time.Hour, vault)
	ownerUsername := "owner_" + suffix
	ownerPassword := "Owner-pass-" + suffix
	if err = service.BootstrapOwner(ctx, ownerUsername, ownerPassword); err != nil {
		t.Fatalf("bootstrap owner: %v", err)
	}
	ownerSession, err := service.AuthenticateLocal(ctx, ownerUsername, ownerPassword, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("owner login: %v", err)
	}
	ownerPrincipal, err := service.AuthenticateSession(ctx, ownerSession.Token)
	if err != nil || !ownerPrincipal.HasPermission("roles.write") {
		t.Fatalf("owner RBAC was not loaded: principal=%+v err=%v", ownerPrincipal, err)
	}
	actor := ownerPrincipal.Actor

	username := "web_" + suffix
	password := "User-pass-" + suffix
	account, err := service.RegisterLocal(ctx, username, password, "Web user", identity.Actor{Kind: "anonymous", ID: "registration"})
	if err != nil {
		t.Fatalf("Web registration without Telegram: %v", err)
	}
	for _, item := range account.Identities {
		if item.Kind == "telegram" {
			t.Fatal("Web registration unexpectedly required Telegram")
		}
	}
	userSession, err := service.AuthenticateLocal(ctx, username, password, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("local login: %v", err)
	}
	userPrincipal, err := service.AuthenticateSession(ctx, userSession.Token)
	if err != nil || userPrincipal.HasPermission("accounts.read") {
		t.Fatalf("regular user has unexpected management access: %+v err=%v", userPrincipal, err)
	}

	link, err := service.StartTelegramLink(ctx, account.ID, userPrincipal.Actor)
	if err != nil {
		t.Fatalf("start Telegram link: %v", err)
	}
	if err = service.ConfirmTelegramLink(ctx, link.Code, 987654321, "linked_user", identity.Actor{Kind: "service", ID: "telegram-bot"}); err != nil {
		t.Fatalf("confirm Telegram link: %v", err)
	}
	linked, err := service.GetAccount(ctx, account.ID)
	if err != nil || !hasIdentity(linked, "telegram", "987654321") {
		t.Fatalf("Telegram identity was not bound: %+v err=%v", linked, err)
	}

	settings, err := service.ListSettings(ctx)
	if err != nil {
		t.Fatalf("list settings: %v", err)
	}
	registration := findSetting(t, settings, "auth.local_registration_enabled")
	updated, err := service.UpdateSetting(ctx, registration.Key, false, "boolean", "acceptance test", registration.Revision, actor)
	if err != nil {
		t.Fatalf("versioned setting update: %v", err)
	}
	rolledBack, err := service.RollbackSetting(ctx, registration.Key, registration.Revision, updated.Revision, "acceptance rollback", actor)
	if err != nil || rolledBack.Value != true {
		t.Fatalf("setting rollback: value=%v err=%v", rolledBack.Value, err)
	}

	roleCode := "support_" + suffix
	if _, err = service.SaveRole(ctx, roleCode, "Support", []string{"audit.read"}, actor); err != nil {
		t.Fatalf("save custom role: %v", err)
	}
	assigned, err := service.AssignRoles(ctx, account.ID, []string{roleCode}, actor)
	if err != nil || len(assigned.Roles) != 1 || assigned.Roles[0] != roleCode {
		t.Fatalf("assign role: %+v err=%v", assigned, err)
	}
	assignedPrincipal, err := service.AuthenticateSession(ctx, userSession.Token)
	if err != nil || !assignedPrincipal.HasPermission("audit.read") || assignedPrincipal.HasPermission("accounts.read") {
		t.Fatalf("custom RBAC permissions were not applied: %+v err=%v", assignedPrincipal, err)
	}
	if _, err = service.AssignRoles(ctx, ownerSession.Account.ID, []string{"user"}, actor); err != identity.ErrForbidden {
		t.Fatalf("last owner safety returned %v", err)
	}
	if _, err = service.ChangeLifecycle(ctx, ownerSession.Account.ID, "suspended", "safety test", actor); err != identity.ErrForbidden {
		t.Fatalf("last active owner lifecycle safety returned %v", err)
	}

	credentialName := "test.secret." + suffix
	if _, err = service.PutCredential(ctx, credentialName, "test", "top-secret", map[string]any{"purpose": "test"}, actor); err != nil {
		t.Fatalf("put credential: %v", err)
	}
	credentials, err := service.ListCredentials(ctx)
	if err != nil || !credentialIsMasked(credentials, credentialName) {
		t.Fatalf("credential metadata is not masked: %+v err=%v", credentials, err)
	}
	revealed, err := service.RevealCredential(ctx, credentialName, identity.Actor{Kind: "service", ID: "test"})
	if err != nil || revealed != "top-secret" {
		t.Fatalf("reveal credential: %q err=%v", revealed, err)
	}

	client, err := service.CreateAPIClient(ctx, "test-"+suffix, []string{"system:read"}, actor)
	if err != nil || client.Token == "" {
		t.Fatalf("create API client: %+v err=%v", client, err)
	}
	apiPrincipal, err := service.AuthenticateAPIClient(ctx, client.Token)
	if err != nil || !apiPrincipal.HasScope("system:read") {
		t.Fatalf("API scope was not enforced: %+v err=%v", apiPrincipal, err)
	}

	handler := httpapi.New(httpapi.Options{Logger: logger, Identity: service, SessionCookie: "test_session", InternalBotToken: "internal-test-token"})
	webUsername := "browser_" + suffix
	registerBody, _ := json.Marshal(map[string]any{"username": webUsername, "password": "Browser-pass-" + suffix, "display_name": "Browser user"})
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v3/auth/register", bytes.NewReader(registerBody))
	registerResponse := httptest.NewRecorder()
	handler.ServeHTTP(registerResponse, registerRequest)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("Web register returned %d: %s", registerResponse.Code, registerResponse.Body.String())
	}
	loginBody, _ := json.Marshal(map[string]any{"username": webUsername, "password": "Browser-pass-" + suffix})
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v3/auth/login", bytes.NewReader(loginBody))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK || len(loginResponse.Result().Cookies()) != 2 {
		t.Fatalf("Web login did not create session and CSRF cookies: status=%d cookies=%d body=%s", loginResponse.Code, len(loginResponse.Result().Cookies()), loginResponse.Body.String())
	}
	assertStatus(t, handler, http.MethodGet, "/api/v3/admin/accounts", nil, userSession.Token, "", http.StatusForbidden)
	assertStatus(t, handler, http.MethodGet, "/api/v3/admin/accounts", nil, ownerSession.Token, "", http.StatusOK)
	assertStatus(t, handler, http.MethodPut, "/api/v3/admin/accounts/"+account.ID.String()+"/roles", map[string]any{"roles": []string{"user"}}, ownerSession.Token, "", http.StatusForbidden)

	logs, err := service.Audit(ctx, 200)
	if err != nil || len(logs) == 0 {
		t.Fatalf("audit trail is empty: err=%v", err)
	}
}

func hasIdentity(account identity.Account, kind, subject string) bool {
	for _, item := range account.Identities {
		if item.Kind == kind && item.Subject == subject {
			return true
		}
	}
	return false
}

func findSetting(t *testing.T, settings []identity.Setting, key string) identity.Setting {
	t.Helper()
	for _, item := range settings {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("setting %s not found", key)
	return identity.Setting{}
}

func credentialIsMasked(items []identity.Credential, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return item.Masked == "********"
		}
	}
	return false
}

func assertStatus(t *testing.T, handler http.Handler, method, path string, body any, cookie, csrf string, expected int) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		payload = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, path, payload)
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: "test_session", Value: cookie})
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != expected {
		t.Fatalf("%s %s returned %d, expected %d: %s", method, path, response.Code, expected, response.Body.String())
	}
}
