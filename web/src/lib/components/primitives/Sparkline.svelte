<script lang="ts">
	import { fmtRelative } from '$lib/format';

	interface Props {
		data: number[];
		/** Optional timestamps (seconds since epoch) for hover tooltip. Same length as `data`. */
		xs?: number[];
		/** Optional value formatter for the tooltip. Defaults to one-decimal number. */
		format?: (v: number) => string;
		width?: number;
		height?: number;
		color?: string;
		fill?: boolean;
		stroke?: number;
		/** Suffix appended to the formatted value (e.g. "%"). Ignored when `format` is provided. */
		suffix?: string;
	}

	let {
		data = [],
		xs,
		format,
		width = 140,
		height = 26,
		color = 'var(--accent)',
		fill = true,
		stroke = 1.5,
		suffix = ''
	}: Props = $props();

	const path = $derived.by(() => {
		const pts = data.filter((v) => isFinite(v));
		if (pts.length < 2) return { line: '', area: '' };

		const min = Math.min(...pts);
		const max = Math.max(...pts);
		const range = max - min || 1;
		const pad = stroke;

		const xCoords = pts.map((_, i) => (i / (pts.length - 1)) * width);
		const yCoords = pts.map((v) => pad + ((1 - (v - min) / range) * (height - pad * 2)));

		const line = xCoords.map((x, i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${yCoords[i].toFixed(1)}`).join(' ');
		const area = `${line} L${width},${height} L0,${height} Z`;

		return { line, area };
	});

	// Hover state — only computed when there's a meaningful series. Tooltip is
	// suppressed when fewer than 2 points exist (the chart is empty anyway).
	let hovered = $state(false);
	let hoverIdx = $state(-1);
	let svgEl: SVGSVGElement;

	function fmtValue(v: number): string {
		if (format) return format(v);
		const fixed = v >= 100 ? v.toFixed(0) : v.toFixed(1);
		return `${fixed}${suffix}`;
	}

	function onPointerMove(ev: PointerEvent) {
		if (data.length < 2 || !svgEl) return;
		const rect = svgEl.getBoundingClientRect();
		const x = ev.clientX - rect.left;
		const ratio = Math.max(0, Math.min(1, x / rect.width));
		hoverIdx = Math.round(ratio * (data.length - 1));
		hovered = true;
	}

	function onPointerLeave() {
		hovered = false;
		hoverIdx = -1;
	}

	const hoverPos = $derived.by(() => {
		if (!hovered || hoverIdx < 0 || data.length < 2) return null;
		const x = (hoverIdx / (data.length - 1)) * width;
		return { x, value: data[hoverIdx], ts: xs?.[hoverIdx] };
	});
</script>

<div
	class="spark"
	style="width:{width}px; height:{height}px"
	onpointermove={onPointerMove}
	onpointerleave={onPointerLeave}
>
	<svg
		bind:this={svgEl}
		{width}
		{height}
		viewBox="0 0 {width} {height}"
		fill="none"
		aria-hidden="true"
	>
		{#if fill && path.area}
			<path d={path.area} fill={color} opacity="0.18" />
		{/if}
		{#if path.line}
			<path d={path.line} stroke={color} stroke-width={stroke} stroke-linecap="round" stroke-linejoin="round" fill="none" />
		{/if}
		{#if hoverPos}
			<line x1={hoverPos.x} x2={hoverPos.x} y1={0} y2={height} stroke="var(--text-faint)" stroke-width="0.75" opacity="0.5" />
			<circle cx={hoverPos.x} cy={path.line ? 0 : 0} r="2.5" fill={color} stroke="var(--bg)" stroke-width="1" style="transform: translateY({(() => {
				const pts = data.filter((v) => isFinite(v));
				if (pts.length < 2) return 0;
				const min = Math.min(...pts);
				const max = Math.max(...pts);
				const range = max - min || 1;
				const pad = stroke;
				return pad + ((1 - (hoverPos.value - min) / range) * (height - pad * 2));
			})()}px)" />
		{/if}
	</svg>
	{#if hoverPos}
		<div class="tip mono" style="left:{Math.min(hoverPos.x + 10, width - 110)}px">
			<span class="tip-val">{fmtValue(hoverPos.value)}</span>
			{#if hoverPos.ts}
				<span class="tip-ts">{fmtRelative(hoverPos.ts * 1000)}</span>
			{/if}
		</div>
	{/if}
</div>

<style>
	.spark {
		position: relative;
		display: inline-block;
		flex-shrink: 0;
	}

	.tip {
		position: absolute;
		top: -22px;
		background: color-mix(in srgb, var(--bg-elev) 92%, transparent);
		border: 1px solid var(--line);
		padding: 2px 6px;
		font-size: 10px;
		border-radius: var(--r-sm);
		white-space: nowrap;
		pointer-events: none;
		display: inline-flex;
		gap: 6px;
		z-index: 10;
		box-shadow: 0 4px 12px -4px rgba(0,0,0,0.3);
	}

	@media (prefers-reduced-motion: no-preference) {
		.tip {
			animation: fade-in 100ms ease-out;
		}
		@keyframes fade-in {
			from { opacity: 0; transform: translateY(2px); }
			to   { opacity: 1; transform: translateY(0); }
		}
	}

	.tip-val { color: var(--text); }
	.tip-ts { color: var(--text-faint); }
</style>
