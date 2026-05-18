<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id);
</script>

<svelte:head><title>Aperture — Shell · {id}</title></svelte:head>

<div class="page">
	<header class="page-header">
		<div>
			<a href={`/hosts/${id}/overview`} class="back">← back to host</a>
			<h1>Shell</h1>
		</div>
	</header>

	<nav class="sub-nav">
		<a href={`/hosts/${id}/overview`}>Overview</a>
		<a href={`/hosts/${id}/containers`}>Containers</a>
		<a href={`/hosts/${id}/stacks`}>Stacks</a>
		<a href={`/hosts/${id}/networks`}>Networks</a>
		<a href={`/hosts/${id}/logs`}>Logs</a>
		<a href={`/hosts/${id}/volumes`}>Volumes</a>
		<a href={`/hosts/${id}/images`}>Images</a>
		<a href={`/hosts/${id}/shell`} class="active">Shell</a>
	</nav>

	<div class="stub">
		<div class="icon"><Icon name="shell" size={32} /></div>
		<h2>Host shell coming soon</h2>
		<p>
			Container-level shells are available from the
			<a href={`/hosts/${id}/containers`}>Containers</a> tab.
		</p>
	</div>
</div>

<style>
	.page { display: flex; flex-direction: column; gap: 14px; }
	.page-header { display: flex; justify-content: space-between; align-items: flex-start; }
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }

	.sub-nav {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border, var(--line));
	}
	.sub-nav a {
		padding: 8px 16px;
		font-size: 13px;
		color: var(--text-dim);
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		text-decoration: none;
	}
	.sub-nav a:hover { color: var(--text); }
	.sub-nav a.active { color: var(--accent); border-bottom-color: var(--accent); }

	.stub {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 50vh;
		gap: 10px;
		color: var(--text-faint);
		text-align: center;
	}
	.icon { opacity: 0.4; }
	h2 { font-size: 16px; color: var(--text-dim); margin: 0; }
	.stub p { font-size: 13px; margin: 0; }
	.stub p a { color: var(--accent); }
</style>
