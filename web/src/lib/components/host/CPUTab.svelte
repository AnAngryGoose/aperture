<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import Chart from '$lib/Chart.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const cpuCoresHistory = $derived(bundle.history.cpuCores);
	const aggregate = $derived(bundle.history.metrics ?? []);

	// Aggregate CPU% chart series.
	const aggX = $derived(aggregate.map((s) => Math.floor(new Date(s.timestamp).getTime() / 1000)));
	const aggSeries = $derived([
		{ label: 'CPU %', values: aggregate.map((s) => s.cpu_percent), stroke: 'var(--accent)' }
	]);
	const loadSeries = $derived([
		{ label: '1m',  values: aggregate.map((s) => s.load_avg_1),  stroke: 'var(--accent)' },
		{ label: '5m',  values: aggregate.map((s) => s.load_avg_5),  stroke: 'var(--warn)' },
		{ label: '15m', values: aggregate.map((s) => s.load_avg_15), stroke: 'var(--text-faint)' }
	]);

	// Per-core: one series per core.
	const corePalette = [
		'#5cc8ff','#7ce38b','#ffcb6b','#c792ea','#ff6b6b','#f78c6c','#89ddff','#82aaff',
		'#94e2d5','#fab387','#a6e3a1','#cba6f7','#f38ba8','#74c7ec','#fcd34d','#d4d4aa'
	];
	const coreSeries = $derived.by(() => {
		if (!cpuCoresHistory) return { x: [] as number[], series: [] as Array<{label: string; values: number[]; stroke: string}> };
		const coreEntries = Object.entries(cpuCoresHistory.cores)
			.map(([k, v]) => ({ core: Number(k), values: v }))
			.sort((a, b) => a.core - b.core);
		return {
			x: cpuCoresHistory.timestamps,
			series: coreEntries.map(({ core, values }, i) => ({
				label: `core ${core}`,
				values,
				stroke: corePalette[i % corePalette.length]
			}))
		};
	});

	// Live per-core grid.
	const liveCores = $derived(latest?.cpu_per_core ?? []);
	function coreColor(pct: number): string {
		if (pct >= 85) return 'var(--crit)';
		if (pct >= 70) return 'var(--warn)';
		return 'var(--accent)';
	}
</script>

<div class="tab">
	<!-- Aggregate CPU + load -->
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Aggregate CPU usage</h3>
			<div class="live">
				<span class="live-num mono">{(latest?.cpu_percent ?? 0).toFixed(1)}%</span>
				<span class="live-sub mono">load {(latest?.load_avg_1 ?? 0).toFixed(2)} / {(latest?.load_avg_5 ?? 0).toFixed(2)} / {(latest?.load_avg_15 ?? 0).toFixed(2)}</span>
			</div>
		</header>
		<Chart x={aggX} series={aggSeries} height={160} valueSuffix="%" yMin={0} yMax={100} />
	</section>

	{#if liveCores.length > 0}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Per-core (live)</h3>
				<span class="card-sub mono">{liveCores.length} cores</span>
			</header>
			<div class="core-grid">
				{#each liveCores as pct, i}
					<div class="core-cell">
						<div class="core-bar" style="background: var(--bg-hover)">
							<div class="core-fill" style="width:{Math.min(100, pct)}%; background:{coreColor(pct)}"></div>
						</div>
						<div class="core-meta mono">
							<span class="core-id">{i}</span>
							<span class="core-pct" style="color:{coreColor(pct)}">{pct.toFixed(0)}%</span>
						</div>
					</div>
				{/each}
			</div>
		</section>
	{/if}

	{#if coreSeries.x.length > 0}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Per-core history</h3>
			</header>
			<Chart x={coreSeries.x} series={coreSeries.series} height={220} valueSuffix="%" yMin={0} yMax={100} />
		</section>
	{/if}

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Load average</h3>
		</header>
		<Chart x={aggX} series={loadSeries} height={160} />
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
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 10px;
	}

	.card-title {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}

	.card-sub {
		font-size: 11px;
		color: var(--text-faint);
	}

	.live { display: inline-flex; align-items: baseline; gap: 10px; }
	.live-num { font-size: 18px; font-weight: 500; color: var(--text); letter-spacing: -0.02em; }
	.live-sub { font-size: 11px; color: var(--text-faint); }

	.core-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(90px, 1fr));
		gap: 8px;
	}

	.core-cell {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 10px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}

	.core-bar {
		height: 4px;
		border-radius: var(--r-pill);
		overflow: hidden;
	}

	.core-fill {
		height: 100%;
		transition: width 200ms ease-out, background 200ms;
	}

	.core-meta {
		display: flex;
		justify-content: space-between;
		font-size: 11px;
	}

	.core-id { color: var(--text-faint); }
	.core-pct { color: var(--text); font-weight: 500; }
</style>
