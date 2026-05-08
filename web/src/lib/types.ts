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
	source: string;
}

export interface MetricSample {
	host_id: string;
	timestamp: string;
	cpu_percent: number;
	mem_used: number;
	mem_total: number;
	mem_percent: number;
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
}

export interface PortMapping {
	ip?: string;
	private_port: number;
	public_port?: number;
	type: string;
}

export interface AlertRule {
	id: number;
	host_id?: string | null;
	metric: string;
	op: string;
	threshold: number;
	duration_s: number;
	enabled: boolean;
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
