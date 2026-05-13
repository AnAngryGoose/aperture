<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		variant?: 'primary' | 'ghost' | 'mini' | 'icon' | 'danger';
		type?: 'button' | 'submit' | 'reset';
		disabled?: boolean;
		loading?: boolean;
		onclick?: (e: MouseEvent) => void;
		children: Snippet;
		class?: string;
	}

	let {
		variant = 'ghost',
		type = 'button',
		disabled = false,
		loading = false,
		onclick,
		children,
		class: cls = ''
	}: Props = $props();
</script>

<button
	{type}
	{disabled}
	class="btn btn-{variant} {cls}"
	class:loading
	onclick={onclick}
>
	{@render children()}
</button>

<style>
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 6px;
		font-family: var(--font-sans);
		font-size: 13px;
		font-weight: 500;
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 120ms, color 120ms, transform 120ms, box-shadow 120ms;
		white-space: nowrap;
		border: none;
		outline: none;
	}

	.btn:disabled, .btn.loading {
		opacity: 0.5;
		pointer-events: none;
	}

	@media (prefers-reduced-motion: no-preference) {
		.btn:active { transform: scale(0.96); }
	}

	/* Primary */
	.btn-primary {
		padding: 7px 14px;
		color: #fff;
		background-image: linear-gradient(
			180deg,
			color-mix(in srgb, var(--accent) 100%, white 8%),
			var(--accent)
		);
		box-shadow: 0 1px 0 rgba(255,255,255,.18) inset, 0 4px 16px -8px var(--accent);
	}

	.btn-primary:hover {
		filter: brightness(1.08);
	}

	/* Ghost */
	.btn-ghost {
		padding: 6px 12px;
		color: var(--text-dim);
		background: var(--bg-elev);
		border: 1px solid var(--line);
	}

	.btn-ghost:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	/* Mini */
	.btn-mini {
		padding: 4px 8px;
		font-size: 12px;
		color: var(--text-dim);
		background: var(--bg-elev);
		border: 1px solid var(--line);
	}

	.btn-mini:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	/* Icon */
	.btn-icon {
		width: 28px;
		height: 28px;
		padding: 0;
		color: var(--text-dim);
		background: transparent;
		border: 1px solid var(--line);
	}

	.btn-icon:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	/* Danger */
	.btn-danger {
		padding: 6px 12px;
		color: var(--crit);
		background: var(--crit-soft);
		border: 1px solid var(--crit);
	}

	.btn-danger:hover {
		background: var(--crit);
		color: #fff;
	}
</style>
