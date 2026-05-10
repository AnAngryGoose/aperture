<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import type { Host, MetricSample, AlertEvent } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import { formatBytes, formatBytesRate, formatPct, formatDuration, relTime, absTime } from '$lib/format';

	let hosts      = $state<Host[]>([]);
	let latest     = $state<Record<string, MetricSample | null>>({});
	let prevLatest = $state<Record<string, MetricSample | null>>({});
	let ccounts    = $state<Record<string, { running: number; total: number }>>({});
	let openAlerts = $state<AlertEvent[]>([]);
	let error      = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	let alertsByHost = $derived(
		openAlerts.reduce<Record<string, number>>((acc, e) => {
			acc[e.host_id] = (acc[e.host_id] ?? 0) + 1;
			return acc;
		}, {})
	);

	function netRate(cur: MetricSample | null, prev: MetricSample | null): { rx: number; tx: number } {
		if (!cur || !prev) return { rx: 0, tx: 0 };
		const dt = (new Date(cur.timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
		if (dt <= 0) return { rx: 0, tx: 0 };
		return {
			rx: Math.max(0, (cur.net_rx_bytes - prev.net_rx_bytes) / dt),
			tx: Math.max(0, (cur.net_tx_bytes - prev.net_tx_bytes) / dt)
		};
	}

	function hostStatus(h: Host): 'online' | 'stale' | 'offline' {
		const age = (Date.now() - new Date(h.last_seen).getTime()) / 1000;
		if (age < 15) return 'online';
		if (age < 90) return 'stale';
		return 'offline';
	}

	async function refresh() {
		try {
			hosts = await api.hosts();
			const [results, alerts] = await Promise.all([
				Promise.all(
					hosts.map(async h => {
						const [m, cs] = await Promise.all([
							api.latest(h.id).catch(() => null),
							api.containers(h.id, true).catch(() => [])
						]);
						return [h.id, m, cs] as const;
					})
				),
				api.alertEvents({ openOnly: true, limit: 500 }).catch(() => [])
			]);
			const newLatest: typeof latest = {};
			const newCounts: typeof ccounts = {};
			for (const [id, m, cs] of results) {
				newLatest[id] = m;
				newCounts[id] = { running: cs.filter(c => c.state === 'running').length, total: cs.length };
			}
			prevLatest = latest;
			latest     = newLatest;
			ccounts    = newCounts;
			openAlerts = alerts;
			error      = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	onMount(() => { refresh(); timer = setInterval(refresh, 5000); });
	onDestroy(() => { if (timer) clearInterval(timer); });
</script>

<svelte:head><title>Aperture — Hosts</title></svelte:head>

<div class="page-header">
	<h1>Hosts</h1>
	<span class="muted">{hosts.length} host{hosts.length === 1 ? '' : 's'} · auto-refresh 5s</span>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

{#if hosts.length === 0 && !error}
	<div class="card muted" style="text-align:center;padding:32px">
		No hosts registered. The hub auto-registers its local host on startup.
	</div>
{/if}

<div class="grid cols-2">
	{#each hosts as h (h.id)}
		{@const m = latest[h.id]}
		{@const p = prevLatest[h.id]}
		{@const cc = ccounts[h.id] ?? { running: 0, total: 0 }}
		{@const status = hostStatus(h)}
		{@const alertCount = alertsByHost[h.id] ?? 0}
		{@const rate = netRate(m, p)}
		<a class="host-link" href={`/hosts/${h.id}`}>
			<div class="card host"
				class:host-stale={status === 'stale'}
				class:host-offline={status === 'offline'}
				class:host-alert={alertCount > 0 && status !== 'offline'}>
				<div class="head">
					<div class="head-left">
						<div class="name-row">
							<div class="name">{h.name}</div>
							{#if alertCount > 0}
								<span class="alert-badge" title="{alertCount} alert{alertCount === 1 ? '' : 's'} currently firing">
									⚠ {alertCount}
								</span>
							{/if}
						</div>
						<div class="muted mono small">{h.platform || h.os} · {h.arch} · {h.cpu_count} vCPU</div>
					</div>
					<span class="status-pill status-{status}">{status}</span>
				</div>

				{#if m}
					<div class="metric">
						<div class="metric-row">
							<span>CPU</span>
							<span class="mono">{formatPct(m.cpu_percent)}</span>
						</div>
						<Bar value={m.cpu_percent} />
					</div>
					<div class="metric">
						<div class="metric-row">
							<span>Memory</span>
							<span class="mono">{formatBytes(m.mem_used)} / {formatBytes(m.mem_total)} · {formatPct(m.mem_percent)}</span>
						</div>
						<Bar value={m.mem_percent} />
					</div>
					<div class="metric">
						<div class="metric-row">
							<span>Disk</span>
							<span class="mono">{formatBytes(m.disk_used)} / {formatBytes(m.disk_total)} · {formatPct(m.disk_percent)}</span>
						</div>
						<Bar value={m.disk_percent} />
					</div>
					<div class="footer-row muted">
						<span title="Load 1m / 5m / 15m">load {m.load_avg_1.toFixed(2)} {m.load_avg_5.toFixed(2)} {m.load_avg_15.toFixed(2)}</span>
						<span>up {formatDuration(m.uptime_secs)}</span>
						{#if rate.rx > 500 || rate.tx > 500}
							<span class="mono" title="Network throughput">↓{formatBytesRate(rate.rx)} ↑{formatBytesRate(rate.tx)}</span>
						{/if}
						<span>{cc.running}/{cc.total} containers</span>
						<span title={absTime(h.last_seen)}>seen {relTime(h.last_seen)}</span>
					</div>
				{:else}
					<div class="muted small">no samples yet…</div>
				{/if}
			</div>
		</a>
	{/each}
</div>

<style>
	.page-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 16px;
	}
	h1 { margin: 0; font-size: 20px; font-weight: 600; }
	.host-link { display: block; color: inherit; }
	.host-link:hover { text-decoration: none; }
	.host-link:hover .host { border-color: var(--accent); }
	.host { transition: border-color 0.2s ease; }
	.host-stale   { border-color: rgba(255, 203, 107, 0.35); }
	.host-offline { border-color: rgba(255, 107, 107, 0.3); opacity: 0.7; }
	.host-alert   { border-color: rgba(255, 107, 107, 0.45); }

	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 12px;
		gap: 8px;
	}
	.head-left { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
	.name-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.name { font-weight: 600; font-size: 15px; }
	.small { font-size: 11px; }

	.alert-badge {
		background: rgba(255, 107, 107, 0.15);
		color: var(--bad);
		border: 1px solid rgba(255, 107, 107, 0.4);
		border-radius: 4px;
		padding: 1px 6px;
		font-size: 11px;
		font-weight: 600;
		white-space: nowrap;
	}

	.status-pill {
		display: inline-block;
		padding: 2px 9px;
		border-radius: 999px;
		font-size: 11px;
		font-weight: 500;
		flex-shrink: 0;
	}
	.status-online  { background: rgba(124,227,139,0.12); color: var(--ok);   border: 1px solid rgba(124,227,139,0.35); }
	.status-stale   { background: rgba(255,203,107,0.12); color: var(--warn); border: 1px solid rgba(255,203,107,0.35); }
	.status-offline { background: rgba(255,107,107,0.12); color: var(--bad);  border: 1px solid rgba(255,107,107,0.35); }

	.metric { margin-bottom: 10px; }
	.metric-row {
		display: flex;
		justify-content: space-between;
		font-size: 12px;
		margin-bottom: 4px;
	}
	.footer-row {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
		margin-top: 12px;
		font-size: 11px;
	}
	.err { color: var(--bad); border-color: var(--bad); }
</style>
