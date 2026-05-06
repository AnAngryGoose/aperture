<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Host, MetricSample } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import Chart from '$lib/Chart.svelte';
	import { formatBytes, formatPct, formatDuration } from '$lib/format';

	let id = $derived(page.params.id);
	let host = $state<Host | null>(null);
	let samples = $state<MetricSample[]>([]);
	let latest = $derived(samples.length > 0 ? samples[samples.length - 1] : null);
	let range = $state<'15m' | '1h' | '6h' | '24h'>('1h');
	let error = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function load() {
		try {
			const [h, ms] = await Promise.all([api.host(id), api.metrics(id, range, 300)]);
			host = h;
			samples = ms;
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	$effect(() => {
		// reload when id or range change
		void id;
		void range;
		load();
	});

	onMount(() => {
		timer = setInterval(load, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	let xs = $derived(samples.map((s) => Math.floor(new Date(s.timestamp).getTime() / 1000)));
	// Net rates: derive bytes/sec from cumulative counters between consecutive samples.
	let netRx = $derived(
		samples.map((s, i) => {
			if (i === 0) return 0;
			const prev = samples[i - 1];
			const dt = (new Date(s.timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) return 0;
			return Math.max(0, (s.net_rx_bytes - prev.net_rx_bytes) / dt / 1024);
		})
	);
	let netTx = $derived(
		samples.map((s, i) => {
			if (i === 0) return 0;
			const prev = samples[i - 1];
			const dt = (new Date(s.timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) return 0;
			return Math.max(0, (s.net_tx_bytes - prev.net_tx_bytes) / dt / 1024);
		})
	);
</script>

<div class="page-header">
	<div>
		<a href="/" class="back">← all hosts</a>
		<h1>{host?.name ?? id}</h1>
		{#if host}
			<div class="muted mono">
				{host.platform || host.os} · {host.arch} · {host.cpu_count} vCPU · {formatBytes(host.mem_total)} RAM
			</div>
		{/if}
	</div>
	<div class="actions">
		<a href={`/hosts/${id}/containers`} class="link-btn">Containers →</a>
		<div class="range-picker">
			{#each ['15m', '1h', '6h', '24h'] as r}
				<button class:active={range === r} onclick={() => (range = r as typeof range)}>{r}</button>
			{/each}
		</div>
	</div>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

{#if latest}
	<div class="grid cols-4">
		<div class="card stat">
			<div class="muted">CPU</div>
			<div class="big">{formatPct(latest.cpu_percent)}</div>
			<Bar value={latest.cpu_percent} />
		</div>
		<div class="card stat">
			<div class="muted">Memory</div>
			<div class="big">{formatPct(latest.mem_percent)}</div>
			<Bar value={latest.mem_percent} />
			<div class="muted mono">{formatBytes(latest.mem_used)} / {formatBytes(latest.mem_total)}</div>
		</div>
		<div class="card stat">
			<div class="muted">Disk</div>
			<div class="big">{formatPct(latest.disk_percent)}</div>
			<Bar value={latest.disk_percent} />
			<div class="muted mono">{formatBytes(latest.disk_used)} / {formatBytes(latest.disk_total)}</div>
		</div>
		<div class="card stat">
			<div class="muted">Uptime</div>
			<div class="big">{formatDuration(latest.uptime_secs)}</div>
			<div class="muted mono">load {latest.load_avg_1.toFixed(2)} / {latest.load_avg_5.toFixed(2)} / {latest.load_avg_15.toFixed(2)}</div>
		</div>
	</div>
{/if}

{#if samples.length > 1}
	<div class="card chart-card">
		<div class="chart-title">CPU usage</div>
		<Chart x={xs} series={[{ label: 'CPU %', values: samples.map((s) => s.cpu_percent) }]} valueSuffix="%" yMin={0} yMax={100} />
	</div>
	<div class="card chart-card">
		<div class="chart-title">Memory usage</div>
		<Chart x={xs} series={[
			{ label: 'Memory %', values: samples.map((s) => s.mem_percent), stroke: '#7ce38b' }
		]} valueSuffix="%" yMin={0} yMax={100} />
	</div>
	<div class="card chart-card">
		<div class="chart-title">Disk usage</div>
		<Chart x={xs} series={[
			{ label: 'Disk %', values: samples.map((s) => s.disk_percent), stroke: '#ffcb6b' }
		]} valueSuffix="%" yMin={0} yMax={100} />
	</div>
	<div class="card chart-card">
		<div class="chart-title">Network throughput (KiB/s)</div>
		<Chart x={xs} series={[
			{ label: 'Rx', values: netRx, stroke: '#5cc8ff' },
			{ label: 'Tx', values: netTx, stroke: '#c792ea' }
		]} valueSuffix=" KiB/s" />
	</div>
	<div class="card chart-card">
		<div class="chart-title">Load average</div>
		<Chart x={xs} series={[
			{ label: '1m', values: samples.map((s) => s.load_avg_1) },
			{ label: '5m', values: samples.map((s) => s.load_avg_5), stroke: '#7ce38b' },
			{ label: '15m', values: samples.map((s) => s.load_avg_15), stroke: '#ffcb6b' }
		]} />
	</div>
{:else if !error}
	<div class="card muted">Collecting samples… charts will appear once at least 2 are available.</div>
{/if}

<style>
	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 16px;
		flex-wrap: wrap;
		gap: 12px;
	}
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }
	.actions { display: flex; gap: 12px; align-items: center; }
	.link-btn {
		padding: 6px 12px;
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
	}
	.link-btn:hover { border-color: var(--accent); text-decoration: none; }
	.range-picker { display: flex; gap: 4px; }
	.range-picker button.active {
		background: var(--bg-elev);
		border-color: var(--accent);
		color: var(--accent);
	}
	.stat { display: flex; flex-direction: column; gap: 6px; }
	.big { font-size: 22px; font-weight: 600; font-family: var(--mono); }
	.chart-card { margin-top: 16px; }
	.chart-title { font-size: 12px; color: var(--text-dim); margin-bottom: 8px; }
	.err { color: var(--bad); border-color: var(--bad); }
</style>
