<script lang="ts">
	import type { Snippet } from 'svelte';

	type Variant =
		| 'primary'    // accent-filled — for the headline action on a page
		| 'secondary'  // neutral filled — paired with primary, secondary action
		| 'ghost'      // subtle outline + neutral fill — the workhorse default
		| 'outline'    // transparent + line — for chips and stacked rows
		| 'mini'       // small ghost — for table row actions
		| 'icon'       // square icon button
		| 'warning'    // amber — for disruptive but recoverable actions
		| 'danger';    // red — for destructive/irreversible actions

	type Size = 'sm' | 'md' | 'lg';

	interface Props {
		variant?: Variant;
		size?: Size;
		type?: 'button' | 'submit' | 'reset';
		disabled?: boolean;
		loading?: boolean;
		title?: string;
		onclick?: (e: MouseEvent) => void;
		children: Snippet;
		class?: string;
		ariaLabel?: string;
		active?: boolean;
	}

	let {
		variant = 'ghost',
		size = 'md',
		type = 'button',
		disabled = false,
		loading = false,
		title,
		onclick,
		children,
		class: cls = '',
		ariaLabel,
		active = false
	}: Props = $props();
</script>

<button
	{type}
	{title}
	disabled={disabled || loading}
	aria-label={ariaLabel}
	aria-pressed={active ? 'true' : undefined}
	class="btn btn-{variant} size-{size} {cls}"
	class:loading
	class:active
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
		font-weight: 500;
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 120ms ease, color 120ms ease, border-color 120ms ease, transform 120ms ease;
		white-space: nowrap;
		border: 1px solid transparent;
		outline: none;
		line-height: 1;
		user-select: none;
	}

	/* Sizes — kept compact to match the ops-console density */
	.size-sm { padding: 3px 8px; font-size: 11px; min-height: 22px; }
	.size-md { padding: 6px 12px; font-size: 12px; min-height: 28px; }
	.size-lg { padding: 8px 16px; font-size: 13px; min-height: 34px; }

	.btn:disabled, .btn.loading {
		opacity: 0.5;
		pointer-events: none;
	}

	@media (prefers-reduced-motion: no-preference) {
		.btn:active:not(:disabled) { transform: scale(0.97); }
	}

	/* Primary — accent-filled headline action */
	.btn-primary {
		color: #fff;
		background-image: linear-gradient(
			180deg,
			color-mix(in srgb, var(--accent) 100%, white 8%),
			var(--accent)
		);
		border-color: var(--accent);
		box-shadow: 0 1px 0 rgba(255,255,255,.18) inset, 0 4px 16px -8px var(--accent);
	}
	.btn-primary:hover:not(:disabled) { filter: brightness(1.08); }

	/* Secondary — neutral filled action paired with primary */
	.btn-secondary {
		color: var(--text);
		background: var(--bg-elev-2);
		border-color: var(--line-strong);
	}
	.btn-secondary:hover:not(:disabled) {
		background: var(--bg-hover);
		border-color: var(--text-faint);
	}

	/* Ghost — workhorse default, subtle elevated surface */
	.btn-ghost {
		color: var(--text-dim);
		background: var(--bg-elev);
		border-color: var(--line);
	}
	.btn-ghost:hover:not(:disabled) {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--line-strong);
	}
	.btn-ghost.active {
		color: var(--accent);
		border-color: var(--accent);
		background: var(--accent-soft);
	}

	/* Outline — transparent fill, line only */
	.btn-outline {
		color: var(--text-dim);
		background: transparent;
		border-color: var(--line);
	}
	.btn-outline:hover:not(:disabled) {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--line-strong);
	}
	.btn-outline.active {
		color: var(--accent);
		border-color: var(--accent);
		background: var(--accent-soft);
	}

	/* Mini — table row / inline action */
	.btn-mini {
		padding: 3px 8px;
		font-size: 11px;
		min-height: 22px;
		color: var(--text-dim);
		background: var(--bg-elev);
		border-color: var(--line);
	}
	.btn-mini:hover:not(:disabled) {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--line-strong);
	}

	/* Icon — square button for icons/symbols */
	.btn-icon {
		width: 28px;
		height: 28px;
		padding: 0;
		min-height: 0;
		color: var(--text-dim);
		background: transparent;
		border-color: var(--line);
	}
	.btn-icon.size-sm { width: 22px; height: 22px; }
	.btn-icon.size-lg { width: 34px; height: 34px; }
	.btn-icon:hover:not(:disabled) {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--line-strong);
	}

	/* Warning — disruptive but recoverable (restart, stop) */
	.btn-warning {
		color: var(--warn);
		background: var(--warn-soft);
		border-color: color-mix(in srgb, var(--warn) 40%, transparent);
	}
	.btn-warning:hover:not(:disabled) {
		background: var(--warn);
		color: #0b0e14;
		border-color: var(--warn);
	}

	/* Danger — destructive / irreversible (remove, force-remove, revoke) */
	.btn-danger {
		color: var(--crit);
		background: var(--crit-soft);
		border-color: color-mix(in srgb, var(--crit) 40%, transparent);
	}
	.btn-danger:hover:not(:disabled) {
		background: var(--crit);
		color: #fff;
		border-color: var(--crit);
	}
</style>
