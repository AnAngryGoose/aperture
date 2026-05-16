<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import Chart from '$lib/Chart.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const sensors = $derived(latest?.temps ?? []);
	const tempHistory = $derived(bundle.history.temps);
	const config = $derived(bundle.config);

	function tempColor(c: number): string {
		if (c >= config.crit_temp) return 'var(--crit)';
		if (c >= config.warn_temp) return 'var(--warn)';
		return 'var(--info)';
	}

	// All sensors on one multi-series chart.
	const palette = [
		'#5cc8ff','#7ce38b','#ffcb6b','#c792ea','#ff6b6b','#f78c6c','#89ddff','#82aaff',
		'#94e2d5','#fab387','#a6e3a1','#cba6f7','#f38ba8','#74c7ec','#fcd34d'
	];
	const tempChart = $derived.by(() => {
		if (!tempHistory || !tempHistory.timestamps.length) {
			return { x: [] as number[], series: [] as Array<{label: string; values: number[]; stroke: string}> };
		}
		const series = Object.entries(tempHistory.sensors ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([name, values], i) => ({
				label: name,
				values,
				stroke: palette[i % palette.length]
			}));
		return { x: tempHistory.timestamps, series };
	});
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Sensors (live)</h3>
			<span class="card-sub mono">{sensors.length} reading{sensors.length === 1 ? '' : 's'}</span>
		</header>
		{#if sensors.length === 0}
			<div class="empty">No temperature sensors reported. Many VMs and containers don't expose readable sensors.</div>
		{:else}
			<div class="sensor-grid">
				{#each sensors as s}
					<div class="sensor-cell">
						<div class="sensor-temp mono" style="color:{tempColor(s.temp_celsius)}">
							{s.temp_celsius.toFixed(1)}°C
						</div>
						<div class="sensor-name mono">{s.name}</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	{#if tempChart.x.length > 0}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Temperature history</h3>
				<span class="card-sub mono">warn ≥ {config.warn_temp}°C · crit ≥ {config.crit_temp}°C</span>
			</header>
			<Chart x={tempChart.x} series={tempChart.series} height={220} valueSuffix="°C" />
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
	.card-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text); }
	.card-sub { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }
	.empty { font-size: 12px; color: var(--text-faint); padding: 12px 0; }

	.sensor-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: 10px;
	}

	.sensor-cell {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 10px 12px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}

	.sensor-temp {
		font-size: 17px;
		font-weight: 500;
		letter-spacing: -0.02em;
	}

	.sensor-name {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
	}
</style>
