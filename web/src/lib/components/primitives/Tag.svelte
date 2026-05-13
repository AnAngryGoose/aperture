<script lang="ts">
	interface Props {
		label: string;
		onclick?: () => void;
	}

	let { label, onclick }: Props = $props();
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<span class="tag" class:clickable={!!onclick} onclick={onclick} onkeydown={(e) => e.key === 'Enter' && onclick?.()}>
	{label}
</span>

<style>
	.tag {
		display: inline-flex;
		align-items: center;
		height: 18px;
		padding: 0 6px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-dim);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
		white-space: nowrap;
		user-select: none;
	}

	.clickable {
		cursor: pointer;
	}

	.clickable:hover {
		background: var(--bg-hover);
		color: var(--text);
	}
</style>
