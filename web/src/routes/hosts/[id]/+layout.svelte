<script lang="ts" module>
	import { getContext, setContext } from 'svelte';
	import type { Host, HostStatus, MetricSample } from '$lib/types';

	const KEY = Symbol('host-detail');

	export interface HostDetailContext {
		hostId: string;
		host: Host | null;
		hostName: string;
		latest: MetricSample | null;
		status: HostStatus;
	}

	export function getHostDetail(): () => HostDetailContext {
		return getContext<() => HostDetailContext>(KEY);
	}

	export function provideHostDetail(get: () => HostDetailContext) {
		setContext(KEY, get);
	}
</script>

<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import HostHeader from '$lib/components/host/HostHeader.svelte';
	import HostTabBar from '$lib/components/host/HostTabBar.svelte';
	import { toast } from '$lib/toast';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';

	let { children } = $props();

	const id = $derived(page.params.id ?? '');

	let host = $state<Host | null>(null);
	let latest = $state<MetricSample | null>(null);
	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let bootError = $state<string | null>(null);

	// Use the same fallback thresholds the dashboard's NeedsAttention uses
	// (mirrors store.DefaultHostConfig on the server). The host-specific
	// config lives in the monitoring bundle which only the monitoring tabs
	// load, so the layout falls back to the defaults for the header indicator.
	const T = { warn_cpu: 70, crit_cpu: 90, warn_mem: 80, crit_mem: 90, warn_disk: 80, crit_disk: 90, warn_temp: 70, crit_temp: 85 };
	const status = $derived.by<HostStatus>(() => {
		if (!latest) return 'offline';
		const maxTemp = latest.temps?.reduce((acc, t) => Math.max(acc, t.temp_celsius), 0) ?? 0;
		if (latest.cpu_percent >= T.crit_cpu || latest.mem_percent >= T.crit_mem || latest.disk_percent >= T.crit_disk || maxTemp >= T.crit_temp) return 'crit';
		if (latest.cpu_percent >= T.warn_cpu || latest.mem_percent >= T.warn_mem || latest.disk_percent >= T.warn_disk || maxTemp >= T.warn_temp) return 'warn';
		return 'ok';
	});

	async function loadHost() {
		try {
			host = await api.host(id);
		} catch (e) {
			bootError = e instanceof Error ? e.message : 'Failed to load host';
		}
	}

	async function loadLatest() {
		try {
			latest = await api.latest(id);
		} catch {
			// Latest may not exist yet; status reverts to offline silently.
		}
	}

	// Expose stable, identity-safe context object via a getter — children call
	// the getter inside reactive scopes to track changes.
	provideHostDetail(() => ({
		hostId: id,
		host,
		hostName: host?.name ?? id,
		latest,
		status
	}));

	onMount(async () => {
		if (!id) return;
		await Promise.all([loadHost(), loadLatest()]);
		pollTimer = setInterval(loadLatest, 30_000);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});

	// Host-header action confirmations
	let confirmRemove = $state(false);
	let confirmRestart = $state(false);
	let removeBusy = $state(false);
	let restartBusy = $state(false);

	function onHeaderAction(action: 'restart' | 'ssh' | 'update' | 'stop') {
		if (action === 'restart') {
			confirmRestart = true;
			return;
		}
		if (action === 'stop') {
			confirmRemove = true;
			return;
		}
		toast.info(`${action} not implemented in this build`);
	}

	async function doRestart() {
		// Backend host-level restart is not implemented in this build; the UI
		// surfaces the confirmation step so the danger-action treatment is in
		// place when the API arrives. Toast clearly says so for now.
		restartBusy = false;
		confirmRestart = false;
		toast.info('Host restart is not wired up yet in this build');
	}

	async function doRemoveHost() {
		removeBusy = false;
		confirmRemove = false;
		toast.info('Host removal is not wired up yet in this build');
	}
</script>

<svelte:head>
	<title>Aperture — {host?.name ?? id}</title>
</svelte:head>

<div class="host-detail">
	{#if bootError && !host}
		<div class="boot-error">
			<h2>Couldn't load host</h2>
			<p>{bootError}</p>
		</div>
	{:else if host}
		<HostHeader {host} {status} uptimeSecs={latest?.uptime_secs} onaction={onHeaderAction} />

		<div class="tabbar-wrap">
			<HostTabBar hostId={id} />
		</div>

		<div class="content">
			{@render children()}
		</div>
	{:else}
		<div class="loading">Loading host…</div>
	{/if}
</div>

<ConfirmDialog
	open={confirmRestart}
	tone="warning"
	title="Restart host"
	message="Send a restart signal to this host?"
	detail={host?.name}
	consequences={[
		'The host will reboot and become temporarily unreachable.',
		'Running containers will stop until the host comes back up.'
	]}
	confirmLabel="Restart host"
	busy={restartBusy}
	onconfirm={doRestart}
	oncancel={() => (confirmRestart = false)}
/>

<ConfirmDialog
	open={confirmRemove}
	tone="danger"
	title="Remove host"
	message="Stop monitoring this host and delete its history?"
	detail={host?.name}
	consequences={[
		'Aperture will stop collecting metrics for this host.',
		'Stored metric history and alert events for this host will be deleted.',
		'Workloads on the host itself are not affected.'
	]}
	confirmLabel="Remove host"
	busy={removeBusy}
	onconfirm={doRemoveHost}
	oncancel={() => (confirmRemove = false)}
/>

<style>
	.host-detail {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.tabbar-wrap {
		border-bottom: 1px solid var(--line);
		margin-bottom: 18px;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}

	.content {
		display: block;
	}

	.loading {
		padding: 40px;
		text-align: center;
		color: var(--text-faint);
		font-size: 13px;
	}

	.boot-error {
		padding: 24px;
		background: var(--crit-soft);
		border: 1px solid var(--crit);
		border-radius: var(--r-lg);
	}
	.boot-error h2 { margin: 0 0 6px; color: var(--crit); font-size: 16px; }
	.boot-error p { margin: 0; color: var(--text-dim); font-size: 13px; }
</style>
