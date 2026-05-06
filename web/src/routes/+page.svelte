<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import type { Host, MetricSample } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import { formatBytes, formatPct, formatDuration, relTime } from '$lib/format';

	let hosts = $state<Host[]>([]);
	let latest = $state<Record<string, MetricSample | null>>({});
	let containerCounts = $state<Record<string, { running: number; total: number }>>({});
	let error = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		try {
			hosts = await api.hosts();
			const results = await Promise.all(
				hosts.map(async (h) => {
					const [m, cs] = await Promise.all([
						api.latest(h.id).catch(() => null),
						api.containers(h.id, true).catch(() => [])
					]);
					return [h.id, m, cs] as const;
				})
			);
			const newLatest: typeof latest = {};
			const newCounts: typeof containerCounts = {};
			for (const [id, m, cs] of results) {
				newLatest[id] = m;
				newCounts[id] = {
					running: cs.filter((c) => c.state === 'running').length,
					total: cs.length
				};
			}
			latest = newLatest;
			containerCounts = newCounts;
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<div class="page-header">
	<h1>Hosts</h1>
	<span class="muted">{hosts.length} host{hosts.length === 1 ? '' : 's'} · updates every 5s</span>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

{#if hosts.length === 0 && !error}
	<div class="card muted">No hosts registered yet. The hub auto-registers its local host on startup.</div>
{/if}

<div class="grid cols-2">
	{#each hosts as h (h.id)}
		{@const m = latest[h.id]}
		{@const cc = containerCounts[h.id] ?? { running: 0, total: 0 }}
		<a class="host-link" href={`/hosts/${h.id}`}>
			<div class="card host">
				<div class="head">
					<div>
						<div class="name">{h.name}</div>
						<div class="muted mono">{h.platform || h.os} · {h.arch} · {h.cpu_count} vCPU</div>
					</div>
					<span class="pill {m ? 'running' : 'exited'}">{h.source}</span>
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
						<span>load {m.load_avg_1.toFixed(2)} {m.load_avg_5.toFixed(2)} {m.load_avg_15.toFixed(2)}</span>
						<span>up {formatDuration(m.uptime_secs)}</span>
						<span>{cc.running}/{cc.total} containers</span>
						<span>seen {relTime(h.last_seen)}</span>
					</div>
				{:else}
					<div class="muted">no samples yet…</div>
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
	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 12px;
	}
	.name { font-weight: 600; font-size: 15px; }
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
		gap: 12px;
		margin-top: 12px;
		font-size: 12px;
	}
	.err { color: var(--bad); border-color: var(--bad); }
</style>
