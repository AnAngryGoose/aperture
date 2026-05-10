<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Host } from '$lib/types';
	import { onMount } from 'svelte';

	let id = $derived(page.params.id);
	let host = $state<Host | null>(null);

	onMount(async () => {
		try { host = await api.host(id); } catch {}
	});
</script>

<div class="page-header">
	<div>
		<a href="/" class="back">← all hosts</a>
		<h1>{host?.name ?? id}</h1>
	</div>
</div>

<nav class="subnav">
	<a href={`/hosts/${id}`}>Overview</a>
	<a href={`/hosts/${id}/containers`}>Containers</a>
	<a href={`/hosts/${id}/networks`} class="">Networks</a>
	<a href={`/hosts/${id}/volumes`} class="">Volumes</a>
	<a href={`/hosts/${id}/images`} class="active">Images</a>
	<a href={`/hosts/${id}/logs`} class="placeholder">Logs</a>
</nav>

<div class="card placeholder-card">
	<div class="placeholder-content">
		<div class="placeholder-icon">⬡</div>
		<h2>Images</h2>
		<p>Docker image inventory, pull, tag, and prune controls coming in a future release.</p>
	</div>
</div>

<style>
	.placeholder-card {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 300px;
	}
	.placeholder-content {
		text-align: center;
		color: var(--text-dim);
	}
	.placeholder-icon {
		font-size: 48px;
		margin-bottom: 16px;
		opacity: 0.4;
	}
	h2 { margin: 0 0 8px; font-size: 18px; color: var(--text-dim); }
	p { margin: 0; font-size: 13px; max-width: 320px; }
</style>
