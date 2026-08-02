package api

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIIsValidYAMLWithRequiredPaths(t *testing.T) {
	var document struct {
		OpenAPI string                 `yaml:"openapi"`
		Paths   map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(OpenAPISpec, &document); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version: %s", document.OpenAPI)
	}
	for _, path := range []string{
		"/health/live", "/health/ready", "/api/v3/system/info", "/openapi.yaml",
		"/api/v3/auth/register", "/api/v3/auth/login", "/api/v3/auth/context", "/api/v3/me",
		"/api/v3/me/telegram/link-requests", "/api/v3/admin/accounts", "/api/v3/admin/accounts/{id}",
		"/api/v3/admin/roles", "/api/v3/admin/permissions", "/api/v3/admin/settings", "/api/v3/admin/credentials",
		"/api/v3/admin/api-clients", "/api/v3/admin/audit", "/open/v1/system/info",
		"/api/v3/membership-plans", "/api/v3/me/membership", "/api/v3/me/invitations/redeem",
		"/api/v3/me/emby-bindings", "/api/v3/me/emby/provision-requests",
		"/api/v3/admin/membership-plans", "/api/v3/admin/invitations", "/api/v3/admin/emby/instances",
		"/api/v3/admin/emby/remote-users", "/api/v3/admin/emby/tasks",
		"/api/v3/recharge-products", "/api/v3/membership-products", "/api/v3/me/wallet",
		"/api/v3/me/recharge-orders", "/api/v3/me/membership-purchases", "/api/v3/me/notifications",
		"/api/v3/admin/recharge-products", "/api/v3/admin/membership-products", "/api/v3/admin/recharge-orders",
		"/api/v3/admin/account-tags", "/api/v3/admin/batch-operations",
		"/api/v3/me/playback/online", "/api/v3/me/playback/history", "/api/v3/me/devices",
		"/api/v3/admin/playback/online", "/api/v3/admin/playback/history", "/api/v3/admin/devices",
		"/api/v3/admin/device-rules", "/api/v3/admin/risk-rules", "/api/v3/admin/risk-events",
		"/api/v3/admin/risk-events/{id}/false-positive",
		"/api/v3/media/search", "/api/v3/media", "/api/v3/me/media-requests", "/api/v3/me/media-requests/{id}",
		"/api/v3/admin/media/{id}/matches", "/api/v3/admin/media-requests", "/api/v3/admin/media-requests/{id}", "/api/v3/admin/media-requests/{id}/moviepilot/resources",
		"/api/v3/admin/media-requests/{id}/moviepilot", "/api/v3/admin/moviepilot-jobs",
		"/api/v3/me/tickets", "/api/v3/me/tickets/{id}/messages", "/api/v3/admin/tickets", "/api/v3/admin/tickets/{id}/messages",
		"/api/v3/reviews", "/api/v3/me/reviews", "/api/v3/admin/reviews", "/api/v3/admin/reviews/{id}/moderation",
		"/api/v3/me/entitlements", "/api/v3/me/entitlements/redeem", "/api/v3/lines", "/api/v3/me/favorites", "/api/v3/me/favorites/sync",
		"/api/v3/reviews/{id}/like", "/api/v3/reviews/{id}/reports", "/api/v3/admin/entitlement-codes", "/api/v3/admin/entitlements",
		"/api/v3/admin/lines", "/api/v3/admin/lines/{id}/probe", "/api/v3/admin/review-reports", "/api/v3/admin/favorites",
		"/api/v3/admin/integration-probes", "/api/v3/admin/integrations/{integration}/probe",
		"/api/v3/me/notification-preferences", "/api/v3/admin/broadcasts", "/api/v3/admin/automation-rules", "/api/v3/admin/automation-executions",
		"/api/v3/me/realtime", "/api/v3/admin/realtime", "/api/v3/internal/bot/actions",
	} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI is missing %s", path)
		}
	}
}
