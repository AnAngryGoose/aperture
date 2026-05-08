package types

import "time"

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
	Source string `json:"source"`
}

// MetricSample is one snapshot of host-level resource usage.
type MetricSample struct {
	HostID      string    `json:"host_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemUsed     uint64    `json:"mem_used"`
	MemTotal    uint64    `json:"mem_total"`
	MemPercent  float64   `json:"mem_percent"`
	SwapUsed    uint64    `json:"swap_used"`
	SwapTotal   uint64    `json:"swap_total"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskTotal   uint64    `json:"disk_total"`
	DiskPercent float64   `json:"disk_percent"`
	NetRxBytes  uint64    `json:"net_rx_bytes"`
	NetTxBytes  uint64    `json:"net_tx_bytes"`
	LoadAvg1    float64   `json:"load_avg_1"`
	LoadAvg5    float64   `json:"load_avg_5"`
	LoadAvg15   float64   `json:"load_avg_15"`
	UptimeSecs  uint64    `json:"uptime_secs"`
}

// Container is a docker container observed on a host.
type Container struct {
	HostID      string            `json:"host_id"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	State       string            `json:"state"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	Ports       []PortMapping     `json:"ports"`
	Labels      map[string]string `json:"labels"`
	CPUPercent  float64           `json:"cpu_percent"`
	MemUsage    uint64            `json:"mem_usage"`
	MemLimit    uint64            `json:"mem_limit"`
	MemPercent  float64           `json:"mem_percent"`
	NetRxBytes  uint64            `json:"net_rx_bytes"`
	NetTxBytes  uint64            `json:"net_tx_bytes"`
}

type PortMapping struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}

// AlertRule is a threshold-based check evaluated on every metric ingest.
// HostID is a pointer so a NULL value (rule applies to ALL hosts) is
// distinguishable from an empty string.
type AlertRule struct {
	ID        int64     `json:"id"`
	HostID    *string   `json:"host_id,omitempty"`
	Metric    string    `json:"metric"`     // see alerts.SupportedMetrics
	Op        string    `json:"op"`         // ">", ">=", "<", "<="
	Threshold float64   `json:"threshold"`
	DurationS int       `json:"duration_s"` // sustained breach time before firing
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
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
