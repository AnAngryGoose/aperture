import type { Host, MetricSample } from '$lib/types';

export type HostStatus = 'ok' | 'warn' | 'crit' | 'offline';
export type HostKind = 'docker' | 'linux' | 'edge';

export interface ContainerCounts {
	running: number;
	stopped: number;
	unhealthy: number;
}

export interface HostEntry {
	host: Host;
	latest: MetricSample | null;
	cpuSeries: number[];
	memSeries: number[];
	netInSeries: number[];   // bytes/sec (derived from successive cumulative samples)
	netOutSeries: number[];  // bytes/sec
	tsSeries: number[];
	netInRate: number;       // latest bytes/sec (0 until we have two samples)
	netOutRate: number;
	status: HostStatus;
	containers: ContainerCounts | null; // null = not yet loaded / not a docker host
}

const RING_SIZE = 60;

function pushRing(buf: number[], val: number): number[] {
	const next = [...buf, val];
	return next.length > RING_SIZE ? next.slice(next.length - RING_SIZE) : next;
}

function deriveStatus(sample: MetricSample | null, lastSeen: string | null): HostStatus {
	if (!lastSeen) return 'offline';
	const age = Date.now() - new Date(lastSeen).getTime();
	if (age > 120_000) return 'offline';
	if (!sample) return 'offline';
	if (sample.cpu_percent >= 90 || sample.mem_percent >= 90 || sample.disk_percent >= 90) return 'crit';
	if (sample.cpu_percent >= 70 || sample.mem_percent >= 80 || sample.disk_percent >= 80) return 'warn';
	return 'ok';
}

// Derive a rate (bytes/sec) from successive cumulative byte counters.
// Returns 0 when there's no prior sample, when dt is non-positive, or when the
// counter went backwards (interface reset, host reboot).
function deriveRate(curBytes: number, prevBytes: number | undefined, curTs: number, prevTs: number | undefined): number {
	if (prevBytes === undefined || prevTs === undefined) return 0;
	const dt = curTs - prevTs;
	if (dt <= 0) return 0;
	const delta = curBytes - prevBytes;
	if (delta < 0) return 0;
	return delta / dt;
}

function createHostStore() {
	let entries = $state<Record<string, HostEntry>>({});
	let lastSync = $state<Date | null>(null);
	let sseSource: EventSource | null = null;

	function upsertHost(host: Host, sample: MetricSample | null) {
		const prev = entries[host.id];
		const cpu = sample?.cpu_percent ?? prev?.latest?.cpu_percent ?? 0;
		const mem = sample?.mem_percent ?? prev?.latest?.mem_percent ?? 0;
		const ts = sample ? Date.now() / 1000 : (prev?.tsSeries.at(-1) ?? Date.now() / 1000);

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
			status: deriveStatus(sample ?? prev?.latest ?? null, host.last_seen ?? null),
			containers: prev?.containers ?? null
		};
		lastSync = new Date();
	}

	function setAll(hosts: Host[], samples: Record<string, MetricSample>) {
		for (const host of hosts) {
			upsertHost(host, samples[host.id] ?? null);
		}
	}

	function setContainerCounts(hostId: string, counts: ContainerCounts) {
		const prev = entries[hostId];
		if (!prev) return;
		entries[hostId] = { ...prev, containers: counts };
	}

	function connectSSE(baseUrl: string) {
		if (sseSource) sseSource.close();
		sseSource = new EventSource(`${baseUrl}/api/stream/metrics`, { withCredentials: true });
		sseSource.onmessage = (e) => {
			try {
				const data = JSON.parse(e.data);
				const entry = entries[data.hostId];
				if (!entry) return;
				const sample: MetricSample = {
					...entry.latest,
					cpu_percent: data.cpu,
					mem_percent: data.mem,
					net_rx_bytes: data.netIn,
					net_tx_bytes: data.netOut,
					timestamp: new Date((data.ts ?? Date.now() / 1000) * 1000).toISOString()
				} as MetricSample;
				upsertHost(entry.host, sample);
			} catch { /* ignore malformed */ }
		};
	}

	function disconnectSSE() {
		sseSource?.close();
		sseSource = null;
	}

	return {
		get entries() { return entries; },
		get lastSync() { return lastSync; },
		get list() { return Object.values(entries); },
		upsertHost,
		setAll,
		setContainerCounts,
		connectSSE,
		disconnectSSE
	};
}

export const hostStore = createHostStore();
