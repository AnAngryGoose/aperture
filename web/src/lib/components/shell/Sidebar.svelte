<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import { hostStore } from '$lib/stores/hosts.svelte';

	const WORKSPACE = [
		{ href: '/dashboard', icon: 'dashboard', label: 'Dashboard' },
		{ href: '/hosts',     icon: 'hosts',     label: 'Hosts' },
		{ href: '/containers',icon: 'containers',label: 'Containers' },
		{ href: '/stacks',    icon: 'stacks',    label: 'Stacks' },
		{ href: '/storage',   icon: 'storage',   label: 'Storage' },
		{ href: '/network',   icon: 'network',   label: 'Network' }
	];

	const OBSERVE = [
		{ href: '/logs',       icon: 'logs',       label: 'Logs' },
		{ href: '/shell',      icon: 'shell',       label: 'Shell' },
		{ href: '/automation', icon: 'automation',  label: 'Automation' },
		{ href: '/alerts',     icon: 'alerts',      label: 'Alerts', badge: true }
	];

	const openAlerts = $derived(
		Object.values(hostStore.entries).reduce((n, e) => n + (e.host.open_alerts ?? 0), 0)
	);
</script>

<aside class="sidebar">
	<!-- Brand -->
	<a href="/dashboard" class="brand">
		<div class="brand-mark">
			<svg width="22" height="22" viewBox="0 0 22 22" fill="none">
				<rect width="22" height="22" rx="4" fill="var(--text)"/>
				<circle cx="11" cy="11" r="5" stroke="var(--accent)" stroke-width="1.5" fill="none"/>
				<polygon points="11,6 13.5,10 8.5,10" fill="var(--accent)"/>
			</svg>
		</div>
		<div class="brand-text">
			<span class="brand-name">Aperture</span>
			<span class="brand-version mono">v0.0.4-alpha.3</span>
		</div>
	</a>

	<!-- Workspace section -->
	<nav class="nav-section">
		<span class="section-label label-mono">Workspace</span>
		{#each WORKSPACE as item}
			{@const active = page.url.pathname.startsWith(item.href)}
			<a href={item.href} class="nav-item" class:active>
				<Icon name={item.icon} size={16} />
				<span>{item.label}</span>
			</a>
		{/each}
	</nav>

	<!-- Observe section -->
	<nav class="nav-section">
		<span class="section-label label-mono">Observe</span>
		{#each OBSERVE as item}
			{@const active = page.url.pathname.startsWith(item.href)}
			<a href={item.href} class="nav-item" class:active>
				<Icon name={item.icon} size={16} />
				<span>{item.label}</span>
				{#if item.badge && openAlerts > 0}
					<span class="alert-badge">{openAlerts}</span>
				{/if}
			</a>
		{/each}
	</nav>

	<!-- Bottom -->
	<div class="sidebar-bottom">
		<a href="/settings" class="nav-item" class:active={page.url.pathname.startsWith('/settings')}>
			<Icon name="settings" size={16} />
			<span>Settings</span>
		</a>
	</div>
</aside>

<style>
	.sidebar {
		width: 220px;
		min-height: 100vh;
		background: var(--bg-elev);
		border-right: 1px solid var(--line);
		padding: 16px 12px;
		display: flex;
		flex-direction: column;
		gap: 20px;
		position: sticky;
		top: 0;
		height: 100vh;
		overflow-y: auto;
		flex-shrink: 0;
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		text-decoration: none;
		padding-bottom: 4px;
	}

	.brand-mark { flex-shrink: 0; }

	.brand-text {
		display: flex;
		flex-direction: column;
	}

	.brand-name {
		font-size: 14px;
		font-weight: 600;
		letter-spacing: -0.01em;
		color: var(--text);
	}

	.brand-version {
		font-size: 10px;
		color: var(--text-faint);
	}

	.nav-section {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.section-label {
		display: block;
		padding: 0 10px 6px;
		color: var(--text-faint);
		letter-spacing: 0.12em;
	}

	.nav-item {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 7px 10px;
		border-radius: var(--r-md);
		font-size: 13.5px;
		color: var(--text-dim);
		text-decoration: none;
		position: relative;
		transition: background 120ms, color 120ms;
	}

	.nav-item:hover {
		background: var(--bg-hover);
		color: var(--text);
	}

	.nav-item.active {
		background: var(--accent-soft);
		color: var(--accent);
	}

	.nav-item.active::before {
		content: '';
		position: absolute;
		left: -12px;
		top: 8px;
		bottom: 8px;
		width: 2px;
		background: var(--accent);
		border-radius: var(--r-pill);
	}

	.alert-badge {
		margin-left: auto;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		background: var(--crit);
		color: #fff;
		font-family: var(--font-mono);
		font-size: 10px;
		border-radius: var(--r-pill);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.sidebar-bottom {
		margin-top: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
</style>
