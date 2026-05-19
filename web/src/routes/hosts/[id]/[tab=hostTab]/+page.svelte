<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { MonitoringBundle } from '$lib/types';
	import RangePicker, { type Range } from '$lib/components/host/RangePicker.svelte';
	import OverviewTab from '$lib/components/host/OverviewTab.svelte';
	import CPUTab from '$lib/components/host/CPUTab.svelte';
	import MemoryTab from '$lib/components/host/MemoryTab.svelte';
	import DiskTab from '$lib/components/host/DiskTab.svelte';
	import NetworkTab from '$lib/components/host/NetworkTab.svelte';
	import SensorsTab from '$lib/components/host/SensorsTab.svelte';
	import ProcessesTab from '$lib/components/host/ProcessesTab.svelte';
	import EventsTab from '$lib/components/host/EventsTab.svelte';
	import MonitoringSettingsTab from '$lib/components/host/MonitoringSettingsTab.svelte';
	import type { HostTab } from '../../../../params/hostTab';

	let id = $derived(page.params.id ?? '');
	const activeTab = $derived(page.params.tab as HostTab);

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
	let bundle = $state<MonitoringBundle | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let pollTimer: ReturnType<typeof setInterval> | null = null;

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
		pollTimer = setInterval(load, 30_000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});

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

<div class="tab-pane">
	{#if loading && !bundle}
		<div class="state-msg">Loading monitoring data…</div>
	{:else if error && !bundle}
		<div class="error-card">
			<h2>Couldn't load monitoring data</h2>
			<p>{error}</p>
			<button class="retry" onclick={load}>Retry</button>
		</div>
	{:else if bundle}
		<!-- Show the range picker only for charts; events/settings ignore it. -->
		{#if activeTab !== 'events' && activeTab !== 'settings'}
			<div class="range-row">
				<RangePicker bind:value={range} hostId={id} onchange={onRangeChange} />
			</div>
		{/if}

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
		{:else if activeTab === 'events'}
			<EventsTab {bundle} />
		{:else if activeTab === 'settings'}
			<MonitoringSettingsTab {bundle} onsaved={load} />
		{/if}
	{/if}
</div>

<style>
	.tab-pane { display: flex; flex-direction: column; gap: 14px; }

	.state-msg {
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
	.retry {
		padding: 6px 12px;
		font-size: 12px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		color: var(--text);
	}

	.range-row {
		display: flex;
		justify-content: flex-end;
	}
</style>
