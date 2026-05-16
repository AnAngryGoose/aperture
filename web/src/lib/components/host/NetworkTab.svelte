<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import { fmtBytes } from '$lib/format';
	import Chart from '$lib/Chart.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const ifaces = $derived(latest?.net_interfaces ?? []);
	const netHistory = $derived(bundle.history.net);
	const aggregate = $derived(bundle.history.metrics ?? []);

	function fmtBytesRate(bps: number): string {
		if (bps < 1024) return `${bps.toFixed(0)} B/s`;
		if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
		return `${(bps / 1048576).toFixed(1)} MiB/s`;
	}

	function deriveRates(timestamps: number[], bytes: number[]): number[] {
		return bytes.map((b, i) => {
			if (i === 0) return 0;
			const dt = timestamps[i] - timestamps[i - 1];
			if (dt <= 0) return 0;
			return Math.max(0, (b - bytes[i - 1]) / dt);
		});
	}

	// Aggregate rates from the metric history's cumulative net counters.
	const aggX = $derived(aggregate.map((s) => Math.floor(new Date(s.timestamp).getTime() / 1000)));
	const aggNetSeries = $derived.by(() => {
		const rx: number[] = [];
		const tx: number[] = [];
		for (let i = 0; i < aggregate.length; i++) {
			if (i === 0) { rx.push(0); tx.push(0); continue; }
			const prev = aggregate[i - 1];
			const dt = (new Date(aggregate[i].timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) { rx.push(0); tx.push(0); continue; }
			rx.push(Math.max(0, (aggregate[i].net_rx_bytes - prev.net_rx_bytes) / dt));
			tx.push(Math.max(0, (aggregate[i].net_tx_bytes - prev.net_tx_bytes) / dt));
		}
		return [
			{ label: 'RX', values: rx, stroke: 'var(--info)' },
			{ label: 'TX', values: tx, stroke: 'var(--accent)' }
		];
	});

	// Per-interface charts.
	const ifaceCharts = $derived.by(() => {
		if (!netHistory) return [];
		return Object.entries(netHistory.ifaces ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([name, series]) => ({
				name,
				x: netHistory.timestamps,
				series: [
					{ label: 'RX', values: deriveRates(netHistory.timestamps, series.rx_bytes), stroke: 'var(--info)' },
					{ label: 'TX', values: deriveRates(netHistory.timestamps, series.tx_bytes), stroke: 'var(--accent)' }
				]
			}));
	});
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Network (live)</h3>
			<span class="card-sub mono">{ifaces.length} interfaces</span>
		</header>
		{#if ifaces.length === 0}
			<div class="empty">No interfaces reported.</div>
		{:else}
			<table>
				<thead>
					<tr>
						<th>Interface</th>
						<th class="num">RX rate</th>
						<th class="num">TX rate</th>
						<th class="num">RX total</th>
						<th class="num">TX total</th>
					</tr>
				</thead>
				<tbody>
					{#each ifaces as i}
						<tr>
							<td class="mono">{i.name}</td>
							<td class="num mono">{fmtBytesRate(i.rx_rate)}</td>
							<td class="num mono">{fmtBytesRate(i.tx_rate)}</td>
							<td class="num mono dim">{fmtBytes(i.rx_bytes)}</td>
							<td class="num mono dim">{fmtBytes(i.tx_bytes)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Aggregate network rate</h3>
		</header>
		<Chart x={aggX} series={aggNetSeries} height={180} valueFormatter={fmtBytesRate} />
	</section>

	{#each ifaceCharts as ic}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title mono">{ic.name}</h3>
			</header>
			<Chart x={ic.x} series={ic.series} height={140} valueFormatter={fmtBytesRate} />
		</section>
	{/each}
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
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 12px;
	}
	.card-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text); }
	.card-sub { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }
	.empty { font-size: 12px; color: var(--text-faint); padding: 12px 0; }

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
</style>
