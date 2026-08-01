package telegrambot

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBindCommandUsesInternalAPIWithoutDatabase(t *testing.T) {
	var confirmed atomic.Bool
	var sent atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getUpdates"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{map[string]any{
				"update_id": 1,
				"message": map[string]any{
					"chat": map[string]any{"id": 9, "type": "private"},
					"from": map[string]any{"id": 42, "username": "tester"},
					"text": "/bind one-time-code",
				},
			}}})
		case strings.Contains(r.URL.Path, "/link-requests/confirm"):
			if r.Header.Get("Authorization") != "Bearer internal-test-token" {
				t.Errorf("unexpected internal authorization")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["code"] != "one-time-code" {
				t.Errorf("binding code missing from request body")
			}
			confirmed.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/sendMessage"):
			sent.Store(true)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	bot := New(Config{
		APIBase:          server.URL,
		InternalAPIURL:   server.URL,
		InternalAPIToken: "internal-test-token",
		BotToken:         "telegram-test-token",
		RequestTimeout:   time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := bot.Run(ctx); err != nil {
		t.Fatalf("run bot: %v", err)
	}
	if !confirmed.Load() || !sent.Load() {
		t.Fatalf("confirmed=%v sent=%v", confirmed.Load(), sent.Load())
	}
}
