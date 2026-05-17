// Monitoring store — central state for the dashboard and the global host
// list. Replaces the dashboard's prior `load()` fan-out:
//   - One initial fetch to /api/monitoring/overview
//   - SSE for live updates (metric / host_status / container_summary / alert)
//   - A slow (30s) reconciliation poll as a fallback when SSE drops
//
// Exposes a getter surface matching the legacy hostStore (entries, list,
// lastSync) so existing card components keep working without changes. The
// ring-buffer derivation lives here too.

import type {
	Host,
	MetricSample,
	HostStatus,
	ContainerCounts,
	MonitoringOverview,
	OverviewAlertEvent,
	SSEEnvelope
} from '$lib/types';

// Re-export legacy aliases so older imports (`HostStatus`, `HostKind`) keep
// resolving via `$lib/stores/hosts.svelte` after that file became a passthrough.
export type { HostStatus } from '$lib/types';
export type HostKind = 'docker' | 'linux' | 'edge';

export interface HostEntry {
	host: Host;
	latest: MetricSample | null;
	cpuSeries: number[];
	memSeries: number[];
	netInSeries: number[];  // bytes/sec
	netOutSeries: number[]; // bytes/sec
	tsSeries: number[];
	netInRate: number;
	netOutRate: number;
	status: HostStatus;
	containers: ContainerCounts | null;
	openAlerts: number;
}

const RING_SIZE = 60;

function pushRing(buf: number[], val: number): number[] {
	const next = [...buf, val];
	return next.length > RING_SIZE ? next.slice(next.length - RING_SIZE) : next;
}

// Derive bytes/sec rate from two successive cumulative byte counters. Guards
// against missing prior sample, non-positive dt, and counter resets.
function deriveRate(curBytes: number, prevBytes: number | undefined, curTs: number, prevTs: number | undefined): number {
	if (prevBytes === undefined || prevTs === undefined) return 0;
	const dt = curTs - prevTs;
	if (dt <= 0) return 0;
	const delta = curBytes - prevBytes;
	if (delta < 0) return 0;
	return delta / dt;
}

// Fallback status derivation when the server hasn't emitted a host_status
// event yet (cold start). The server is the source of truth for status,
// but having a sensible fallback avoids cards flashing "offline" on the
// very first render.
function fallbackStatus(sample: MetricSample | null, lastSeen: string | null): HostStatus {
	if (!lastSeen) return 'offline';
	const age = Date.now() - new Date(lastSeen).getTime();
	if (age > 120_000) return 'offline';
	return sample ? 'ok' : 'offline';
}

function createMonitoringStore() {
	let entries = $state<Record<string, HostEntry>>({});
	let lastSync = $state<Date | null>(null);
	let alertNudge = $state(0); // bumped on alert SSE; consumers can $effect this
	// Open alert events from the most recent overview fetch, joined with their
	// rule (metric/op/threshold/severity). Single source of truth consumed by
	// both PageHeader (count) and NeedsAttention (rows) via lib/monitoring/issues.
	// Replaced on each reconcile; an alert SSE bumps `alertNudge` to signal
	// "you should refetch the overview" to consumers that want fresh events
	// without waiting for the 30s poll.
	let events = $state<OverviewAlertEvent[]>([]);
	let sseSource: EventSource | null = null;

	function upsertFromSample(host: Host, sample: MetricSample | null, serverStatus?: HostStatus) {
		const prev = entries[host.id];
		const ts = sample ? Date.now() / 1000 : (prev?.tsSeries.at(-1) ?? Date.now() / 1000);
		const cpu = sample?.cpu_percent ?? prev?.latest?.cpu_percent ?? 0;
		const mem = sample?.mem_percent ?? prev?.latest?.mem_percent ?? 0;

		const curRxBytes = sample?.net_rx_bytes ?? prev?.latest?.net_rx_bytes ?? 0;
		const curTxBytes = sample?.net_tx_bytes ?? prev?.latest?.net_tx_bytes ?? 0;
		const prevRxBytes = prev?.latest?.net_rx_bytes;
		const prevTxBytes = prev?.latest?.net_tx_bytes;
		const prevTs = prev?.tsSeries.at(-1);

		const netInRate = sample ? deriveRate(curRxBytes, prevRxBytes, ts, prevTs) : (prev?.netInRate ?? 0);
		const netOutRate = sample ? deriveRate(curTxBytes, prevTxBytes, ts, prevTs) : (prev?.netOutRate ?? 0);

		entries[host.id] = {
			host,
			latest: sample ?? prev?.latest ?? null,
			cpuSeries: sample ? pushRing(prev?.cpuSeries ?? [], cpu) : (prev?.cpuSeries ?? []),
			memSeries: sample ? pushRing(prev?.memSeries ?? [], mem) : (prev?.memSeries ?? []),
			netInSeries: sample ? pushRing(prev?.netInSeries ?? [], netInRate) : (prev?.netInSeries ?? []),
			netOutSeries: sample ? pushRing(prev?.netOutSeries ?? [], netOutRate) : (prev?.netOutSeries ?? []),
			tsSeries: sample ? pushRing(prev?.tsSeries ?? [], ts) : (prev?.tsSeries ?? []),
			netInRate,
			netOutRate,
			status: serverStatus ?? prev?.status ?? fallbackStatus(sample ?? prev?.latest ?? null, host.last_seen ?? null),
			containers: prev?.containers ?? null,
			openAlerts: prev?.openAlerts ?? 0
		};
		lastSync = new Date();
	}

	// hydrate replaces local state from a /api/monitoring/overview response.
	// Used on initial mount and every 30s as the reconciliation poll.
	function hydrate(overview: MonitoringOverview) {
		const nextEntries: Record<string, HostEntry> = {};
		for (const host of overview.hosts) {
			const sample = overview.latest[host.id] ?? null;
			const status = overview.status[host.id];
			const containers = overview.containers[host.id] ?? null;
			const openAlerts = overview.openAlerts[host.id] ?? 0;
			// Stamp the legacy `host.open_alerts` field too. Six readers across
			// the dashboard (Sidebar badge, FilterBar visibility, HostGrid
			// "alerts" filter, RichCard footer, TileCard pill, /hosts page)
			// still read this field; the v0.4.1 overview endpoint doesn't
			// populate it on the Host row, so without this stamp they all
			// silently see 0. Cheap to mirror — keeps the new and legacy fields
			// in lockstep without a 6-file refactor.
			host.open_alerts = openAlerts;
			// Reuse existing ring buffers if we already had this host so the
			// reconciliation poll doesn't flatten the sparkline.
			const prev = entries[host.id];
			nextEntries[host.id] = {
				host,
				latest: sample,
				cpuSeries: prev?.cpuSeries ?? [],
				memSeries: prev?.memSeries ?? [],
				netInSeries: prev?.netInSeries ?? [],
				netOutSeries: prev?.netOutSeries ?? [],
				tsSeries: prev?.tsSeries ?? [],
				netInRate: prev?.netInRate ?? 0,
				netOutRate: prev?.netOutRate ?? 0,
				status: status ?? prev?.status ?? fallbackStatus(sample, host.last_seen ?? null),
				containers,
				openAlerts
			};
		}
		entries = nextEntries;
		// Replace the events array wholesale — the overview is the source of
		// truth; any incremental updates from SSE only bump alertNudge to
		// nudge consumers (the dashboard refetches the overview on nudge for
		// fresh event details). Falls back to [] if the backend hasn't been
		// updated to include events yet.
		events = overview.events ?? [];
		lastSync = new Date();
	}

	// connectSSE subscribes to the typed-envelope SSE stream and routes each
	// event to the relevant state. Idempotent — calling twice closes the
	// previous source first.
	function connectSSE(baseUrl: string) {
		if (sseSource) sseSource.close();
		sseSource = new EventSource(`${baseUrl}/api/stream/metrics`, { withCredentials: true });
		sseSource.onmessage = (e) => {
			try {
				const env = JSON.parse(e.data) as SSEEnvelope;
				const entry = entries[env.hostId];
				if (!entry) return;
				switch (env.type) {
					case 'metric':
					case undefined as unknown as 'metric': { // legacy events without `type`
						// Synthesize a partial MetricSample and reuse upsertFromSample
						// for the ring-buffer derivation.
						const sample: MetricSample = {
							...(entry.latest ?? ({} as MetricSample)),
							host_id: env.hostId,
							cpu_percent: env.cpu,
							mem_percent: env.mem,
							net_rx_bytes: env.netIn,
							net_tx_bytes: env.netOut,
							disk_percent: env.diskPct ?? entry.latest?.disk_percent ?? 0,
							timestamp: new Date((env.ts ?? Date.now() / 1000) * 1000).toISOString()
						} as MetricSample;
						upsertFromSample(entry.host, sample);
						break;
					}
					case 'host_status': {
						entries[env.hostId] = { ...entry, status: env.status };
						break;
					}
					case 'container_summary': {
						entries[env.hostId] = { ...entry, containers: env.containers };
						break;
					}
					case 'alert': {
						// Bump the alert counter — list view consumers can react.
						// Mirror the change onto host.open_alerts so the legacy
						// readers (Sidebar badge, RichCard footer, etc.) stay
						// in sync without a separate broadcast path.
						const nextCount = env.alert.resolved
							? Math.max(0, entry.openAlerts - 1)
							: entry.openAlerts + 1;
						entries[env.hostId] = {
							...entry,
							host: { ...entry.host, open_alerts: nextCount },
							openAlerts: nextCount
						};
						alertNudge = alertNudge + 1;
						break;
					}
				}
			} catch {
				// Ignore malformed events — better than crashing the page.
			}
		};
		sseSource.onerror = () => {
			// EventSource auto-reconnects; nothing to do. The reconciliation
			// poll covers gaps if reconnection takes a while.
		};
	}

	function disconnectSSE() {
		sseSource?.close();
		sseSource = null;
	}

	return {
		get entries() { return entries; },
		get list() { return Object.values(entries); },
		get lastSync() { return lastSync; },
		get alertNudge() { return alertNudge; },
		get events() { return events; },
		hydrate,
		connectSSE,
		disconnectSSE
	};
}

export const monitoringStore = createMonitoringStore();
