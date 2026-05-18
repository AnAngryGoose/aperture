<script lang="ts">
	import Sparkline from '$lib/components/primitives/Sparkline.svelte';

	interface Props {
		label: string;
		value: string;
		sub?: string;
		data?: number[];
		color?: string;
		/**
		 * Optional deep link. When set, the tile renders as an anchor that
		 * navigates to this href on click — used by the drawer so clicking a
		 * big metric (CPU / Memory / Network / Temperature) opens the matching
		 * host detail tab.
		 */
		href?: string;
		onclick?: () => void;
	}

	let { label, value, sub = '', data = [], color = 'var(--accent)', href, onclick }: Props = $props();
</script>

{#if href}
	<a class="big-metric link" {href} onclick={onclick}>
		<div class="head label-mono">{label}</div>
		<div class="val mono">{value}</div>
		{#if sub}<div class="sub mono">{sub}</div>{/if}
		{#if data.length > 1}
			<Sparkline {data} width={220} height={36} {color} stroke={2} />
		{/if}
	</a>
{:else}
	<div class="big-metric">
		<div class="head label-mono">{label}</div>
		<div class="val mono">{value}</div>
		{#if sub}<div class="sub mono">{sub}</div>{/if}
		{#if data.length > 1}
			<Sparkline {data} width={220} height={36} {color} stroke={2} />
		{/if}
	</div>
{/if}

<style>
	.big-metric {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 16px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		text-decoration: none;
		color: inherit;
	}

	.big-metric.link {
		cursor: pointer;
		transition: border-color 120ms, background 120ms;
	}
	.big-metric.link:hover {
		border-color: var(--line-strong);
		background: var(--bg-hover, var(--bg-elev-2));
	}

	.head { color: var(--text-faint); }

	.val {
		font-size: 26px;
		font-weight: 500;
		letter-spacing: -0.02em;
		color: var(--text);
		line-height: 1;
	}

	.sub {
		font-size: 11px;
		color: var(--text-dim);
	}
</style>
