package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSystemInfoAndOpenAPI(t *testing.T) {
	handler := New(Options{
		Environment:  "test",
		Version:      "test-version",
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ProbeTimeout: time.Second,
	})

	for _, test := range []struct {
		path        string
		contentType string
		body        string
	}{
		{"/api/v3/system/info", "application/json", "test-version"},
		{"/openapi.yaml", "application/yaml", "openapi: 3.1.0"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", test.path, response.Code)
		}
		if !strings.Contains(response.Header().Get("Content-Type"), test.contentType) {
			t.Fatalf("%s has wrong content type", test.path)
		}
		if !strings.Contains(response.Body.String(), test.body) {
			t.Fatalf("%s response does not contain %q", test.path, test.body)
		}
	}
}
