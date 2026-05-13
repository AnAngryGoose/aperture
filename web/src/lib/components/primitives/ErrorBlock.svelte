<script lang="ts">
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';

	interface Props {
		message?: string;
		onretry?: () => void;
	}

	let { message = "Couldn't reach the orchestrator", onretry }: Props = $props();
</script>

<div class="error-block">
	<Icon name="warn" size={20} />
	<p class="msg">{message}</p>
	{#if onretry}
		<Button variant="ghost" onclick={onretry}>Retry</Button>
	{/if}
</div>

<style>
	.error-block {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 14px 18px;
		background: var(--crit-soft);
		border: 1px solid var(--crit);
		border-radius: var(--r-lg);
		color: var(--crit);
	}

	.msg {
		flex: 1;
		font-size: 13px;
		color: var(--text);
	}
</style>
