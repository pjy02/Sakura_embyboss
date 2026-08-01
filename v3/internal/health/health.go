package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

type Probe struct {
	Name  string
	Check func(context.Context) error
}

type Handler struct {
	service string
	timeout time.Duration
	probes  []Probe
}

func New(service string, timeout time.Duration, probes ...Probe) *Handler {
	cloned := append([]Probe(nil), probes...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].Name < cloned[j].Name })
	return &Handler{service: service, timeout: timeout, probes: cloned}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/live", h.live)
	mux.HandleFunc("GET /health/ready", h.ready)
}

func (h *Handler) live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":  "alive",
		"service": h.service,
	})
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	components := make(map[string]string, len(h.probes))
	ready := true
	for _, probe := range h.probes {
		ctx, cancel := context.WithTimeout(request.Context(), h.timeout)
		err := probe.Check(ctx)
		cancel()
		if err != nil {
			components[probe.Name] = "unavailable"
			ready = false
			continue
		}
		components[probe.Name] = "ready"
	}
	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	writeJSON(writer, status, map[string]any{
		"status":     state,
		"service":    h.service,
		"components": components,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
