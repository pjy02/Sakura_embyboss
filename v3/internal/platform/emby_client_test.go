package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestEmbyClientUsesTokenAndSupportsUsers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /prefix/emby/System/Info", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "secret" {
			t.Fatal("missing token header")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "server-1", "ServerName": "Test", "Version": "4.9"})
	})
	mux.HandleFunc("GET /prefix/emby/Users", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "u1", "Name": "alice", "Policy": map[string]any{"IsDisabled": false}}})
	})
	mux.HandleFunc("POST /prefix/emby/Users/New", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "u2", "Name": "bob"})
	})
	mux.HandleFunc("GET /prefix/emby/Sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ActiveWithinSeconds") != "90" {
			t.Fatalf("unexpected session query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "s1", "UserId": "u1", "Client": "Emby Web", "NowPlayingItem": map[string]any{"Id": "m1", "Name": "Movie"}}})
	})
	mux.HandleFunc("GET /prefix/emby/Items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("AnyProviderIdEquals") != "tmdb.123" || r.URL.Query().Get("IncludeItemTypes") != "Movie" {
			t.Fatalf("unexpected media query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": []any{map[string]any{"Id": "m123", "Name": "Matched", "Type": "Movie"}}})
	})
	mux.HandleFunc("POST /prefix/emby/Sessions/s1/Playing/Stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client, err := newEmbyClient(EmbyInstance{ID: uuid.New(), BaseURL: server.URL + "/prefix", VerifyTLS: true}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	info, _, err := client.probe(context.Background())
	if err != nil || info.ID != "server-1" {
		t.Fatalf("probe failed: %#v %v", info, err)
	}
	users, err := client.users(context.Background())
	if err != nil || len(users) != 1 || users[0].Name != "alice" {
		t.Fatalf("users failed: %#v %v", users, err)
	}
	created, err := client.createUser(context.Background(), "bob")
	if err != nil || created.ID != "u2" {
		t.Fatalf("create failed: %#v %v", created, err)
	}
	sessions, err := client.sessions(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("sessions failed: %#v %v", sessions, err)
	}
	if err = client.stopSession(context.Background(), "s1"); err != nil {
		t.Fatalf("stop session failed: %v", err)
	}
	items, err := client.mediaByTMDB(context.Background(), 123, "movie")
	if err != nil || len(items) != 1 || items[0]["Id"] != "m123" {
		t.Fatalf("media match failed: %#v %v", items, err)
	}
}
