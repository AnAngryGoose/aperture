import type {
	Host,
	MetricSample,
	Container,
	ContainerInspect,
	ResourceUpdate,
	AlertRule,
	AlertChannel,
	AlertEvent,
	AlertMetadata,
	CreateSpec,
	SystemInfo,
	NetIfaceHistory,
	DiskMountHistory,
	DiskIOHistory,
	AgentToken,
	ComposeStack,
	ComposeService,
	DockerNetwork,
	NetworkCreateSpec,
	DockerVolume,
	VolumeCreateSpec,
	DockerImage,
	ImageUpdateStatus,
	ComposeVersion
} from './types';

// In dev, the SvelteKit dev server runs on :5173 and the Go hub on :8080.
// In prod, the Go hub serves the static build itself, so same-origin works.
// Override with VITE_API_BASE if you ever split them in prod.
const DEFAULT_DEV_BASE = 'http://localhost:8080';
const API_BASE = import.meta.env.VITE_API_BASE
	?? (import.meta.env.DEV ? DEFAULT_DEV_BASE : '');

// Auth-free paths — 401 on these should not trigger a redirect loop.
const AUTH_FREE = ['/login', '/setup'];

function handleUnauthorized() {
	if (typeof window !== 'undefined' && !AUTH_FREE.includes(window.location.pathname)) {
		window.location.href = '/login';
	}
}

async function get<T>(path: string): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`);
	if (res.status === 401) { handleUnauthorized(); throw new Error('unauthorized'); }
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`GET ${path} -> ${res.status}: ${text}`);
	}
	return res.json();
}

async function post(path: string): Promise<void> {
	const res = await fetch(`${API_BASE}${path}`, { method: 'POST' });
	if (res.status === 401) { handleUnauthorized(); throw new Error('unauthorized'); }
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`POST ${path} -> ${res.status}: ${text}`);
	}
}

async function del(path: string): Promise<void> {
	const res = await fetch(`${API_BASE}${path}`, { method: 'DELETE' });
	if (res.status === 401) { handleUnauthorized(); throw new Error('unauthorized'); }
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
	if (res.status === 401) { handleUnauthorized(); throw new Error('unauthorized'); }
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(`${method} ${path} -> ${res.status}: ${text}`);
	}
	return res.json();
}

export const api = {
	// Auth
	auth: {
		status: () =>
			fetch(`${API_BASE}/api/auth/status`)
				.then((r) => r.json() as Promise<{ configured: boolean; authenticated: boolean }>),
		setup: (password: string) =>
			send<{ ok: boolean }>('/api/auth/setup', 'POST', { password }),
		login: (password: string) =>
			send<{ ok: boolean }>('/api/auth/login', 'POST', { password }),
		logout: () => send<{ ok: boolean }>('/api/auth/logout', 'POST', {}),
		changePassword: (current: string, newPw: string) =>
			send<{ ok: boolean }>('/api/auth/password', 'POST', { current, new: newPw })
	},

	systemInfo: () => get<SystemInfo>('/api/system/info'),

	// Hosts — both the old flat API and a namespaced version for new code.
	hosts: {
		list: () => get<Host[]>('/api/hosts'),
		get: (id: string) => get<Host>(`/api/hosts/${id}`),
		updateTags: (id: string, tags: string[]) =>
			send<{ tags: string[] }>(`/api/hosts/${id}/tags`, 'PUT', tags)
	},

	// Settings / user preferences.
	settings: {
		getDashboardLayout: () => get<Record<string, unknown>>('/api/settings/dashboard-layout'),
		saveDashboardLayout: (layout: unknown) =>
			send<{ ok: boolean }>('/api/settings/dashboard-layout', 'PUT', layout)
	},

	// Legacy flat accessors kept for existing pages.
	host: (id: string) => get<Host>(`/api/hosts/${id}`),
	latest: (id: string) => get<MetricSample | null>(`/api/hosts/${id}/metrics/latest`),
	metrics: (id: string, range = '1h', points = 300) =>
		get<MetricSample[]>(`/api/hosts/${id}/metrics?range=${range}&points=${points}`),
	netHistory: (id: string, range = '1h', points = 300) =>
		get<NetIfaceHistory>(`/api/hosts/${id}/metrics/net?range=${range}&points=${points}`),
	diskMountHistory: (id: string, range = '1h', points = 300) =>
		get<DiskMountHistory>(`/api/hosts/${id}/metrics/mounts?range=${range}&points=${points}`),
	diskIOHistory: (id: string, range = '1h', points = 300) =>
		get<DiskIOHistory>(`/api/hosts/${id}/metrics/diskio?range=${range}&points=${points}`),
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
	containerLogs: async (
		hostID: string,
		cid: string,
		opts: { tail?: number; since?: number; timestamps?: boolean } = {}
	) => {
		const { tail = 200, since, timestamps } = opts;
		const p = new URLSearchParams({ tail: String(tail) });
		if (since) p.set('since', String(since));
		if (timestamps) p.set('timestamps', 'true');
		const res = await fetch(`${API_BASE}/api/hosts/${hostID}/containers/${cid}/logs?${p}`);
		if (!res.ok) throw new Error(`logs -> ${res.status}`);
		return res.text();
	},

	networks: (id: string) => get<DockerNetwork[]>(`/api/hosts/${id}/networks`),
	networkInspect: (hostID: string, netID: string) =>
		get<DockerNetwork>(`/api/hosts/${hostID}/networks/${netID}`),
	createNetwork: (hostID: string, spec: NetworkCreateSpec) =>
		send<{ id: string }>(`/api/hosts/${hostID}/networks`, 'POST', spec),
	removeNetwork: (hostID: string, netID: string) =>
		del(`/api/hosts/${hostID}/networks/${netID}`),
	connectNetwork: (hostID: string, netID: string, containerID: string) =>
		send<{ ok: boolean }>(`/api/hosts/${hostID}/networks/${netID}/connect`, 'POST', { container_id: containerID }),
	disconnectNetwork: (hostID: string, netID: string, containerID: string) =>
		send<{ ok: boolean }>(`/api/hosts/${hostID}/networks/${netID}/disconnect`, 'POST', { container_id: containerID }),

	volumes: (id: string) => get<DockerVolume[]>(`/api/hosts/${id}/volumes`),
	volumeInspect: (hostID: string, name: string) =>
		get<DockerVolume>(`/api/hosts/${hostID}/volumes/${name}`),
	createVolume: (hostID: string, spec: VolumeCreateSpec) =>
		send<{ name: string }>(`/api/hosts/${hostID}/volumes`, 'POST', spec),
	removeVolume: (hostID: string, name: string, force = false) =>
		del(`/api/hosts/${hostID}/volumes/${name}?force=${force}`),

	images: (id: string) => get<DockerImage[]>(`/api/hosts/${id}/images`),
	imageInspect: (hostID: string, name: string) =>
		get<DockerImage>(`/api/hosts/${hostID}/images/${encodeURIComponent(name)}`),
	removeImage: (hostID: string, name: string, force = false) =>
		del(`/api/hosts/${hostID}/images/${encodeURIComponent(name)}?force=${force}`),
	pullImage: (hostID: string, image: string) =>
		send<{ ok: boolean }>(`/api/hosts/${hostID}/images/pull`, 'POST', { image }),
	checkImageUpdate: (hostID: string, name: string) =>
		get<ImageUpdateStatus>(`/api/hosts/${hostID}/images/${encodeURIComponent(name)}/update-check`),


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
	},

	alertChannels: () => get<AlertChannel[]>('/api/alerts/channels'),
	createAlertChannel: (ch: Partial<AlertChannel>) =>
		send<AlertChannel>('/api/alerts/channels', 'POST', ch),
	updateAlertChannel: (id: number, ch: Partial<AlertChannel>) =>
		send<AlertChannel>(`/api/alerts/channels/${id}`, 'PUT', ch),
	deleteAlertChannel: (id: number) => del(`/api/alerts/channels/${id}`),
	testAlertChannel: (id: number) =>
		send<{ ok: boolean }>(`/api/alerts/channels/${id}/test`, 'POST', {}),

	agentTokens: () => get<AgentToken[]>('/api/agents/tokens'),
	createAgentToken: (name: string) =>
		send<AgentToken>('/api/agents/tokens', 'POST', { name }),
	revokeAgentToken: (id: number) => del(`/api/agents/tokens/${id}`),
	connectedAgents: () => get<string[]>('/api/agents/connected'),

	// Compose stack management
	composeStacks: (hostID: string) =>
		get<ComposeStack[]>(`/api/hosts/${hostID}/compose`),
	composeStack: (hostID: string, project: string) =>
		get<ComposeStack>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}`),
	composeAction: (
		hostID: string,
		project: string,
		action: string,
		body: { working_dir?: string; service?: string; volumes?: boolean; extra_args?: string[] } = {}
	) =>
		send<{ output: string }>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}/${action}`, 'POST', body),
	composeLogs: (hostID: string, project: string, opts: { working_dir?: string; service?: string; tail?: number } = {}) => {
		const q = new URLSearchParams();
		if (opts.working_dir) q.set('working_dir', opts.working_dir);
		if (opts.service) q.set('service', opts.service);
		if (opts.tail) q.set('tail', String(opts.tail));
		const qs = q.toString();
		return get<{ logs: string }>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}/logs${qs ? `?${qs}` : ''}`);
	},
	composeFile: (hostID: string, project: string, workingDir?: string) => {
		const q = workingDir ? `?working_dir=${encodeURIComponent(workingDir)}` : '';
		return get<{ content: string; working_dir: string }>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}/file${q}`);
	},
	composeWriteFile: (
		hostID: string,
		project: string,
		body: { content: string; working_dir?: string; deploy?: boolean }
	) =>
		send<{ output: string }>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}/file`, 'PUT', body),
	composeVersions: (hostID: string, project: string) =>
		get<ComposeVersion[]>(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}/versions`),
	composeVersionContent: (hostID: string, vid: number) =>
		get<ComposeVersion>(`/api/hosts/${hostID}/compose/versions/${vid}`),
	createComposeStack: (
		hostID: string,
		body: { working_dir: string; content: string; start?: boolean }
	) =>
		send<ComposeStack>(`/api/hosts/${hostID}/compose`, 'POST', body),
	deleteComposeStack: (hostID: string, project: string, volumes = false) =>
		del(`/api/hosts/${hostID}/compose/${encodeURIComponent(project)}${volumes ? '?volumes=true' : ''}`)
};
