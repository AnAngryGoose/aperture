<script lang="ts">
	/**
	 * Shared host detail tab bar. Renders every tab as a real link to
	 * /hosts/:id/<tab> so each tab is bookmarkable, refreshable, and reachable
	 * via the browser's back/forward buttons. The active tab is highlighted
	 * based on the current URL path rather than internal state.
	 */
	import { page } from '$app/state';

	interface Tab { key: string; label: string; }

	interface Props {
		hostId: string;
		activeKey?: string;
	}

	let { hostId, activeKey }: Props = $props();

	const TABS: ReadonlyArray<Tab> = [
		{ key: 'overview',   label: 'Overview' },
		{ key: 'cpu',        label: 'CPU' },
		{ key: 'memory',     label: 'Memory' },
		{ key: 'disk',       label: 'Disk' },
		{ key: 'network',    label: 'Network' },
		{ key: 'sensors',    label: 'Sensors' },
		{ key: 'processes',  label: 'Processes' },
		{ key: 'containers', label: 'Containers' },
		{ key: 'stacks',     label: 'Stacks' },
		{ key: 'logs',       label: 'Logs' },
		{ key: 'volumes',    label: 'Volumes' },
		{ key: 'images',     label: 'Images' },
		{ key: 'events',     label: 'Events' },
		{ key: 'settings',   label: 'Settings' }
	];

	// Derive active tab from URL when not passed explicitly. The pathname looks
	// like /hosts/<id>/<tab>[/...]; we split and take the segment immediately
	// after the host id.
	const active = $derived.by(() => {
		if (activeKey) return activeKey;
		const parts = page.url.pathname.split('/').filter(Boolean);
		const idx = parts.indexOf('hosts');
		if (idx >= 0 && parts.length > idx + 2) return parts[idx + 2];
		return 'overview';
	});
</script>

<nav class="tabs" aria-label="Host sections">
	{#each TABS as t}
		<a
			href={`/hosts/${hostId}/${t.key}`}
			aria-current={active === t.key ? 'page' : undefined}
			class="tab"
			class:active={active === t.key}
			data-sveltekit-noscroll
		>
			{t.label}
		</a>
	{/each}
</nav>

<style>
	.tabs {
		display: flex;
		gap: 0;
		overflow-x: auto;
		scrollbar-width: thin;
	}

	.tab {
		padding: 10px 14px;
		font-size: 13px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		cursor: pointer;
		transition: color 120ms, border-color 120ms;
		white-space: nowrap;
		text-decoration: none;
	}

	.tab:hover { color: var(--text); text-decoration: none; }

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}
</style>
