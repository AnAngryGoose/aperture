<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { MonitoringBundle, HostStatus } from '$lib/types';
	import HostHeader from '$lib/components/host/HostHeader.svelte';
	import RangePicker, { type Range } from '$lib/components/host/RangePicker.svelte';
	import OverviewTab from '$lib/components/host/OverviewTab.svelte';
	import CPUTab from '$lib/components/host/CPUTab.svelte';
	import MemoryTab from '$lib/components/host/MemoryTab.svelte';
	import DiskTab from '$lib/components/host/DiskTab.svelte';
	import NetworkTab from '$lib/components/host/NetworkTab.svelte';
	import SensorsTab from '$lib/components/host/SensorsTab.svelte';
	import ProcessesTab from '$lib/components/host/ProcessesTab.svelte';
	import DockerTab from '$lib/components/host/DockerTab.svelte';
	import EventsTab from '$lib/components/host/EventsTab.svelte';
	import MonitoringSettingsTab from '$lib/components/host/MonitoringSettingsTab.svelte';

	type TabKey =
		| 'overview' | 'cpu' | 'memory' | 'disk' | 'network'
		| 'sensors' | 'processes' | 'docker' | 'events' | 'settings';

	const TABS: { key: TabKey; label: string }[] = [
		{ key: 'overview',   label: 'Overview' },
		{ key: 'cpu',        label: 'CPU' },
		{ key: 'memory',     label: 'Memory' },
		{ key: 'disk',       label: 'Disk' },
		{ key: 'network',    label: 'Network' },
		{ key: 'sensors',    label: 'Sensors' },
		{ key: 'processes',  label: 'Processes' },
		{ key: 'docker',     label: 'Docker' },
		{ key: 'events',     label: 'Events' },
		{ key: 'settings',   label: 'Settings' }
	];

	let id = $derived(page.params.id ?? '');

	// Range: read persisted preference from localStorage; default to 1h.
	function initialRange(): Range {
		if (typeof window === 'undefined' || !id) return '1h';
		try {
			const stored = localStorage.getItem(`aperture.range.${id}`);
			if (stored && ['15m', '1h', '6h', '24h', '7d', '30d'].includes(stored)) {
				return stored as Range;
			}
		} catch { /* localStorage unavailable */ }
		return '1h';
	}

	let range = $state<Range>('1h');
	let activeTab = $state<TabKey>('overview');
	let bundle = $state<MonitoringBundle | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let pollTimer: ReturnType<typeof setInterval> | null = null;

	// Status comes from server via host_status SSE event into monitoringStore;
	// we don't subscribe here, so derive a fallback from the bundle's latest
	// sample + thresholds. The dashboard owns SSE; this page just renders.
	const status = $derived.by<HostStatus>(() => {
		if (!bundle) return 'offline';
		const s = bundle.latest;
		const cfg = bundle.config;
		if (!s) return 'offline';
		const maxTemp = s.temps?.reduce((acc, t) => Math.max(acc, t.temp_celsius), 0) ?? 0;
		if (s.cpu_percent >= cfg.crit_cpu || s.mem_percent >= cfg.crit_mem || s.disk_percent >= cfg.crit_disk || maxTemp >= cfg.crit_temp) return 'crit';
		if (s.cpu_percent >= cfg.warn_cpu || s.mem_percent >= cfg.warn_mem || s.disk_percent >= cfg.warn_disk || maxTemp >= cfg.warn_temp) return 'warn';
		return 'ok';
	});

	async function load() {
		if (!id) return;
		loading = bundle === null;
		try {
			bundle = await api.monitoring.bundle(id, range, 300);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load monitoring bundle';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		range = initialRange();
		// 30s reconciliation — SSE handles live samples; this poll only fills
		// in history gaps after disconnects or missed events.
		pollTimer = setInterval(load, 30_000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});

	// Re-fetch the bundle when host or range changes. Tracks ONLY id and
	// range — must not read any state that `load()` writes (`bundle`,
	// `loading`, `error`), or we'd loop. The `lastFetchKey` guard
	// de-duplicates redundant runs (e.g. Svelte's initial pre-mount run).
	let lastFetchKey = $state('');
	$effect(() => {
		const key = `${id}|${range}`;
		if (!id || key === lastFetchKey) return;
		lastFetchKey = key;
		load();
	});

	function onRangeChange(r: Range) {
		range = r;
	}
</script>

<svelte:head>
	<title>Aperture — {bundle?.host.name ?? id}</title>
</svelte:head>

<div class="host-page">
	{#if loading && !bundle}
		<div class="loading">Loading monitoring data…</div>
	{:else if error && !bundle}
		<div class="error-card">
			<h2>Couldn't load host</h2>
			<p>{error}</p>
			<button class="btn" onclick={load}>Retry</button>
		</div>
	{:else if bundle}
		<HostHeader host={bundle.host} {status} uptimeSecs={bundle.latest?.uptime_secs} />

		<nav class="tab-nav" role="tablist" aria-label="Monitoring sections">
			<div class="tabs">
				{#each TABS as t}
					<button
						class="tab"
						role="tab"
						aria-selected={activeTab === t.key}
						class:active={activeTab === t.key}
						onclick={() => (activeTab = t.key)}
					>
						{t.label}
					</button>
				{/each}
			</div>
			<RangePicker bind:value={range} hostId={id} onchange={onRangeChange} />
		</nav>

		<div class="tab-content">
			{#if activeTab === 'overview'}
				<OverviewTab {bundle} />
			{:else if activeTab === 'cpu'}
				<CPUTab {bundle} />
			{:else if activeTab === 'memory'}
				<MemoryTab {bundle} />
			{:else if activeTab === 'disk'}
				<DiskTab {bundle} />
			{:else if activeTab === 'network'}
				<NetworkTab {bundle} />
			{:else if activeTab === 'sensors'}
				<SensorsTab {bundle} />
			{:else if activeTab === 'processes'}
				<ProcessesTab {bundle} {range} />
			{:else if activeTab === 'docker'}
				<DockerTab {bundle} />
			{:else if activeTab === 'events'}
				<EventsTab {bundle} />
			{:else if activeTab === 'settings'}
				<MonitoringSettingsTab {bundle} onsaved={load} />
			{/if}
		</div>
	{/if}
</div>

<style>
	.host-page {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.loading {
		padding: 40px;
		text-align: center;
		color: var(--text-faint);
		font-size: 13px;
	}

	.error-card {
		padding: 24px;
		background: var(--crit-soft);
		border: 1px solid var(--crit);
		border-radius: var(--r-lg);
	}
	.error-card h2 { margin: 0 0 6px; color: var(--crit); font-size: 16px; }
	.error-card p { margin: 0 0 12px; color: var(--text-dim); font-size: 13px; }
	.btn {
		padding: 6px 12px;
		font-size: 12px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		color: var(--text);
	}

	.tab-nav {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		border-bottom: 1px solid var(--line);
		margin-bottom: 18px;
		overflow-x: auto;
	}

	.tabs {
		display: flex;
		gap: 0;
	}

	.tab {
		padding: 10px 14px;
		font-size: 13px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		cursor: pointer;
		transition: color 120ms, border-color 120ms;
		white-space: nowrap;
	}

	.tab:hover { color: var(--text); }

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.tab-content {
		display: block;
	}
</style>
