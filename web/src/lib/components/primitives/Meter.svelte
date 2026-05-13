<script lang="ts">
	interface Props {
		value: number;
		max?: number;
		height?: number;
	}

	let { value = 0, max = 100, height = 4 }: Props = $props();

	const pct = $derived(Math.min(100, Math.max(0, (value / max) * 100)));
	const color = $derived(
		pct >= 90 ? 'var(--crit)' :
		pct >= 75 ? 'var(--warn)' :
		'var(--accent)'
	);
</script>

<div class="meter" style="height: {height}px;">
	<div class="fill" style="width: {pct}%; background: {color};"></div>
</div>

<style>
	.meter {
		width: 100%;
		background: var(--line);
		border-radius: var(--r-pill);
		overflow: hidden;
	}
	.fill {
		height: 100%;
		border-radius: var(--r-pill);
		transition: width 0.4s ease;
	}
</style>
