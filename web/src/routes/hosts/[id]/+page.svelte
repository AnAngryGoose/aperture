<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Host, MetricSample, NetIfaceHistory, DiskMountHistory, DiskIOHistory } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import Chart from '$lib/Chart.svelte';
	import type { AlertEvent } from '$lib/types';
	import { formatBytes, formatBytesRate, formatPct, formatDuration, relTime, absTime } from '$lib/format';

	let id = $derived(page.params.id);
	let host = $state<Host | null>(null);
	let samples = $state<MetricSample[]>([]);
	let latest = $state<MetricSample | null>(null);
	let netH = $state<NetIfaceHistory | null>(null);
	let mountH = $state<DiskMountHistory | null>(null);
	let diskIOH = $state<DiskIOHistory | null>(null);
	let range = $state<'15m' | '1h' | '6h' | '24h'>('1h');
	let error = $state<string | null>(null);
	let procSort = $state<'cpu' | 'mem'>('cpu');
	let openAlerts = $state<AlertEvent[]>([]);
	let timer: ReturnType<typeof setInterval> | null = null;

	let hostStatus = $derived.by<'online' | 'stale' | 'offline'>(() => {
		if (!host) return 'offline';
		const age = (Date.now() - new Date(host.last_seen).getTime()) / 1000;
		if (age < 15) return 'online';
		if (age < 90) return 'stale';
		return 'offline';
	});

	async function load() {
		try {
			const [h, ms, lv, nh, mh, dh, alerts] = await Promise.all([
				api.host(id),
				api.metrics(id, range, 300),
				api.latest(id),
				api.netHistory(id, range, 300),
				api.diskMountHistory(id, range, 300),
				api.diskIOHistory(id, range, 300),
				api.alertEvents({ hostID: id, openOnly: true, limit: 100 }).catch(() => [] as AlertEvent[])
			]);
			host = h;
			samples = ms;
			latest = lv;
			netH = nh;
			mountH = mh;
			diskIOH = dh;
			openAlerts = alerts;
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

	// --- Aggregate chart derivations ---
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

	// --- Per-interface history chart data ---
	function deriveRates(timestamps: number[], bytes: number[]): number[] {
		return bytes.map((b, i) => {
			if (i === 0) return 0;
			const dt = timestamps[i] - timestamps[i - 1];
			if (dt <= 0) return 0;
			return Math.max(0, (b - bytes[i - 1]) / dt);
		});
	}

	let ifaceCharts = $derived(
		Object.entries(netH?.ifaces ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([name, series]) => ({
				name,
				x: netH!.timestamps,
				rxRates: deriveRates(netH!.timestamps, series.rx_bytes),
				txRates: deriveRates(netH!.timestamps, series.tx_bytes)
			}))
	);

	let mountCharts = $derived(
		Object.entries(mountH?.mounts ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([mount, series]) => ({
				mount,
				x: mountH!.timestamps,
				usedGiB: series.used.map((v) => v / 1073741824),
				totalGiB: series.total.map((v) => v / 1073741824)
			}))
	);

	// All devices in one chart with distinct colors per device pair
	const devColors = ['#5cc8ff', '#7ce38b', '#ffcb6b', '#c792ea', '#ff6b6b'];
	let diskIOSeries = $derived(() => {
		if (!diskIOH || !diskIOH.timestamps.length) return { x: [] as number[], series: [] as { label: string; values: number[]; stroke?: string }[] };
		const x = diskIOH.timestamps;
		const series: { label: string; values: number[]; stroke?: string }[] = [];
		Object.entries(diskIOH.devices)
			.sort(([a], [b]) => a.localeCompare(b))
			.forEach(([device, s], i) => {
				const col = devColors[i % devColors.length];
				series.push({ label: `${device} read`, values: deriveRates(x, s.read_bytes), stroke: col });
				series.push({ label: `${device} write`, values: deriveRates(x, s.write_bytes), stroke: col + '99' });
			});
		return { x, series };
	});

	// --- Process list ---
	let sortedProcs = $derived(
		(latest?.processes ?? []).slice().sort((a, b) =>
			procSort === 'cpu' ? b.cpu_pct - a.cpu_pct : b.mem_rss - a.mem_rss
		)
	);

	// --- Memory breakdown ---
	let memBreakdown = $derived(() => {
		if (!latest) return null;
		const total = latest.mem_total;
		if (!total) return null;
		const used = latest.mem_used;
		const cached = latest.mem_cached ?? 0;
		const free = Math.max(0, total - used - cached);
		return {
			usedPct: (used / total) * 100,
			cachedPct: (cached / total) * 100,
			freePct: (free / total) * 100,
			used, cached, free, total
		};
	});

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
	function rateStr(bps: number): string {
		if (bps <= 0) return '0 B/s';
		return fmtBytesRate(bps);
	}
	function fmtRSS(bytes: number): string {
		const gib = bytes / 1073741824;
		if (gib >= 1) return `${gib.toFixed(2)} GiB`;
		const mib = bytes / 1048576;
		if (mib >= 1) return `${mib.toFixed(1)} MiB`;
		return `${(bytes / 1024).toFixed(0)} KiB`;
	}
</script>

<svelte:head><title>Aperture — {host?.name ?? id}</title></svelte:head>

<div class="page-header">
	<div>
		<a href="/" class="back">← all hosts</a>
		<h1>
			{host?.name ?? id}
			{#if hostStatus !== 'online'}
				<span class="status-pill status-{hostStatus}">{hostStatus}</span>
			{/if}
		</h1>
		{#if host}
			<div class="muted mono">
				{host.platform || host.os} · {host.arch} · {host.cpu_count} vCPU · {formatBytes(host.mem_total)} RAM
				{#if host.source === 'agent'}
					· <span class="source-tag">agent{host.agent_version ? ` v${host.agent_version}` : ''}</span>
				{/if}
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
	<a href={`/hosts/${id}/compose`}>Compose</a>
	<a href={`/hosts/${id}/networks`} class="">Networks</a>
	<a href={`/hosts/${id}/volumes`} class="">Volumes</a>
	<a href={`/hosts/${id}/images`} class="placeholder">Images</a>
	<a href={`/hosts/${id}/logs`} class="placeholder">Logs</a>
</nav>

{#if openAlerts.length > 0}
	<div class="alert-banner">
		<span class="alert-icon">⚠</span>
		<span>
			{openAlerts.length} alert{openAlerts.length === 1 ? '' : 's'} currently firing —
			{openAlerts.map(a => a.metric).join(', ')}
		</span>
		<a href="/alerts" class="alert-link">View alerts →</a>
	</div>
{/if}

{#if hostStatus === 'offline'}
	<div class="stale-banner stale-offline">
		Host appears offline — last seen <span title={absTime(host?.last_seen ?? '')}>{relTime(host?.last_seen ?? '')}</span>. Data may be outdated.
	</div>
{:else if hostStatus === 'stale'}
	<div class="stale-banner stale-warn">
		Data may be stale — last seen <span title={absTime(host?.last_seen ?? '')}>{relTime(host?.last_seen ?? '')}</span>.
	</div>
{/if}

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
			{#if memBreakdown()}
				{@const mb = memBreakdown()!}
				<div class="big">{formatBytes(mb.used)}</div>
				<div class="mem-seg-bar">
					<div class="seg seg-used" style="width:{mb.usedPct}%"></div>
					<div class="seg seg-cached" style="width:{mb.cachedPct}%"></div>
					<div class="seg seg-free" style="width:{mb.freePct}%"></div>
				</div>
				<div class="muted mono small">{formatBytes(mb.used)} / {formatBytes(mb.total)} · {formatPct(latest.mem_percent)}</div>
				{#if mb.cached > 0}
					<div class="muted mono small">
						<span class="dot-cached">●</span> cached {formatBytes(mb.cached)}
						<span class="dot-free" style="margin-left:6px">●</span> free {formatBytes(mb.free)}
					</div>
				{/if}
			{:else}
				<div class="big">{formatBytes(latest.mem_used)}</div>
				<Bar value={latest.mem_percent} />
				<div class="muted mono small">{formatBytes(latest.mem_used)} / {formatBytes(latest.mem_total)} · {formatPct(latest.mem_percent)}</div>
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
				<div class="muted mono small" title={absTime(host.last_seen)}>seen {relTime(host.last_seen)}</div>
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
			<div class="section-title">Network interfaces — live</div>
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
			<div class="section-title">Disk mounts — live</div>
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
			<div class="section-title">Disk I/O — live</div>
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

	<!-- ── Processes ─────────────────────────────────────────────── -->
	{#if latest.processes && latest.processes.length > 0}
		<div class="card section">
			<div class="section-title-row">
				<span class="section-title">Processes — live</span>
				<div class="sort-pills">
					<button class:active={procSort === 'cpu'} onclick={() => (procSort = 'cpu')}>CPU</button>
					<button class:active={procSort === 'mem'} onclick={() => (procSort = 'mem')}>Memory</button>
				</div>
			</div>
			<table>
				<thead>
					<tr>
						<th>Name</th>
						<th>PID</th>
						<th>CPU%</th>
						<th>Mem%</th>
						<th>RSS</th>
					</tr>
				</thead>
				<tbody>
					{#each sortedProcs as p}
						<tr>
							<td class="mono">{p.name}</td>
							<td class="mono muted small">{p.pid}</td>
							<td class="mono {p.cpu_pct >= 50 ? 'bad' : p.cpu_pct >= 20 ? 'warn' : 'accent'}">{p.cpu_pct.toFixed(1)}%</td>
							<td class="mono muted">{p.mem_pct.toFixed(1)}%</td>
							<td class="mono muted">{fmtRSS(p.mem_rss)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/if}

<!-- ── Historical charts ──────────────────────────────────────────── -->
{#if samples.length > 1}
	<div class="charts-header muted small">Historical — {range} window</div>
	<div class="chart-grid">
		<div class="card chart-card">
			<div class="chart-title">CPU usage</div>
			<Chart x={xs} series={[{ label: 'CPU %', values: samples.map((s) => s.cpu_percent) }]}
				valueSuffix="%" yMin={0} yMax={100} />
		</div>
		<div class="card chart-card">
			<div class="chart-title">Memory usage</div>
			<Chart x={xs} series={[
				{ label: 'Used', values: samples.map((s) => s.mem_used), stroke: '#7ce38b' },
				{ label: 'Total', values: samples.map((s) => s.mem_total), stroke: '#3a4258', fill: false }
			]} valueFormatter={fmtGiB} />
		</div>
		<div class="card chart-card">
			<div class="chart-title">Disk usage (/)</div>
			<Chart x={xs} series={[
				{ label: 'Used', values: samples.map((s) => s.disk_used), stroke: '#ffcb6b' },
				{ label: 'Total', values: samples.map((s) => s.disk_total), stroke: '#3a4258', fill: false }
			]} valueFormatter={fmtGiB} />
		</div>
		<div class="card chart-card">
			<div class="chart-title">Network throughput (aggregate)</div>
			<Chart x={xs} series={[
				{ label: 'Rx', values: netRx, stroke: '#5cc8ff' },
				{ label: 'Tx', values: netTx, stroke: '#c792ea' }
			]} valueFormatter={fmtBytesRate} />
		</div>
		<div class="card chart-card span-full">
			<div class="chart-title">Load average</div>
			<Chart x={xs} series={[
				{ label: '1m',  values: samples.map((s) => s.load_avg_1), stroke: '#5cc8ff' },
				{ label: '5m',  values: samples.map((s) => s.load_avg_5), stroke: '#7ce38b' },
				{ label: '15m', values: samples.map((s) => s.load_avg_15), stroke: '#ffcb6b' }
			]} />
		</div>
	</div>

	<!-- ── Per-interface network history ─────────────────────────── -->
	{#if ifaceCharts.length > 0}
		<div class="charts-header muted small">Network — per interface</div>
		<div class="chart-grid">
			{#each ifaceCharts as ifc}
				{#if ifc.x.length > 1}
					<div class="card chart-card">
						<div class="chart-title">{ifc.name}</div>
						<Chart x={ifc.x} series={[
							{ label: 'RX', values: ifc.rxRates, stroke: '#5cc8ff' },
							{ label: 'TX', values: ifc.txRates, stroke: '#c792ea' }
						]} valueFormatter={fmtBytesRate} />
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	<!-- ── Per-mount disk history ─────────────────────────────────── -->
	{#if mountCharts.length > 0}
		<div class="charts-header muted small">Disk — per mount</div>
		<div class="chart-grid">
			{#each mountCharts as mc}
				{#if mc.x.length > 1}
					<div class="card chart-card">
						<div class="chart-title">{mc.mount}</div>
						<Chart x={mc.x} series={[
							{ label: 'Used',  values: mc.usedGiB,  stroke: '#ffcb6b' },
							{ label: 'Total', values: mc.totalGiB, stroke: '#3a4258', fill: false }
						]} valueFormatter={fmtGiB} />
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	<!-- ── Disk I/O history ───────────────────────────────────────── -->
	{@const diskIO = diskIOSeries()}
	{#if diskIO.x.length > 1 && diskIO.series.length > 0}
		<div class="charts-header muted small">Disk I/O — per device</div>
		<div class="card chart-card">
			<div class="chart-title">Read / Write rates</div>
			<Chart x={diskIO.x} series={diskIO.series} valueFormatter={fmtBytesRate} />
		</div>
	{/if}
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
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	.source-tag { color: var(--accent); font-size: 11px; }
	.small { font-size: 11px; }
	.range-picker { display: flex; gap: 4px; align-self: flex-start; margin-top: 4px; }
	.range-picker button.active {
		background: var(--bg-elev);
		border-color: var(--accent);
		color: var(--accent);
	}

	.status-pill {
		display: inline-block;
		padding: 2px 9px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 500;
	}
	.status-stale   { background: rgba(255,203,107,0.12); color: var(--warn); border: 1px solid rgba(255,203,107,0.35); }
	.status-offline { background: rgba(255,107,107,0.12); color: var(--bad);  border: 1px solid rgba(255,107,107,0.35); }

	.alert-banner {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 16px;
		margin-bottom: 14px;
		background: rgba(255,107,107,0.08);
		border: 1px solid rgba(255,107,107,0.35);
		border-radius: 7px;
		font-size: 13px;
		color: var(--bad);
	}
	.alert-icon { font-size: 14px; flex-shrink: 0; }
	.alert-link { margin-left: auto; font-size: 12px; color: var(--bad); opacity: 0.8; }
	.alert-link:hover { opacity: 1; }

	.stale-banner {
		padding: 8px 16px;
		margin-bottom: 14px;
		border-radius: 7px;
		font-size: 12px;
	}
	.stale-warn    { background: rgba(255,203,107,0.08); border: 1px solid rgba(255,203,107,0.3); color: var(--warn); }
	.stale-offline { background: rgba(255,107,107,0.08); border: 1px solid rgba(255,107,107,0.3); color: var(--bad); }

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

	/* Memory segmented bar */
	.mem-seg-bar {
		display: flex;
		height: 6px;
		border-radius: 3px;
		overflow: hidden;
		background: var(--bg-elev-2);
	}
	.seg { height: 100%; transition: width 0.4s ease; }
	.seg-used { background: var(--accent); }
	.seg-cached { background: #5cc8ff66; }
	.seg-free { background: transparent; }
	.dot-cached { color: #5cc8ff66; }
	.dot-free { color: var(--bg-elev-2); }

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
	.section-title-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 12px;
	}
	.section-title-row .section-title { margin-bottom: 0; }
	.sort-pills { display: flex; gap: 4px; }
	.sort-pills button { font-size: 11px; padding: 2px 8px; }
	.sort-pills button.active {
		background: var(--bg-elev);
		border-color: var(--accent);
		color: var(--accent);
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
	/* 2-column responsive grid for all chart sections */
	.chart-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
		gap: 12px;
		margin-top: 12px;
	}
	.chart-grid .chart-card { margin-top: 0; }
	/* Load avg and I/O charts span the full width when in a grid */
	.chart-grid .span-full { grid-column: 1 / -1; }
	.chart-card { margin-top: 12px; }
	.chart-title { font-size: 12px; color: var(--text-dim); margin-bottom: 8px; font-weight: 500; }
	.err { color: var(--bad); border-color: var(--bad); }
</style>
