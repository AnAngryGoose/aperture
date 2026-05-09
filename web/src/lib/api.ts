import type {
	Host,
	MetricSample,
	Container,
	ContainerInspect,
	ResourceUpdate,
	AlertRule,
	AlertEvent,
	AlertMetadata,
	CreateSpec,
	SystemInfo
} from './types';

// In dev, the SvelteKit dev server runs on :5173 and the Go hub on :8080.
// In prod, the Go hub serves the static build itself, so same-origin works.
// Override with VITE_API_BASE if you ever split them in prod.
const DEFAULT_DEV_BASE = 'http://localhost:8080';
const API_BASE = import.meta.env.VITE_API_BASE
	?? (import.meta.env.DEV ? DEFAULT_DEV_BASE : '');

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`);
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`GET ${path} -> ${res.status}: ${text}`);
	}
	return res.json();
}

async function post(path: string): Promise<void> {
	const res = await fetch(`${API_BASE}${path}`, { method: 'POST' });
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`POST ${path} -> ${res.status}: ${text}`);
	}
}

async function del(path: string): Promise<void> {
	const res = await fetch(`${API_BASE}${path}`, { method: 'DELETE' });
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`DELETE ${path} -> ${res.status}: ${text}`);
	}
}

async function send<T>(path: string, method: 'POST' | 'PUT', body: unknown): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`${method} ${path} -> ${res.status}: ${text}`);
	}
	return res.json();
}

export const api = {
	systemInfo: () => get<SystemInfo>('/api/system/info'),
	hosts: () => get<Host[]>('/api/hosts'),
	host: (id: string) => get<Host>(`/api/hosts/${id}`),
	latest: (id: string) => get<MetricSample | null>(`/api/hosts/${id}/metrics/latest`),
	metrics: (id: string, range = '1h', points = 300) =>
		get<MetricSample[]>(`/api/hosts/${id}/metrics?range=${range}&points=${points}`),
	containers: (id: string, all = true) =>
		get<Container[]>(`/api/hosts/${id}/containers?all=${all}`),
	createContainer: (hostID: string, spec: CreateSpec) =>
		send<{ id: string; warning?: string }>(`/api/hosts/${hostID}/containers`, 'POST', spec),
	containerInspect: (hostID: string, cid: string) =>
		get<ContainerInspect>(`/api/hosts/${hostID}/containers/${cid}/inspect`),
	containerUpdateResources: (hostID: string, cid: string, update: ResourceUpdate) =>
		send<{ ok: boolean }>(`/api/hosts/${hostID}/containers/${cid}/resources`, 'PUT', update),
	containerRecreate: (hostID: string, cid: string) =>
		send<{ id: string; warning?: string }>(`/api/hosts/${hostID}/containers/${cid}/recreate`, 'POST', {}),

	containerAction: (hostID: string, cid: string, action: string) =>
		post(`/api/hosts/${hostID}/containers/${cid}/${action}`),
	containerRemove: (hostID: string, cid: string, force = false) =>
		del(`/api/hosts/${hostID}/containers/${cid}?force=${force}`),
	containerLogs: async (hostID: string, cid: string, tail = 200) => {
		const res = await fetch(`${API_BASE}/api/hosts/${hostID}/containers/${cid}/logs?tail=${tail}`);
		if (!res.ok) throw new Error(`logs -> ${res.status}`);
		return res.text();
	},

	alertMetadata: () => get<AlertMetadata>('/api/alerts/metadata'),
	alertRules: (hostID?: string) =>
		get<AlertRule[]>(`/api/alerts/rules${hostID ? `?host_id=${hostID}` : ''}`),
	createAlertRule: (rule: Partial<AlertRule>) =>
		send<AlertRule>('/api/alerts/rules', 'POST', rule),
	updateAlertRule: (id: number, rule: Partial<AlertRule>) =>
		send<AlertRule>(`/api/alerts/rules/${id}`, 'PUT', rule),
	deleteAlertRule: (id: number) => del(`/api/alerts/rules/${id}`),
	alertEvents: (params: { hostID?: string; openOnly?: boolean; limit?: number } = {}) => {
		const q = new URLSearchParams();
		if (params.hostID) q.set('host_id', params.hostID);
		if (params.openOnly) q.set('open', 'true');
		if (params.limit) q.set('limit', String(params.limit));
		const qs = q.toString();
		return get<AlertEvent[]>(`/api/alerts/events${qs ? `?${qs}` : ''}`);
	}
};
