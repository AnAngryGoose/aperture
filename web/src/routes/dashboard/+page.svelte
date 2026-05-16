<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { monitoringStore, type HostEntry } from '$lib/stores/monitoring.svelte';
	import { dashboardLayout } from '$lib/stores/dashboardLayout.svelte';
	import PageHeader from '$lib/components/dashboard/PageHeader.svelte';
	import FilterBar from '$lib/components/dashboard/FilterBar.svelte';
	import HostGrid from '$lib/components/dashboard/HostGrid.svelte';
	import DrillIn from '$lib/components/host/DrillIn.svelte';
	import AddHostModal from '$lib/components/addhost/AddHostModal.svelte';

	let loading = $state(true);
	let error = $state<string | null>(null);
	let selectedEntry = $state<HostEntry | null>(null);
	let showAddHost = $state(false);
	let pollTimer: ReturnType<typeof setInterval>;

	const API_BASE = import.meta.env.VITE_API_BASE ?? (import.meta.env.DEV ? 'http://localhost:8080' : '');

	// Single overview fetch — replaces the prior N+1 fan-out (hosts list +
	// latest-per-host + containers-per-docker-host). Live updates come from
	// SSE; this poll is just a reconciliation safety net at 30s cadence.
	async function reconcile() {
		try {
			const overview = await api.monitoring.overview();
			monitoringStore.hydrate(overview);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load hosts';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		dashboardLayout.init();
		reconcile();
		// 30s, not 5s — SSE handles live updates; this poll only catches up
		// after disconnects or missed events.
		pollTimer = setInterval(reconcile, 30_000);
		monitoringStore.connectSSE(API_BASE);
	});

	onDestroy(() => {
		clearInterval(pollTimer);
		monitoringStore.disconnectSSE();
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
		entries={monitoringStore.list}
		layout={dashboardLayout.cardLayout}
		{loading}
		{error}
		filter={dashboardLayout.activeFilter}
		onselect={openDrillIn}
		onaddhost={() => (showAddHost = true)}
		onretry={reconcile}
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
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
</style>
