package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadinessReportsDependencyFailure(t *testing.T) {
	mux := http.NewServeMux()
	New("api", time.Second,
		Probe{Name: "database", Check: func(_ context.Context) error { return nil }},
		Probe{Name: "redis", Check: func(_ context.Context) error { return errors.New("down") }},
	).Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
