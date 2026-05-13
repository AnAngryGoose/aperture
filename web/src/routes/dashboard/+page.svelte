<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { hostStore } from '$lib/stores/hosts';
	import { dashboardLayout } from '$lib/stores/dashboardLayout';
	import { toast } from '$lib/toast';
	import PageHeader from '$lib/components/dashboard/PageHeader.svelte';
	import FilterBar from '$lib/components/dashboard/FilterBar.svelte';
	import HostGrid from '$lib/components/dashboard/HostGrid.svelte';
	import DrillIn from '$lib/components/host/DrillIn.svelte';
	import AddHostModal from '$lib/components/addhost/AddHostModal.svelte';
	import type { HostEntry } from '$lib/stores/hosts';

	let loading = $state(true);
	let error = $state<string | null>(null);
	let selectedEntry = $state<HostEntry | null>(null);
	let showAddHost = $state(false);
	let pollTimer: ReturnType<typeof setInterval>;

	const API_BASE = import.meta.env.VITE_API_BASE ?? (import.meta.env.DEV ? 'http://localhost:8080' : '');

	async function load() {
		try {
			const [hosts, samples] = await Promise.all([
				api.hosts.list(),
				Promise.all([]).then(() => ({})) // batch latest metrics if needed
			]);
			// Fetch latest metrics per host in parallel.
			const metricResults = await Promise.allSettled(
				hosts.map((h) => api.latest(h.id))
			);
			const sampleMap: Record<string, any> = {};
			hosts.forEach((h, i) => {
				const r = metricResults[i];
				if (r.status === 'fulfilled' && r.value) sampleMap[h.id] = r.value;
			});
			hostStore.setAll(hosts, sampleMap);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load hosts';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		dashboardLayout.init();
		load();
		pollTimer = setInterval(load, 5000);
		// Connect SSE for live sparkline updates.
		hostStore.connectSSE(API_BASE);
	});

	onDestroy(() => {
		clearInterval(pollTimer);
		hostStore.disconnectSSE();
	});

	function openDrillIn(entry: HostEntry) {
		selectedEntry = entry;
	}

	function closeDrillIn() {
		selectedEntry = null;
	}
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') closeDrillIn(); }} />

<div class="dashboard">
	<PageHeader />
	<FilterBar onaddhost={() => (showAddHost = true)} />
	<HostGrid
		entries={hostStore.list}
		layout={dashboardLayout.cardLayout}
		{loading}
		{error}
		filter={dashboardLayout.activeFilter}
		onselect={openDrillIn}
		onaddhost={() => (showAddHost = true)}
		onretry={load}
	/>
</div>

{#if selectedEntry}
	<DrillIn entry={selectedEntry} onclose={closeDrillIn} />
{/if}

{#if showAddHost}
	<AddHostModal onclose={() => (showAddHost = false)} />
{/if}

<style>
	.dashboard {
		padding: 22px 28px 60px;
		max-width: 1600px;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
</style>
