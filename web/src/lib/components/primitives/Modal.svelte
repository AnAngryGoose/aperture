<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		open: boolean;
		onclose: () => void;
		title?: string;
		width?: string;
		children: Snippet;
	}

	let { open, onclose, title, width = '560px', children }: Props = $props();

	function onBackdrop(e: MouseEvent) {
		if (e.currentTarget === e.target) onclose();
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape') onclose();
	}
</script>

<svelte:window onkeydown={onKey} />

{#if open}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="backdrop" onclick={onBackdrop}>
		<div class="modal" style="width: {width};" role="dialog" aria-modal="true">
			{#if title}
				<div class="modal-head">
					<span class="modal-title">{title}</span>
					<button class="close-btn" onclick={onclose} aria-label="Close">✕</button>
				</div>
			{/if}
			<div class="modal-body">
				{@render children()}
			</div>
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(0, 0, 0, 0.55);
		backdrop-filter: blur(6px) saturate(1.2);
	}

	.modal {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		max-height: 90vh;
		overflow-y: auto;
		box-shadow: 0 24px 60px -20px rgba(0, 0, 0, 0.5);
		max-width: 95vw;
	}

	@media (prefers-reduced-motion: no-preference) {
		.modal {
			animation: modal-in var(--dur-modal) var(--ease-card) both;
		}

		@keyframes modal-in {
			from { opacity: 0; transform: scale(0.97); }
			to   { opacity: 1; transform: scale(1); }
		}
	}

	.modal-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 18px 20px 0;
	}

	.modal-title {
		font-size: 16px;
		font-weight: 600;
		letter-spacing: -0.01em;
		color: var(--text);
	}

	.close-btn {
		background: none;
		border: none;
		color: var(--text-faint);
		cursor: pointer;
		font-size: 14px;
		padding: 4px;
		border-radius: var(--r-sm);
	}

	.close-btn:hover { color: var(--text); }

	.modal-body {
		padding: 18px 20px;
	}
</style>
