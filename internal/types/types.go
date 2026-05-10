package types

import "time"

// AgentToken represents a pre-shared secret used by remote agents to
// authenticate the WebSocket upgrade. The plaintext is only returned once
// (on creation); thereafter only the SHA-256 hash is stored.
type AgentToken struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	Revoked   bool       `json:"revoked"`
	// Token is only populated on creation — the plaintext is never stored.
	Token string `json:"token,omitempty"`
}

// Host is a machine that aperture knows about. In v0.1 the only host is the
// local one (auto-registered at startup), but the data model is multi-host
// from day 1.
type Host struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OS        string    `json:"os"`
	Platform  string    `json:"platform"`
	Kernel    string    `json:"kernel"`
	Arch      string    `json:"arch"`
	CPUModel  string    `json:"cpu_model"`
	CPUCount  int       `json:"cpu_count"`
	MemTotal  uint64    `json:"mem_total"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	// Source describes where this host's metrics come from. "local" means the
	// hub process collects them in-process. Future values: "agent" (remote push).
	Source       string `json:"source"`
	AgentVersion string `json:"agent_version,omitempty"`
}

// --- Rich metric sub-types ---
// These are populated by the collector and returned by /metrics/latest via the
// hub's in-memory snapshot map. They are NOT persisted in the metrics table.

// NetInterfaceSample is one network interface's cumulative byte counters plus
// computed rates for the current interval.
type NetInterfaceSample struct {
	Name    string  `json:"name"`
	RxBytes uint64  `json:"rx_bytes"`
	TxBytes uint64  `json:"tx_bytes"`
	RxRate  float64 `json:"rx_rate"` // bytes/s since last sample
	TxRate  float64 `json:"tx_rate"` // bytes/s since last sample
}

// DiskMountSample is one mounted filesystem's point-in-time usage.
type DiskMountSample struct {
	Device  string  `json:"device"`
	Mount   string  `json:"mount"`
	FSType  string  `json:"fstype"`
	Used    uint64  `json:"used"`
	Total   uint64  `json:"total"`
	Percent float64 `json:"percent"`
}

// DiskIOSample is one block device's cumulative I/O byte counters plus rates.
type DiskIOSample struct {
	Device    string  `json:"device"`
	ReadBytes uint64  `json:"read_bytes"`
	WriteBytes uint64 `json:"write_bytes"`
	ReadRate  float64 `json:"read_rate"`  // bytes/s since last sample
	WriteRate float64 `json:"write_rate"` // bytes/s since last sample
}

// TempSample is one temperature sensor reading.
type TempSample struct {
	Name string  `json:"name"`
	Temp float64 `json:"temp_celsius"`
}

// MetricSample is one snapshot of host-level resource usage.
// The base fields (CPUPercent…UptimeSecs) are stored in the metrics table.
// The rich fields (CPUPerCore…Temps) are live-only: populated by the collector,
// held in the hub's in-memory snapshot, returned by /metrics/latest, and absent
// from historical /metrics?range=… responses.
type MetricSample struct {
	HostID     string    `json:"host_id"`
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemUsed    uint64    `json:"mem_used"`
	MemTotal   uint64    `json:"mem_total"`
	MemPercent float64   `json:"mem_percent"`
	MemAvail   uint64    `json:"mem_avail,omitempty"`
	MemCached  uint64    `json:"mem_cached,omitempty"`
	SwapUsed   uint64    `json:"swap_used"`
	SwapTotal  uint64    `json:"swap_total"`
	DiskUsed   uint64    `json:"disk_used"`
	DiskTotal  uint64    `json:"disk_total"`
	DiskPercent float64  `json:"disk_percent"`
	NetRxBytes uint64    `json:"net_rx_bytes"`
	NetTxBytes uint64    `json:"net_tx_bytes"`
	LoadAvg1   float64   `json:"load_avg_1"`
	LoadAvg5   float64   `json:"load_avg_5"`
	LoadAvg15  float64   `json:"load_avg_15"`
	UptimeSecs uint64    `json:"uptime_secs"`

	// Rich live-only fields (not in DB schema).
	CPUPerCore []float64            `json:"cpu_per_core,omitempty"`
	NetIfaces  []NetInterfaceSample `json:"net_interfaces,omitempty"`
	DiskMounts []DiskMountSample    `json:"disk_mounts,omitempty"`
	DiskIO     []DiskIOSample       `json:"disk_io,omitempty"`
	Temps      []TempSample         `json:"temps,omitempty"`
	Processes  []ProcessSample      `json:"processes,omitempty"`
}

// ProcessSample is one process's point-in-time resource usage.
// Returned by /metrics/latest only; never stored in the DB.
type ProcessSample struct {
	PID    int32   `json:"pid"`
	Name   string  `json:"name"`
	CPUPct float64 `json:"cpu_pct"`
	MemPct float64 `json:"mem_pct"`
	MemRSS uint64  `json:"mem_rss"` // bytes of resident set size
}

// --- Historical rich-metric response types ---
// These are returned by the /metrics/net, /metrics/mounts, and /metrics/diskio
// endpoints and are built from the three supplemental SQLite tables.

// NetIfaceSeries holds the per-interface byte counter arrays aligned to
// NetIfaceHistory.Timestamps.
type NetIfaceSeries struct {
	RxBytes []uint64 `json:"rx_bytes"`
	TxBytes []uint64 `json:"tx_bytes"`
}

// NetIfaceHistory is the response shape for GET /api/hosts/{id}/metrics/net.
// Timestamps are Unix seconds; Ifaces maps interface name to its series.
type NetIfaceHistory struct {
	Timestamps []int64                    `json:"timestamps"`
	Ifaces     map[string]*NetIfaceSeries `json:"ifaces"`
}

// DiskMountSeries holds per-mount used/total byte arrays.
type DiskMountSeries struct {
	Used  []uint64 `json:"used"`
	Total []uint64 `json:"total"`
}

// DiskMountHistory is the response shape for GET /api/hosts/{id}/metrics/mounts.
type DiskMountHistory struct {
	Timestamps []int64                      `json:"timestamps"`
	Mounts     map[string]*DiskMountSeries  `json:"mounts"`
}

// DiskIOSeries holds per-device cumulative read/write byte arrays.
// Rates are derived client-side from consecutive deltas.
type DiskIOSeries struct {
	ReadBytes  []uint64 `json:"read_bytes"`
	WriteBytes []uint64 `json:"write_bytes"`
}

// DiskIOHistory is the response shape for GET /api/hosts/{id}/metrics/diskio.
type DiskIOHistory struct {
	Timestamps []int64                    `json:"timestamps"`
	Devices    map[string]*DiskIOSeries   `json:"devices"`
}

// Container is a docker container observed on a host.
type Container struct {
	HostID     string            `json:"host_id"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	State      string            `json:"state"`
	Status     string            `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	Ports      []PortMapping     `json:"ports"`
	Labels     map[string]string `json:"labels"`
	CPUPercent float64           `json:"cpu_percent"`
	MemUsage   uint64            `json:"mem_usage"`
	MemLimit   uint64            `json:"mem_limit"`
	MemPercent float64           `json:"mem_percent"`
	NetRxBytes uint64            `json:"net_rx_bytes"`
	NetTxBytes uint64            `json:"net_tx_bytes"`
}

type PortMapping struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}

// ContainerMount is one mount point from a container inspect.
type ContainerMount struct {
	Type        string `json:"type"`        // "bind", "volume", "tmpfs"
	Source      string `json:"source"`      // host path or volume name
	Destination string `json:"destination"` // container path
	Mode        string `json:"mode"`
	RW          bool   `json:"rw"`
}

// ContainerInspect is the full detail view for a single container, returned by
// GET /api/hosts/{id}/containers/{cid}/inspect. Includes live stats when running.
type ContainerInspect struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	FinishedAt    *time.Time        `json:"finished_at,omitempty"`
	RestartPolicy string            `json:"restart_policy"`
	Entrypoint    []string          `json:"entrypoint,omitempty"`
	Cmd           []string          `json:"cmd,omitempty"`
	Env           []string          `json:"env"`
	Ports         []PortMapping     `json:"ports"`
	Mounts        []ContainerMount  `json:"mounts"`
	Labels        map[string]string `json:"labels"`
	// Live stats (zero when not running).
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   uint64  `json:"mem_usage"`
	MemLimit   uint64  `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
	NetRxBytes uint64  `json:"net_rx_bytes"`
	NetTxBytes uint64  `json:"net_tx_bytes"`
	// Configured resource limits (0 = unlimited).
	NanoCPUs      int64 `json:"nano_cpus"`
	MemLimitBytes int64 `json:"mem_limit_bytes"`
}

// DockerNetwork represents a docker network observed on a host.
type DockerNetwork struct {
	HostID     string             `json:"host_id"`
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Driver     string             `json:"driver"`
	Scope      string             `json:"scope"`
	Subnet     string             `json:"subnet,omitempty"`
	Gateway    string             `json:"gateway,omitempty"`
	Internal   bool               `json:"internal"`
	Labels     map[string]string  `json:"labels"`
	Containers []NetworkContainer `json:"containers,omitempty"` // For inspect
}

// NetworkContainer is a container connected to a network.
type NetworkContainer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EndpointID  string `json:"endpoint_id"`
	MacAddress  string `json:"mac_address"`
	IPv4Address string `json:"ipv4_address"`
	IPv6Address string `json:"ipv6_address"`
}

// NetworkCreateSpec is the surface-layer network-create request.
type NetworkCreateSpec struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"` // default "bridge"
	Internal bool              `json:"internal"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// ResourceUpdate is the body for PUT /api/hosts/{id}/containers/{cid}/resources.
// nil pointer fields mean "leave unchanged".
type ResourceUpdate struct {
	NanoCPUs    *int64 `json:"nano_cpus,omitempty"`    // 0 = unlimited
	MemoryBytes *int64 `json:"memory_bytes,omitempty"` // 0 = unlimited
}

// AlertRule is a threshold-based check evaluated on every metric ingest.
// HostID is a pointer so a NULL value (rule applies to ALL hosts) is
// distinguishable from an empty string.
type AlertRule struct {
	ID        int64     `json:"id"`
	HostID    *string   `json:"host_id,omitempty"`
	Metric    string    `json:"metric"`    // see alerts.SupportedMetrics
	Op        string    `json:"op"`        // ">", ">=", "<", "<="
	Threshold float64   `json:"threshold"`
	DurationS int       `json:"duration_s"` // sustained breach time before firing
	Enabled   bool      `json:"enabled"`
	Severity  string    `json:"severity"`  // "info"|"warning"|"critical"
	CreatedAt time.Time `json:"created_at"`
}

// AlertChannel is a notification destination configured by the user.
// Config holds type-specific JSON (webhook URL, topic, token, etc.).
type AlertChannel struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`          // "discord"|"slack"|"ntfy"|"gotify"|"webhook"
	Config        []byte    `json:"config"`        // raw JSON, type-specific
	Enabled       bool      `json:"enabled"`
	MinSeverity   string    `json:"min_severity"`  // "info"|"warning"|"critical"
	NotifyResolve bool      `json:"notify_resolve"`
	CreatedAt     time.Time `json:"created_at"`
}

// AlertEvent records a fired alert, with ResolvedAt set once the breach ends.
type AlertEvent struct {
	ID         int64      `json:"id"`
	RuleID     int64      `json:"rule_id"`
	HostID     string     `json:"host_id"`
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Value      float64    `json:"value"`
}

// CreateSpec is the surface-layer container-create request: only the fields
// needed for the common case (image, name, restart policy, env, ports,
// volumes, auto-start). Deeper container config — capabilities, ulimits,
// healthcheck spec, security opts, network aliases, etc. — waits for the
// compose-first work in roadmap section 2, where editing a full spec is
// natural via YAML.
type CreateSpec struct {
	Image         string            `json:"image"`
	Name          string            `json:"name,omitempty"`
	RestartPolicy string            `json:"restart_policy,omitempty"` // "", "no", "on-failure", "always", "unless-stopped"
	Env           map[string]string `json:"env,omitempty"`
	Ports         []PortBinding     `json:"ports,omitempty"`
	Volumes       []VolumeBinding   `json:"volumes,omitempty"`
	AutoStart     bool              `json:"auto_start"`
}

type PortBinding struct {
	HostPort      int    `json:"host_port"`      // 0 = let docker pick
	ContainerPort int    `json:"container_port"` // required
	Protocol      string `json:"protocol"`       // tcp, udp; empty -> tcp
}

type VolumeBinding struct {
	HostPath      string `json:"host_path"`      // required
	ContainerPath string `json:"container_path"` // required
	ReadOnly      bool   `json:"read_only"`
}

// SystemInfo is the small operational snapshot returned by /api/system/info.
// Polled by the layout footer; intentionally cheap to compute (one stat() and
// a few in-memory reads) so it can be hit frequently without measurable cost.
//
// The on-disk DB size includes the live `aperture.db` file plus its `-shm`
// and `-wal` companions when WAL mode is active — that gives a more honest
// number than reading the main file alone (the WAL can be a large fraction
// of total bytes between checkpoints).
type SystemInfo struct {
	Version     string    `json:"version"`
	StartedAt   time.Time `json:"started_at"`
	DBPath      string    `json:"db_path"`
	DBSizeBytes int64     `json:"db_size_bytes"`
}

// ComposeStack represents a discovered docker-compose project.
type ComposeStack struct {
	Project      string          `json:"project"`
	WorkingDir   string          `json:"working_dir"`
	ConfigFiles  string          `json:"config_files"`
	Services     []ComposeService `json:"services"`
	Status       string          `json:"status"`        // "running"|"partial"|"stopped"
	RunningCount int             `json:"running_count"`
	TotalCount   int             `json:"total_count"`
}

// ComposeService is one service entry within a compose stack.
type ComposeService struct {
	Name        string        `json:"name"`
	ContainerID string        `json:"container_id,omitempty"`
	Image       string        `json:"image,omitempty"`
	State       string        `json:"state"`             // "running"|"exited"|"paused"|"dead"
	Status      string        `json:"status"`            // human-readable "Up 2 hours"
	Health      string        `json:"health,omitempty"`  // "healthy"|"unhealthy"|"starting"
	ExitCode    int           `json:"exit_code,omitempty"`
	Ports       []PortMapping `json:"ports,omitempty"`
}

// HostInfo is the static descriptor a metric source produces once at start.
// MetricSource implementations populate this so the hub can register the host.
type HostInfo struct {
	Name     string
	OS       string
	Platform string
	Kernel   string
	Arch     string
	CPUModel string
	CPUCount int
	MemTotal uint64
	Source   string
}
