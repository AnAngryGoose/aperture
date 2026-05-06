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
		yMin = undefined,
		yMax = undefined
	}: {
		x: number[];
		series: Series[];
		height?: number;
		title?: string;
		valueSuffix?: string;
		yMin?: number;
		yMax?: number;
	} = $props();

	let container: HTMLDivElement;
	let plot: uPlot | null = null;

	const colors = ['#5cc8ff', '#7ce38b', '#ffcb6b', '#ff6b6b', '#c792ea'];

	function buildOptions(width: number): Options {
		return {
			title,
			width,
			height,
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
					values: (_self: uPlot, ticks: number[]) =>
						ticks.map((t) => `${Math.round(t)}${valueSuffix}`)
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
		const opts = buildOptions(container.clientWidth);
		plot = new uPlot(opts, data(), container);
		const ro = new ResizeObserver(() => {
			if (plot && container) plot.setSize({ width: container.clientWidth, height });
		});
		ro.observe(container);
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

<div bind:this={container} class="chart" style="height: {height}px"></div>

<style>
	.chart { width: 100%; }
</style>
