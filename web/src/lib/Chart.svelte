<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import uPlot from 'uplot';
	import type { Options } from 'uplot';

	type Series = { label: string; values: number[]; stroke?: string; fill?: string };

	let {
		x,
		series,
		height = 200,
		title,
		valueSuffix = '',
		valueFormatter = null,
		yMin = undefined,
		yMax = undefined
	}: {
		x: number[];
		series: Series[];
		height?: number;
		title?: string;
		valueSuffix?: string;
		valueFormatter?: ((v: number) => string) | null;
		yMin?: number;
		yMax?: number;
	} = $props();

	let canvasHost: HTMLDivElement;
	let plot: uPlot | null = null;

	const colors = ['#5cc8ff', '#7ce38b', '#ffcb6b', '#ff6b6b', '#c792ea'];

	// Resolved per-series colors so the chip legend matches the rendered lines.
	let strokes = $derived(series.map((s, i) => s.stroke ?? colors[i % colors.length]));

	function formatTick(v: number): string {
		if (valueFormatter) return valueFormatter(v);
		return `${Math.round(v)}${valueSuffix}`;
	}

	function buildOptions(width: number): Options {
		return {
			title,
			width,
			height,
			// uPlot's built-in legend was overflowing into adjacent chart cards
			// (single-series charts wasted a row, multi-series stacked vertically
			// and bled into the next chart's header). Disable it and render a
			// compact chip-row above the canvas in the wrapper instead — visible
			// labels stay tied to colors, hover still works through the cursor.
			legend: { show: false },
			scales: {
				x: { time: true },
				y: yMin !== undefined || yMax !== undefined
					? { range: [yMin ?? null, yMax ?? null] as [number | null, number | null] }
					: {}
			},
			axes: [
				{ stroke: '#8b93a7', grid: { stroke: '#232a3d' } },
				{
					stroke: '#8b93a7',
					grid: { stroke: '#232a3d' },
					values: (_self: uPlot, ticks: number[]) => ticks.map(formatTick)
				}
			],
			series: [
				{ label: 'time' },
				...series.map((s, i) => ({
					label: s.label,
					stroke: s.stroke ?? colors[i % colors.length],
					fill: s.fill,
					width: 1.5,
					points: { show: false }
				}))
			],
			cursor: { drag: { x: true, y: false }, points: { size: 6 } }
		};
	}

	function data(): uPlot.AlignedData {
		return [x, ...series.map((s) => s.values)] as uPlot.AlignedData;
	}

	onMount(() => {
		const opts = buildOptions(canvasHost.clientWidth);
		plot = new uPlot(opts, data(), canvasHost);
		const ro = new ResizeObserver(() => {
			if (plot && canvasHost) plot.setSize({ width: canvasHost.clientWidth, height });
		});
		ro.observe(canvasHost);
		return () => ro.disconnect();
	});

	$effect(() => {
		if (plot) plot.setData(data());
	});

	onDestroy(() => {
		plot?.destroy();
		plot = null;
	});
</script>

<div class="chart-wrap">
	{#if series.length > 1}
		<div class="legend">
			{#each series as s, i}
				<span class="chip">
					<span class="dot" style="background: {strokes[i]}"></span>
					{s.label}
				</span>
			{/each}
		</div>
	{/if}
	<div bind:this={canvasHost} class="canvas-host" style="height: {height}px"></div>
</div>

<style>
	.chart-wrap { width: 100%; }
	.canvas-host { width: 100%; }
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin-bottom: 6px;
		font-size: 11px;
		color: var(--text-dim);
	}
	.chip { display: inline-flex; align-items: center; gap: 4px; }
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 2px;
	}
</style>
