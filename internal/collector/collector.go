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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aperture/aperture/internal/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	gopsprocess "github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Local samples the machine the hub process runs on.
type Local struct {
	Interval   time.Duration
	DiskPath   string // root path to report disk usage for; default "/"
	NetDevice  string // empty means aggregate across all interfaces

	hostInfo   types.HostInfo
	infoLoaded bool

	// Rate tracking for per-interface and per-disk I/O deltas.
	prevTime    time.Time
	prevNetIO   map[string]netPrev
	prevDiskIO  map[string]diskPrev

	// Process cache: reused across ticks so CPUPercent(0) measures elapsed
	// time since last call rather than since object creation.
	procMu    sync.Mutex
	procCache map[int32]*gopsprocess.Process
}

type netPrev struct{ rx, tx uint64 }
type diskPrev struct{ rd, wr uint64 }

// pseudoFS is the set of filesystem types that are not real mounted storage.
// overlay is Docker's container layer FS and is filtered separately.
var pseudoFS = map[string]bool{
	"sysfs": true, "proc": true, "devtmpfs": true, "devpts": true,
	"tmpfs": true, "securityfs": true, "cgroup": true, "cgroup2": true,
	"pstore": true, "configfs": true, "debugfs": true, "hugetlbfs": true,
	"mqueue": true, "fusectl": true, "overlay": true, "squashfs": true,
	"efivarfs": true, "bpf": true, "tracefs": true, "autofs": true,
	"ramfs": true, "nsfs": true, "binfmt_misc": true,
}

func NewLocal(interval time.Duration) *Local {
	return &Local{
		Interval:   interval,
		DiskPath:   "/",
		prevNetIO:  make(map[string]netPrev),
		prevDiskIO: make(map[string]diskPrev),
		procCache:  make(map[int32]*gopsprocess.Process),
	}
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
	_, _ = cpu.Percent(0, true) // also prime per-core

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
	dt := now.Sub(l.prevTime).Seconds()
	if dt <= 0 {
		dt = l.Interval.Seconds()
	}
	s := types.MetricSample{Timestamp: now}

	// --- Aggregate CPU (stored in DB) ---
	if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
		s.CPUPercent = pcts[0]
	}

	// --- Per-core CPU (live only) ---
	if pcts, err := cpu.PercentWithContext(ctx, 0, true); err == nil && len(pcts) > 0 {
		s.CPUPerCore = pcts
	}

	// --- Memory ---
	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.MemUsed = v.Used
		s.MemTotal = v.Total
		s.MemPercent = v.UsedPercent
		s.MemAvail = v.Available
		s.MemCached = v.Cached
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		s.SwapUsed = sw.Used
		s.SwapTotal = sw.Total
	}

	// --- Disk usage for configured path (stored in DB) ---
	path := l.DiskPath
	if path == "" {
		path = "/"
	}
	if du, err := disk.UsageWithContext(ctx, path); err == nil {
		s.DiskUsed = du.Used
		s.DiskTotal = du.Total
		s.DiskPercent = du.UsedPercent
	}

	// --- All disk mounts (live only) ---
	s.DiskMounts = l.diskMounts(ctx)

	// --- Network aggregate (stored in DB) ---
	if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
		s.NetRxBytes = io[0].BytesRecv
		s.NetTxBytes = io[0].BytesSent
	}

	// --- Per-interface network (live only) ---
	s.NetIfaces = l.netIfaces(ctx, dt)

	// --- Disk I/O per device (live only) ---
	s.DiskIO = l.diskIO(ctx, dt)

	// --- Load average ---
	if la, err := load.AvgWithContext(ctx); err == nil {
		s.LoadAvg1 = la.Load1
		s.LoadAvg5 = la.Load5
		s.LoadAvg15 = la.Load15
	}

	// --- Uptime ---
	if u, err := host.UptimeWithContext(ctx); err == nil {
		s.UptimeSecs = u
	}

	// --- Temperature sensors (live only, optional) ---
	if temps, err := sensors.TemperaturesWithContext(ctx); err == nil {
		seen := make(map[string]bool)
		for _, t := range temps {
			if t.Temperature <= 0 {
				continue
			}
			if seen[t.SensorKey] {
				continue
			}
			seen[t.SensorKey] = true
			s.Temps = append(s.Temps, types.TempSample{
				Name: t.SensorKey,
				Temp: t.Temperature,
			})
		}
	}

	// --- Process list (live only) ---
	s.Processes = l.processes(ctx)

	l.prevTime = now
	return s, nil
}

const maxTopProc = 20

// processes returns the top processes by CPU ∪ top processes by RSS memory.
// Process objects are cached across ticks so CPUPercent(0) accumulates
// correctly — on the first tick a newly-seen process reports 0% CPU.
func (l *Local) processes(ctx context.Context) []types.ProcessSample {
	l.procMu.Lock()
	defer l.procMu.Unlock()

	pids, err := gopsprocess.PidsWithContext(ctx)
	if err != nil {
		return nil
	}

	live := make(map[int32]bool, len(pids))
	for _, pid := range pids {
		live[pid] = true
	}
	// Evict dead processes from cache.
	for pid := range l.procCache {
		if !live[pid] {
			delete(l.procCache, pid)
		}
	}
	// Add new processes to cache (first CPU call establishes baseline).
	for _, pid := range pids {
		if _, ok := l.procCache[pid]; !ok {
			if p, err := gopsprocess.NewProcessWithContext(ctx, pid); err == nil {
				l.procCache[pid] = p
			}
		}
	}

	var all []types.ProcessSample
	for _, p := range l.procCache {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		cpuPct, err := p.CPUPercentWithContext(ctx)
		if err != nil {
			continue
		}
		memInfo, err := p.MemoryInfoWithContext(ctx)
		if err != nil {
			continue
		}
		memPct, _ := p.MemoryPercentWithContext(ctx)
		all = append(all, types.ProcessSample{
			PID:    p.Pid,
			Name:   name,
			CPUPct: cpuPct,
			MemPct: float64(memPct),
			MemRSS: memInfo.RSS,
		})
	}

	// Union of top N by CPU and top N by RSS.
	byCPU := append([]types.ProcessSample(nil), all...)
	sort.Slice(byCPU, func(i, j int) bool { return byCPU[i].CPUPct > byCPU[j].CPUPct })
	byMem := append([]types.ProcessSample(nil), all...)
	sort.Slice(byMem, func(i, j int) bool { return byMem[i].MemRSS > byMem[j].MemRSS })

	seen := make(map[int32]bool)
	var result []types.ProcessSample
	for _, s := range byCPU {
		if len(result) >= maxTopProc {
			break
		}
		if !seen[s.PID] {
			seen[s.PID] = true
			result = append(result, s)
		}
	}
	for _, s := range byMem {
		if len(result) >= maxTopProc*2 {
			break
		}
		if !seen[s.PID] {
			seen[s.PID] = true
			result = append(result, s)
		}
	}
	return result
}

// diskMounts returns real (non-pseudo) mounted filesystems with their usage.
func (l *Local) diskMounts(ctx context.Context) []types.DiskMountSample {
	parts, err := disk.PartitionsWithContext(ctx, true) // all=true to catch NFS etc.
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []types.DiskMountSample
	for _, p := range parts {
		if pseudoFS[p.Fstype] {
			continue
		}
		// Skip Docker overlay / containerd paths.
		if strings.Contains(p.Mountpoint, "/docker/") ||
			strings.Contains(p.Mountpoint, "/containerd/") ||
			strings.Contains(p.Mountpoint, "/overlay") {
			continue
		}
		// Skip autofs meta-entries.
		if p.Device == "systemd-1" || p.Device == "none" || p.Device == "" {
			continue
		}
		if seen[p.Mountpoint] {
			continue
		}
		seen[p.Mountpoint] = true

		du, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || du.Total == 0 {
			continue
		}
		out = append(out, types.DiskMountSample{
			Device:  p.Device,
			Mount:   p.Mountpoint,
			FSType:  p.Fstype,
			Used:    du.Used,
			Total:   du.Total,
			Percent: du.UsedPercent,
		})
	}
	return out
}

// netIfaces returns per-interface cumulative byte counters and derived rates.
func (l *Local) netIfaces(ctx context.Context, dt float64) []types.NetInterfaceSample {
	ifaces, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil
	}
	var out []types.NetInterfaceSample
	for _, iface := range ifaces {
		// Skip loopback and per-container virtual interfaces.
		if iface.Name == "lo" || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		prev := l.prevNetIO[iface.Name]
		var rxRate, txRate float64
		if prev.rx > 0 && iface.BytesRecv >= prev.rx {
			rxRate = float64(iface.BytesRecv-prev.rx) / dt
		}
		if prev.tx > 0 && iface.BytesSent >= prev.tx {
			txRate = float64(iface.BytesSent-prev.tx) / dt
		}
		l.prevNetIO[iface.Name] = netPrev{rx: iface.BytesRecv, tx: iface.BytesSent}
		out = append(out, types.NetInterfaceSample{
			Name:    iface.Name,
			RxBytes: iface.BytesRecv,
			TxBytes: iface.BytesSent,
			RxRate:  rxRate,
			TxRate:  txRate,
		})
	}
	return out
}

// diskIO returns per-device cumulative I/O byte counters and derived rates.
func (l *Local) diskIO(ctx context.Context, dt float64) []types.DiskIOSample {
	counters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return nil
	}
	var out []types.DiskIOSample
	for dev, c := range counters {
		// Skip loop, ram, and zram devices.
		if strings.HasPrefix(dev, "loop") ||
			strings.HasPrefix(dev, "ram") ||
			strings.HasPrefix(dev, "zram") {
			continue
		}
		prev := l.prevDiskIO[dev]
		var rdRate, wrRate float64
		if prev.rd > 0 && c.ReadBytes >= prev.rd {
			rdRate = float64(c.ReadBytes-prev.rd) / dt
		}
		if prev.wr > 0 && c.WriteBytes >= prev.wr {
			wrRate = float64(c.WriteBytes-prev.wr) / dt
		}
		l.prevDiskIO[dev] = diskPrev{rd: c.ReadBytes, wr: c.WriteBytes}
		out = append(out, types.DiskIOSample{
			Device:     dev,
			ReadBytes:  c.ReadBytes,
			WriteBytes: c.WriteBytes,
			ReadRate:   rdRate,
			WriteRate:  wrRate,
		})
	}
	// Sort by device name for stable ordering.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Device < out[j-1].Device; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
