// Package api exposes the HTTP surface used by the SvelteKit frontend.
//
// Routes are versioned under /api so the frontend assets (served separately
// or embedded) can live at the root without colliding.
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aperture/aperture/internal/alerts"
	"github.com/aperture/aperture/internal/hub"
	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	hub       *hub.Hub
	evaluator *alerts.Evaluator
}

func NewServer(h *hub.Hub, ev *alerts.Evaluator) *Server {
	return &Server{hub: h, evaluator: ev}
}

// Router builds the chi router. webFS may be nil during early development
// (in which case only /api/* is served and the SvelteKit dev server runs
// separately on another port).
func (s *Server) Router(webFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(corsForDev)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", s.health)
		r.Get("/hosts", s.listHosts)
		r.Get("/hosts/{id}", s.getHost)
		r.Get("/hosts/{id}/metrics/latest", s.latestMetric)
		r.Get("/hosts/{id}/metrics", s.metricsRange)
		r.Get("/hosts/{id}/containers", s.listContainers)
		r.Post("/hosts/{id}/containers/{cid}/{action}", s.containerAction)
		r.Delete("/hosts/{id}/containers/{cid}", s.containerRemove)
		r.Get("/hosts/{id}/containers/{cid}/logs", s.containerLogs)

		r.Get("/alerts/metadata", s.alertsMetadata)
		r.Get("/alerts/rules", s.listAlertRules)
		r.Post("/alerts/rules", s.createAlertRule)
		r.Get("/alerts/rules/{id}", s.getAlertRule)
		r.Put("/alerts/rules/{id}", s.updateAlertRule)
		r.Delete("/alerts/rules/{id}", s.deleteAlertRule)
		r.Get("/alerts/events", s.listAlertEvents)
	})

	if webFS != nil {
		r.Handle("/*", spaHandler(webFS))
	}
	return r
}

// spaHandler serves files from the built SvelteKit output. Any path that
// doesn't resolve to a file falls back to index.html so client-side
// routing works on direct page loads / refreshes.
func spaHandler(webFS fs.FS) http.Handler {
	fileSrv := http.FileServer(http.FS(webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, err := fs.Stat(webFS, clean); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileSrv.ServeHTTP(w, r2)
			return
		}
		fileSrv.ServeHTTP(w, r)
	})
}

// --- handlers ---

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().UTC()})
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.hub.Store().ListHosts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := s.hub.Store().GetHost(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if host == nil {
		writeErr(w, http.StatusNotFound, errors.New("host not found"))
		return
	}
	writeJSON(w, http.StatusOK, host)
}

func (s *Server) latestMetric(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := s.hub.Store().LatestMetric(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if m == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) metricsRange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()
	dur := parseDuration(q.Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	maxPoints, _ := strconv.Atoi(q.Get("points"))
	if maxPoints == 0 {
		maxPoints = 300
	}
	ms, err := s.hub.Store().MetricsRange(r.Context(), id, since, until, maxPoints)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ms == nil {
		ms = []types.MetricSample{}
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	all := r.URL.Query().Get("all") == "true"
	cs, err := d.List(r.Context(), all)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if cs == nil {
		cs = []types.Container{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) containerAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	action := chi.URLParam(r, "action")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var err error
	switch action {
	case "start":
		err = d.Start(r.Context(), cid)
	case "stop":
		err = d.Stop(r.Context(), cid, nil)
	case "restart":
		err = d.Restart(r.Context(), cid, nil)
	case "pause":
		err = d.Pause(r.Context(), cid)
	case "unpause":
		err = d.Unpause(r.Context(), cid)
	case "kill":
		sig := r.URL.Query().Get("signal")
		err = d.Kill(r.Context(), cid, sig)
	default:
		writeErr(w, http.StatusBadRequest, errors.New("unknown action: "+action))
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) containerRemove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	q := r.URL.Query()
	force := q.Get("force") == "true"
	volumes := q.Get("volumes") == "true"
	if err := d.Remove(r.Context(), cid, force, volumes); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) containerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail == 0 {
		tail = 200
	}
	logs, err := d.Logs(r.Context(), cid, tail)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(logs))
}

// --- alerts ---

func (s *Server) alertsMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": alerts.SupportedMetrics,
		"ops":     alerts.SupportedOps,
	})
}

func (s *Server) listAlertRules(w http.ResponseWriter, r *http.Request) {
	var hostFilter *string
	if v := r.URL.Query().Get("host_id"); v != "" {
		hostFilter = &v
	}
	rules, err := s.hub.Store().ListAlertRules(r.Context(), hostFilter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if rules == nil {
		rules = []types.AlertRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// alertRulePayload is what create/update accept on the wire. Mirrors
// types.AlertRule but with HostID as a regular string for JSON simplicity:
// empty string means "all hosts".
type alertRulePayload struct {
	HostID    string  `json:"host_id"`
	Metric    string  `json:"metric"`
	Op        string  `json:"op"`
	Threshold float64 `json:"threshold"`
	DurationS int     `json:"duration_s"`
	Enabled   *bool   `json:"enabled"`
}

func (p alertRulePayload) toRule(id int64) types.AlertRule {
	r := types.AlertRule{
		ID:        id,
		Metric:    p.Metric,
		Op:        p.Op,
		Threshold: p.Threshold,
		DurationS: p.DurationS,
		Enabled:   true,
	}
	if p.HostID != "" {
		hid := p.HostID
		r.HostID = &hid
	}
	if p.Enabled != nil {
		r.Enabled = *p.Enabled
	}
	return r
}

func (s *Server) createAlertRule(w http.ResponseWriter, r *http.Request) {
	var p alertRulePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rule := p.toRule(0)
	if err := alerts.ValidateRule(rule); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	id, err := s.hub.Store().CreateAlertRule(r.Context(), rule)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Read back so created_at (DB default) is populated in the response.
	created, err := s.hub.Store().GetAlertRule(r.Context(), id)
	if err != nil || created == nil {
		rule.ID = id
		writeJSON(w, http.StatusCreated, rule)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rule, err := s.hub.Store().GetAlertRule(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if rule == nil {
		writeErr(w, http.StatusNotFound, errors.New("rule not found"))
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) updateAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var p alertRulePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rule := p.toRule(id)
	if err := alerts.ValidateRule(rule); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.hub.Store().UpdateAlertRule(r.Context(), rule); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := s.hub.Store().GetAlertRule(r.Context(), id)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusOK, rule)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.hub.Store().DeleteAlertRule(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.evaluator != nil {
		s.evaluator.HandleRuleDelete(id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listAlertEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AlertEventFilter{
		HostID:   q.Get("host_id"),
		OpenOnly: q.Get("open") == "true",
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	events, err := s.hub.Store().ListAlertEvents(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if events == nil {
		events = []types.AlertEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

// corsForDev relaxes CORS so the SvelteKit dev server (default :5173) can
// hit /api directly during development. In production the SPA is served
// from the same origin and this becomes a no-op for same-origin requests.
func corsForDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (strings.HasPrefix(origin, "http://localhost") || strings.HasPrefix(origin, "http://127.0.0.1")) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
