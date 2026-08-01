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
		"/api/v3/auth/register", "/api/v3/auth/login", "/api/v3/me",
		"/api/v3/me/telegram/link-requests", "/api/v3/admin/accounts", "/api/v3/admin/accounts/{id}",
		"/api/v3/admin/roles", "/api/v3/admin/permissions", "/api/v3/admin/settings", "/api/v3/admin/credentials",
		"/api/v3/admin/api-clients", "/api/v3/admin/audit", "/open/v1/system/info",
	} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI is missing %s", path)
		}
	}
}
