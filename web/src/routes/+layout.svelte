<script lang="ts">
	import 'uplot/dist/uPlot.min.css';
	import '$lib/styles/global.css';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import Toast from '$lib/Toast.svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { accent } from '$lib/stores/accent.svelte';

	let { children } = $props();
	let authReady = $state(false);
	let bootError = $state<string | null>(null);

	const AUTH_PAGES = ['/login', '/setup'];
	const isAuthPage = $derived(AUTH_PAGES.some((p) => page.url.pathname.startsWith(p)));

	async function checkAuth() {
		if (isAuthPage) { authReady = true; return; }
		try {
			const status = await api.auth.status();
			if (!status.configured) { await goto('/setup'); return; }
			if (!status.authenticated) { await goto('/login'); return; }
		} catch (e) {
			// hub unreachable — surface the error rather than silently allowing through.
			bootError = e instanceof Error ? e.message : 'Hub unreachable';
		}
		authReady = true;
	}

	// Initialize theme + accent before first paint of any page (auth or not),
	// so the login page also picks up the user's saved preferences.
	onMount(() => {
		theme.init();
		accent.init();
		// Global error surface so JS crashes never produce a silent black screen.
		window.addEventListener('error', (e) => {
			if (!bootError) bootError = e.message || 'A script error occurred.';
		});
		window.addEventListener('unhandledrejection', (e) => {
			if (!bootError) bootError = String(e.reason ?? 'Unhandled rejection');
		});
	});

	// Re-run on every URL change so returning from /login sets authReady correctly.
	// onMount fires only once; navigating /login → / would leave authReady=false forever.
	$effect(() => {
		if (!isAuthPage && !authReady) checkAuth();
	});
</script>

{#if bootError}
	<div class="boot-error">
		<div class="boot-card">
			<h2>Aperture failed to load</h2>
			<p class="msg mono">{bootError}</p>
			<button onclick={() => location.reload()}>Reload</button>
		</div>
	</div>
{:else if isAuthPage}
	<div class="auth-wrap">
		{@render children()}
	</div>
{:else if authReady}
	<AppShell>
		{@render children()}
	</AppShell>
{:else}
	<div class="boot-loader" aria-busy="true">
		<div class="spinner"></div>
		<span class="boot-text mono">Loading Aperture…</span>
	</div>
{/if}

<Toast />

<style>
	.auth-wrap {
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg);
	}

	.boot-loader {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 16px;
		background: var(--bg);
		color: var(--text-dim);
	}

	.spinner {
		width: 28px;
		height: 28px;
		border: 2px solid var(--line);
		border-top-color: var(--accent);
		border-radius: 50%;
		animation: boot-spin 800ms linear infinite;
	}

	@keyframes boot-spin {
		to { transform: rotate(360deg); }
	}

	.boot-text {
		font-size: 12px;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--text-faint);
	}

	.boot-error {
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		background: var(--bg);
		padding: 20px;
	}

	.boot-card {
		max-width: 480px;
		padding: 24px;
		background: var(--bg-elev);
		border: 1px solid var(--crit);
		border-radius: var(--r-lg);
		text-align: center;
	}

	.boot-card h2 {
		margin: 0 0 8px;
		font-size: 16px;
		color: var(--crit);
	}

	.boot-card .msg {
		font-size: 12px;
		color: var(--text-dim);
		word-break: break-word;
		margin-bottom: 16px;
	}

	.boot-card button {
		padding: 7px 16px;
		font-size: 13px;
		color: #fff;
		background: var(--accent);
		border: none;
		border-radius: var(--r-md);
		cursor: pointer;
	}
</style>
