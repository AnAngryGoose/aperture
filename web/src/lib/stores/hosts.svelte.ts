import type { Host, MetricSample } from '$lib/types';

export type HostStatus = 'ok' | 'warn' | 'crit' | 'offline';
export type HostKind = 'docker' | 'linux' | 'edge';

export interface HostEntry {
	host: Host;
	latest: MetricSample | null;
	cpuSeries: number[];
	memSeries: number[];
	netInSeries: number[];
	netOutSeries: number[];
	tsSeries: number[];
	status: HostStatus;
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

function createHostStore() {
	let entries = $state<Record<string, HostEntry>>({});
	let lastSync = $state<Date | null>(null);
	let sseSource: EventSource | null = null;

	function upsertHost(host: Host, sample: MetricSample | null) {
		const prev = entries[host.id];
		const cpu = sample?.cpu_percent ?? prev?.latest?.cpu_percent ?? 0;
		const mem = sample?.mem_percent ?? prev?.latest?.mem_percent ?? 0;
		const netIn = sample?.net_rx_bytes ?? prev?.latest?.net_rx_bytes ?? 0;
		const netOut = sample?.net_tx_bytes ?? prev?.latest?.net_tx_bytes ?? 0;
		const ts = sample ? Date.now() / 1000 : (prev?.tsSeries.at(-1) ?? Date.now() / 1000);

		entries[host.id] = {
			host,
			latest: sample ?? prev?.latest ?? null,
			cpuSeries: sample ? pushRing(prev?.cpuSeries ?? [], cpu) : (prev?.cpuSeries ?? []),
			memSeries: sample ? pushRing(prev?.memSeries ?? [], mem) : (prev?.memSeries ?? []),
			netInSeries: sample ? pushRing(prev?.netInSeries ?? [], netIn) : (prev?.netInSeries ?? []),
			netOutSeries: sample ? pushRing(prev?.netOutSeries ?? [], netOut) : (prev?.netOutSeries ?? []),
			tsSeries: sample ? pushRing(prev?.tsSeries ?? [], ts) : (prev?.tsSeries ?? []),
			status: deriveStatus(sample ?? prev?.latest ?? null, host.last_seen ?? null)
		};
		lastSync = new Date();
	}

	function setAll(hosts: Host[], samples: Record<string, MetricSample>) {
		for (const host of hosts) {
			upsertHost(host, samples[host.id] ?? null);
		}
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
		connectSSE,
		disconnectSSE
	};
}

export const hostStore = createHostStore();
