package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeJSONUsesHeadersAndDoesNotExposeCredentialsInTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-API-KEY") != "secret" {
			t.Fatalf("missing integration headers")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": "test"})
	}))
	defer server.Close()
	result, err := probeJSON(context.Background(), server.URL+"/health", map[string]string{"Authorization": "Bearer secret", "X-API-KEY": "secret"})
	if err != nil || result["ok"] != true {
		t.Fatalf("probe failed: %#v %v", result, err)
	}
	redacted := safeTarget("https://user:password@example.com:8443/path?token=secret")
	if strings.Contains(redacted, "password") || strings.Contains(redacted, "token") || redacted != "https://example.com:8443" {
		t.Fatalf("unsafe diagnostic target: %q", redacted)
	}
}

func TestProbeJSONReportsUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, err := probeJSON(context.Background(), server.URL, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("expected sanitized upstream status, got %v", err)
	}
}
