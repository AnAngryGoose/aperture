<script lang="ts">
	import type { MonitoringBundle, Container } from '$lib/types';
	import { fmtBytes } from '$lib/format';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	let containers = $state<Container[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function load() {
		const kind = (bundle.host.kind as 'docker' | 'linux' | 'edge') || 'linux';
		if (kind !== 'docker') return;
		loading = true;
		try {
			containers = await api.containers(bundle.host.id, true);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load containers';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function fmtRSS(b: number): string {
		if (b < 1048576) return `${(b / 1024).toFixed(0)} KiB`;
		if (b < 1073741824) return `${(b / 1048576).toFixed(1)} MiB`;
		return `${(b / 1073741824).toFixed(2)} GiB`;
	}

	const running   = $derived(containers.filter((c) => c.state === 'running').length);
	const stopped   = $derived(containers.filter((c) => c.state !== 'running').length);
	const unhealthy = $derived(containers.filter((c) => /unhealthy/i.test(c.status ?? '')).length);
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Containers</h3>
			<a class="full-link" href="/hosts/{bundle.host.id}/containers">
				Full container management
				<Icon name="external" size={12} />
			</a>
		</header>

		<div class="stats">
			<div class="stat">
				<span class="stat-num mono" style="color:var(--ok)">{running}</span>
				<span class="stat-label">running</span>
			</div>
			<div class="stat">
				<span class="stat-num mono" style="color:var(--text-faint)">{stopped}</span>
				<span class="stat-label">stopped</span>
			</div>
			<div class="stat">
				<span class="stat-num mono" style="color:{unhealthy > 0 ? 'var(--crit)' : 'var(--text-faint)'}">{unhealthy}</span>
				<span class="stat-label">unhealthy</span>
			</div>
		</div>

		{#if loading}
			<div class="empty">Loading containers…</div>
		{:else if error}
			<div class="empty err">{error}</div>
		{:else if containers.length === 0}
			<div class="empty">No containers on this host.</div>
		{:else}
			<table>
				<thead>
					<tr>
						<th>Name</th>
						<th>Image</th>
						<th>State</th>
						<th class="num">CPU %</th>
						<th class="num">Mem</th>
						<th class="num">Net RX/TX</th>
					</tr>
				</thead>
				<tbody>
					{#each containers as c}
						<tr>
							<td class="mono">{c.name}</td>
							<td class="mono dim">{c.image}</td>
							<td>
								<span class="pill {c.state}">{c.state}</span>
							</td>
							<td class="num mono">{(c.cpu_percent ?? 0).toFixed(1)}</td>
							<td class="num mono">{fmtRSS(c.mem_usage ?? 0)}</td>
							<td class="num mono dim">↓{fmtBytes(c.net_rx_bytes ?? 0)} ↑{fmtBytes(c.net_tx_bytes ?? 0)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>
</div>

<style>
	.tab { display: flex; flex-direction: column; gap: 14px; }
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 14px 16px;
	}
	.card-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 12px;
	}
	.card-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text); }

	.full-link {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: 11px;
		color: var(--accent);
		text-decoration: none;
		font-family: var(--font-mono);
	}
	.full-link:hover { text-decoration: underline; }

	.stats {
		display: flex;
		gap: 24px;
		padding: 8px 0 14px;
		border-bottom: 1px solid var(--line);
		margin-bottom: 10px;
	}

	.stat { display: flex; flex-direction: column; gap: 2px; }
	.stat-num { font-size: 22px; font-weight: 500; letter-spacing: -0.02em; }
	.stat-label { font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint); font-family: var(--font-mono); }

	.empty { font-size: 12px; color: var(--text-faint); padding: 12px 0; }
	.empty.err { color: var(--crit); }

	table { width: 100%; border-collapse: collapse; font-size: 12px; }
	th {
		text-align: left;
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-family: var(--font-mono);
		color: var(--text-faint);
		font-weight: 400;
		padding: 6px 10px 8px;
		border-bottom: 1px solid var(--line);
	}
	th.num, td.num { text-align: right; }
	td {
		padding: 8px 10px;
		border-bottom: 1px solid var(--line);
		color: var(--text);
	}
	tr:last-child td { border-bottom: none; }
	td.mono { font-family: var(--font-mono); }
	td.dim { color: var(--text-dim); }

	.pill {
		display: inline-block;
		padding: 1px 8px;
		font-size: 11px;
		font-family: var(--font-mono);
		border-radius: var(--r-pill);
		background: var(--bg-elev-2);
		color: var(--text-dim);
	}
	.pill.running { background: color-mix(in srgb, var(--ok) 14%, transparent); color: var(--ok); }
</style>
