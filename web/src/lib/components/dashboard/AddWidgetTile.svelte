<script lang="ts">
	interface Props {
		onclick?: () => void;
	}
	let { onclick }: Props = $props();
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="add-tile" onclick={onclick} role="button" tabindex="0"
     onkeydown={(e) => e.key === 'Enter' && onclick?.()}>
	<span class="plus">+</span>
	<span class="label">Add host</span>
	<span class="sub mono">or pin a widget</span>
</div>

<style>
	.add-tile {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
		padding: 40px 24px;
		border: 1.5px dashed var(--line-strong);
		border-radius: var(--r-lg);
		cursor: pointer;
		color: var(--text-faint);
		transition: border-color 180ms, background 180ms, color 180ms;
	}

	.add-tile:hover {
		border-color: var(--accent);
		background: var(--accent-soft);
		color: var(--accent);
	}

	.plus {
		font-size: 28px;
		font-weight: 300;
		line-height: 1;
	}

	.label {
		font-size: 14px;
		font-weight: 500;
	}

	.sub {
		font-size: 11px;
		color: var(--text-faint);
	}

	.add-tile:hover .sub {
		color: var(--accent);
		opacity: 0.7;
	}
</style>
