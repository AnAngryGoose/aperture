<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import uPlot from 'uplot';

	type Series = { label: string; values: number[]; stroke?: string; fill?: string | false };

	let {
		x,
		series,
		height = 200,
		valueSuffix = '',
		valueFormatter = null,
		yMin = undefined,
		yMax = undefined
	}: {
		x: number[];
		series: Series[];
		height?: number;
		valueSuffix?: string;
		valueFormatter?: ((v: number) => string) | null;
		yMin?: number;
		yMax?: number;
	} = $props();

	const palette = ['#5cc8ff', '#7ce38b', '#ffcb6b', '#c792ea', '#ff6b6b', '#f78c6c', '#89ddff', '#82aaff'];
	let strokes = $derived(series.map((s, i) => s.stroke ?? palette[i % palette.length]));

	let wrapEl: HTMLDivElement;
	let canvasEl: HTMLDivElement;
	let plot: uPlot | null = null;

	// uPlot writes directly to canvas, so it can't read CSS variables. We
	// resolve the design tokens once at construction time. On theme switch the
	// chart is currently not rebuilt; the colors will look stale until the
	// next mount. Acceptable today because the host detail page rebuilds
	// charts on range changes, which the user does frequently.
	function resolveToken(name: string, fallback: string): string {
		if (typeof window === 'undefined') return fallback;
		const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
		return v || fallback;
	}

	// Tooltip reactive state — updated from inside uPlot's setCursor hook.
	type TT = {
		show: boolean;
		left: number;
		top: number;
		ts: number;
		rows: { label: string; val: string; color: string }[];
	};
	let tt = $state<TT>({ show: false, left: 0, top: 0, ts: 0, rows: [] });

	function fmtVal(v: number | null | undefined): string {
		if (v == null || !Number.isFinite(v as number)) return '—';
		if (valueFormatter) return valueFormatter(v as number);
		return `${Math.round(v as number)}${valueSuffix}`;
	}

	function fmtTime(unix: number): string {
		return new Date(unix * 1000).toLocaleString(undefined, {
			month: 'short', day: 'numeric',
			hour: 'numeric', minute: '2-digit'
		});
	}

	function buildOpts(w: number): uPlot.Options {
		// Capture strokes at build-time so hook closures stay consistent.
		const ss = series.map((s, i) => s.stroke ?? palette[i % palette.length]);
		// Resolve design tokens for canvas-side rendering. Fallbacks preserve
		// a sensible default if anything goes wrong.
		const axisStroke = resolveToken('--text-faint', '#6b7494');
		const gridStroke = resolveToken('--line', '#1c253a');

		return {
			width: w,
			height,
			legend: { show: false },
			axes: [
				{
					stroke: axisStroke,
					grid: { stroke: gridStroke, width: 1 },
					ticks: { show: false },
					font: '11px var(--font-mono), ui-monospace, monospace',
					gap: 5
				},
				{
					stroke: axisStroke,
					grid: { stroke: gridStroke, width: 1 },
					ticks: { show: false },
					font: '11px var(--font-mono), ui-monospace, monospace',
					gap: 8,
					values: (_u: uPlot, ticks: number[]) =>
						ticks.map(v => (v == null ? '' : fmtVal(v))),
					// Dynamically widen the axis to fit the longest label so nothing
					// gets clipped — the cutoff bug shown in the screenshot.
					size: (_u: uPlot, vals: string[], _ai: number, _ci: number): number => {
						if (!vals?.length) return 60;
						const maxLen = Math.max(3, ...vals.filter(Boolean).map(v => v.length));
						return Math.ceil(maxLen * 7.5) + 20;
					}
				}
			],
			scales: {
				x: { time: true },
				y: (yMin !== undefined || yMax !== undefined)
					? { range: [yMin ?? null, yMax ?? null] as [number | null, number | null] }
					: {}
			},
			series: [
				{ label: 'time' },
				...series.map((s, i) => {
					const stroke = ss[i];
					// fill: false  → no area fill (reference / "Total" lines)
					// fill: string → explicit color override
					// default      → hex-alpha of the stroke color (16% opacity)
					const fill =
						s.fill === false ? undefined
						: typeof s.fill === 'string' ? s.fill
						: stroke + '29';
					return { label: s.label, stroke, fill, width: 1.5, points: { show: false } };
				})
			],
			cursor: {
				drag: { x: true, y: false },
				points: { size: 5, fill: '#ffffffbb', stroke: '#ffffff33', width: 1 }
			},
			hooks: {
				// Double-click resets any drag-zoom to the full data range.
				ready: [(u: uPlot) => {
					u.over?.addEventListener('dblclick', () => {
						const d = u.data[0];
						if (!d?.length) return;
						u.setScale('x', { min: d[0] as number, max: d[d.length - 1] as number });
					});
				}],
				setCursor: [(u: uPlot) => {
					const idx = u.cursor.idx;
					const curL = u.cursor.left ?? -1;
					if (idx == null || curL < 0) {
						if (tt.show) tt = { ...tt, show: false };
						return;
					}
					const dpr = window.devicePixelRatio || 1;
					// cursor.left/top are relative to .u-over; add axis offsets to
					// get canvas-wrapper-relative coordinates for the tooltip div.
					const axLeft = u.bbox.left / dpr;
					const axTop  = u.bbox.top  / dpr;
					const ts   = u.data[0][idx] as number;
					const rows = series.map((s, i) => ({
						label: s.label,
						val:   fmtVal(u.data[i + 1]?.[idx] as number),
						color: ss[i]
					}));
					const TT_W  = 190;
					const wrapW = wrapEl?.clientWidth ?? 600;
					const absL  = curL + axLeft;
					const left  = absL + TT_W + 14 > wrapW ? absL - TT_W - 6 : absL + 10;
					const top   = Math.max(2, Math.min((u.cursor.top ?? 20) + axTop - 14, height - 20));
					tt = { show: true, left, top, ts, rows };
				}]
			}
		};
	}

	function chartData(): uPlot.AlignedData {
		return [x, ...series.map(s => s.values)] as uPlot.AlignedData;
	}

	onMount(() => {
		plot = new uPlot(buildOpts(canvasEl.clientWidth), chartData(), canvasEl);
		const ro = new ResizeObserver(() => {
			if (plot && canvasEl) plot.setSize({ width: canvasEl.clientWidth, height });
		});
		ro.observe(canvasEl);
		return () => ro.disconnect();
	});

	$effect(() => { if (plot) plot.setData(chartData()); });

	onDestroy(() => { plot?.destroy(); plot = null; });
</script>

<div class="chart-wrap" bind:this={wrapEl} onmouseleave={() => (tt.show = false)} role="img" aria-label="time series chart">
	{#if series.length > 1}
		<div class="legend">
			{#each series as s, i}
				<span class="chip">
					<span class="dot" style="background:{strokes[i]}"></span>
					{s.label}
				</span>
			{/each}
		</div>
	{/if}
	<div class="canvas-wrap">
		<div bind:this={canvasEl} class="canvas-host" style="height:{height}px"></div>
		{#if tt.show}
			<div class="tooltip" style="left:{tt.left}px;top:{tt.top}px">
				<div class="tt-ts">{fmtTime(tt.ts)}</div>
				{#each tt.rows as row}
					<div class="tt-row">
						<span class="tt-dot" style="background:{row.color}"></span>
						<span class="tt-label">{row.label}</span>
						<span class="tt-val">{row.val}</span>
					</div>
				{/each}
			</div>
		{/if}
	</div>
	{#if series.length > 1}
		<div class="zoom-hint">drag to zoom · double-click to reset</div>
	{/if}
</div>

<style>
	.chart-wrap { width: 100%; }
	.canvas-wrap { position: relative; }
	.canvas-host { width: 100%; }

	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin-bottom: 8px;
		font-size: 11px;
		color: var(--text-dim);
	}
	.chip { display: inline-flex; align-items: center; gap: 5px; }
	.dot { width: 8px; height: 8px; border-radius: 2px; flex-shrink: 0; }

	.zoom-hint {
		margin-top: 4px;
		font-size: 10px;
		text-align: right;
		color: var(--text-dim);
		opacity: 0.4;
		pointer-events: none;
		user-select: none;
	}

	/* Rich hover tooltip */
	.tooltip {
		position: absolute;
		z-index: 20;
		pointer-events: none;
		background: color-mix(in srgb, var(--bg-elev) 92%, transparent);
		backdrop-filter: blur(10px);
		-webkit-backdrop-filter: blur(10px);
		border: 1px solid var(--line);
		border-radius: 7px;
		padding: 9px 11px;
		font-size: 11px;
		min-width: 150px;
		max-width: 230px;
		box-shadow: 0 6px 24px rgba(0, 0, 0, 0.55);
	}
	.tt-ts {
		color: var(--text-dim);
		margin-bottom: 7px;
		padding-bottom: 6px;
		border-bottom: 1px solid var(--line);
		white-space: nowrap;
		font-size: 10.5px;
	}
	.tt-row {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 2px 0;
	}
	.tt-dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.tt-label {
		flex: 1;
		color: var(--text-dim);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.tt-val {
		font-family: var(--font-mono);
		color: var(--text);
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
		font-size: 11px;
	}
</style>
