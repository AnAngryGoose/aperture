<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Host, MetricSample } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import Chart from '$lib/Chart.svelte';
	import { formatBytes, formatPct, formatDuration, relTime } from '$lib/format';

	let id = $derived(page.params.id);
	let host = $state<Host | null>(null);
	let samples = $state<MetricSample[]>([]);
	let latest = $state<MetricSample | null>(null);
	let range = $state<'15m' | '1h' | '6h' | '24h'>('1h');
	let error = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function load() {
		try {
			const [h, ms, lv] = await Promise.all([
				api.host(id),
				api.metrics(id, range, 300),
				api.latest(id)
			]);
			host = h;
			samples = ms;
			latest = lv;
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	$effect(() => {
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

	// --- Chart data derivations ---
	let xs = $derived(samples.map((s) => Math.floor(new Date(s.timestamp).getTime() / 1000)));

	let netRx = $derived(
		samples.map((s, i) => {
			if (i === 0) return 0;
			const prev = samples[i - 1];
			const dt = (new Date(s.timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) return 0;
			return Math.max(0, (s.net_rx_bytes - prev.net_rx_bytes) / dt);
		})
	);
	let netTx = $derived(
		samples.map((s, i) => {
			if (i === 0) return 0;
			const prev = samples[i - 1];
			const dt = (new Date(s.timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) return 0;
			return Math.max(0, (s.net_tx_bytes - prev.net_tx_bytes) / dt);
		})
	);

	// Formatters for chart Y-axes.
	function fmtGiB(bytes: number): string {
		const gib = bytes / 1073741824;
		return gib >= 10 ? `${gib.toFixed(1)} GiB` : `${gib.toFixed(2)} GiB`;
	}
	function fmtBytesRate(bps: number): string {
		if (bps < 1024) return `${bps.toFixed(0)} B/s`;
		if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
		return `${(bps / 1048576).toFixed(1)} MiB/s`;
	}

	// Human-friendly rate for table cells.
	function rateStr(bps: number): string {
		if (bps <= 0) return '0 B/s';
		return fmtBytesRate(bps);
	}
</script>

<div class="page-header">
	<div>
		<a href="/" class="back">← all hosts</a>
		<h1>{host?.name ?? id}</h1>
		{#if host}
			<div class="muted mono">
				{host.platform || host.os} · {host.arch} · {host.cpu_count} vCPU · {formatBytes(host.mem_total)} RAM
			</div>
			{#if host.cpu_model}
				<div class="muted mono small">{host.cpu_model}</div>
			{/if}
		{/if}
	</div>
	<div class="range-picker">
		{#each ['15m', '1h', '6h', '24h'] as r}
			<button class:active={range === r} onclick={() => (range = r as typeof range)}>{r}</button>
		{/each}
	</div>
</div>

<!-- Sub-navigation -->
<nav class="subnav">
	<a href={`/hosts/${id}`} class="active">Overview</a>
	<a href={`/hosts/${id}/containers`}>Containers</a>
	<a href={`/hosts/${id}/networks`} class="placeholder">Networks</a>
	<a href={`/hosts/${id}/volumes`} class="placeholder">Volumes</a>
	<a href={`/hosts/${id}/images`} class="placeholder">Images</a>
	<a href={`/hosts/${id}/logs`} class="placeholder">Logs</a>
</nav>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

<!-- ── Summary stat cards ─────────────────────────────────────────── -->
{#if latest}
	<div class="grid cols-4">
		<div class="card stat">
			<div class="label">CPU</div>
			<div class="big">{formatPct(latest.cpu_percent)}</div>
			<Bar value={latest.cpu_percent} />
			<div class="muted mono small">
				load {latest.load_avg_1.toFixed(2)} / {latest.load_avg_5.toFixed(2)} / {latest.load_avg_15.toFixed(2)}
			</div>
		</div>
		<div class="card stat">
			<div class="label">Memory</div>
			<div class="big">{formatBytes(latest.mem_used)}</div>
			<Bar value={latest.mem_percent} />
			<div class="muted mono small">{formatBytes(latest.mem_used)} / {formatBytes(latest.mem_total)} · {formatPct(latest.mem_percent)}</div>
			{#if latest.mem_avail}
				<div class="muted mono small">avail {formatBytes(latest.mem_avail)}{#if latest.mem_cached} · cached {formatBytes(latest.mem_cached)}{/if}</div>
			{/if}
			{#if latest.swap_total > 0}
				<div class="muted mono small">swap {formatBytes(latest.swap_used)} / {formatBytes(latest.swap_total)}</div>
			{/if}
		</div>
		<div class="card stat">
			<div class="label">Disk (/)</div>
			<div class="big">{formatBytes(latest.disk_used)}</div>
			<Bar value={latest.disk_percent} />
			<div class="muted mono small">{formatBytes(latest.disk_used)} / {formatBytes(latest.disk_total)} · {formatPct(latest.disk_percent)}</div>
		</div>
		<div class="card stat">
			<div class="label">Uptime</div>
			<div class="big">{formatDuration(latest.uptime_secs)}</div>
			{#if host}
				<div class="muted mono small">seen {relTime(host.last_seen)}</div>
			{/if}
		</div>
	</div>

	<!-- ── Per-core CPU ───────────────────────────────────────────── -->
	{#if latest.cpu_per_core && latest.cpu_per_core.length > 0}
		<div class="card section">
			<div class="section-title">CPU — per core</div>
			<div class="core-grid">
				{#each latest.cpu_per_core as pct, i}
					<div class="core-item">
						<div class="core-label muted mono small">C{i}</div>
						<div class="core-bar-wrap">
							<div class="core-bar-fill" style="width:{Math.min(100,pct)}%; background:{pct>=90?'var(--bad)':pct>=70?'var(--warn)':'var(--accent)'}"></div>
						</div>
						<div class="core-pct mono small">{pct.toFixed(0)}%</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- ── Network interfaces ─────────────────────────────────────── -->
	{#if latest.net_interfaces && latest.net_interfaces.length > 0}
		<div class="card section">
			<div class="section-title">Network interfaces</div>
			<table>
				<thead>
					<tr>
						<th>Interface</th>
						<th>RX rate</th>
						<th>TX rate</th>
						<th>RX total</th>
						<th>TX total</th>
					</tr>
				</thead>
				<tbody>
					{#each latest.net_interfaces as iface}
						<tr>
							<td class="mono">{iface.name}</td>
							<td class="mono accent">{rateStr(iface.rx_rate)}</td>
							<td class="mono warn">{rateStr(iface.tx_rate)}</td>
							<td class="mono muted">{formatBytes(iface.rx_bytes)}</td>
							<td class="mono muted">{formatBytes(iface.tx_bytes)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<!-- ── Disk mounts ────────────────────────────────────────────── -->
	{#if latest.disk_mounts && latest.disk_mounts.length > 0}
		<div class="card section">
			<div class="section-title">Disk mounts</div>
			<table>
				<thead>
					<tr>
						<th>Mount</th>
						<th>Device</th>
						<th>FS</th>
						<th>Used</th>
						<th>Total</th>
						<th style="width:180px">Usage</th>
					</tr>
				</thead>
				<tbody>
					{#each latest.disk_mounts as m}
						<tr>
							<td class="mono">{m.mount}</td>
							<td class="mono muted small">{m.device.replace('/dev/', '')}</td>
							<td class="mono muted small">{m.fstype}</td>
							<td class="mono">{formatBytes(m.used)}</td>
							<td class="mono muted">{formatBytes(m.total)}</td>
							<td>
								<div class="bar-row">
									<Bar value={m.percent} />
									<span class="mono small muted">{m.percent.toFixed(1)}%</span>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<!-- ── Disk I/O ───────────────────────────────────────────────── -->
	{#if latest.disk_io && latest.disk_io.length > 0}
		<div class="card section">
			<div class="section-title">Disk I/O</div>
			<table>
				<thead>
					<tr>
						<th>Device</th>
						<th>Read rate</th>
						<th>Write rate</th>
						<th>Read total</th>
						<th>Write total</th>
					</tr>
				</thead>
				<tbody>
					{#each latest.disk_io as dev}
						<tr>
							<td class="mono">{dev.device}</td>
							<td class="mono accent">{rateStr(dev.read_rate)}</td>
							<td class="mono warn">{rateStr(dev.write_rate)}</td>
							<td class="mono muted">{formatBytes(dev.read_bytes)}</td>
							<td class="mono muted">{formatBytes(dev.write_bytes)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<!-- ── Temperature sensors ───────────────────────────────────── -->
	{#if latest.temps && latest.temps.length > 0}
		<div class="card section">
			<div class="section-title">Temperatures</div>
			<div class="temp-grid">
				{#each latest.temps as t}
					<div class="temp-item">
						<div class="muted mono small">{t.name}</div>
						<div class="temp-val mono {t.temp_celsius >= 90 ? 'bad' : t.temp_celsius >= 70 ? 'warn' : ''}">
							{t.temp_celsius.toFixed(1)}°C
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}
{/if}

<!-- ── Historical charts ──────────────────────────────────────────── -->
{#if samples.length > 1}
	<div class="charts-header muted small">Historical — {range} window</div>
	<div class="card chart-card">
		<div class="chart-title">CPU usage</div>
		<Chart x={xs} series={[{ label: 'CPU %', values: samples.map((s) => s.cpu_percent) }]} valueSuffix="%" yMin={0} yMax={100} />
	</div>
	<div class="card chart-card">
		<div class="chart-title">Memory usage</div>
		<Chart
			x={xs}
			series={[
				{ label: 'Used', values: samples.map((s) => s.mem_used), stroke: '#7ce38b' },
				{ label: 'Total', values: samples.map((s) => s.mem_total), stroke: '#232a3d' }
			]}
			valueFormatter={fmtGiB}
		/>
	</div>
	<div class="card chart-card">
		<div class="chart-title">Disk usage (/)</div>
		<Chart
			x={xs}
			series={[
				{ label: 'Used', values: samples.map((s) => s.disk_used), stroke: '#ffcb6b' },
				{ label: 'Total', values: samples.map((s) => s.disk_total), stroke: '#232a3d' }
			]}
			valueFormatter={fmtGiB}
		/>
	</div>
	<div class="card chart-card">
		<div class="chart-title">Network throughput</div>
		<Chart x={xs} series={[
			{ label: 'Rx', values: netRx, stroke: '#5cc8ff' },
			{ label: 'Tx', values: netTx, stroke: '#c792ea' }
		]} valueFormatter={fmtBytesRate} />
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
		margin-bottom: 12px;
		flex-wrap: wrap;
		gap: 12px;
	}
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }
	.small { font-size: 11px; }
	.range-picker { display: flex; gap: 4px; align-self: flex-start; margin-top: 4px; }
	.range-picker button.active {
		background: var(--bg-elev);
		border-color: var(--accent);
		color: var(--accent);
	}

	/* Sub-navigation */
	.subnav {
		display: flex;
		gap: 0;
		margin-bottom: 20px;
		border-bottom: 1px solid var(--border);
	}
	.subnav a {
		padding: 8px 16px;
		font-size: 13px;
		color: var(--text-dim);
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
	}
	.subnav a:hover { color: var(--text); text-decoration: none; }
	.subnav a.active { color: var(--accent); border-bottom-color: var(--accent); }
	.subnav a.placeholder { opacity: 0.45; cursor: default; }
	.subnav a.placeholder:hover { color: var(--text-dim); }

	/* Stat cards */
	.stat { display: flex; flex-direction: column; gap: 5px; }
	.label { font-size: 11px; color: var(--text-dim); text-transform: uppercase; letter-spacing: 0.05em; }
	.big { font-size: 22px; font-weight: 600; font-family: var(--mono); }

	/* Section cards */
	.section { margin-top: 16px; }
	.section-title {
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--text-dim);
		margin-bottom: 12px;
	}

	/* Per-core CPU grid */
	.core-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
		gap: 6px 12px;
	}
	.core-item {
		display: grid;
		grid-template-columns: 28px 1fr 36px;
		align-items: center;
		gap: 6px;
	}
	.core-label { text-align: right; }
	.core-bar-wrap {
		height: 6px;
		background: var(--bg-elev-2);
		border-radius: 3px;
		overflow: hidden;
	}
	.core-bar-fill {
		height: 100%;
		border-radius: 3px;
		transition: width 0.4s ease;
	}
	.core-pct { text-align: right; }

	/* Tables */
	.accent { color: var(--accent); }
	.warn { color: var(--warn); }
	.bad { color: var(--bad); }
	.bar-row { display: flex; align-items: center; gap: 8px; }
	.bar-row :global(.bar) { flex: 1; }

	/* Temperature grid */
	.temp-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 16px;
	}
	.temp-item { display: flex; flex-direction: column; gap: 2px; }
	.temp-val { font-size: 16px; font-weight: 600; }

	/* Charts */
	.charts-header {
		margin: 20px 0 6px;
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	.chart-card { margin-top: 12px; }
	.chart-title { font-size: 12px; color: var(--text-dim); margin-bottom: 8px; }
	.err { color: var(--bad); border-color: var(--bad); }
</style>
