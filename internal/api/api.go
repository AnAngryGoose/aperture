// Package api exposes the HTTP surface used by the SvelteKit frontend.
//
// Routes are versioned under /api so the frontend assets (served separately
// or embedded) can live at the root without colliding.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
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

// composeProvider returns the ComposeProvider for a host, or writes a 503 and returns nil.
func (s *Server) composeProvider(w http.ResponseWriter, hostID string) (hub.ComposeProvider, bool) {
	cp, ok := s.hub.Compose(hostID)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "docker compose not available for this host (host offline or compose not installed)",
		})
	}
	return cp, ok
}

type Server struct {
	hub          *hub.Hub
	evaluator    *alerts.Evaluator
	notifier     *alerts.Notifier
	agentHandler *hub.AgentHandler
	version      string
	startedAt    time.Time
}

// NewServer builds the HTTP server. version + startedAt are surfaced via
// /api/system/info; the store's path is read directly from h.Store().Path()
// at request time so a future runtime DB swap (not currently supported)
// would Just Work.
func NewServer(h *hub.Hub, ev *alerts.Evaluator, notifier *alerts.Notifier, agentH *hub.AgentHandler, version string, startedAt time.Time) *Server {
	return &Server{hub: h, evaluator: ev, notifier: notifier, agentHandler: agentH, version: version, startedAt: startedAt}
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
		r.Get("/system/info", s.systemInfo)
		r.Get("/hosts", s.listHosts)
		r.Get("/hosts/{id}", s.getHost)
		r.Get("/hosts/{id}/metrics/latest", s.latestMetric)
		r.Get("/hosts/{id}/metrics/net", s.netIfaceHistory)
		r.Get("/hosts/{id}/metrics/mounts", s.diskMountHistory)
		r.Get("/hosts/{id}/metrics/diskio", s.diskIOHistory)
		r.Get("/hosts/{id}/metrics", s.metricsRange)
		r.Get("/hosts/{id}/containers", s.listContainers)
		r.Post("/hosts/{id}/containers", s.containerCreate)
		// Specific container sub-routes must be registered before the
		// parameterized /{action} route so chi prefers them on exact match.
		r.Get("/hosts/{id}/containers/{cid}/inspect", s.containerInspect)
		r.Get("/hosts/{id}/containers/{cid}/logs", s.containerLogs)
		r.Put("/hosts/{id}/containers/{cid}/resources", s.containerUpdateResources)
		r.Post("/hosts/{id}/containers/{cid}/recreate", s.containerRecreate)
		r.Post("/hosts/{id}/containers/{cid}/{action}", s.containerAction)
		r.Delete("/hosts/{id}/containers/{cid}", s.containerRemove)

		// Network management.
		r.Get("/hosts/{id}/networks", s.listNetworks)
		r.Post("/hosts/{id}/networks", s.createNetwork)
		r.Get("/hosts/{id}/networks/{net_id}", s.inspectNetwork)
		r.Delete("/hosts/{id}/networks/{net_id}", s.removeNetwork)
		r.Post("/hosts/{id}/networks/{net_id}/connect", s.connectNetwork)
		r.Post("/hosts/{id}/networks/{net_id}/disconnect", s.disconnectNetwork)

		// Compose stack management. Specific sub-routes registered before /{action}.
		r.Get("/hosts/{id}/compose", s.listCompose)
		r.Post("/hosts/{id}/compose", s.createCompose)
		r.Get("/hosts/{id}/compose/{project}/file", s.composeReadFile)
		r.Put("/hosts/{id}/compose/{project}/file", s.composeWriteFile)
		r.Get("/hosts/{id}/compose/{project}/logs", s.composeLogs)
		r.Get("/hosts/{id}/compose/{project}", s.getCompose)
		r.Post("/hosts/{id}/compose/{project}/{action}", s.composeAction)
		r.Delete("/hosts/{id}/compose/{project}", s.deleteCompose)

		// Agent WebSocket and token management.
		r.Get("/agents/ws", s.agentHandler.ServeHTTP)
		r.Get("/agents/tokens", s.listAgentTokens)
		r.Post("/agents/tokens", s.createAgentToken)
		r.Delete("/agents/tokens/{id}", s.revokeAgentToken)
		r.Get("/agents/connected", s.connectedAgents)

		r.Get("/alerts/metadata", s.alertsMetadata)
		r.Get("/alerts/rules", s.listAlertRules)
		r.Post("/alerts/rules", s.createAlertRule)
		r.Get("/alerts/rules/{id}", s.getAlertRule)
		r.Put("/alerts/rules/{id}", s.updateAlertRule)
		r.Delete("/alerts/rules/{id}", s.deleteAlertRule)
		r.Get("/alerts/events", s.listAlertEvents)
		r.Get("/alerts/channels", s.listAlertChannels)
		r.Post("/alerts/channels", s.createAlertChannel)
		r.Get("/alerts/channels/{id}", s.getAlertChannel)
		r.Put("/alerts/channels/{id}", s.updateAlertChannel)
		r.Delete("/alerts/channels/{id}", s.deleteAlertChannel)
		r.Post("/alerts/channels/{id}/test", s.testAlertChannel)
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

func (s *Server) systemInfo(w http.ResponseWriter, _ *http.Request) {
	info := types.SystemInfo{
		Version:   s.version,
		StartedAt: s.startedAt,
		DBPath:    s.hub.Store().Path(),
	}
	// Include the WAL and SHM companions in the size — between checkpoints
	// the WAL can be a non-trivial fraction of total on-disk footprint, and
	// for retention/footprint sizing the user probably wants the truth.
	info.DBSizeBytes = sizeOnDisk(info.DBPath) +
		sizeOnDisk(info.DBPath+"-wal") +
		sizeOnDisk(info.DBPath+"-shm")
	writeJSON(w, http.StatusOK, info)
}

// sizeOnDisk returns the file size for path, or 0 if it doesn't exist.
// Missing companions (e.g. -wal when WAL is checkpointed and not yet
// re-created) are normal, not an error.
func sizeOnDisk(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
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

// latestMetric returns the most recent sample for a host. It prefers the
// hub's in-memory live snapshot (which includes rich per-core/per-interface/
// per-mount fields) over the DB value, falling back to the DB only when no
// sample has arrived in the current process lifetime.
func (s *Server) latestMetric(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if sample, ok := s.hub.LatestSample(id); ok {
		writeJSON(w, http.StatusOK, sample)
		return
	}
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

func (s *Server) netIfaceHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dur := parseDuration(r.URL.Query().Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	pts, _ := strconv.Atoi(r.URL.Query().Get("points"))
	if pts == 0 {
		pts = 300
	}
	h, err := s.hub.Store().NetIfaceRange(r.Context(), id, since, until, pts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if h == nil {
		h = &types.NetIfaceHistory{Timestamps: []int64{}, Ifaces: map[string]*types.NetIfaceSeries{}}
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) diskMountHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dur := parseDuration(r.URL.Query().Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	pts, _ := strconv.Atoi(r.URL.Query().Get("points"))
	if pts == 0 {
		pts = 300
	}
	h, err := s.hub.Store().DiskMountRange(r.Context(), id, since, until, pts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if h == nil {
		h = &types.DiskMountHistory{Timestamps: []int64{}, Mounts: map[string]*types.DiskMountSeries{}}
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) diskIOHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dur := parseDuration(r.URL.Query().Get("range"), time.Hour)
	until := time.Now().UTC()
	since := until.Add(-dur)
	pts, _ := strconv.Atoi(r.URL.Query().Get("points"))
	if pts == 0 {
		pts = 300
	}
	h, err := s.hub.Store().DiskIORange(r.Context(), id, since, until, pts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if h == nil {
		h = &types.DiskIOHistory{Timestamps: []int64{}, Devices: map[string]*types.DiskIOSeries{}}
	}
	writeJSON(w, http.StatusOK, h)
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

func (s *Server) containerCreate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var spec types.CreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cid, err := d.Create(r.Context(), spec)
	if err != nil {
		// Surface partial-success: the container may have been created but
		// failed to start (Create returns the id alongside the start error).
		if cid != "" {
			writeJSON(w, http.StatusAccepted, map[string]any{"id": cid, "warning": err.Error()})
			return
		}
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": cid})
}

func (s *Server) containerInspect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	info, err := d.Inspect(r.Context(), cid)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) containerUpdateResources(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var update types.ResourceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := d.UpdateResources(r.Context(), cid, update); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// containerRecreate stops and removes the current container, then creates a new
// one from the same image + config. Used to pick up a newer image version or
// reset container state without rewriting the compose spec.
func (s *Server) containerRecreate(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	cid := chi.URLParam(r, "cid")
	d, ok := s.hub.Docker(hostID)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}

	info, err := d.Inspect(r.Context(), cid)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("inspect: %w", err))
		return
	}

	spec := inspectToSpec(info)

	// Stop old container (ignore error — may already be stopped).
	_ = d.Stop(r.Context(), cid, nil)

	if err := d.Remove(r.Context(), cid, true, false); err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Errorf("remove: %w", err))
		return
	}

	newID, err := d.Create(r.Context(), spec)
	if err != nil {
		if newID != "" {
			writeJSON(w, http.StatusAccepted, map[string]any{"id": newID, "warning": err.Error()})
			return
		}
		writeErr(w, http.StatusBadGateway, fmt.Errorf("create: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

// inspectToSpec rebuilds a minimal CreateSpec from a ContainerInspect so the
// recreate path can bring up a replacement with the same configuration.
func inspectToSpec(ci *types.ContainerInspect) types.CreateSpec {
	spec := types.CreateSpec{
		Image:         ci.Image,
		Name:          ci.Name,
		RestartPolicy: ci.RestartPolicy,
		AutoStart:     ci.State == "running",
	}
	if len(ci.Env) > 0 {
		env := make(map[string]string, len(ci.Env))
		for _, kv := range ci.Env {
			if i := strings.IndexByte(kv, '='); i >= 0 {
				env[kv[:i]] = kv[i+1:]
			}
		}
		spec.Env = env
	}
	seen := make(map[string]bool)
	for _, p := range ci.Ports {
		if p.PublicPort == 0 {
			continue
		}
		key := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		spec.Ports = append(spec.Ports, types.PortBinding{
			HostPort: int(p.PublicPort), ContainerPort: int(p.PrivatePort), Protocol: p.Type,
		})
	}
	for _, m := range ci.Mounts {
		if m.Type != "bind" {
			continue
		}
		spec.Volumes = append(spec.Volumes, types.VolumeBinding{
			HostPath: m.Source, ContainerPath: m.Destination, ReadOnly: !m.RW,
		})
	}
	return spec
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

// --- networks ---

func (s *Server) listNetworks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	nets, err := d.ListNetworks(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if nets == nil {
		nets = []types.DockerNetwork{}
	}
	writeJSON(w, http.StatusOK, nets)
}

func (s *Server) inspectNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	netID := chi.URLParam(r, "net_id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	n, err := d.InspectNetwork(r.Context(), netID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) createNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var spec types.NetworkCreateSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	netID, err := d.CreateNetwork(r.Context(), spec)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": netID})
}

func (s *Server) removeNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	netID := chi.URLParam(r, "net_id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	if err := d.RemoveNetwork(r.Context(), netID); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) connectNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	netID := chi.URLParam(r, "net_id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var p struct {
		ContainerID string `json:"container_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := d.ConnectContainer(r.Context(), netID, p.ContainerID); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) disconnectNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	netID := chi.URLParam(r, "net_id")
	d, ok := s.hub.Docker(id)
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("no docker provider for host"))
		return
	}
	var p struct {
		ContainerID string `json:"container_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := d.DisconnectContainer(r.Context(), netID, p.ContainerID); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- alerts ---

func (s *Server) alertsMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":       alerts.SupportedMetrics,
		"ops":           alerts.SupportedOps,
		"severities":    []string{"info", "warning", "critical"},
		"channel_types": []string{"discord", "slack", "ntfy", "gotify", "webhook"},
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
	Severity  string  `json:"severity"`
}

func (p alertRulePayload) toRule(id int64) types.AlertRule {
	r := types.AlertRule{
		ID:        id,
		Metric:    p.Metric,
		Op:        p.Op,
		Threshold: p.Threshold,
		DurationS: p.DurationS,
		Enabled:   true,
		Severity:  p.Severity,
	}
	if r.Severity == "" {
		r.Severity = "warning"
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

// --- alert channels ---

type alertChannelPayload struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Config        json.RawMessage `json:"config"`
	Enabled       *bool           `json:"enabled"`
	MinSeverity   string          `json:"min_severity"`
	NotifyResolve *bool           `json:"notify_resolve"`
}

func (p alertChannelPayload) toChannel(id int64) types.AlertChannel {
	ch := types.AlertChannel{
		ID:            id,
		Name:          p.Name,
		Type:          p.Type,
		Config:        []byte(p.Config),
		Enabled:       true,
		MinSeverity:   p.MinSeverity,
		NotifyResolve: true,
	}
	if len(ch.Config) == 0 {
		ch.Config = []byte("{}")
	}
	if ch.MinSeverity == "" {
		ch.MinSeverity = "info"
	}
	if p.Enabled != nil {
		ch.Enabled = *p.Enabled
	}
	if p.NotifyResolve != nil {
		ch.NotifyResolve = *p.NotifyResolve
	}
	return ch
}

func validateChannelType(t string) error {
	for _, v := range []string{"discord", "slack", "ntfy", "gotify", "webhook"} {
		if t == v {
			return nil
		}
	}
	return fmt.Errorf("unknown channel type %q", t)
}

func (s *Server) listAlertChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.hub.Store().ListAlertChannels(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if channels == nil {
		channels = []types.AlertChannel{}
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) createAlertChannel(w http.ResponseWriter, r *http.Request) {
	var p alertChannelPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validateChannelType(p.Type); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ch := p.toChannel(0)
	id, err := s.hub.Store().CreateAlertChannel(r.Context(), ch)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	created, err := s.hub.Store().GetAlertChannel(r.Context(), id)
	if err != nil || created == nil {
		ch.ID = id
		writeJSON(w, http.StatusCreated, ch)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getAlertChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ch, err := s.hub.Store().GetAlertChannel(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ch == nil {
		writeErr(w, http.StatusNotFound, errors.New("channel not found"))
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (s *Server) updateAlertChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var p alertChannelPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validateChannelType(p.Type); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ch := p.toChannel(id)
	if err := s.hub.Store().UpdateAlertChannel(r.Context(), ch); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := s.hub.Store().GetAlertChannel(r.Context(), id)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusOK, ch)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAlertChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.hub.Store().DeleteAlertChannel(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) testAlertChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ch, err := s.hub.Store().GetAlertChannel(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ch == nil {
		writeErr(w, http.StatusNotFound, errors.New("channel not found"))
		return
	}
	// Build a synthetic notification for testing.
	now := time.Now().UTC()
	testNotif := alerts.Notification{
		Event: types.AlertEvent{ID: 0, RuleID: 0, HostID: "test", FiredAt: now, Value: 75.5},
		Rule:  types.AlertRule{Metric: "cpu_pct", Op: ">", Threshold: 75, Severity: "warning"},
		Host:  types.Host{ID: "test", Name: "test-host"},
	}
	sender, err := alerts.BuildSender(*ch)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err)
		return
	}
	if err := sender.Send(r.Context(), testNotif); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- agent tokens ---

func (s *Server) listAgentTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.hub.Store().ListAgentTokens(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if tokens == nil {
		tokens = []types.AgentToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (s *Server) createAgentToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	tok, err := s.hub.Store().CreateAgentToken(r.Context(), body.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// 201 Created; the plaintext token is in the response body only this once.
	writeJSON(w, http.StatusCreated, tok)
}

func (s *Server) revokeAgentToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.hub.Store().RevokeAgentToken(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) connectedAgents(w http.ResponseWriter, r *http.Request) {
	ids := s.agentHandler.ConnectedAgents()
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, ids)
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── compose handlers ─────────────────────────────────────────────────────────

func (s *Server) listCompose(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}
	stacks, err := cp.DiscoverStacks(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stacks == nil {
		stacks = []types.ComposeStack{}
	}
	writeJSON(w, http.StatusOK, stacks)
}

func (s *Server) getCompose(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")
	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}
	stack, err := cp.GetStack(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stack)
}

// composeAction handles POST /hosts/{id}/compose/{project}/{action}
// Supported actions: up, down, restart, pull, stop, start
func (s *Server) composeAction(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")
	action := chi.URLParam(r, "action")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	validActions := map[string]bool{"up": true, "down": true, "restart": true, "pull": true, "stop": true, "start": true}
	if !validActions[action] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action: " + action})
		return
	}

	var body struct {
		WorkingDir string   `json:"working_dir"`
		Service    string   `json:"service"`
		ExtraArgs  []string `json:"extra_args"`
		Volumes    bool     `json:"volumes"` // for "down"
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	extraArgs := body.ExtraArgs
	if action == "down" && body.Volumes {
		extraArgs = append(extraArgs, "--volumes")
	}

	out, err := cp.StackAction(r.Context(), project, body.WorkingDir, action, body.Service, extraArgs...)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error":  err.Error(),
			"output": out,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": out})
}

func (s *Server) composeLogs(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	service := r.URL.Query().Get("service")
	workingDir := r.URL.Query().Get("working_dir")
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	if tail <= 0 {
		tail = 200
	}

	logs, err := cp.Logs(r.Context(), project, workingDir, service, tail)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}

func (s *Server) composeReadFile(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	workingDir := r.URL.Query().Get("working_dir")
	if workingDir == "" {
		if stack, err := cp.GetStack(r.Context(), project); err == nil {
			workingDir = stack.WorkingDir
		}
	}
	if workingDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "working_dir required"})
		return
	}

	content, err := cp.ReadFile(r.Context(), workingDir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content, "working_dir": workingDir})
}

func (s *Server) composeWriteFile(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	var body struct {
		Content    string `json:"content"`
		WorkingDir string `json:"working_dir"`
		Deploy     bool   `json:"deploy"` // run `up -d` after writing
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content required"})
		return
	}

	workingDir := body.WorkingDir
	if workingDir == "" {
		if stack, err := cp.GetStack(r.Context(), project); err == nil {
			workingDir = stack.WorkingDir
		}
	}
	if workingDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "working_dir required"})
		return
	}

	if err := cp.WriteFile(r.Context(), workingDir, body.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var output string
	if body.Deploy {
		out, err := cp.StackAction(r.Context(), project, workingDir, "up", "", "--remove-orphans")
		output = out
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": err.Error(), "output": output,
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": output})
}

func (s *Server) createCompose(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	var body struct {
		WorkingDir string `json:"working_dir"`
		Content    string `json:"content"`
		Start      bool   `json:"start"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WorkingDir == "" || body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "working_dir and content required"})
		return
	}

	if err := cp.WriteFile(r.Context(), body.WorkingDir, body.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if body.Start {
		if _, err := cp.StackAction(r.Context(), "", body.WorkingDir, "up", "", "--remove-orphans"); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
	}

	stacks, err := cp.DiscoverStacks(r.Context())
	if err != nil || len(stacks) == 0 {
		writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
		return
	}
	for _, st := range stacks {
		if st.WorkingDir == body.WorkingDir {
			writeJSON(w, http.StatusCreated, st)
			return
		}
	}
	writeJSON(w, http.StatusCreated, stacks[len(stacks)-1])
}

func (s *Server) deleteCompose(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "id")
	project := chi.URLParam(r, "project")

	cp, ok := s.composeProvider(w, hostID)
	if !ok {
		return
	}

	var workingDir string
	if stack, err := cp.GetStack(r.Context(), project); err == nil {
		workingDir = stack.WorkingDir
	}

	extraArgs := []string{}
	if r.URL.Query().Get("volumes") == "true" {
		extraArgs = append(extraArgs, "--volumes")
	}

	if _, err := cp.StackAction(r.Context(), project, workingDir, "down", "", extraArgs...); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
