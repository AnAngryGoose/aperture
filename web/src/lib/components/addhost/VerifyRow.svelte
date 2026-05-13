<script lang="ts">
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		label: string;
		status: 'pending' | 'ok' | 'error' | 'checking';
		detail?: string;
	}

	let { label, status, detail = '' }: Props = $props();
</script>

<div class="row" class:ok={status === 'ok'} class:error={status === 'error'}>
	<div class="row-icon">
		{#if status === 'checking'}
			<div class="spinner"></div>
		{:else if status === 'ok'}
			<Icon name="check" size={14} />
		{:else if status === 'error'}
			<Icon name="x" size={14} />
		{:else}
			<div class="dot"></div>
		{/if}
	</div>
	<div class="row-body">
		<span class="row-label">{label}</span>
		{#if detail}
			<span class="row-detail mono">{detail}</span>
		{/if}
	</div>
</div>

<style>
	.row {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 12px;
		border-radius: var(--r-md);
		font-size: 13px;
	}

	.row-icon {
		width: 20px;
		height: 20px;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		color: var(--text-faint);
	}

	.row.ok .row-icon { color: var(--ok); }
	.row.error .row-icon { color: var(--crit); }

	.row-body {
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.row-label { color: var(--text-dim); }
	.row.ok .row-label { color: var(--text); }
	.row.error .row-label { color: var(--crit); }

	.row-detail {
		font-size: 11px;
		color: var(--text-faint);
	}

	.dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--line-strong);
	}

	.spinner {
		width: 14px;
		height: 14px;
		border: 1.5px solid var(--line-strong);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: spin 700ms linear infinite;
	}

	@media (prefers-reduced-motion: no-preference) {
		@keyframes spin {
			to { transform: rotate(360deg); }
		}
	}
</style>
