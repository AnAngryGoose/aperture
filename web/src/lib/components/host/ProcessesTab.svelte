<script lang="ts">
	import type { MonitoringBundle, ProcessHistory } from '$lib/types';
	import { fmtBytes } from '$lib/format';
	import { api } from '$lib/api';
	import Chart from '$lib/Chart.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		bundle: MonitoringBundle;
		range: string;
	}

	let { bundle, range }: Props = $props();

	const latest = $derived(bundle.latest);
	const procs = $derived(latest?.processes ?? []);

	let sortBy = $state<'cpu' | 'mem'>('cpu');
	let expandedName = $state<string | null>(null);
	let procHistory = $state<ProcessHistory | null>(null);
	let loadingHistory = $state(false);

	const sorted = $derived(
		[...procs].sort((a, b) =>
			sortBy === 'cpu' ? b.cpu_pct - a.cpu_pct : b.mem_rss - a.mem_rss
		)
	);

	function fmtRSS(b: number): string {
		const g = b / 1073741824;
		if (g >= 1) return `${g.toFixed(2)} GiB`;
		const m = b / 1048576;
		if (m >= 1) return `${m.toFixed(1)} MiB`;
		return `${(b / 1024).toFixed(0)} KiB`;
	}

	async function toggleExpand(name: string) {
		if (expandedName === name) {
			expandedName = null;
			procHistory = null;
			return;
		}
		expandedName = name;
		procHistory = null;
		loadingHistory = true;
		try {
			procHistory = await api.processHistory(bundle.host.id, name, range, 300);
		} catch {
			procHistory = null;
		} finally {
			loadingHistory = false;
		}
	}

	const expandedChart = $derived.by(() => {
		if (!procHistory || !procHistory.timestamps.length) {
			return { x: [] as number[], series: [] as Array<{label: string; values: number[]; stroke: string}> };
		}
		return {
			x: procHistory.timestamps,
			series: [
				{ label: 'CPU %', values: procHistory.cpu_pct, stroke: 'var(--accent)' }
			]
		};
	});
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Processes (live)</h3>
			<div class="sort">
				<button class="sort-btn" class:active={sortBy === 'cpu'} onclick={() => (sortBy = 'cpu')}>CPU</button>
				<button class="sort-btn" class:active={sortBy === 'mem'} onclick={() => (sortBy = 'mem')}>Memory</button>
			</div>
		</header>
		{#if sorted.length === 0}
			<div class="empty">No processes reported. The processes collector may be disabled in host config.</div>
		{:else}
			<table>
				<thead>
					<tr>
						<th></th>
						<th>Name</th>
						<th class="num">PID</th>
						<th class="num">CPU %</th>
						<th class="num">Mem %</th>
						<th class="num">RSS</th>
					</tr>
				</thead>
				<tbody>
					{#each sorted as p}
						<tr class="row" class:expanded={expandedName === p.name} onclick={() => toggleExpand(p.name)}>
							<td class="chev">
								<Icon name={expandedName === p.name ? 'chevron-down' : 'chevron-right'} size={12} />
							</td>
							<td class="mono">{p.name}</td>
							<td class="num mono dim">{p.pid}</td>
							<td class="num mono">{p.cpu_pct.toFixed(1)}</td>
							<td class="num mono">{p.mem_pct.toFixed(1)}</td>
							<td class="num mono">{fmtRSS(p.mem_rss)}</td>
						</tr>
						{#if expandedName === p.name}
							<tr class="expanded-row">
								<td colspan="6">
									{#if loadingHistory}
										<div class="loading">Loading {p.name} history…</div>
									{:else if expandedChart.x.length === 0}
										<div class="empty">No persisted history for {p.name} in this range. Process metrics retention may be short.</div>
									{:else}
										<Chart x={expandedChart.x} series={expandedChart.series} height={140} valueSuffix="%" yMin={0} yMax={100} />
									{/if}
								</td>
							</tr>
						{/if}
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
	.empty { font-size: 12px; color: var(--text-faint); padding: 12px 0; }

	.sort {
		display: inline-flex;
		gap: 0;
		padding: 3px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}
	.sort-btn {
		padding: 4px 10px;
		font-size: 11px;
		font-family: var(--font-mono);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-dim);
		background: none;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
	}
	.sort-btn:hover { color: var(--text); }
	.sort-btn.active { background: var(--bg-hover); color: var(--text); }

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

	.row { cursor: pointer; transition: background 120ms; }
	.row:hover { background: var(--bg-hover); }
	.row.expanded { background: var(--bg-hover); }

	.chev { width: 24px; padding-right: 0; color: var(--text-faint); }

	.expanded-row td {
		padding: 12px 14px;
		background: var(--bg-elev-2);
		border-bottom: 1px solid var(--line);
	}
	.loading { font-size: 12px; color: var(--text-faint); padding: 12px 0; }
</style>
