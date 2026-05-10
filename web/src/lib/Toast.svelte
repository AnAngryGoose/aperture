<script lang="ts">
	import { toast } from '$lib/toast';
</script>

<div class="toast-stack" aria-live="polite">
	{#each $toast as t (t.id)}
		<div class="toast toast-{t.kind}" role="alert">
			<span class="toast-msg">{t.message}</span>
			<button class="toast-close" onclick={() => toast.remove(t.id)} aria-label="Dismiss">✕</button>
		</div>
	{/each}
</div>

<style>
	.toast-stack {
		position: fixed;
		bottom: 24px;
		right: 24px;
		z-index: 1000;
		display: flex;
		flex-direction: column;
		gap: 8px;
		pointer-events: none;
	}
	.toast {
		pointer-events: all;
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 14px;
		border-radius: 7px;
		border: 1px solid var(--border);
		background: var(--bg-elev-2);
		box-shadow: 0 4px 20px rgba(0, 0, 0, 0.5);
		font-size: 13px;
		max-width: 380px;
		animation: slide-in 0.2s ease;
	}
	.toast-info    { border-left: 3px solid var(--accent); }
	.toast-success { border-left: 3px solid var(--ok); }
	.toast-error   { border-left: 3px solid var(--bad); }
	.toast-msg { flex: 1; }
	.toast-close {
		background: none;
		border: none;
		color: var(--text-dim);
		cursor: pointer;
		padding: 0 2px;
		font-size: 12px;
		flex-shrink: 0;
	}
	.toast-close:hover { color: var(--text); border-color: transparent; }
	@keyframes slide-in {
		from { opacity: 0; transform: translateY(8px); }
		to   { opacity: 1; transform: translateY(0); }
	}
</style>
