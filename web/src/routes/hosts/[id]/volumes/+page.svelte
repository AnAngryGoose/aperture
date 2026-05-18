<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerVolume, VolumeCreateSpec } from '$lib/types';
	import { formatBytes, relTime, absTime } from '$lib/format';

	let id = $derived(page.params.id);
	let hostName = $state('');
	let volumes = $state<DockerVolume[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let timer: ReturnType<typeof setInterval> | null = null;

	let inspectVolName = $state<string | null>(null);
	let inspectData = $state<DockerVolume | null>(null);
	let inspectLoading = $state(false);

	let showCreate = $state(false);
	let creating = $state(false);
	let createError = $state<string | null>(null);

	const blankForm = (): VolumeCreateSpec => ({
		name: '',
		driver: 'local',
		driver_opts: {},
		labels: {}
	});
	let form = $state(blankForm());
	
	let optKey = $state('');
	let optVal = $state('');
	let labelKey = $state('');
	let labelVal = $state('');

	async function refresh() {
		try {
			volumes = await api.volumes(id);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function openInspect(name: string) {
		if (inspectVolName === name) { inspectVolName = null; inspectData = null; return; }
		inspectVolName = name;
		inspectData = null;
		inspectLoading = true;
		try {
			inspectData = await api.volumeInspect(id, name);
		} catch (e) {
			error = (e as Error).message;
			inspectVolName = null;
		} finally {
			inspectLoading = false;
		}
	}

	async function remove(name: string, force = false) {
		if (!confirm(`Remove volume ${name.slice(0, 12)}?`)) return;
		busy[name] = true;
		try {
			await api.removeVolume(id, name, force);
			if (inspectVolName === name) { inspectVolName = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[name] = false;
		}
	}

	function openCreate() { 
		form = blankForm(); 
		optKey = ''; optVal = ''; 
		labelKey = ''; labelVal = ''; 
		createError = null; 
		showCreate = true; 
	}

	async function submitCreate(ev: Event) {
		ev.preventDefault();
		createError = null;
		if (!form.name.trim()) { createError = 'name is required'; return; }
		creating = true;
		try {
			await api.createVolume(id, { ...form, name: form.name.trim() });
			showCreate = false;
			await refresh();
		} catch (e) {
			createError = (e as Error).message;
		} finally {
			creating = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (showCreate) { showCreate = false; return; }
		if (inspectVolName) { inspectVolName = null; inspectData = null; return; }
	}

	onMount(async () => {
		refresh();
		timer = setInterval(refresh, 5000);
		try { const h = await api.host(id); hostName = h.name; } catch { /* best-effort */ }
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<svelte:head><title>Aperture — {hostName || id} — Volumes</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<div>
		<a href={`/hosts/${id}/overview`} class="back">← back to host</a>
		<h1>Volumes</h1>
		<div class="muted small">{volumes.length} total</div>
	</div>
	<button onclick={openCreate}>+ New volume</button>
</div>

<!-- Sub-navigation -->
<nav class="subnav">
	<a href={`/hosts/${id}/overview`}>Overview</a>
	<a href={`/hosts/${id}/containers`}>Containers</a>
	<a href={`/hosts/${id}/stacks`}>Stacks</a>
	<a href={`/hosts/${id}/networks`}>Networks</a>
	<a href={`/hosts/${id}/logs`}>Logs</a>
	<a href={`/hosts/${id}/volumes`} class="active">Volumes</a>
	<a href={`/hosts/${id}/images`}>Images</a>
</nav>

{#if error}
	<div class="card err">{error} <button class="x" onclick={() => (error = null)}>×</button></div>
{/if}

<div class="card no-pad">
	<table>
		<thead>
			<tr>
				<th>Name</th>
				<th>Driver</th>
				<th>Size</th>
				<th>Usage</th>
				<th>Created</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>
			{#each volumes as v (v.name)}
				<tr class:expanded={inspectVolName === v.name} onclick={() => openInspect(v.name)}>
					<td>
						<div class="vname">{v.name}</div>
					</td>
					<td><span class="pill">{v.driver}</span></td>
					<td class="mono small">{formatBytes(v.size_bytes)}</td>
					<td>
						{#if v.ref_count > 0}
							<span class="pill usage">In use ({v.ref_count})</span>
						{:else}
							<span class="pill unused">Unused</span>
						{/if}
					</td>
					<td class="mono small muted" title={absTime(v.created_at)}>{relTime(v.created_at)}</td>
					<td class="actions" onclick={(e) => e.stopPropagation()}>
						<button class="danger" disabled={busy[v.name]} onclick={() => remove(v.name)}>Remove</button>
					</td>
				</tr>

				<!-- Expand: deep inspect panel -->
				{#if inspectVolName === v.name}
					<tr class="inspect-row">
						<td colspan="6" class="inspect-cell">
							{#if inspectLoading}
								<div class="muted small">Loading…</div>
							{:else if inspectData}
								<div class="inspect-grid">
									<!-- Left: config -->
									<div class="inspect-col">
										<div class="inspect-section">
											<div class="section-label">Configuration</div>
											<div class="kv">
												<span class="k">Name</span><span class="v mono">{inspectData.name}</span>
												<span class="k">Driver</span><span class="v mono">{inspectData.driver}</span>
												<span class="k">Scope</span><span class="v mono">{inspectData.scope}</span>
												<span class="k">Mountpoint</span><span class="v mono small">{inspectData.mountpoint}</span>
											</div>
										</div>

										{#if inspectData.options && Object.keys(inspectData.options).length > 0}
											<div class="inspect-section">
												<div class="section-label">Driver Options</div>
												<div class="env-list">
													{#each Object.entries(inspectData.options) as [k,v]}
														<div class="mono small">{k}={v}</div>
													{/each}
												</div>
											</div>
										{/if}

										{#if Object.keys(inspectData.labels || {}).length > 0}
											<div class="inspect-section">
												<div class="section-label">Labels</div>
												<div class="env-list">
													{#each Object.entries(inspectData.labels || {}) as [k,v]}
														<div class="mono small">{k}={v}</div>
													{/each}
												</div>
											</div>
										{/if}
									</div>

									<!-- Right: usage / actions -->
									<div class="inspect-col">
										<div class="inspect-section">
											<div class="section-label">Usage</div>
											<div class="kv">
												<span class="k">Total Size</span><span class="v mono">{formatBytes(inspectData.size_bytes)}</span>
												<span class="k">Containers</span><span class="v mono">{inspectData.ref_count} references</span>
											</div>
										</div>
										<div class="inspect-section">
											<div class="section-label">Actions</div>
											<div class="action-buttons">
												<button class="danger" disabled={busy[v.name]} onclick={() => remove(v.name)}>Remove Volume</button>
												<button class="danger force" disabled={busy[v.name]} onclick={() => remove(v.name, true)} title="Force remove the volume, even if it is in use">Force Remove</button>
											</div>
										</div>
									</div>
								</div>
							{/if}
						</td>
					</tr>
				{/if}
			{/each}
			{#if volumes.length === 0 && !error}
				<tr><td colspan="6" class="muted center">no volumes</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<!-- Create modal -->
{#if showCreate}
	<div class="modal-bg" onclick={() => (showCreate = false)} role="presentation">
		<div class="modal create" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
			<div class="modal-head">
				<div>New volume</div>
				<button onclick={() => (showCreate = false)}>close</button>
			</div>
			<form onsubmit={submitCreate} class="create-form">
				<label class="row">
					<span>Name *</span>
					<input type="text" placeholder="my-volume" bind:value={form.name} required />
				</label>
				<label class="row">
					<span>Driver</span>
					<input type="text" placeholder="local" bind:value={form.driver} />
				</label>

				<div class="env-editor">
					<div class="section-label">Driver Options</div>
					{#each Object.entries(form.driver_opts || {}) as [k, v]}
						<div class="env-row">
							<input type="text" readonly value={k} class="mono small" />
							<input type="text" readonly value={v} class="mono small" />
							<button type="button" class="x" onclick={() => { const o = { ...form.driver_opts }; delete o[k]; form.driver_opts = o; }}>×</button>
						</div>
					{/each}
					<div class="env-row new-env">
						<input type="text" placeholder="type" bind:value={optKey} class="mono small" />
						<input type="text" placeholder="nfs" bind:value={optVal} class="mono small" />
						<button type="button" onclick={() => { if(optKey) { form.driver_opts = {...form.driver_opts, [optKey]: optVal}; optKey=''; optVal=''; } }}>Add</button>
					</div>
				</div>

				<div class="env-editor mt">
					<div class="section-label">Labels</div>
					{#each Object.entries(form.labels || {}) as [k, v]}
						<div class="env-row">
							<input type="text" readonly value={k} class="mono small" />
							<input type="text" readonly value={v} class="mono small" />
							<button type="button" class="x" onclick={() => { const l = { ...form.labels }; delete l[k]; form.labels = l; }}>×</button>
						</div>
					{/each}
					<div class="env-row new-env">
						<input type="text" placeholder="key" bind:value={labelKey} class="mono small" />
						<input type="text" placeholder="value" bind:value={labelVal} class="mono small" />
						<button type="button" onclick={() => { if(labelKey) { form.labels = {...form.labels, [labelKey]: labelVal}; labelKey=''; labelVal=''; } }}>Add</button>
					</div>
				</div>

				{#if createError}
					<div class="err-box">{createError}</div>
				{/if}

				<div class="foot">
					<button type="button" onclick={() => (showCreate = false)}>Cancel</button>
					<button type="submit" class="primary" disabled={creating || !form.name.trim()}>{creating ? 'Creating…' : 'Create'}</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<style>
	.page-header { display: flex; justify-content: space-between; align-items: flex-end; margin-bottom: 20px; }
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; display: flex; align-items: center; gap: 10px; }
	.small { font-size: 11px; }

	/* Subnav */
	.subnav { display: flex; gap: 0; margin-bottom: 20px; border-bottom: 1px solid var(--border); }
	.subnav a { padding: 8px 16px; font-size: 13px; color: var(--text-dim); border-bottom: 2px solid transparent; margin-bottom: -1px; }
	.subnav a:hover { color: var(--text); text-decoration: none; }
	.subnav a.active { color: var(--accent); border-bottom-color: var(--accent); }
	.subnav a.placeholder { opacity: 0.45; cursor: default; }

	.vname { font-weight: 500; margin-bottom: 2px; }
	.pill {
		display: inline-block; padding: 2px 6px; border-radius: 4px;
		background: var(--bg-elev); font-size: 11px; color: var(--text-dim);
	}
	.usage { background: rgba(92,200,255,0.1); color: var(--accent); border: 1px solid rgba(92,200,255,0.2); }
	.unused { background: var(--bg-elev-2); border: 1px solid var(--border); }
	
	.err { color: var(--bad); border-color: var(--bad); margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; }
	button.x { background: transparent; border: none; font-size: 16px; color: var(--bad); padding: 0 4px; }
	
	table { width: 100%; border-collapse: collapse; font-size: 13px; }
	th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid var(--border); }
	th { font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-dim); font-weight: 600; }
	tr { transition: background 0.15s; }
	tr:hover:not(.inspect-row) { background: var(--bg-elev); cursor: pointer; }
	tr.expanded { background: var(--bg-elev); border-left: 2px solid var(--accent); }

	.actions { text-align: right; }
	.actions button { font-size: 11px; padding: 4px 8px; }
	button.danger { border-color: rgba(255,107,107,0.3); color: var(--bad); }
	button.danger:hover { background: rgba(255,107,107,0.1); border-color: var(--bad); }
	button.danger.force { background: rgba(255,107,107,0.05); }

	/* Deep inspect */
	.inspect-row { background: var(--bg-elev-2) !important; border-bottom: 2px solid var(--border) !important; }
	.inspect-cell { padding: 16px 20px; }
	.inspect-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 32px; }
	.inspect-section { margin-bottom: 20px; }
	.section-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-dim); margin-bottom: 8px; font-weight: 600; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 6px; font-size: 13px; }
	.k { color: var(--text-dim); }
	.env-list { background: var(--bg); padding: 8px 12px; border-radius: 4px; display: flex; flex-direction: column; gap: 4px; max-height: 150px; overflow-y: auto; border: 1px solid var(--border); }
	.action-buttons { display: flex; gap: 8px; flex-direction: column; max-width: 150px; }

	/* Modal */
	.modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px); z-index: 100; display: flex; align-items: center; justify-content: center; }
	.modal { background: var(--bg-elev); border: 1px solid var(--border); border-radius: 8px; width: 440px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); display: flex; flex-direction: column; max-height: 90vh; }
	.modal-head { padding: 16px 20px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; font-weight: 600; }
	.modal-head button { font-size: 12px; padding: 4px 8px; }
	.create-form { padding: 20px; overflow-y: auto; }
	.row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 16px; }
	.row span { font-size: 12px; color: var(--text-dim); font-weight: 500; }
	.row input, .env-row input { background: var(--bg); border: 1px solid var(--border); padding: 8px 12px; color: var(--text); border-radius: 4px; font-size: 13px; width: 100%; box-sizing: border-box; }
	.row input:focus, .env-row input:focus { outline: none; border-color: var(--accent); }
	
	.env-editor.mt { margin-top: 16px; }
	.env-row { display: flex; gap: 6px; margin-bottom: 6px; align-items: center; }
	.env-row button { padding: 4px 10px; font-size: 12px; }
	.new-env { margin-top: 8px; }
	
	.err-box { padding: 10px; margin-top: 16px; background: rgba(255,107,107,0.1); border: 1px solid var(--bad); color: var(--bad); font-size: 13px; border-radius: 4px; }
	.foot { margin-top: 24px; display: flex; justify-content: flex-end; gap: 12px; }
	button.primary { background: var(--accent); color: #fff; border-color: var(--accent); }
	button.primary:hover:not(:disabled) { background: #4bb3e6; }
</style>
