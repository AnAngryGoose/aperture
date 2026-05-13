<script lang="ts">
	interface Props {
		data: number[];
		width?: number;
		height?: number;
		color?: string;
		fill?: boolean;
		stroke?: number;
	}

	let { data = [], width = 140, height = 26, color = 'var(--accent)', fill = true, stroke = 1.5 }: Props = $props();

	const path = $derived.by(() => {
		const pts = data.filter((v) => isFinite(v));
		if (pts.length < 2) return { line: '', area: '' };

		const min = Math.min(...pts);
		const max = Math.max(...pts);
		const range = max - min || 1;
		const pad = stroke;

		const xs = pts.map((_, i) => (i / (pts.length - 1)) * width);
		const ys = pts.map((v) => pad + ((1 - (v - min) / range) * (height - pad * 2)));

		const line = xs.map((x, i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${ys[i].toFixed(1)}`).join(' ');
		const area = `${line} L${width},${height} L0,${height} Z`;

		return { line, area };
	});
</script>

<svg {width} {height} viewBox="0 0 {width} {height}" fill="none" aria-hidden="true">
	{#if fill && path.area}
		<path
			d={path.area}
			fill={color}
			opacity="0.18"
		/>
	{/if}
	{#if path.line}
		<path
			d={path.line}
			stroke={color}
			stroke-width={stroke}
			stroke-linecap="round"
			stroke-linejoin="round"
			fill="none"
		/>
	{/if}
</svg>
