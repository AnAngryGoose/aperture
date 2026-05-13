<script lang="ts">
	import 'uplot/dist/uPlot.min.css';
	import '$lib/styles/global.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import Toast from '$lib/Toast.svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';

	let { children } = $props();
	let authReady = $state(false);

	const AUTH_PAGES = ['/login', '/setup'];
	const isAuthPage = $derived(AUTH_PAGES.some((p) => page.url.pathname.startsWith(p)));

	async function checkAuth() {
		if (isAuthPage) { authReady = true; return; }
		try {
			const status = await api.auth.status();
			if (!status.configured) { await goto('/setup'); return; }
			if (!status.authenticated) { await goto('/login'); return; }
		} catch {
			// hub unreachable — allow through
		}
		authReady = true;
	}

	// Re-run on every URL change so returning from /login sets authReady correctly.
	// onMount fires only once; navigating /login → / would leave authReady=false forever.
	$effect(() => {
		if (!isAuthPage && !authReady) checkAuth();
	});
</script>

{#if isAuthPage}
	<div class="auth-wrap">
		{@render children()}
	</div>
{:else if authReady}
	<AppShell>
		{@render children()}
	</AppShell>
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
</style>
