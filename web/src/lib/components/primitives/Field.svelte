<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		label?: string;
		error?: string;
		hint?: string;
		required?: boolean;
		children: Snippet;
	}

	let { label, error, hint, required = false, children }: Props = $props();
</script>

<div class="field" class:has-error={!!error}>
	{#if label}
		<label class="field-label">
			{label}{#if required}<span class="req">*</span>{/if}
		</label>
	{/if}
	{@render children()}
	{#if error}
		<span class="field-error">{error}</span>
	{:else if hint}
		<span class="field-hint">{hint}</span>
	{/if}
</div>

<style>
	.field {
		display: flex;
		flex-direction: column;
		gap: 5px;
	}

	.field-label {
		font-size: 12px;
		font-weight: 500;
		color: var(--text-dim);
	}

	.req {
		color: var(--crit);
		margin-left: 2px;
	}

	.field-error {
		font-size: 11px;
		color: var(--crit);
	}

	.field-hint {
		font-size: 11px;
		color: var(--text-faint);
	}

	.has-error :global(input),
	.has-error :global(select),
	.has-error :global(textarea) {
		border-color: var(--crit);
	}
</style>
