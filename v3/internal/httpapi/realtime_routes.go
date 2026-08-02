package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func registerRealtimeRoutes(mux *http.ServeMux, o Options) {
	mux.Handle("GET /api/v3/me/realtime", session(o, "", false, func(w http.ResponseWriter, r *http.Request, p identity.Principal) {
		streamSnapshots(w, r, func() (any, error) { return o.Platform.UserRealtime(r.Context(), *p.AccountID) })
	}))
	mux.Handle("GET /api/v3/admin/realtime", session(o, "dashboard.read", false, func(w http.ResponseWriter, r *http.Request, _ identity.Principal) {
		streamSnapshots(w, r, func() (any, error) { return o.Platform.AdminRealtime(r.Context()) })
	}))
}

func streamSnapshots(w http.ResponseWriter, r *http.Request, snapshot func() (any, error)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respond(w, 0, nil, fmt.Errorf("streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		value, err := snapshot()
		if err != nil {
			body, _ := json.Marshal(map[string]string{"message": "snapshot temporarily unavailable"})
			_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", body)
		} else {
			body, _ := json.Marshal(value)
			_, _ = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", body)
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
