// Package api — monitoring spine.
//
// The endpoints in this file aggregate what the dashboard and host-detail
// page need into single calls, replacing the old N+1 fan-out (list hosts +
// latest-per-host + containers-per-host + 7 history calls per range change).
// Frontend code uses these as the primary monitoring data source; the older
// per-host endpoints stay for the few cases that need a single slice.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aperture/aperture/internal/alerts"
	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
)

// monitoringOverview is the response shape for GET /api/monitoring/overview.
// One request returns everything the dashboard needs for first paint plus
// the data it would otherwise poll for: hosts + latest sample + container
// counts + open alert counts + status. After this, the dashboard relies on
// SSE for live updates and a slow (30s) reconciliation re-fetch.
type monitoringOverview struct {
	Hosts      []types.Host                  `json:"hosts"`
	Latest     map[string]*types.MetricSample `json:"latest"`
	Containers map[string]containerCountsDTO  `json:"containers"`
	OpenAlerts map[string]int                 `json:"openAlerts"`
	Status     map[string]string              `json:"status"`
	Ts         int64                          `json:"ts"`
}

// containerCountsDTO is the wire shape for per-host container summary.
// Separate from hub.ContainerCounts to avoid leaking the hub type across
// package boundaries via the API.
type containerCountsDTO struct {
	Running   int `json:"running"`
	Stopped   int `json:"stopped"`
	Unhealthy int `json:"unhealthy"`
	Total     int `json:"total"`
}

func (s *Server) monitoringOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hosts, err := s.hub.Store().ListHosts(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := monitoringOverview{
		Hosts:      hosts,
		Latest:     make(map[string]*types.MetricSample, len(hosts)),
		Containers: make(map[string]containerCountsDTO, len(hosts)),
		OpenAlerts: make(map[string]int, len(hosts)),
		Status:     make(map[string]string, len(hosts)),
		Ts:         time.Now().Unix(),
	}
	for _, h := range hosts {
		// Prefer the in-memory rich snapshot; fall back to DB row only when
		// the hub hasn't seen a sample yet this process lifetime.
		if sample, ok := s.hub.LatestSample(h.ID); ok {
			cp := sample
			out.Latest[h.ID] = &cp
		} else if dbSample, err := s.hub.Store().LatestMetric(ctx, h.ID); err == nil && dbSample != nil {
			out.Latest[h.ID] = dbSample
		}
		if counts, ok := s.hub.ContainerCounts(h.ID); ok {
			out.Containers[h.ID] = containerCountsDTO{
				Running:   counts.Running,
				Stopped:   counts.Stopped,
				Unhealthy: counts.Unhealthy,
				Total:     counts.Total,
			}
		}
		if status := s.hub.LatestStatus(h.ID); status != "" {
			out.Status[h.ID] = status
		}
	}
	// Open alert counts per host — one query, group in Go.
	openEvents, err := s.hub.Store().ListAlertEvents(ctx, store.AlertEventFilter{
		OpenOnly: true,
		Limit:    10000,
	})
	if err == nil {
		for _, ev := range openEvents {
			out.OpenAlerts[ev.HostID]++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// openEventsForHost returns the open alert events for a single host. Used
// by the bundle endpoint to surface alerts inline.
func (s *Server) openEventsForHost(ctx context.Context, hostID string) ([]types.AlertEvent, error) {
	return s.hub.Store().ListAlertEvents(ctx, store.AlertEventFilter{
		HostID:   hostID,
		OpenOnly: true,
		Limit:    100,
	})
}

// monitoringBundle is the response shape for GET /api/hosts/{id}/monitoring/bundle.
// One request returns the host record, latest sample, host_config, all
// history series for the given range, and the host's open alerts. Each tab
// of the host-detail page reads the slice it needs from a single bundle
// fetch; switching tabs doesn't re-fetch unless the bundle is stale.
type monitoringBundle struct {
	Host       types.Host                  `json:"host"`
	Latest     *types.MetricSample         `json:"latest"`
	Config     types.HostConfig            `json:"config"`
	History    bundleHistory               `json:"history"`
	OpenAlerts []types.AlertEvent          `json:"openAlerts"`
}

type bundleHistory struct {
	Metrics  []types.MetricSample      `json:"metrics,omitempty"`
	Net      *types.NetIfaceHistory    `json:"net,omitempty"`
	Mounts   *types.DiskMountHistory   `json:"mounts,omitempty"`
	DiskIO   *types.DiskIOHistory      `json:"diskio,omitempty"`
	Temps    *types.TempHistory        `json:"temps,omitempty"`
	CPUCores *types.CPUCoreHistory     `json:"cpuCores,omitempty"`
}

func (s *Server) monitoringBundle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	q := r.URL.Query()
	dur := parseDuration(q.Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	maxPoints, _ := strconv.Atoi(q.Get("points"))
	if maxPoints == 0 {
		maxPoints = 300
	}

	// Optional include=metrics,net,mounts,diskio,temps,cpu lets the caller
	// skip series they don't need. Empty (or missing) = include everything.
	want := includeSet(q.Get("include"))

	host, err := s.hub.Store().GetHost(ctx, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if host == nil {
		writeErr(w, http.StatusNotFound, errors.New("host not found"))
		return
	}

	bundle := monitoringBundle{Host: *host}
	if sample, ok := s.hub.LatestSample(id); ok {
		cp := sample
		bundle.Latest = &cp
	} else if dbSample, err := s.hub.Store().LatestMetric(ctx, id); err == nil {
		bundle.Latest = dbSample
	}
	cfg, _ := s.hub.Store().GetHostConfig(ctx, id)
	bundle.Config = cfg

	if want("metrics") {
		bundle.History.Metrics, _ = s.hub.Store().MetricsRange(ctx, id, since, until, maxPoints)
	}
	if want("net") {
		bundle.History.Net, _ = s.hub.Store().NetIfaceRange(ctx, id, since, until, maxPoints)
	}
	if want("mounts") {
		bundle.History.Mounts, _ = s.hub.Store().DiskMountRange(ctx, id, since, until, maxPoints)
	}
	if want("diskio") {
		bundle.History.DiskIO, _ = s.hub.Store().DiskIORange(ctx, id, since, until, maxPoints)
	}
	if want("temps") {
		bundle.History.Temps, _ = s.hub.Store().TempRange(ctx, id, since, until, maxPoints)
	}
	if want("cpu") {
		bundle.History.CPUCores, _ = s.hub.Store().CPUCoreRange(ctx, id, since, until, maxPoints)
	}

	openEvents, _ := s.openEventsForHost(ctx, id)
	bundle.OpenAlerts = openEvents

	writeJSON(w, http.StatusOK, bundle)
}

// includeSet parses the `?include=a,b,c` query param. Returns a function
// reporting whether `name` should be included; empty/missing param means
// "everything".
func includeSet(raw string) func(string) bool {
	if raw == "" {
		return func(string) bool { return true }
	}
	set := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		set[strings.TrimSpace(name)] = true
	}
	return func(name string) bool { return set[name] }
}

// monitoringCatalog is the response shape for GET /api/monitoring/catalog.
// Returns the families/metrics/alertTargets the UI uses to drive the metric
// picker, alert rule editor, and widget config. Single source of truth so
// the frontend stays in sync as new metrics land server-side.
type monitoringCatalog struct {
	Families        []familyDescriptor `json:"families"`
	ScalarMetrics   []string           `json:"scalarMetrics"`
	AlertCategories map[string][]string `json:"alertCategories"`
	AlertOps        []string           `json:"alertOps"`
	Templates       []alertTemplate    `json:"templates"`
}

type familyDescriptor struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Experimental bool   `json:"experimental"` // true for stub families (smart/gpu/etc.)
}

type alertTemplate struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Rules       []alertTemplateRule       `json:"rules"`
}

type alertTemplateRule struct {
	Metric    string  `json:"metric"`
	Op        string  `json:"op"`
	Threshold float64 `json:"threshold"`
	DurationS int     `json:"duration_s"`
	Severity  string  `json:"severity"`
}

func (s *Server) monitoringCatalog(w http.ResponseWriter, _ *http.Request) {
	out := monitoringCatalog{
		Families: []familyDescriptor{
			{Key: "cpu", Label: "CPU"},
			{Key: "cpu_per_core", Label: "Per-core CPU"},
			{Key: "mem", Label: "Memory"},
			{Key: "disk", Label: "Disk usage"},
			{Key: "mounts", Label: "Disk mounts"},
			{Key: "disk_io", Label: "Disk I/O"},
			{Key: "net", Label: "Network"},
			{Key: "load", Label: "Load average"},
			{Key: "uptime", Label: "Uptime"},
			{Key: "temps", Label: "Temperatures"},
			{Key: "processes", Label: "Processes"},
			{Key: "containers", Label: "Containers"},
			{Key: "smart", Label: "S.M.A.R.T. disk health", Experimental: true},
			{Key: "gpu", Label: "GPU", Experimental: true},
			{Key: "battery", Label: "Battery", Experimental: true},
			{Key: "systemd", Label: "Systemd services", Experimental: true},
		},
		ScalarMetrics:   alerts.SupportedMetrics,
		AlertCategories: alerts.MetricCategories,
		AlertOps:        alerts.SupportedOps,
		Templates:       buildCatalogTemplates(),
	}
	writeJSON(w, http.StatusOK, out)
}

// buildCatalogTemplates maps the canonical template registry in
// internal/alerts/templates.go onto the catalog wire shape. Keeping the
// alerts package free of API-layer types means we map fields explicitly here.
func buildCatalogTemplates() []alertTemplate {
	src := alerts.Templates()
	out := make([]alertTemplate, len(src))
	for i, t := range src {
		rules := make([]alertTemplateRule, len(t.Rules))
		for j, r := range t.Rules {
			rules[j] = alertTemplateRule{
				Metric: r.Metric, Op: r.Op, Threshold: r.Threshold,
				DurationS: r.DurationS, Severity: r.Severity,
			}
		}
		out[i] = alertTemplate{Name: t.Name, Description: t.Description, Rules: rules}
	}
	return out
}

// hostConfig handlers: GET returns the per-host monitoring policy (or
// defaults), PUT validates and persists then pushes the change to the
// running collector (local) or agent (remote).
func (s *Server) getHostConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := s.hub.Store().GetHostConfig(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	cfg.HostID = id
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) putHostConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var cfg types.HostConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg.HostID = id
	if err := s.hub.Store().UpsertHostConfig(r.Context(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Push to whoever owns this host (local collector or remote agent).
	// Errors are logged but don't fail the request — config is persisted.
	if err := s.hub.PushConfigToAgent(r.Context(), id); err != nil {
		// Logged via hub; expose so the user knows the agent didn't apply.
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// applyAlertTemplate POSTs a named template's rules into alert_rules. Body
// shape: {template:"Beszel defaults", host_id:"abc..." | null}. host_id null
// = global rules. Returns the created rule IDs.
func (s *Server) applyAlertTemplate(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Template string  `json:"template"`
		HostID   *string `json:"host_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if p.Template == "" {
		writeErr(w, http.StatusBadRequest, errors.New("template is required"))
		return
	}
	tpl := alerts.TemplateByName(p.Template)
	if tpl == nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("unknown template %q", p.Template))
		return
	}
	created, err := alerts.ApplyTemplate(r.Context(), s.hub.Store(), *tpl, p.HostID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if s.evaluator != nil && len(created) > 0 {
		s.evaluator.Invalidate()
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"template":    p.Template,
		"created":     created,
		"created_n":   len(created),
		"skipped_n":   len(tpl.Rules) - len(created),
	})
}

// Global monitoring defaults: shared across all hosts that don't have a
// host_config row. Edited from the Settings page.
func (s *Server) getMonitoringDefaults(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.hub.Store().GetMonitoringDefaults(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) putMonitoringDefaults(w http.ResponseWriter, r *http.Request) {
	var cfg types.HostConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.hub.Store().SetMonitoringDefaults(r.Context(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Per-metric history endpoints. Mirror metricsRange's shape.
func (s *Server) tempHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	since, until, maxPoints := parseRangeQuery(r)
	hist, err := s.hub.Store().TempRange(r.Context(), id, since, until, maxPoints)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func (s *Server) cpuCoreHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	since, until, maxPoints := parseRangeQuery(r)
	hist, err := s.hub.Store().CPUCoreRange(r.Context(), id, since, until, maxPoints)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func (s *Server) procHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	name := r.URL.Query().Get("name")
	if name == "" {
		writeErr(w, http.StatusBadRequest, errors.New("name query param is required"))
		return
	}
	since, until, maxPoints := parseRangeQuery(r)
	hist, err := s.hub.Store().ProcessRange(r.Context(), id, name, since, until, maxPoints)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func (s *Server) containerHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	since, until, maxPoints := parseRangeQuery(r)
	hist, err := s.hub.Store().ContainerMetricRange(r.Context(), id, cid, since, until, maxPoints)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

// parseRangeQuery extracts (since, until, maxPoints) from the standard
// `range` and `points` query params. Defaults: range=1h, points=300.
func parseRangeQuery(r *http.Request) (time.Time, time.Time, int) {
	q := r.URL.Query()
	dur := parseDuration(q.Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	maxPoints, _ := strconv.Atoi(q.Get("points"))
	if maxPoints == 0 {
		maxPoints = 300
	}
	return since, until, maxPoints
}
