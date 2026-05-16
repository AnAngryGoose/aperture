<script lang="ts">
	/**
	 * Range picker for the host-detail page. Segmented control with the
	 * canonical Beszel-style range presets (15m / 1h / 6h / 24h / 7d / 30d).
	 * Persists the last choice per host in localStorage so a user returning
	 * to a host sees the range they were last looking at.
	 */
	export type Range = '15m' | '1h' | '6h' | '24h' | '7d' | '30d';

	interface Props {
		value: Range;
		hostId?: string;        // when provided, persists per-host
		onchange: (v: Range) => void;
	}

	let { value = $bindable('1h'), hostId, onchange }: Props = $props();

	const RANGES: Range[] = ['15m', '1h', '6h', '24h', '7d', '30d'];

	function select(r: Range) {
		if (r === value) return;
		value = r;
		if (hostId) {
			try {
				localStorage.setItem(`aperture.range.${hostId}`, r);
			} catch {
				/* localStorage may be unavailable (private mode); silent. */
			}
		}
		onchange(r);
	}
</script>

<div class="picker" role="tablist" aria-label="Time range">
	{#each RANGES as r}
		<button
			type="button"
			role="tab"
			aria-selected={value === r}
			class="opt"
			class:active={value === r}
			onclick={() => select(r)}
		>
			{r}
		</button>
	{/each}
</div>

<style>
	.picker {
		display: inline-flex;
		gap: 0;
		padding: 3px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}

	.opt {
		padding: 5px 10px;
		font-size: 12px;
		font-family: var(--font-mono);
		color: var(--text-dim);
		background: none;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
		transition: background 120ms, color 120ms;
	}

	.opt:hover { background: var(--bg-hover); color: var(--text); }

	.opt.active {
		background: var(--bg-hover);
		color: var(--text);
	}
</style>
