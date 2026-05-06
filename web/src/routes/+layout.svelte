<script lang="ts">
	import '$lib/styles.css';
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';

	let { children } = $props();
	let firing = $state(0);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refreshFiring() {
		try {
			const evs = await api.alertEvents({ openOnly: true, limit: 200 });
			firing = evs.length;
		} catch {
			// ignore — banner already surfaces page-level errors
		}
	}

	onMount(() => {
		refreshFiring();
		timer = setInterval(refreshFiring, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<svelte:head>
	<title>Aperture</title>
</svelte:head>

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
	</nav>
</header>

<main>
	{@render children()}
</main>

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
</style>
