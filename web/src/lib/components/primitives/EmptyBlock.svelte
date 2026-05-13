<script lang="ts">
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';

	interface Props {
		icon?: string;
		title?: string;
		description?: string;
		action?: string;
		onaction?: () => void;
	}

	let {
		icon = 'hosts',
		title = 'Nothing here yet',
		description = '',
		action = '',
		onaction
	}: Props = $props();
</script>

<div class="empty">
	<div class="icon-chip">
		<Icon name={icon} size={24} />
	</div>
	<p class="empty-title">{title}</p>
	{#if description}
		<p class="empty-desc">{description}</p>
	{/if}
	{#if action && onaction}
		<Button variant="primary" onclick={onaction}>{action}</Button>
	{/if}
</div>

<style>
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 12px;
		padding: 48px 24px;
		border: 1.5px dashed var(--line-strong);
		border-radius: var(--r-lg);
		text-align: center;
	}

	.icon-chip {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 48px;
		height: 48px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		color: var(--text-faint);
	}

	.empty-title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text);
	}

	.empty-desc {
		font-size: 13px;
		color: var(--text-dim);
		max-width: 320px;
	}
</style>
