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
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
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
	cpuPct, memPct        float64
	memUsed, memLimit     uint64
	netRx, netTx          uint64
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
