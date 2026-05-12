<script lang="ts">
	import 'uplot/dist/uPlot.min.css';
	import '$lib/styles.css';
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { SystemInfo } from '$lib/types';
	import { formatBytes, formatDuration } from '$lib/format';
	import Toast from '$lib/Toast.svelte';

	let { children } = $props();
	let firing = $state(0);
	let sys = $state<SystemInfo | null>(null);
	let now = $state(Date.now());
	let authReady = $state(false);
	let alertTimer: ReturnType<typeof setInterval> | null = null;
	let sysTimer: ReturnType<typeof setInterval> | null = null;
	let clockTimer: ReturnType<typeof setInterval> | null = null;

	const AUTH_PAGES = ['/login', '/setup'];
	$: isAuthPage = AUTH_PAGES.includes(page.url.pathname);

	async function checkAuth() {
		if (isAuthPage) { authReady = true; return; }
		try {
			const status = await api.auth.status();
			if (!status.configured) { await goto('/setup'); return; }
			if (!status.authenticated) { await goto('/login'); return; }
		} catch {
			// hub unreachable — allow through, individual pages will show errors
		}
		authReady = true;
	}

	async function refreshFiring() {
		try {
			const evs = await api.alertEvents({ openOnly: true, limit: 200 });
			firing = evs.length;
		} catch {
			// silent — badge hides when API is unreachable
		}
	}

	async function refreshSystem() {
		try {
			sys = await api.systemInfo();
		} catch {
			// silent — footer hides when API is unreachable
		}
	}

	let uptimeSecs = $derived(
		sys ? Math.max(0, Math.floor((now - new Date(sys.started_at).getTime()) / 1000)) : 0
	);

	onMount(() => {
		checkAuth();
		if (!isAuthPage) {
			refreshFiring();
			refreshSystem();
			alertTimer = setInterval(refreshFiring, 5000);
			sysTimer   = setInterval(refreshSystem, 30000);
		}
		clockTimer = setInterval(() => (now = Date.now()), 1000);
	});
	onDestroy(() => {
		if (alertTimer) clearInterval(alertTimer);
		if (sysTimer)   clearInterval(sysTimer);
		if (clockTimer) clearInterval(clockTimer);
	});
</script>

{#if !isAuthPage}
<header>
	<div class="brand">
		<span class="dot"></span>
		<a href="/">Aperture</a>
	</div>
	<nav>
		<a href="/" class:active={page.url.pathname === '/'}>Hosts</a>
		<a href="/alerts" class:active={page.url.pathname.startsWith('/alerts')}>
			Alerts
			{#if firing > 0}
				<span class="badge">{firing}</span>
			{/if}
		</a>
		<a href="/settings" class:active={page.url.pathname.startsWith('/settings')}>Settings</a>
	</nav>
</header>
{/if}

<main class:auth-page={isAuthPage}>
	{#if authReady}
		{@render children()}
	{/if}
</main>

{#if !isAuthPage}
<footer>
	{#if sys}
		<span class="mono">v{sys.version}</span>
		<span class="sep">·</span>
		<span title={sys.db_path}>DB {formatBytes(sys.db_size_bytes)}</span>
		<span class="sep">·</span>
		<span>uptime {formatDuration(uptimeSecs)}</span>
	{:else}
		<span>—</span>
	{/if}
</footer>

<Toast />
{/if}

<style>
	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 24px;
		background: var(--bg-elev);
		border-bottom: 1px solid var(--border);
		position: sticky;
		top: 0;
		z-index: 10;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-weight: 600;
		font-size: 16px;
	}
	.brand :global(a) { color: var(--text); }
	.brand :global(a:hover) { text-decoration: none; }
	.dot {
		width: 10px; height: 10px;
		border-radius: 50%;
		background: var(--accent);
		box-shadow: 0 0 8px var(--accent);
	}
	nav { display: flex; gap: 16px; align-items: center; }
	nav :global(a) {
		color: var(--text-dim);
		font-size: 13px;
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	nav :global(a.active) { color: var(--text); }
	.badge {
		background: var(--bad);
		color: white;
		font-size: 10px;
		font-weight: 600;
		padding: 1px 6px;
		border-radius: 999px;
		min-width: 16px;
		text-align: center;
	}
	main {
		padding: 24px;
		max-width: 1400px;
		margin: 0 auto;
	}
	main.auth-page {
		max-width: 100%;
		padding: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
	}
	footer {
		margin-top: 24px;
		padding: 12px 24px;
		border-top: 1px solid var(--border);
		background: var(--bg-elev);
		color: var(--text-dim);
		font-size: 11px;
		display: flex;
		gap: 8px;
		align-items: center;
		justify-content: center;
	}
	footer .sep { opacity: 0.4; }
	footer .mono { font-family: var(--mono); }
</style>
