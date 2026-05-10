// Package dockerctl wraps the docker engine API for the local host.
//
// Multi-host note: in v0.1 only the hub's local docker socket is queried.
// When remote agents land, each agent will expose the same surface via a
// network transport and a Manager implementation will dispatch by host_id.
package dockerctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aperture/aperture/internal/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Client struct {
	hostID string
	cli    *client.Client
}

func New(hostID string) (*Client, error) {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{hostID: hostID, cli: c}, nil
}

func (c *Client) Close() error { return c.cli.Close() }

// Ping verifies the docker daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// List returns containers on this host with point-in-time CPU/mem stats.
func (c *Client) List(ctx context.Context, all bool) ([]types.Container, error) {
	cs, err := c.cli.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, err
	}
	out := make([]types.Container, 0, len(cs))
	for _, ci := range cs {
		name := ""
		if len(ci.Names) > 0 {
			name = strings.TrimPrefix(ci.Names[0], "/")
		}
		ports := make([]types.PortMapping, 0, len(ci.Ports))
		for _, p := range ci.Ports {
			ports = append(ports, types.PortMapping{
				IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type,
			})
		}
		c2 := types.Container{
			HostID:    c.hostID,
			ID:        ci.ID,
			Name:      name,
			Image:     ci.Image,
			State:     ci.State,
			Status:    ci.Status,
			CreatedAt: time.Unix(ci.Created, 0).UTC(),
			Ports:     ports,
			Labels:    ci.Labels,
		}
		// Stats only meaningful for running containers.
		if ci.State == "running" {
			if st, err := c.stats(ctx, ci.ID); err == nil {
				c2.CPUPercent = st.cpuPct
				c2.MemUsage = st.memUsed
				c2.MemLimit = st.memLimit
				c2.MemPercent = st.memPct
				c2.NetRxBytes = st.netRx
				c2.NetTxBytes = st.netTx
			}
		}
		out = append(out, c2)
	}
	return out, nil
}

type containerStats struct {
	cpuPct, memPct    float64
	memUsed, memLimit uint64
	netRx, netTx      uint64
}

func (c *Client) stats(ctx context.Context, id string) (containerStats, error) {
	var zero containerStats
	resp, err := c.cli.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	var v container.StatsResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return zero, err
	}
	return computeStats(&v), nil
}

func computeStats(v *container.StatsResponse) containerStats {
	var s containerStats
	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	cores := float64(v.CPUStats.OnlineCPUs)
	if cores == 0 {
		cores = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta > 0 && cpuDelta > 0 && cores > 0 {
		s.cpuPct = (cpuDelta / systemDelta) * cores * 100.0
	}
	s.memUsed = v.MemoryStats.Usage
	if cache, ok := v.MemoryStats.Stats["cache"]; ok && cache <= s.memUsed {
		s.memUsed -= cache
	}
	s.memLimit = v.MemoryStats.Limit
	if s.memLimit > 0 {
		s.memPct = float64(s.memUsed) / float64(s.memLimit) * 100.0
	}
	for _, n := range v.Networks {
		s.netRx += n.RxBytes
		s.netTx += n.TxBytes
	}
	return s
}

// --- Lifecycle actions (used by v0.1's container-management story) ---

func (c *Client) Start(ctx context.Context, id string) error {
	return c.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (c *Client) Stop(ctx context.Context, id string, timeoutSec *int) error {
	return c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: timeoutSec})
}

func (c *Client) Restart(ctx context.Context, id string, timeoutSec *int) error {
	return c.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: timeoutSec})
}

func (c *Client) Pause(ctx context.Context, id string) error {
	return c.cli.ContainerPause(ctx, id)
}

func (c *Client) Unpause(ctx context.Context, id string) error {
	return c.cli.ContainerUnpause(ctx, id)
}

func (c *Client) Kill(ctx context.Context, id, signal string) error {
	if signal == "" {
		signal = "SIGKILL"
	}
	return c.cli.ContainerKill(ctx, id, signal)
}

func (c *Client) Remove(ctx context.Context, id string, force, removeVolumes bool) error {
	return c.cli.ContainerRemove(ctx, id, container.RemoveOptions{
		Force: force, RemoveVolumes: removeVolumes,
	})
}

// Logs returns the last n lines of container logs.
func (c *Client) Logs(ctx context.Context, id string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	r, err := c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: fmt.Sprintf("%d", tail),
	})
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	// Strip the 8-byte multiplexed log header docker prepends to each line.
	return stripLogHeaders(b), nil
}

func stripLogHeaders(b []byte) string {
	var sb strings.Builder
	for len(b) > 0 {
		if len(b) < 8 {
			sb.Write(b)
			break
		}
		sz := int(b[4])<<24 | int(b[5])<<16 | int(b[6])<<8 | int(b[7])
		b = b[8:]
		if sz > len(b) {
			sz = len(b)
		}
		sb.Write(b[:sz])
		b = b[sz:]
	}
	return sb.String()
}

// Inspect returns the full configuration and live stats for a container.
func (c *Client) Inspect(ctx context.Context, id string) (*types.ContainerInspect, error) {
	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return nil, err
	}
	return buildInspect(ctx, c, &info), nil
}

func buildInspect(ctx context.Context, c *Client, info *dockertypes.ContainerJSON) *types.ContainerInspect {
	ci := &types.ContainerInspect{
		ID:    info.ID,
		Name:  strings.TrimPrefix(info.Name, "/"),
		Image: info.Config.Image,
		State: info.State.Status,
	}

	// Timestamps.
	if t, err := time.Parse(time.RFC3339Nano, info.Created); err == nil {
		ci.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil && t.Year() > 1970 {
		ci.StartedAt = &t
	}
	if t, err := time.Parse(time.RFC3339Nano, info.State.FinishedAt); err == nil && t.Year() > 1970 && info.State.FinishedAt != "0001-01-01T00:00:00Z" {
		ci.FinishedAt = &t
	}

	// Status string from docker (e.g. "Up 2 hours").
	ci.Status = info.State.Status

	// Config.
	ci.RestartPolicy = string(info.HostConfig.RestartPolicy.Name)
	ci.Env = info.Config.Env
	if ci.Env == nil {
		ci.Env = []string{}
	}
	if len(info.Config.Entrypoint) > 0 {
		ci.Entrypoint = []string(info.Config.Entrypoint)
	}
	if len(info.Config.Cmd) > 0 {
		ci.Cmd = []string(info.Config.Cmd)
	}
	ci.Labels = info.Config.Labels
	if ci.Labels == nil {
		ci.Labels = map[string]string{}
	}

	// Resource limits.
	ci.NanoCPUs = info.HostConfig.NanoCPUs
	ci.MemLimitBytes = info.HostConfig.Memory

	// Ports from NetworkSettings (actual host-bound ports).
	ci.Ports = []types.PortMapping{}
	for portKey, bindings := range info.NetworkSettings.Ports {
		priv := parsePort(string(portKey))
		proto := parseProto(string(portKey))
		if len(bindings) == 0 {
			ci.Ports = append(ci.Ports, types.PortMapping{PrivatePort: priv, Type: proto})
			continue
		}
		for _, b := range bindings {
			var pub uint16
			if b.HostPort != "" {
				var n int
				_, _ = fmt.Sscanf(b.HostPort, "%d", &n)
				pub = uint16(n)
			}
			ci.Ports = append(ci.Ports, types.PortMapping{
				IP: b.HostIP, PrivatePort: priv, PublicPort: pub, Type: proto,
			})
		}
	}

	// Mounts.
	ci.Mounts = []types.ContainerMount{}
	for _, m := range info.Mounts {
		ci.Mounts = append(ci.Mounts, types.ContainerMount{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			RW:          m.RW,
		})
	}

	// Live stats for running containers.
	if info.State.Running {
		if st, err := c.stats(ctx, info.ID); err == nil {
			ci.CPUPercent = st.cpuPct
			ci.MemUsage = st.memUsed
			ci.MemLimit = st.memLimit
			ci.MemPercent = st.memPct
			ci.NetRxBytes = st.netRx
			ci.NetTxBytes = st.netTx
		}
	}

	return ci
}

func parsePort(portStr string) uint16 {
	if i := strings.Index(portStr, "/"); i >= 0 {
		portStr = portStr[:i]
	}
	var n int
	_, _ = fmt.Sscanf(portStr, "%d", &n)
	return uint16(n)
}

func parseProto(portStr string) string {
	if i := strings.Index(portStr, "/"); i >= 0 {
		return portStr[i+1:]
	}
	return "tcp"
}

// UpdateResources updates the CPU and/or memory limits for a container.
// Both limits can be changed live without restarting the container (cgroup v2
// required for CPU limits). A value of 0 removes the limit (unlimited).
func (c *Client) UpdateResources(ctx context.Context, id string, update types.ResourceUpdate) error {
	res := container.Resources{}
	if update.NanoCPUs != nil {
		res.NanoCPUs = *update.NanoCPUs
	}
	if update.MemoryBytes != nil {
		res.Memory = *update.MemoryBytes
	}
	_, err := c.cli.ContainerUpdate(ctx, id, container.UpdateConfig{Resources: res})
	return err
}

// Create makes a new container from the surface-layer spec and (optionally)
// starts it. If the image isn't local, it's pulled before retrying create —
// this keeps the common case (image already cached) fast while still working
// for fresh images. Returns the new container's id.
//
// Surface scope: only image, name, restart policy, env, ports, volumes, and
// auto-start are accepted. Deeper container configuration belongs in the
// compose-first work (roadmap section 2) where YAML is the natural surface
// for the long tail of options.
func (c *Client) Create(ctx context.Context, spec types.CreateSpec) (string, error) {
	if strings.TrimSpace(spec.Image) == "" {
		return "", errors.New("image is required")
	}

	cfg, hostCfg, err := buildCreateConfig(spec)
	if err != nil {
		return "", err
	}

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, spec.Name)
	if err != nil && client.IsErrNotFound(err) {
		// Image not local — pull, then retry create once.
		rc, perr := c.cli.ImagePull(ctx, spec.Image, image.PullOptions{})
		if perr != nil {
			return "", fmt.Errorf("pull %s: %w", spec.Image, perr)
		}
		// Drain the progress stream so the pull actually completes.
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
		resp, err = c.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, spec.Name)
	}
	if err != nil {
		return "", err
	}

	if spec.AutoStart {
		if serr := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); serr != nil {
			// Container exists but didn't start — return the id with the error
			// so the caller can decide whether to leave or remove it.
			return resp.ID, fmt.Errorf("start: %w", serr)
		}
	}
	return resp.ID, nil
}

// buildCreateConfig translates a CreateSpec into the docker SDK's container
// and host configs. Pulled out for testability and to keep Create readable.
func buildCreateConfig(spec types.CreateSpec) (*container.Config, *container.HostConfig, error) {
	cfg := &container.Config{Image: spec.Image}

	// Env: map -> []"K=V" with stable ordering (sort keys would be cleaner;
	// for v0.1 the docker create order doesn't affect runtime semantics).
	if len(spec.Env) > 0 {
		env := make([]string, 0, len(spec.Env))
		for k, v := range spec.Env {
			env = append(env, k+"="+v)
		}
		cfg.Env = env
	}

	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	for _, p := range spec.Ports {
		if p.ContainerPort <= 0 {
			return nil, nil, fmt.Errorf("port row missing container_port")
		}
		proto := strings.ToLower(p.Protocol)
		if proto == "" {
			proto = "tcp"
		}
		if proto != "tcp" && proto != "udp" {
			return nil, nil, fmt.Errorf("port protocol must be tcp or udp, got %q", p.Protocol)
		}
		port, err := nat.NewPort(proto, fmt.Sprintf("%d", p.ContainerPort))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid container port %d/%s: %w", p.ContainerPort, proto, err)
		}
		exposed[port] = struct{}{}
		hostPort := ""
		if p.HostPort > 0 {
			hostPort = fmt.Sprintf("%d", p.HostPort)
		}
		bindings[port] = append(bindings[port], nat.PortBinding{HostPort: hostPort})
	}
	cfg.ExposedPorts = exposed

	hostCfg := &container.HostConfig{PortBindings: bindings}

	for _, v := range spec.Volumes {
		if v.HostPath == "" || v.ContainerPath == "" {
			return nil, nil, fmt.Errorf("volume row needs both host_path and container_path")
		}
		bind := v.HostPath + ":" + v.ContainerPath
		if v.ReadOnly {
			bind += ":ro"
		}
		hostCfg.Binds = append(hostCfg.Binds, bind)
	}

	if spec.RestartPolicy != "" {
		switch spec.RestartPolicy {
		case "no", "on-failure", "always", "unless-stopped":
			hostCfg.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)}
		default:
			return nil, nil, fmt.Errorf("unsupported restart_policy %q", spec.RestartPolicy)
		}
	}

	return cfg, hostCfg, nil
}

// FilterRunning returns only the containers in state "running".
func FilterRunning(in []types.Container) []types.Container {
	out := in[:0:0]
	for _, c := range in {
		if c.State == "running" {
			out = append(out, c)
		}
	}
	return out
}

var ErrNoMatch = errors.New("no container matched")

// FindByName resolves a name (or id prefix) to a single container ID.
func (c *Client) FindByName(ctx context.Context, name string) (string, error) {
	args := filters.NewArgs()
	args.Add("name", name)
	cs, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return "", err
	}
	if len(cs) == 0 {
		return "", ErrNoMatch
	}
	return cs[0].ID, nil
}

// --- Network actions ---

func (c *Client) ListNetworks(ctx context.Context) ([]types.DockerNetwork, error) {
	nets, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]types.DockerNetwork, 0, len(nets))
	for _, n := range nets {
		var subnet, gateway string
		if len(n.IPAM.Config) > 0 {
			subnet = n.IPAM.Config[0].Subnet
			gateway = n.IPAM.Config[0].Gateway
		}
		out = append(out, types.DockerNetwork{
			HostID:   c.hostID,
			ID:       n.ID,
			Name:     n.Name,
			Driver:   n.Driver,
			Scope:    n.Scope,
			Subnet:   subnet,
			Gateway:  gateway,
			Internal: n.Internal,
			Labels:   n.Labels,
		})
	}
	return out, nil
}

func (c *Client) InspectNetwork(ctx context.Context, id string) (*types.DockerNetwork, error) {
	n, err := c.cli.NetworkInspect(ctx, id, network.InspectOptions{})
	if err != nil {
		return nil, err
	}
	var subnet, gateway string
	if len(n.IPAM.Config) > 0 {
		subnet = n.IPAM.Config[0].Subnet
		gateway = n.IPAM.Config[0].Gateway
	}
	var containers []types.NetworkContainer
	for cid, ep := range n.Containers {
		containers = append(containers, types.NetworkContainer{
			ID:          cid,
			Name:        strings.TrimPrefix(ep.Name, "/"),
			EndpointID:  ep.EndpointID,
			MacAddress:  ep.MacAddress,
			IPv4Address: ep.IPv4Address,
			IPv6Address: ep.IPv6Address,
		})
	}
	return &types.DockerNetwork{
		HostID:     c.hostID,
		ID:         n.ID,
		Name:       n.Name,
		Driver:     n.Driver,
		Scope:      n.Scope,
		Subnet:     subnet,
		Gateway:    gateway,
		Internal:   n.Internal,
		Labels:     n.Labels,
		Containers: containers,
	}, nil
}

func (c *Client) CreateNetwork(ctx context.Context, spec types.NetworkCreateSpec) (string, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return "", errors.New("network name is required")
	}
	opts := network.CreateOptions{
		Driver:   spec.Driver,
		Internal: spec.Internal,
		Labels:   spec.Labels,
	}
	resp, err := c.cli.NetworkCreate(ctx, spec.Name, opts)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	return c.cli.NetworkRemove(ctx, id)
}

func (c *Client) ConnectContainer(ctx context.Context, networkID, containerID string) error {
	return c.cli.NetworkConnect(ctx, networkID, containerID, nil)
}

func (c *Client) DisconnectContainer(ctx context.Context, networkID, containerID string) error {
	return c.cli.NetworkDisconnect(ctx, networkID, containerID, false)
}
