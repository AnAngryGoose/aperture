// Single source of truth for "which metrics can be shown / alerted on / charted".
//
// Used by:
//   - Dashboard card widget picker (CardConfigModal)
//   - Host-detail Settings tab (collector toggles)
//   - Alert rule editor (category → target → leaf)
//
// The backend's /api/monitoring/catalog returns the authoritative list of
// scalar metrics and dotted-target categories; this module merges that with
// the UI-only metadata (label, color, format hints).

import type { MetricSample } from '$lib/types';

export type MetricCategory = 'cpu' | 'mem' | 'disk' | 'net' | 'load' | 'temp' | 'misc';

export interface MetricSpec {
	/** Stable key — used by dashboard widget config and the alert rule editor. */
	key: string;
	label: string;
	category: MetricCategory;
	unit: string;         // "%" | "bytes/s" | "°C" | "GiB" | "" (count)
	/** How the value is rendered on cards/sparkline tooltips. */
	format?: (v: number) => string;
	/** Default sparkline color in the v0.4 token system. */
	color: string;
	/** Resolve the metric value from a MetricSample for card display. */
	resolve: (s: MetricSample) => number;
	/** Description shown in the widget picker. */
	description?: string;
}

function pct(v: number): string { return `${v.toFixed(1)}%`; }
function gib(b: number): string {
	const g = b / 1073741824;
	return g >= 10 ? `${g.toFixed(1)} GiB` : `${g.toFixed(2)} GiB`;
}
function rate(bps: number): string {
	if (bps < 1024) return `${bps.toFixed(0)} B/s`;
	if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
	return `${(bps / 1048576).toFixed(1)} MiB/s`;
}
function tempC(v: number): string { return `${v.toFixed(1)}°C`; }

/** Top-level scalar metrics displayable as dashboard card rows. */
export const SCALAR_METRICS: MetricSpec[] = [
	{
		key: 'cpu_pct', label: 'CPU', category: 'cpu', unit: '%',
		format: pct, color: 'var(--accent)',
		resolve: (s) => s.cpu_percent,
		description: 'Aggregate CPU usage across all cores.'
	},
	{
		key: 'mem_pct', label: 'Memory', category: 'mem', unit: '%',
		format: pct, color: 'var(--accent)',
		resolve: (s) => s.mem_percent,
		description: 'Used memory as a percentage of total.'
	},
	{
		key: 'disk_pct', label: 'Disk', category: 'disk', unit: '%',
		format: pct, color: 'var(--accent)',
		resolve: (s) => s.disk_percent,
		description: 'Root filesystem usage.'
	},
	{
		key: 'swap_pct', label: 'Swap', category: 'mem', unit: '%',
		format: pct, color: 'var(--warn)',
		resolve: (s) => (s.swap_total ? (s.swap_used / s.swap_total) * 100 : 0),
		description: 'Swap usage as a percentage of total swap.'
	},
	{
		key: 'load_1', label: 'Load 1m', category: 'load', unit: '',
		format: (v) => v.toFixed(2), color: 'var(--info)',
		resolve: (s) => s.load_avg_1,
		description: '1-minute system load average.'
	},
	{
		key: 'load_5', label: 'Load 5m', category: 'load', unit: '',
		format: (v) => v.toFixed(2), color: 'var(--info)',
		resolve: (s) => s.load_avg_5,
		description: '5-minute system load average.'
	},
	{
		key: 'net_rx_rate', label: 'Net RX', category: 'net', unit: 'bytes/s',
		format: rate, color: 'var(--info)',
		resolve: (s) => s.net_rx_bytes,  // displayed using the entry-derived rate, not the cumulative — see RichCard
		description: 'Aggregate inbound network rate.'
	},
	{
		key: 'net_tx_rate', label: 'Net TX', category: 'net', unit: 'bytes/s',
		format: rate, color: 'var(--info)',
		resolve: (s) => s.net_tx_bytes,
		description: 'Aggregate outbound network rate.'
	},
	{
		key: 'temp_max', label: 'Temp', category: 'temp', unit: '°C',
		format: tempC, color: 'var(--warn)',
		resolve: (s) => (s.temps ?? []).reduce((a, t) => Math.max(a, t.temp_celsius), 0),
		description: 'Hottest temperature sensor on the host.'
	},
	{
		key: 'mem_used', label: 'Memory used', category: 'mem', unit: 'bytes',
		format: gib, color: 'var(--accent)',
		resolve: (s) => s.mem_used,
		description: 'Used memory in bytes (not percent).'
	},
	{
		key: 'uptime', label: 'Uptime', category: 'misc', unit: 's',
		format: (v) => {
			const h = Math.floor(v / 3600);
			const d = Math.floor(h / 24);
			if (d > 0) return `${d}d ${h % 24}h`;
			if (h > 0) return `${h}h`;
			return `${Math.floor(v / 60)}m`;
		},
		color: 'var(--text-faint)',
		resolve: (s) => s.uptime_secs,
		description: 'How long the host has been up.'
	}
];

const byKey: Record<string, MetricSpec> = Object.fromEntries(
	SCALAR_METRICS.map((m) => [m.key, m])
);

/** Resolve a scalar metric spec by key. Returns undefined for unknown keys. */
export function getMetric(key: string): MetricSpec | undefined {
	return byKey[key];
}

/** Default widget set rendered on a card when no per-host override is set. */
export const DEFAULT_WIDGETS = ['cpu_pct', 'mem_pct', 'net_rx_rate', 'disk_pct'];

/** Categories grouped for the widget picker UI. */
export function metricsByCategory(): Record<MetricCategory, MetricSpec[]> {
	const groups: Record<MetricCategory, MetricSpec[]> = {
		cpu: [], mem: [], disk: [], net: [], load: [], temp: [], misc: []
	};
	for (const m of SCALAR_METRICS) groups[m.category].push(m);
	return groups;
}
