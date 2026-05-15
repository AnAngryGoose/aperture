// Package hub orchestrates metric ingestion, host registry, and the docker
// surface used by the API. It is the seam between local v0.1 collection and
// future remote agents: anything that can produce types.MetricSample for a
// host can be Registered here.
package hub

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aperture/aperture/internal/dockerctl"
	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
)

// MetricSource produces samples for a single host. v0.1 ships one impl
// (collector.Local). Future remote-agent transports will satisfy the same
// interface, allowing the hub's ingestion path to stay unchanged.
type MetricSource interface {
	HostInfo() types.HostInfo
	Run(ctx context.Context, out chan<- types.MetricSample) error
}

// DockerProvider abstracts how the hub reaches a host's docker engine.
// In v0.1 there is one impl (the local docker socket). Remote agents will
// expose the same surface over the wire.
type DockerProvider interface {
	List(ctx context.Context, all bool) ([]types.Container, error)
	Create(ctx context.Context, spec types.CreateSpec) (string, error)
	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string, timeoutSec *int) error
	Restart(ctx context.Context, id string, timeoutSec *int) error
	Pause(ctx context.Context, id string) error
	Unpause(ctx context.Context, id string) error
	Kill(ctx context.Context, id, signal string) error
	Remove(ctx context.Context, id string, force, removeVolumes bool) error
	Logs(ctx context.Context, id string, tail int, since time.Time, timestamps bool) (string, error)
	Inspect(ctx context.Context, id string) (*types.ContainerInspect, error)
	UpdateResources(ctx context.Context, id string, update types.ResourceUpdate) error
	ListNetworks(ctx context.Context) ([]types.DockerNetwork, error)
	InspectNetwork(ctx context.Context, id string) (*types.DockerNetwork, error)
	CreateNetwork(ctx context.Context, spec types.NetworkCreateSpec) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
	ConnectContainer(ctx context.Context, networkID, containerID string) error
	DisconnectContainer(ctx context.Context, networkID, containerID string) error
	ListVolumes(ctx context.Context) ([]types.DockerVolume, error)
	InspectVolume(ctx context.Context, name string) (*types.DockerVolume, error)
	CreateVolume(ctx context.Context, spec types.VolumeCreateSpec) (string, error)
	RemoveVolume(ctx context.Context, name string, force bool) error
	ListImages(ctx context.Context) ([]types.DockerImage, error)
	InspectImage(ctx context.Context, id string) (*types.DockerImage, error)
	RemoveImage(ctx context.Context, id string, force bool) error
	PullImage(ctx context.Context, image string) error
	CheckImageUpdate(ctx context.Context, image string) (*types.ImageUpdateStatus, error)
}

// ComposeProvider abstracts docker compose stack operations for a single host.
// Local hosts use compose.Local; remote agents satisfy this via agentComposeProvider.
type ComposeProvider interface {
	DiscoverStacks(ctx context.Context) ([]types.ComposeStack, error)
	GetStack(ctx context.Context, project string) (*types.ComposeStack, error)
	StackAction(ctx context.Context, project, workingDir, action, service string, extraArgs ...string) (string, error)
	Logs(ctx context.Context, project, workingDir, service string, tail int) (string, error)
	ReadFile(ctx context.Context, workingDir string) (string, error)
	WriteFile(ctx context.Context, workingDir, content string) error
}

// TerminalProvider abstracts exec/attach terminal sessions for a single host.
// Local hosts use localTerminalProvider; remote agents use agentTerminalProvider.
type TerminalProvider interface {
	StartTerminal(ctx context.Context, cid, cmd string) (reqID string, output <-chan []byte, err error)
	SendTerminalData(ctx context.Context, reqID string, data []byte) error
	ResizeTerminal(ctx context.Context, reqID string, cols, rows uint) error
	CloseTerminal(ctx context.Context, reqID string) error
}

// SSEEvent is the typed envelope pushed to SSE subscribers. The Type field
// disambiguates between event categories — older clients that don't read
// Type still see the flat metric fields (cpu, mem, netIn, netOut, temp) under
// the default "metric" type, preserving wire compatibility. Newer clients
// switch on Type to handle host_status / container_summary / alert events.
//
// Event types:
//   - "metric":            CPU + Mem + NetIn + NetOut + Temp + DiskPct flat fields
//   - "host_status":       Status field ("ok" | "warn" | "crit" | "offline")
//   - "container_summary": Containers nested struct
//   - "alert":             Alert nested struct (rule_id, event_id, severity, fired/resolved, value)
type SSEEvent struct {
	Type   string `json:"type"` // default: "metric"
	HostID string `json:"hostId"`
	Ts     int64  `json:"ts"`

	// Metric payload (Type == "metric"). NOT omitempty — a host at exactly 0%
	// CPU/mem must still emit "cpu":0 so the frontend updates. These fields
	// are technically meaningless for other event types; subscribers ignore
	// them based on Type. Backwards-compat wire shape with pre-v0.5 clients.
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	NetIn   uint64  `json:"netIn"`
	NetOut  uint64  `json:"netOut"`
	Temp    float64 `json:"temp"`
	DiskPct float64 `json:"diskPct"`

	// host_status payload.
	Status string `json:"status,omitempty"`

	// container_summary payload.
	Containers *ContainerCounts `json:"containers,omitempty"`

	// alert payload.
	Alert *AlertEnvelope `json:"alert,omitempty"`
}

// ContainerCounts is the per-host container summary the dashboard shows on
// each card. Populated by either the hub-side ticker (local hosts) or the
// remote agent and broadcast as "container_summary" events on change.
type ContainerCounts struct {
	Running   int `json:"running"`
	Stopped   int `json:"stopped"`
	Unhealthy int `json:"unhealthy"`
	Total     int `json:"total"`
}

// AlertEnvelope is the SSE payload for fired/resolved alert state changes.
// The full event details remain available via /api/alerts/events; this
// envelope just nudges connected clients to refresh or show a toast.
type AlertEnvelope struct {
	RuleID   int64   `json:"ruleId"`
	EventID  int64   `json:"eventId"`
	Severity string  `json:"severity"` // "info" | "warning" | "critical"
	Metric   string  `json:"metric"`
	Value    float64 `json:"value"`
	Resolved bool    `json:"resolved"`
}

type Hub struct {
	store    *store.Store
	log      *slog.Logger
	retain   time.Duration
	mu       sync.RWMutex
	dockers   map[string]DockerProvider   // host_id -> docker
	composes  map[string]ComposeProvider  // host_id -> compose
	terminals map[string]TerminalProvider // host_id -> terminal
	hosts     map[string]types.Host       // host_id -> host (cached)
	samples  chan types.MetricSample
	// latestRich caches the most recent full sample per host including the
	// live-only rich fields (per-core CPU, per-interface net, disk mounts, etc.)
	// that are NOT stored in the metrics table.
	latestRich map[string]types.MetricSample
	// latestContainerCounts is the per-host container summary read by the
	// monitoring overview endpoint and broadcast as container_summary SSE
	// events on change. Populated by a hub-side ticker for local hosts and
	// by the agent for remote hosts. Guarded by mu.
	latestContainerCounts map[string]ContainerCounts
	// latestStatus is the per-host health status. Computed in ingestLoop from
	// host_config thresholds + sample, broadcast as host_status SSE events on
	// transition. Sources moved out of the client so all clients agree.
	latestStatus map[string]string
	// evaluator is set by SetEvaluator before Run is called. The interface
	// type avoids an import cycle with internal/alerts.
	evaluator Evaluator
	// configPusher routes per-host config changes to a remote agent.
	// Set by cmd/hub to the agent handler. Nil = no agent transport configured.
	configPusher ConfigPusher
	// localApplier mirrors per-host config changes to the in-process local
	// collector when the changed host is the hub's own. Nil = no local source.
	localApplier      LocalApplier
	localApplierHosts map[string]bool // hostIDs whose config should be applied locally
	// sseMu guards sseSubscribers.
	sseMu          sync.Mutex
	sseSubscribers map[string]chan SSEEvent // subscriber-id -> channel
}

// Evaluator is the seam used by the hub to dispatch every persisted sample
// to the alert evaluator. *alerts.Evaluator satisfies this; tests can
// substitute a stub.
type Evaluator interface {
	Evaluate(ctx context.Context, sample types.MetricSample)
}

// ConfigPusher pushes a per-host monitoring config to a remote agent over
// the agent transport. AgentHandler satisfies this; the hub uses it to
// notify a connected agent that its policy changed (on host_config PUT or
// agent reconnect).
type ConfigPusher interface {
	PushConfig(ctx context.Context, hostID string, cfg types.HostConfig) error
}

// LocalApplier applies a config to the in-process local collector. The hub
// uses it to keep the local collector in sync when host_config for the
// hub's own host changes. *collector.Local satisfies this.
type LocalApplier interface {
	ApplyConfig(cfg types.HostConfig)
}

type Config struct {
	Store  *store.Store
	Logger *slog.Logger
	Retain time.Duration // how long to keep metric samples; 0 = forever
}

func New(cfg Config) *Hub {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Hub{
		store:                 cfg.Store,
		log:                   cfg.Logger,
		retain:                cfg.Retain,
		dockers:               make(map[string]DockerProvider),
		composes:              make(map[string]ComposeProvider),
		terminals:             make(map[string]TerminalProvider),
		hosts:                 make(map[string]types.Host),
		samples:               make(chan types.MetricSample, 256),
		latestRich:            make(map[string]types.MetricSample),
		latestContainerCounts: make(map[string]ContainerCounts),
		latestStatus:          make(map[string]string),
		sseSubscribers:        make(map[string]chan SSEEvent),
		localApplierHosts:     make(map[string]bool),
	}
}

// SetConfigPusher installs the agent config pusher. Called by cmd/hub after
// constructing the agent handler. Nil-safe — PushConfigToAgent becomes a no-op
// for remote hosts when the pusher isn't set.
func (h *Hub) SetConfigPusher(p ConfigPusher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configPusher = p
}

// SetLocalApplier installs the in-process collector that should receive
// config updates for the hub's own host(s). Call after RegisterSource for
// each local host so the hub knows which host ID maps to the local collector.
func (h *Hub) SetLocalApplier(a LocalApplier, hostIDs ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.localApplier = a
	for _, id := range hostIDs {
		h.localApplierHosts[id] = true
	}
}

// PushConfigToAgent applies a per-host monitoring config to the right
// transport. Reads the current config from store, then:
//   - For hosts the localApplier owns: calls ApplyConfig directly.
//   - For remote hosts with a registered configPusher: sends a ConfigFrame.
//   - Otherwise: silent no-op (host might be offline).
//
// Returns an error only when the transport call itself fails; missing
// transport/applier is treated as success.
func (h *Hub) PushConfigToAgent(ctx context.Context, hostID string) error {
	cfg, err := h.store.GetHostConfig(ctx, hostID)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}
	h.mu.RLock()
	isLocal := h.localApplierHosts[hostID]
	applier := h.localApplier
	pusher := h.configPusher
	h.mu.RUnlock()
	if isLocal && applier != nil {
		applier.ApplyConfig(cfg)
		return nil
	}
	if pusher != nil {
		return pusher.PushConfig(ctx, hostID, cfg)
	}
	return nil
}

// SubscribeSSE registers a channel to receive live metric events. Returns
// an unsubscribe function that must be called when the subscriber disconnects.
func (h *Hub) SubscribeSSE() (string, <-chan SSEEvent, func()) {
	id := fmt.Sprintf("sse-%d", time.Now().UnixNano())
	ch := make(chan SSEEvent, 64)
	h.sseMu.Lock()
	h.sseSubscribers[id] = ch
	h.sseMu.Unlock()
	unsub := func() {
		h.sseMu.Lock()
		delete(h.sseSubscribers, id)
		h.sseMu.Unlock()
		close(ch)
	}
	return id, ch, unsub
}

func (h *Hub) broadcastSSE(ev SSEEvent) {
	h.sseMu.Lock()
	defer h.sseMu.Unlock()
	for _, ch := range h.sseSubscribers {
		select {
		case ch <- ev:
		default: // subscriber is slow; drop rather than block ingest
		}
	}
}

// Run starts background workers: ingestion, retention, container-summary.
// Returns when ctx is done.
func (h *Hub) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.ingestLoop(ctx)
	}()
	// Retention loop always runs (handles per-host policies even when the
	// hub-wide default h.retain is 0).
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.retentionLoop(ctx)
	}()
	// Container summary loop: polls each registered docker provider on a
	// slow interval and broadcasts container_summary SSE events on change.
	// Eliminates the dashboard's per-poll Docker round-trips.
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.containerSummaryLoop(ctx)
	}()
	<-ctx.Done()
	wg.Wait()
	return nil
}

func (h *Hub) ingestLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case s := <-h.samples:
			// Cache the full live snapshot (rich fields not stored in DB).
			h.mu.Lock()
			h.latestRich[s.HostID] = s
			h.mu.Unlock()

			if err := h.store.InsertMetric(ctx, s); err != nil {
				h.log.Error("insert metric", "host_id", s.HostID, "err", err)
				continue
			}
			// Best-effort persists: failures don't abort the ingest loop and
			// each table is independent (a disk-mount write failure doesn't
			// poison network or temp history). Slices are empty for families
			// disabled by host_config (the collector skips them), so a
			// len() > 0 gate is enough — no explicit family check here.
			if len(s.NetIfaces) > 0 {
				if err := h.store.InsertNetIfaces(ctx, s); err != nil {
					h.log.Warn("insert net ifaces", "host_id", s.HostID, "err", err)
				}
			}
			if len(s.DiskMounts) > 0 {
				if err := h.store.InsertDiskMounts(ctx, s); err != nil {
					h.log.Warn("insert disk mounts", "host_id", s.HostID, "err", err)
				}
			}
			if len(s.DiskIO) > 0 {
				if err := h.store.InsertDiskIO(ctx, s); err != nil {
					h.log.Warn("insert disk io", "host_id", s.HostID, "err", err)
				}
			}
			if len(s.Temps) > 0 {
				if err := h.store.InsertTemps(ctx, s); err != nil {
					h.log.Warn("insert temps", "host_id", s.HostID, "err", err)
				}
			}
			if len(s.CPUPerCore) > 0 {
				if err := h.store.InsertCPUCores(ctx, s); err != nil {
					h.log.Warn("insert cpu cores", "host_id", s.HostID, "err", err)
				}
			}
			if len(s.Processes) > 0 {
				if err := h.store.InsertProcessSnapshot(ctx, s); err != nil {
					h.log.Warn("insert processes", "host_id", s.HostID, "err", err)
				}
			}
			_ = h.store.TouchHost(ctx, s.HostID, s.Timestamp)
			if h.evaluator != nil {
				h.evaluator.Evaluate(ctx, s)
			}

			// Derive average temperature for SSE payload (cards show "the" temp).
			var avgTemp float64
			if len(s.Temps) > 0 {
				for _, t := range s.Temps {
					avgTemp += t.Temp
				}
				avgTemp /= float64(len(s.Temps))
			}

			// Broadcast metric event (backwards-compatible flat shape).
			h.broadcastSSE(SSEEvent{
				Type:    "metric",
				HostID:  s.HostID,
				Ts:      s.Timestamp.Unix(),
				CPU:     s.CPUPercent,
				Mem:     s.MemPercent,
				NetIn:   s.NetRxBytes,
				NetOut:  s.NetTxBytes,
				Temp:    avgTemp,
				DiskPct: s.DiskPercent,
			})

			// Compute host status from per-host config thresholds (or global
			// defaults if no host_config row exists). Broadcast status only
			// on transition — saves SSE bandwidth and avoids client churn.
			newStatus := h.computeStatus(ctx, s, avgTemp)
			h.mu.Lock()
			prevStatus := h.latestStatus[s.HostID]
			h.latestStatus[s.HostID] = newStatus
			h.mu.Unlock()
			if newStatus != prevStatus {
				h.broadcastSSE(SSEEvent{
					Type:   "host_status",
					HostID: s.HostID,
					Ts:     s.Timestamp.Unix(),
					Status: newStatus,
				})
			}
		}
	}
}

// computeStatus returns the per-host status string ("ok"|"warn"|"crit"|"offline")
// from the host's configured thresholds. Read from host_config or defaults.
// Falls back gracefully if the config read fails — never panics on a
// transient DB error during ingest.
func (h *Hub) computeStatus(ctx context.Context, s types.MetricSample, maxTemp float64) string {
	cfg, err := h.store.GetHostConfig(ctx, s.HostID)
	if err != nil {
		// Defaults are reasonable; don't fail the broadcast.
		cfg = store.DefaultHostConfig(s.HostID)
	}
	if s.CPUPercent >= cfg.CritCPU || s.MemPercent >= cfg.CritMem || s.DiskPercent >= cfg.CritDisk || (maxTemp > 0 && maxTemp >= cfg.CritTemp) {
		return "crit"
	}
	if s.CPUPercent >= cfg.WarnCPU || s.MemPercent >= cfg.WarnMem || s.DiskPercent >= cfg.WarnDisk || (maxTemp > 0 && maxTemp >= cfg.WarnTemp) {
		return "warn"
	}
	return "ok"
}

// SetEvaluator installs the alert evaluator. Must be called before Run for
// alerts to fire on the first sample. Calling later is permitted (rules just
// won't evaluate until then) — useful for tests that want to register hosts
// before any rules exist.
func (h *Hub) SetEvaluator(e Evaluator) {
	h.evaluator = e
}

// retentionLoop runs hourly. For each host it computes per-table cutoffs
// from the host's config (retention_days + retention_overrides) and prunes
// scoped to that host. Hosts without a host_config row use h.retain (the
// hub-wide default flag) or fall back to 30 days.
func (h *Hub) retentionLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.pruneAllHosts(ctx)
		}
	}
}

// pruneAllHosts iterates every host and prunes its metric tables according
// to its host_config retention policy. Errors are logged per host so a single
// failing host doesn't stop the others. Returns rows deleted across all
// hosts (also logged at the loop level).
func (h *Hub) pruneAllHosts(ctx context.Context) int64 {
	hosts, err := h.store.ListHosts(ctx)
	if err != nil {
		h.log.Warn("retention: list hosts", "err", err)
		return 0
	}
	now := time.Now().UTC()
	var total int64
	for _, host := range hosts {
		cfg, err := h.store.GetHostConfig(ctx, host.ID)
		if err != nil {
			h.log.Warn("retention: get config", "host_id", host.ID, "err", err)
			continue
		}
		// Default retention for tables not in retention_overrides.
		defaultDays := cfg.RetentionDays
		if defaultDays <= 0 {
			if h.retain > 0 {
				defaultDays = int(h.retain.Hours() / 24)
			} else {
				defaultDays = 30
			}
		}
		tables := []string{"metrics", "net_iface_metrics", "disk_mount_metrics", "disk_io_metrics",
			"temp_metrics", "cpu_core_metrics", "process_metrics", "container_metrics"}
		cutoffs := make(map[string]time.Time, len(tables))
		for _, table := range tables {
			days := defaultDays
			if d, ok := cfg.RetentionOverrides[table]; ok && d > 0 {
				days = d
			}
			cutoffs[table] = now.AddDate(0, 0, -days)
		}
		n, err := h.store.PruneHostMetrics(ctx, host.ID, cutoffs)
		if err != nil {
			h.log.Warn("retention: prune", "host_id", host.ID, "err", err)
			continue
		}
		total += n
	}
	if total > 0 {
		h.log.Info("retention: pruned", "rows", total)
	}
	return total
}

// containerSummaryLoop polls every registered docker provider every 15
// seconds, computes running/stopped/unhealthy counts, and broadcasts a
// container_summary SSE event when the counts change. This replaces the
// dashboard's previous per-poll Docker round-trips: one hub-side call
// services all connected clients.
func (h *Hub) containerSummaryLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	// Tick once immediately so we have counts before the first 15s window.
	h.refreshAllContainerCounts(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.refreshAllContainerCounts(ctx)
		}
	}
}

// refreshAllContainerCounts iterates every registered docker provider and
// updates the latestContainerCounts map, broadcasting on change.
func (h *Hub) refreshAllContainerCounts(ctx context.Context) {
	h.mu.RLock()
	providers := make(map[string]DockerProvider, len(h.dockers))
	for id, p := range h.dockers {
		providers[id] = p
	}
	h.mu.RUnlock()

	for hostID, p := range providers {
		list, err := p.List(ctx, true)
		if err != nil {
			continue
		}
		var counts ContainerCounts
		counts.Total = len(list)
		for _, c := range list {
			switch c.State {
			case "running":
				counts.Running++
			default:
				counts.Stopped++
			}
			if strings.Contains(strings.ToLower(c.Status), "unhealthy") {
				counts.Unhealthy++
			}
		}
		h.SetContainerCounts(hostID, counts)
	}
}

// SetContainerCounts updates the cached counts for a host and broadcasts a
// container_summary SSE event when the counts changed. Public so remote
// agent transports can push their container summary on a different cadence.
func (h *Hub) SetContainerCounts(hostID string, counts ContainerCounts) {
	h.mu.Lock()
	prev, hadPrev := h.latestContainerCounts[hostID]
	h.latestContainerCounts[hostID] = counts
	h.mu.Unlock()
	if hadPrev && prev == counts {
		return
	}
	h.broadcastSSE(SSEEvent{
		Type:       "container_summary",
		HostID:     hostID,
		Ts:         time.Now().Unix(),
		Containers: &counts,
	})
}

// ContainerCounts returns the cached per-host container summary. Used by the
// monitoring overview endpoint so the dashboard gets counts in one call
// without fanning out a per-host Docker request.
func (h *Hub) ContainerCounts(hostID string) (ContainerCounts, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.latestContainerCounts[hostID]
	return c, ok
}

// LatestStatus returns the cached per-host status string. Used by the
// monitoring overview endpoint.
func (h *Hub) LatestStatus(hostID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latestStatus[hostID]
}

// RegisterSource attaches a metric source for a host. Returns the host_id the
// source is bound to. Idempotent: re-registering the same logical host (by
// name + source kind) keeps the same id so historical data stays linked.
func (h *Hub) RegisterSource(ctx context.Context, src MetricSource) (string, error) {
	info := src.HostInfo()
	id := DeriveHostID(info)
	now := time.Now().UTC()
	host := types.Host{
		ID: id, Name: info.Name, OS: info.OS, Platform: info.Platform,
		Kernel: info.Kernel, Arch: info.Arch, CPUModel: info.CPUModel,
		CPUCount: info.CPUCount, MemTotal: info.MemTotal,
		Source: info.Source, CreatedAt: now, LastSeen: now,
	}
	if err := h.store.UpsertHost(ctx, host); err != nil {
		return "", err
	}
	h.mu.Lock()
	h.hosts[id] = host
	h.mu.Unlock()

	go func() {
		if err := src.Run(ctx, h.samplesIn(id)); err != nil && !errors.Is(err, context.Canceled) {
			h.log.Error("metric source exited", "host_id", id, "err", err)
		}
	}()
	h.log.Info("registered host", "id", id, "name", info.Name, "source", info.Source)
	return id, nil
}

// samplesIn returns a one-way send channel that stamps host_id onto every
// sample before forwarding to the central ingestion channel. This means
// MetricSource implementations don't need to know the host_id they were
// assigned.
func (h *Hub) samplesIn(hostID string) chan<- types.MetricSample {
	out := make(chan types.MetricSample, 16)
	go func() {
		for s := range out {
			s.HostID = hostID
			select {
			case h.samples <- s:
			default:
				h.log.Warn("dropping metric: ingest buffer full", "host_id", hostID)
			}
		}
	}()
	return out
}

// RegisterDocker attaches a docker provider for a host and marks the host kind as "docker".
func (h *Hub) RegisterDocker(hostID string, p DockerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dockers[hostID] = p
	// Best-effort: update kind so the UI can show the docker chip.
	go func() {
		ctx := context.Background()
		if err := h.store.UpdateHostKind(ctx, hostID, "docker"); err != nil {
			h.log.Warn("update host kind", "host_id", hostID, "err", err)
		}
	}()
}

func (h *Hub) Docker(hostID string) (DockerProvider, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.dockers[hostID]
	return p, ok
}

// RegisterCompose attaches a compose provider for a host.
func (h *Hub) RegisterCompose(hostID string, p ComposeProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.composes[hostID] = p
}

// Compose returns the compose provider for a host, if one is registered.
func (h *Hub) Compose(hostID string) (ComposeProvider, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.composes[hostID]
	return p, ok
}

// RegisterTerminal attaches a terminal provider for a host.
func (h *Hub) RegisterTerminal(hostID string, p TerminalProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.terminals[hostID] = p
}

// Terminal returns the terminal provider for a host, if one is registered.
func (h *Hub) Terminal(hostID string) (TerminalProvider, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.terminals[hostID]
	return p, ok
}

// UnregisterDocker removes the docker provider for a host (e.g. on disconnect).
func (h *Hub) UnregisterDocker(hostID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.dockers, hostID)
}

// UnregisterCompose removes the compose provider for a host (e.g. on disconnect).
func (h *Hub) UnregisterCompose(hostID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.composes, hostID)
}

// UnregisterTerminal removes the terminal provider for a host (e.g. on disconnect).
func (h *Hub) UnregisterTerminal(hostID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.terminals, hostID)
}

// LatestSample returns the most recently ingested full sample for a host.
// The returned sample includes rich live-only fields (per-core CPU, per-interface
// net, disk mounts, disk I/O, temps) that are not in the DB. Returns false if no
// sample has been ingested yet for this host.
func (h *Hub) LatestSample(hostID string) (types.MetricSample, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.latestRich[hostID]
	return s, ok
}

func (h *Hub) Store() *store.Store { return h.store }

// DeriveHostID makes a stable ID from a host's identity fields. We use a
// hash of name + source kind so the same host always maps to the same row,
// even across hub restarts. (When multi-host lands, an agent will provide
// its own UUID and this fallback is only used for the local source.)
func DeriveHostID(info types.HostInfo) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s|%s", info.Source, info.Name)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Compile-time check: dockerctl.Client satisfies DockerProvider.
var _ DockerProvider = (*dockerctl.Client)(nil)
