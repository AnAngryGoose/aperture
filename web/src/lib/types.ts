// Mirrors of the Go types in internal/types/types.go. Keep in sync — when
// the Go types change, update these manually (or replace this file with a
// codegen step later).

export interface Host {
	id: string;
	name: string;
	os: string;
	platform: string;
	kernel: string;
	arch: string;
	cpu_model: string;
	cpu_count: number;
	mem_total: number;
	created_at: string;
	last_seen: string;
	source: string;       // "local" | "agent"
	agent_version?: string;
	kind: string;         // "docker" | "linux" | "edge"
	tags: string[];
	open_alerts: number;
}

export interface AgentToken {
	id: number;
	name: string;
	created_at: string;
	last_used?: string | null;
	revoked: boolean;
	// Only populated on creation — never stored or returned again.
	token?: string;
}

export interface NetInterfaceSample {
	name: string;
	rx_bytes: number;
	tx_bytes: number;
	rx_rate: number; // bytes/s
	tx_rate: number; // bytes/s
}

export interface DiskMountSample {
	device: string;
	mount: string;
	fstype: string;
	used: number;
	total: number;
	percent: number;
}

export interface DiskIOSample {
	device: string;
	read_bytes: number;
	write_bytes: number;
	read_rate: number;  // bytes/s
	write_rate: number; // bytes/s
}

export interface TempSample {
	name: string;
	temp_celsius: number;
}

export interface ProcessSample {
	pid: number;
	name: string;
	cpu_pct: number;
	mem_pct: number;
	mem_rss: number; // bytes
}

export interface MetricSample {
	host_id: string;
	timestamp: string;
	cpu_percent: number;
	mem_used: number;
	mem_total: number;
	mem_percent: number;
	mem_avail?: number;
	mem_cached?: number;
	swap_used: number;
	swap_total: number;
	disk_used: number;
	disk_total: number;
	disk_percent: number;
	net_rx_bytes: number;
	net_tx_bytes: number;
	load_avg_1: number;
	load_avg_5: number;
	load_avg_15: number;
	uptime_secs: number;
	// Rich live-only fields (present in /latest, absent in /metrics?range=)
	cpu_per_core?: number[];
	net_interfaces?: NetInterfaceSample[];
	disk_mounts?: DiskMountSample[];
	disk_io?: DiskIOSample[];
	temps?: TempSample[];
	processes?: ProcessSample[];
}

// Historical rich-metric response types (from /metrics/net, /metrics/mounts, /metrics/diskio)

export interface NetIfaceSeries { rx_bytes: number[]; tx_bytes: number[]; }
export interface NetIfaceHistory { timestamps: number[]; ifaces: Record<string, NetIfaceSeries>; }

export interface DiskMountSeries { used: number[]; total: number[]; }
export interface DiskMountHistory { timestamps: number[]; mounts: Record<string, DiskMountSeries>; }

export interface DiskIOSeries { read_bytes: number[]; write_bytes: number[]; }
export interface DiskIOHistory { timestamps: number[]; devices: Record<string, DiskIOSeries>; }

export interface PortMapping {
	ip?: string;
	private_port: number;
	public_port?: number;
	type: string;
}

export interface ContainerMount {
	type: string;
	source: string;
	destination: string;
	mode: string;
	rw: boolean;
}

export interface ContainerInspect {
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	created_at: string;
	started_at?: string;
	finished_at?: string;
	restart_policy: string;
	entrypoint?: string[];
	cmd?: string[];
	env: string[];
	ports: PortMapping[];
	mounts: ContainerMount[];
	labels: Record<string, string>;
	cpu_percent: number;
	mem_usage: number;
	mem_limit: number;
	mem_percent: number;
	net_rx_bytes: number;
	net_tx_bytes: number;
	nano_cpus: number;
	mem_limit_bytes: number;
}

export interface ResourceUpdate {
	nano_cpus?: number;    // 0 = unlimited
	memory_bytes?: number; // 0 = unlimited
}

export interface AlertRule {
	id: number;
	host_id?: string | null;
	metric: string;
	op: string;
	threshold: number;
	duration_s: number;
	enabled: boolean;
	severity: string; // 'info' | 'warning' | 'critical'
	created_at: string;
}

export interface AlertChannel {
	id: number;
	name: string;
	type: string; // 'discord' | 'slack' | 'ntfy' | 'gotify' | 'webhook'
	config: Record<string, unknown>;
	enabled: boolean;
	min_severity: string; // 'info' | 'warning' | 'critical'
	notify_resolve: boolean;
	created_at: string;
}

export interface AlertEvent {
	id: number;
	rule_id: number;
	host_id: string;
	fired_at: string;
	resolved_at?: string | null;
	value: number;
}

export interface AlertMetadata {
	metrics: string[];
	ops: string[];
	severities: string[];
	channel_types: string[];
}

export interface CreatePortBinding {
	host_port: number;
	container_port: number;
	protocol: string; // 'tcp' | 'udp'
}

export interface CreateVolumeBinding {
	host_path: string;
	container_path: string;
	read_only: boolean;
}

export interface CreateSpec {
	image: string;
	name?: string;
	restart_policy?: '' | 'no' | 'on-failure' | 'always' | 'unless-stopped';
	env?: Record<string, string>;
	ports?: CreatePortBinding[];
	volumes?: CreateVolumeBinding[];
	auto_start: boolean;
}

export interface SystemInfo {
	version: string;
	started_at: string;
	db_path: string;
	db_size_bytes: number;
}

export interface Container {
	host_id: string;
	id: string;
	name: string;
	image: string;
	state: string;
	status: string;
	created_at: string;
	started_at?: string;
	ports: PortMapping[];
	labels: Record<string, string>;
	cpu_percent: number;
	mem_usage: number;
	mem_limit: number;
	mem_percent: number;
	net_rx_bytes: number;
	net_tx_bytes: number;
}

export interface NetworkContainer {
	id: string;
	name: string;
	endpoint_id: string;
	mac_address: string;
	ipv4_address: string;
	ipv6_address: string;
}

export interface DockerNetwork {
	host_id: string;
	id: string;
	name: string;
	driver: string;
	scope: string;
	subnet?: string;
	gateway?: string;
	internal: boolean;
	labels: Record<string, string>;
	containers?: NetworkContainer[];
}

export interface NetworkCreateSpec {
	name: string;
	driver?: string;
	internal: boolean;
	labels?: Record<string, string>;
}

export interface DockerVolume {
	name: string;
	driver: string;
	mountpoint: string;
	created_at: string;
	labels: Record<string, string>;
	options: Record<string, string>;
	scope: string;
	size_bytes: number;
	ref_count: number;
}

export interface VolumeCreateSpec {
	name: string;
	driver?: string;
	driver_opts?: Record<string, string>;
	labels?: Record<string, string>;
}

export interface DockerImage {
	id: string;
	repo_tags: string[];
	repo_digests: string[];
	created: number;
	size_bytes: number;
	containers: number;
	labels?: Record<string, string>;
}

export interface ImageUpdateStatus {
	up_to_date: boolean;
	local_digest: string;
	remote_digest: string;
	error?: string;
}


export interface ComposeService {
	name: string;
	container_id?: string;
	image?: string;
	state: string; // 'running' | 'exited' | 'paused' | 'dead'
	status: string;
	health?: string; // 'healthy' | 'unhealthy' | 'starting'
	exit_code?: number;
	ports?: PortMapping[];
}

export interface ComposeVersion {
	id: number;
	host_id: string;
	project: string;
	created_at: string;
	content?: string;
}


export interface ComposeStack {
	project: string;
	working_dir: string;
	config_files: string;
	services: ComposeService[];
	status: string; // 'running' | 'partial' | 'stopped'
	running_count: number;
	total_count: number;
}
