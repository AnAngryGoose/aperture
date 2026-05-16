<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import { fmtBytes } from '$lib/format';
	import Chart from '$lib/Chart.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const aggregate = $derived(bundle.history.metrics ?? []);
	const aggX = $derived(aggregate.map((s) => Math.floor(new Date(s.timestamp).getTime() / 1000)));

	// Memory used vs total over time (GiB).
	const memSeries = $derived([
		{
			label: 'Used',
			values: aggregate.map((s) => s.mem_used / 1073741824),
			stroke: 'var(--accent)'
		},
		{
			label: 'Total',
			values: aggregate.map((s) => s.mem_total / 1073741824),
			stroke: 'var(--text-faint)',
			fill: false as const
		}
	]);

	const swapSeries = $derived([
		{
			label: 'Swap used',
			values: aggregate.map((s) => s.swap_used / 1073741824),
			stroke: 'var(--warn)'
		}
	]);

	// Live memory breakdown for the segmented bar.
	const breakdown = $derived.by(() => {
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

	const swap = $derived.by(() => {
		if (!latest || !latest.swap_total) return null;
		return {
			usedPct: (latest.swap_used / latest.swap_total) * 100,
			used: latest.swap_used,
			total: latest.swap_total
		};
	});

	function fmtGiB(bytes: number): string {
		const g = bytes / 1073741824;
		return g >= 10 ? `${g.toFixed(1)} GiB` : `${g.toFixed(2)} GiB`;
	}
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Memory</h3>
			<div class="live">
				<span class="live-num mono">{fmtBytes(latest?.mem_used ?? 0)}</span>
				<span class="live-sub mono">of {fmtBytes(bundle.host.mem_total)} ({(latest?.mem_percent ?? 0).toFixed(1)}%)</span>
			</div>
		</header>

		{#if breakdown}
			<div class="bar">
				<div class="bar-seg used"    style="width:{breakdown.usedPct}%"   title="used: {fmtBytes(breakdown.used)}"></div>
				<div class="bar-seg cached"  style="width:{breakdown.cachedPct}%" title="cached: {fmtBytes(breakdown.cached)}"></div>
				<div class="bar-seg free"    style="width:{breakdown.freePct}%"   title="free: {fmtBytes(breakdown.free)}"></div>
			</div>
			<div class="bar-legend">
				<span class="leg-item"><i class="dot used"></i>used {fmtBytes(breakdown.used)}</span>
				<span class="leg-item"><i class="dot cached"></i>cached {fmtBytes(breakdown.cached)}</span>
				<span class="leg-item"><i class="dot free"></i>free {fmtBytes(breakdown.free)}</span>
			</div>
		{/if}
	</section>

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Memory over time</h3>
		</header>
		<Chart x={aggX} series={memSeries} height={180} valueFormatter={fmtGiB} />
	</section>

	{#if swap}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Swap</h3>
				<div class="live">
					<span class="live-num mono">{fmtBytes(swap.used)}</span>
					<span class="live-sub mono">of {fmtBytes(swap.total)} ({swap.usedPct.toFixed(1)}%)</span>
				</div>
			</header>
			<Chart x={aggX} series={swapSeries} height={140} valueFormatter={fmtGiB} />
		</section>
	{/if}
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

	.card-title {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}

	.live { display: inline-flex; align-items: baseline; gap: 10px; }
	.live-num { font-size: 18px; font-weight: 500; color: var(--text); font-family: var(--font-mono); letter-spacing: -0.02em; }
	.live-sub { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }

	.bar {
		display: flex;
		height: 10px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-pill);
		overflow: hidden;
	}

	.bar-seg { height: 100%; }
	.bar-seg.used   { background: var(--accent); }
	.bar-seg.cached { background: var(--text-faint); opacity: 0.6; }
	.bar-seg.free   { background: var(--bg-hover); }

	.bar-legend {
		display: flex;
		gap: 18px;
		margin-top: 10px;
		font-size: 11px;
		color: var(--text-dim);
		font-family: var(--font-mono);
	}

	.leg-item { display: inline-flex; align-items: center; gap: 6px; }
	.dot { width: 8px; height: 8px; border-radius: var(--r-sm); flex-shrink: 0; }
	.dot.used   { background: var(--accent); }
	.dot.cached { background: var(--text-faint); opacity: 0.6; }
	.dot.free   { background: var(--bg-hover); border: 1px solid var(--line); }
</style>
