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

func TestRegisterCommandUsesSharedProvisioningAPI(t *testing.T) {
	var requested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/internal/emby/provision-requests" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer internal-test-token" {
			t.Fatal("missing scoped internal token")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["telegram_user_id"] != float64(42) || body["username"] != "emby_user" || body["invitation_code"] != "invite-code" {
			t.Fatalf("unexpected body: %#v", body)
		}
		requested.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"task": map[string]any{"id": "task-1", "status": "pending"}, "username": "emby_user"})
	}))
	defer server.Close()
	bot := New(Config{InternalAPIURL: server.URL, InternalAPIToken: "internal-test-token", RequestTimeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	result, err := bot.requestProvision(context.Background(), 42, "tester", "emby_user", "invite-code", "", "same-request")
	if err != nil || !requested.Load() || result.Task.ID != "task-1" {
		t.Fatalf("result=%+v requested=%v err=%v", result, requested.Load(), err)
	}
}

func TestTelegramNotificationDeliveryUsesInternalOutbox(t *testing.T) {
	var sent, completed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/internal/notifications/telegram/next":
			if r.Header.Get("Authorization") != "Bearer internal-test-token" {
				t.Fatal("missing internal authorization")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"notification_id": "notification-1", "telegram_user_id": 42, "title": "Notice", "body": "Hello"})
		case strings.Contains(r.URL.Path, "/sendMessage"):
			sent.Store(true)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case strings.HasSuffix(r.URL.Path, "/notification-1/complete"):
			completed.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	bot := New(Config{APIBase: server.URL, InternalAPIURL: server.URL, InternalAPIToken: "internal-test-token", RequestTimeout: time.Second}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worked, err := bot.deliverNextNotification(context.Background(), "telegram-token")
	if err != nil || !worked || !sent.Load() || !completed.Load() {
		t.Fatalf("worked=%v sent=%v completed=%v err=%v", worked, sent.Load(), completed.Load(), err)
	}
}
