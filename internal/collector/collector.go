// Package collector samples host-level system metrics.
//
// The Local collector implements hub.MetricSource by reading from the local
// machine via gopsutil. Future remote agents will produce samples in the
// same shape (types.MetricSample) and feed them into the hub's ingestion
// path the same way.
package collector

import (
	"container/heap"
	"context"
	"fmt"
	"os"
	"runtime"
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

	// Mount partition cache. disk.PartitionsWithContext is a comparatively
	// expensive syscall whose result barely changes between ticks; cache it
	// with a short TTL and only re-query disk.UsageWithContext per cached
	// mount each tick.
	mountListMu  sync.Mutex
	mountList    []disk.PartitionStat
	mountListAt  time.Time
	mountListTTL time.Duration

	// Configurable family enablement + filters. Default is "all on, no
	// filters" matching pre-config behavior; ApplyConfig swaps in a host_config.
	// Reads happen on every sample() call; mutex protects swap during writes.
	cfgMu      sync.RWMutex
	enabledSet map[string]bool // nil = all enabled (default)
	filters    types.HostConfigFilters
	memCalc    string // "used" (default) | "avail"
}

// AllFamilies is the canonical list of families inlined in Local.sample().
// Used as the default when no host_config rows exists. Kept here (not in
// host_config.go) so adding/removing a family in this file is a one-line
// change. The optional families (smart, gpu, battery, systemd) live in the
// families/ subpackage and are not represented here.
var AllFamilies = []string{
	"cpu", "mem", "disk", "net", "load", "uptime",
	"temps", "processes", "cpu_per_core", "disk_io", "mounts",
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
		Interval:     interval,
		DiskPath:     "/",
		prevNetIO:    make(map[string]netPrev),
		prevDiskIO:   make(map[string]diskPrev),
		procCache:    make(map[int32]*gopsprocess.Process),
		mountListTTL: 30 * time.Second,
		// enabledSet=nil means "all families enabled" — pre-config default.
	}
}

// ApplyConfig swaps in a per-host monitoring policy. Safe to call at any
// time (including concurrently with sample()); the new config takes effect
// on the next tick. Passing the zero value resets to "all families enabled".
func (l *Local) ApplyConfig(cfg types.HostConfig) {
	l.cfgMu.Lock()
	defer l.cfgMu.Unlock()
	if len(cfg.EnabledFamilies) == 0 {
		l.enabledSet = nil
	} else {
		set := make(map[string]bool, len(cfg.EnabledFamilies))
		for _, f := range cfg.EnabledFamilies {
			set[f] = true
		}
		l.enabledSet = set
	}
	l.filters = cfg.Filters
	l.memCalc = cfg.MemCalc
	if cfg.SampleIntervalS > 0 {
		l.Interval = time.Duration(cfg.SampleIntervalS) * time.Second
	}
}

// familyEnabled reports whether the named family should run this tick.
// Always-on when no config applied. Reads under RLock so sampling threads
// can't starve a config push.
func (l *Local) familyEnabled(name string) bool {
	l.cfgMu.RLock()
	defer l.cfgMu.RUnlock()
	if l.enabledSet == nil {
		return true
	}
	return l.enabledSet[name]
}

// getFilters returns the current filter set (a copy, so callers can iterate
// without holding the lock).
func (l *Local) getFilters() types.HostConfigFilters {
	l.cfgMu.RLock()
	defer l.cfgMu.RUnlock()
	return l.filters
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
	if l.familyEnabled("cpu") {
		if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
			s.CPUPercent = pcts[0]
		}
	}

	// --- Per-core CPU ---
	if l.familyEnabled("cpu_per_core") {
		if pcts, err := cpu.PercentWithContext(ctx, 0, true); err == nil && len(pcts) > 0 {
			s.CPUPerCore = pcts
		}
	}

	// --- Memory ---
	if l.familyEnabled("mem") {
		if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
			s.MemTotal = v.Total
			s.MemAvail = v.Available
			s.MemCached = v.Cached
			// Memory "used" can be computed two ways. Default ("used"):
			// gopsutil's v.Used / v.UsedPercent (which is total - free - cached - buffers).
			// "avail" mode: total - MemAvailable, which matches what htop and
			// most modern tools show. Useful on hosts with heavy page cache
			// where the default looks deceptively idle.
			if l.getMemCalc() == "avail" && v.Available > 0 && v.Total > 0 {
				s.MemUsed = v.Total - v.Available
				s.MemPercent = float64(s.MemUsed) / float64(v.Total) * 100.0
			} else {
				s.MemUsed = v.Used
				s.MemPercent = v.UsedPercent
			}
		}
		if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
			s.SwapUsed = sw.Used
			s.SwapTotal = sw.Total
		}
	}

	// --- Disk usage for configured path (stored in DB) ---
	if l.familyEnabled("disk") {
		path := l.DiskPath
		if path == "" {
			path = "/"
		}
		if du, err := disk.UsageWithContext(ctx, path); err == nil {
			s.DiskUsed = du.Used
			s.DiskTotal = du.Total
			s.DiskPercent = du.UsedPercent
		}
	}

	// --- All disk mounts (live + persisted history) ---
	if l.familyEnabled("mounts") {
		s.DiskMounts = l.diskMounts(ctx)
	}

	// --- Network aggregate ---
	if l.familyEnabled("net") {
		if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
			s.NetRxBytes = io[0].BytesRecv
			s.NetTxBytes = io[0].BytesSent
		}
		// Per-interface lives under the same "net" family — they're collected
		// together from gopsutil's IOCountersWithContext(true).
		s.NetIfaces = l.netIfaces(ctx, dt)
	}

	// --- Disk I/O per device ---
	if l.familyEnabled("disk_io") {
		s.DiskIO = l.diskIO(ctx, dt)
	}

	// --- Load average ---
	if l.familyEnabled("load") {
		if la, err := load.AvgWithContext(ctx); err == nil {
			s.LoadAvg1 = la.Load1
			s.LoadAvg5 = la.Load5
			s.LoadAvg15 = la.Load15
		}
	}

	// --- Uptime ---
	if l.familyEnabled("uptime") {
		if u, err := host.UptimeWithContext(ctx); err == nil {
			s.UptimeSecs = u
		}
	}

	// --- Temperature sensors ---
	if l.familyEnabled("temps") {
		s.Temps = l.tempSensors(ctx)
	}

	// --- Process list ---
	if l.familyEnabled("processes") {
		s.Processes = l.processes(ctx)
	}

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

	// Union of top-K by CPU and top-K by RSS, using min-heaps so we are
	// O(N log K) per axis rather than O(N log N) full sorts. With K=20 this
	// is meaningful on hosts with many processes.
	topCPU := topKByCPU(all, maxTopProc)
	topMem := topKByRSS(all, maxTopProc)

	seen := make(map[int32]bool, maxTopProc*2)
	result := make([]types.ProcessSample, 0, maxTopProc*2)
	for _, s := range topCPU {
		if !seen[s.PID] {
			seen[s.PID] = true
			result = append(result, s)
		}
	}
	for _, s := range topMem {
		if !seen[s.PID] {
			seen[s.PID] = true
			result = append(result, s)
		}
	}
	return result
}

// procCPUHeap is a min-heap on CPUPct: heap[0] is the lowest-CPU element so
// we can cheaply evict when the heap grows past K and keep only the top-K.
type procCPUHeap []types.ProcessSample

func (h procCPUHeap) Len() int            { return len(h) }
func (h procCPUHeap) Less(i, j int) bool  { return h[i].CPUPct < h[j].CPUPct }
func (h procCPUHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *procCPUHeap) Push(x any)         { *h = append(*h, x.(types.ProcessSample)) }
func (h *procCPUHeap) Pop() any           { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

type procRSSHeap []types.ProcessSample

func (h procRSSHeap) Len() int           { return len(h) }
func (h procRSSHeap) Less(i, j int) bool { return h[i].MemRSS < h[j].MemRSS }
func (h procRSSHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *procRSSHeap) Push(x any)        { *h = append(*h, x.(types.ProcessSample)) }
func (h *procRSSHeap) Pop() any          { old := *h; n := len(old); x := old[n-1]; *h = old[:n-1]; return x }

// topKByCPU returns the top-k entries by CPUPct in descending order.
func topKByCPU(xs []types.ProcessSample, k int) []types.ProcessSample {
	if k <= 0 || len(xs) == 0 {
		return nil
	}
	h := &procCPUHeap{}
	heap.Init(h)
	for _, x := range xs {
		if h.Len() < k {
			heap.Push(h, x)
		} else if x.CPUPct > (*h)[0].CPUPct {
			(*h)[0] = x
			heap.Fix(h, 0)
		}
	}
	out := make([]types.ProcessSample, h.Len())
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(h).(types.ProcessSample)
	}
	return out
}

// topKByRSS returns the top-k entries by MemRSS in descending order.
func topKByRSS(xs []types.ProcessSample, k int) []types.ProcessSample {
	if k <= 0 || len(xs) == 0 {
		return nil
	}
	h := &procRSSHeap{}
	heap.Init(h)
	for _, x := range xs {
		if h.Len() < k {
			heap.Push(h, x)
		} else if x.MemRSS > (*h)[0].MemRSS {
			(*h)[0] = x
			heap.Fix(h, 0)
		}
	}
	out := make([]types.ProcessSample, h.Len())
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(h).(types.ProcessSample)
	}
	return out
}

// diskMounts returns real (non-pseudo) mounted filesystems with their usage.
// The partition list is cached (TTL = mountListTTL, default 30s) since the
// set of mounts rarely changes between ticks and disk.PartitionsWithContext
// is comparatively expensive. disk.UsageWithContext is still called fresh
// per mount each tick. Mount allow/deny filters from host_config are applied.
func (l *Local) diskMounts(ctx context.Context) []types.DiskMountSample {
	parts, err := l.cachedPartitions(ctx)
	if err != nil {
		return nil
	}
	filters := l.getFilters()
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
		if !allowName(p.Mountpoint, filters.MountAllow, filters.MountDeny) {
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
// NIC allow/deny filters from host_config are applied on top of the
// built-in (lo, veth*) exclusions.
func (l *Local) netIfaces(ctx context.Context, dt float64) []types.NetInterfaceSample {
	ifaces, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil
	}
	filters := l.getFilters()
	var out []types.NetInterfaceSample
	for _, iface := range ifaces {
		// Skip loopback and per-container virtual interfaces.
		if iface.Name == "lo" || strings.HasPrefix(iface.Name, "veth") {
			continue
		}
		if !allowName(iface.Name, filters.NICAllow, filters.NICDeny) {
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

// getMemCalc returns the configured memory-calculation mode (default "used").
func (l *Local) getMemCalc() string {
	l.cfgMu.RLock()
	defer l.cfgMu.RUnlock()
	return l.memCalc
}

// tempSensors collects temperature readings, applying sensor allow/deny
// filters from the host_config. Duplicate sensor keys (gopsutil reports
// some sensors twice on multi-socket systems) are de-duplicated, and zero
// readings are skipped (gopsutil's "unavailable" marker).
func (l *Local) tempSensors(ctx context.Context) []types.TempSample {
	raw, err := sensors.TemperaturesWithContext(ctx)
	if err != nil {
		return nil
	}
	filters := l.getFilters()
	seen := make(map[string]bool)
	var out []types.TempSample
	for _, t := range raw {
		if t.Temperature <= 0 {
			continue
		}
		if seen[t.SensorKey] {
			continue
		}
		if !allowName(t.SensorKey, filters.SensorAllow, filters.SensorDeny) {
			continue
		}
		seen[t.SensorKey] = true
		out = append(out, types.TempSample{Name: t.SensorKey, Temp: t.Temperature})
	}
	return out
}

// allowName applies allow/deny semantics: deny wins over allow; an empty
// allow list means "all allowed (subject to deny)". Used by every filter
// (NIC, sensor, mount, container).
func allowName(name string, allow, deny []string) bool {
	for _, d := range deny {
		if d == name {
			return false
		}
	}
	if len(allow) == 0 {
		return true
	}
	for _, a := range allow {
		if a == name {
			return true
		}
	}
	return false
}

// cachedPartitions returns disk.PartitionStat list, using a short-TTL cache to
// avoid the comparatively expensive disk.PartitionsWithContext syscall on
// every sample. Mount points rarely change between ticks; usage is still
// re-queried per call.
func (l *Local) cachedPartitions(ctx context.Context) ([]disk.PartitionStat, error) {
	l.mountListMu.Lock()
	defer l.mountListMu.Unlock()
	if l.mountList != nil && time.Since(l.mountListAt) < l.mountListTTL {
		return l.mountList, nil
	}
	parts, err := disk.PartitionsWithContext(ctx, true) // all=true to catch NFS etc.
	if err != nil {
		return nil, err
	}
	l.mountList = parts
	l.mountListAt = time.Now()
	return parts, nil
}
