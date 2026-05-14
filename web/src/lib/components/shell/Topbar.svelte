<script lang="ts">
	import { theme } from '$lib/stores/theme.svelte';
	import { hostStore } from '$lib/stores/hosts.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import Kbd from '$lib/components/primitives/Kbd.svelte';
	import { fmtRelative } from '$lib/format';

	interface Props {
		onrefresh?: () => void;
	}

	let { onrefresh }: Props = $props();

	const lastSyncLabel = $derived(
		hostStore.lastSync ? `synced ${fmtRelative(hostStore.lastSync.toISOString())}` : 'connecting…'
	);
</script>

<header class="topbar glass-topbar">
	<!-- Search -->
	<div class="search-wrap">
		<Icon name="search" size={14} />
		<input
			type="search"
			placeholder="Search hosts, containers, stacks…"
			readonly
			onclick={() => {}}
			aria-label="Search (⌘K)"
		/>
		<Kbd key="⌘K" />
	</div>

	<!-- Right actions -->
	<div class="right">
		<span class="sync-indicator mono">
			<span class="sync-dot"></span>
			{lastSyncLabel}
		</span>

		<button class="icon-btn" onclick={onrefresh} aria-label="Refresh" title="Refresh">
			<Icon name="refresh" size={14} />
		</button>

		<button class="icon-btn" onclick={theme.toggle} aria-label="Toggle theme" title="Toggle theme">
			{#if theme.resolved === 'dark'}
				<Icon name="sun" size={14} />
			{:else}
				<Icon name="moon" size={14} />
			{/if}
		</button>

		<div class="avatar" aria-label="User menu">A</div>
	</div>
</header>

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 50;
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 10px 28px;
		border-bottom: 1px solid color-mix(in srgb, var(--line) 70%, transparent);
	}

	.search-wrap {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 360px;
		max-width: 50%;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 6px 10px;
		color: var(--text-faint);
		transition: border-color 120ms;
	}

	.search-wrap:focus-within {
		border-color: var(--accent-line);
	}

	.search-wrap input {
		flex: 1;
		background: none;
		border: none;
		padding: 0;
		font-size: 13px;
		color: var(--text);
		width: 100%;
		cursor: text;
	}

	.search-wrap input:focus { outline: none; }

	.right {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.sync-indicator {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 11px;
		color: var(--text-faint);
	}

	.sync-dot {
		width: 6px;
		height: 6px;
		border-radius: var(--r-pill);
		background: var(--ok);
		flex-shrink: 0;
	}

	.icon-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: transparent;
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text-dim);
		cursor: pointer;
		transition: background 120ms, color 120ms;
	}

	.icon-btn:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	.avatar {
		width: 28px;
		height: 28px;
		border-radius: var(--r-pill);
		background: var(--accent-soft);
		color: var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
		border: 1px solid var(--accent-line);
	}
</style>
