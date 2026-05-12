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
	// evaluator is set by SetEvaluator before Run is called. The interface
	// type avoids an import cycle with internal/alerts.
	evaluator Evaluator
}

// Evaluator is the seam used by the hub to dispatch every persisted sample
// to the alert evaluator. *alerts.Evaluator satisfies this; tests can
// substitute a stub.
type Evaluator interface {
	Evaluate(ctx context.Context, sample types.MetricSample)
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
		store:      cfg.Store,
		log:        cfg.Logger,
		retain:     cfg.Retain,
		dockers:    make(map[string]DockerProvider),
		composes:   make(map[string]ComposeProvider),
		terminals:  make(map[string]TerminalProvider),
		hosts:      make(map[string]types.Host),
		samples:    make(chan types.MetricSample, 256),
		latestRich: make(map[string]types.MetricSample),
	}
}

// Run starts background workers: ingestion, retention. Returns when ctx is done.
func (h *Hub) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.ingestLoop(ctx)
	}()
	if h.retain > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.retentionLoop(ctx)
		}()
	}
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
			// Best-effort: rich table failures don't abort the ingest loop.
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
			_ = h.store.TouchHost(ctx, s.HostID, s.Timestamp)
			if h.evaluator != nil {
				h.evaluator.Evaluate(ctx, s)
			}
		}
	}
}

// SetEvaluator installs the alert evaluator. Must be called before Run for
// alerts to fire on the first sample. Calling later is permitted (rules just
// won't evaluate until then) — useful for tests that want to register hosts
// before any rules exist.
func (h *Hub) SetEvaluator(e Evaluator) {
	h.evaluator = e
}

func (h *Hub) retentionLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().UTC().Add(-h.retain)
			if n, err := h.store.PruneMetrics(ctx, cutoff); err == nil && n > 0 {
				h.log.Info("pruned metrics", "rows", n, "cutoff", cutoff)
			}
		}
	}
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

// RegisterDocker attaches a docker provider for a host.
func (h *Hub) RegisterDocker(hostID string, p DockerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dockers[hostID] = p
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
