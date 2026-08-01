package httpapi

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEveryAdminRouteDeclaresAPermission(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve source path")
	}
	file, err := os.Open(filepath.Join(filepath.Dir(filename), "identity_routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `mux.Handle("`) && strings.Contains(line, `/api/v3/admin/`) {
			count++
			if !strings.Contains(line, `session(o, "`) || strings.Contains(line, `session(o, "",`) {
				t.Fatalf("admin route does not declare a permission: %s", strings.TrimSpace(line))
			}
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 10 {
		t.Fatalf("expected the admin permission guard to inspect all routes, got %d", count)
	}
}

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
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s is missing security headers", test.path)
		}
	}
}
