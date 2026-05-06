// Package collector samples host-level system metrics.
//
// The Local collector implements hub.MetricSource by reading from the local
// machine via gopsutil. Future remote agents will produce samples in the
// same shape (types.MetricSample) and feed them into the hub's ingestion
// path the same way.
package collector

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/aperture/aperture/internal/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Local samples the machine the hub process runs on.
type Local struct {
	Interval   time.Duration
	DiskPath   string // root path to report disk usage for; default "/"
	NetDevice  string // empty means aggregate across all interfaces
	hostInfo   types.HostInfo
	infoLoaded bool
}

func NewLocal(interval time.Duration) *Local {
	return &Local{Interval: interval, DiskPath: "/"}
}

func (l *Local) HostInfo() types.HostInfo {
	if l.infoLoaded {
		return l.hostInfo
	}
	info := types.HostInfo{Source: "local", Arch: runtime.GOARCH}
	if h, err := host.Info(); err == nil {
		info.Name = h.Hostname
		info.OS = h.OS
		info.Platform = fmt.Sprintf("%s %s", h.Platform, h.PlatformVersion)
		info.Kernel = h.KernelVersion
	} else {
		hn, _ := os.Hostname()
		info.Name = hn
		info.OS = runtime.GOOS
	}
	if cs, err := cpu.Info(); err == nil && len(cs) > 0 {
		info.CPUModel = cs[0].ModelName
	}
	if n, err := cpu.Counts(true); err == nil {
		info.CPUCount = n
	}
	if v, err := mem.VirtualMemory(); err == nil {
		info.MemTotal = v.Total
	}
	l.hostInfo = info
	l.infoLoaded = true
	return info
}

// Run samples on Interval until ctx is cancelled. Each sample is sent on out;
// if the receiver is slow the sample is dropped to avoid backing up collection.
func (l *Local) Run(ctx context.Context, out chan<- types.MetricSample) error {
	if l.Interval <= 0 {
		l.Interval = 5 * time.Second
	}
	// Prime cpu.Percent so the first reading is meaningful.
	_, _ = cpu.Percent(0, false)

	t := time.NewTicker(l.Interval)
	defer t.Stop()
	// Send one sample immediately, then on each tick.
	if s, err := l.sample(ctx); err == nil {
		send(out, s)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			s, err := l.sample(ctx)
			if err != nil {
				continue
			}
			send(out, s)
		}
	}
}

func send(out chan<- types.MetricSample, s types.MetricSample) {
	select {
	case out <- s:
	default:
		// drop on backpressure
	}
}

func (l *Local) sample(ctx context.Context) (types.MetricSample, error) {
	now := time.Now().UTC()
	s := types.MetricSample{Timestamp: now}

	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
		s.CPUPercent = pcts[0]
	}
	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.MemUsed = v.Used
		s.MemTotal = v.Total
		s.MemPercent = v.UsedPercent
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		s.SwapUsed = sw.Used
		s.SwapTotal = sw.Total
	}
	path := l.DiskPath
	if path == "" {
		path = "/"
	}
	if du, err := disk.UsageWithContext(ctx, path); err == nil {
		s.DiskUsed = du.Used
		s.DiskTotal = du.Total
		s.DiskPercent = du.UsedPercent
	}
	if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
		s.NetRxBytes = io[0].BytesRecv
		s.NetTxBytes = io[0].BytesSent
	}
	if la, err := load.AvgWithContext(ctx); err == nil {
		s.LoadAvg1 = la.Load1
		s.LoadAvg5 = la.Load5
		s.LoadAvg15 = la.Load15
	}
	if u, err := host.UptimeWithContext(ctx); err == nil {
		s.UptimeSecs = u
	}
	return s, nil
}
