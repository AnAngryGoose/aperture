<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerNetwork, NetworkCreateSpec } from '$lib/types';

	let id = $derived(page.params.id);
	let hostName = $state('');
	let networks = $state<DockerNetwork[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let timer: ReturnType<typeof setInterval> | null = null;

	let inspectNetID = $state<string | null>(null);
	let inspectData = $state<DockerNetwork | null>(null);
	let inspectLoading = $state(false);

	let showCreate = $state(false);
	let creating = $state(false);
	let createError = $state<string | null>(null);

	const blankForm = (): NetworkCreateSpec => ({
		name: '',
		driver: 'bridge',
		internal: false,
		labels: {}
	});
	let form = $state(blankForm());

	async function refresh() {
		try {
			networks = await api.networks(id);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function openInspect(netID: string) {
		if (inspectNetID === netID) { inspectNetID = null; inspectData = null; return; }
		inspectNetID = netID;
		inspectData = null;
		inspectLoading = true;
		try {
			inspectData = await api.networkInspect(id, netID);
		} catch (e) {
			error = (e as Error).message;
			inspectNetID = null;
		} finally {
			inspectLoading = false;
		}
	}

	async function remove(netID: string) {
		if (!confirm(`Remove network ${netID.slice(0, 12)}?`)) return;
		busy[netID] = true;
		try {
			await api.removeNetwork(id, netID);
			if (inspectNetID === netID) { inspectNetID = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[netID] = false;
		}
	}

	function openCreate() { form = blankForm(); createError = null; showCreate = true; }

	async function submitCreate(ev: Event) {
		ev.preventDefault();
		createError = null;
		if (!form.name.trim()) { createError = 'name is required'; return; }
		creating = true;
		try {
			await api.createNetwork(id, { ...form, name: form.name.trim() });
			showCreate = false;
			await refresh();
		} catch (e) {
			createError = (e as Error).message;
		} finally {
			creating = false;
		}
	}

	let disconnectContainerID = $state('');
	async function disconnect(netID: string, containerID: string) {
		if (!confirm(`Disconnect container from network?`)) return;
		busy[netID] = true;
		try {
			await api.disconnectNetwork(id, netID, containerID);
			inspectData = await api.networkInspect(id, netID);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[netID] = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (showCreate) { showCreate = false; return; }
		if (inspectNetID) { inspectNetID = null; inspectData = null; return; }
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

<svelte:head><title>Aperture — {hostName || id} — Networks</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<div>
		<a href={`/hosts/${id}`} class="back">← back to host</a>
		<h1>Networks</h1>
		<div class="muted small">{networks.length} total</div>
	</div>
	<button onclick={openCreate}>+ New network</button>
</div>

<!-- Sub-navigation -->
<nav class="subnav">
	<a href={`/hosts/${id}`}>Overview</a>
	<a href={`/hosts/${id}/containers`}>Containers</a>
	<a href={`/hosts/${id}/compose`}>Compose</a>
	<a href={`/hosts/${id}/networks`} class="active">Networks</a>
	<a href={`/hosts/${id}/volumes`} class="">Volumes</a>
	<a href={`/hosts/${id}/images`} class="placeholder">Images</a>
	<a href={`/hosts/${id}/logs`} class="placeholder">Logs</a>
</nav>

{#if error}
	<div class="card err">{error} <button class="x" onclick={() => (error = null)}>×</button></div>
{/if}

<div class="card no-pad">
	<table>
		<thead>
			<tr>
				<th>Name / ID</th>
				<th>Driver</th>
				<th>Scope</th>
				<th>Subnet</th>
				<th>Gateway</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>
			{#each networks as n (n.id)}
				<tr class:expanded={inspectNetID === n.id} onclick={() => openInspect(n.id)}>
					<td>
						<div class="nname">{n.name}</div>
						<div class="muted mono small">{n.id.slice(0, 12)}</div>
					</td>
					<td><span class="pill">{n.driver}</span></td>
					<td><span class="pill">{n.scope}</span></td>
					<td class="mono small">{n.subnet || '—'}</td>
					<td class="mono small">{n.gateway || '—'}</td>
					<td class="actions" onclick={(e) => e.stopPropagation()}>
						<button class="danger" disabled={busy[n.id]} onclick={() => remove(n.id)}>Remove</button>
					</td>
				</tr>

				<!-- Expand: deep inspect panel -->
				{#if inspectNetID === n.id}
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
												<span class="k">ID</span><span class="v mono">{inspectData.id}</span>
												<span class="k">Driver</span><span class="v mono">{inspectData.driver}</span>
												<span class="k">Scope</span><span class="v mono">{inspectData.scope}</span>
												<span class="k">Internal</span><span class="v mono">{inspectData.internal ? 'true' : 'false'}</span>
											</div>
										</div>

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

									<!-- Right: containers -->
									<div class="inspect-col">
										<div class="inspect-section">
											<div class="section-label">Connected Containers ({inspectData.containers?.length || 0})</div>
											{#if inspectData.containers && inspectData.containers.length > 0}
												<div class="container-list">
													{#each inspectData.containers as c}
														<div class="container-item">
															<div>
																<div class="cname">{c.name}</div>
																<div class="mono small muted">{c.id.slice(0, 12)}</div>
																<div class="mono small">{c.ipv4_address || c.ipv6_address}</div>
															</div>
															<button disabled={busy[n.id]} onclick={() => disconnect(n.id, c.id)}>Disconnect</button>
														</div>
													{/each}
												</div>
											{:else}
												<div class="muted small">No containers connected</div>
											{/if}
										</div>
									</div>
								</div>
							{/if}
						</td>
					</tr>
				{/if}
			{/each}
			{#if networks.length === 0 && !error}
				<tr><td colspan="6" class="muted center">no networks</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<!-- Create modal -->
{#if showCreate}
	<div class="modal-bg" onclick={() => (showCreate = false)} role="presentation">
		<div class="modal create" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
			<div class="modal-head">
				<div>New network</div>
				<button onclick={() => (showCreate = false)}>close</button>
			</div>
			<form onsubmit={submitCreate} class="create-form">
				<label class="row">
					<span>Name *</span>
					<input type="text" placeholder="my-network" bind:value={form.name} />
				</label>
				<label class="row">
					<span>Driver</span>
					<select bind:value={form.driver}>
						<option value="bridge">bridge</option>
						<option value="macvlan">macvlan</option>
						<option value="ipvlan">ipvlan</option>
						<option value="overlay">overlay</option>
					</select>
				</label>
				<label class="row checkbox">
					<input type="checkbox" bind:checked={form.internal} />
					<span>Internal</span>
				</label>

				{#if createError}
					<div class="err-text">{createError}</div>
				{/if}
				<div class="form-actions">
					<button type="button" onclick={() => (showCreate = false)}>Cancel</button>
					<button type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create'}</button>
				</div>
			</form>
		</div>
	</div>
{/if}

<style>
	.page-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px; }
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }
	.small { font-size: 11px; }

	.subnav {
		display: flex;
		gap: 0;
		margin-bottom: 16px;
		border-bottom: 1px solid var(--border);
	}
	.subnav a {
		padding: 8px 16px;
		font-size: 13px;
		color: var(--text-dim);
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
	}
	.subnav a:hover { color: var(--text); text-decoration: none; }
	.subnav a.active { color: var(--accent); border-bottom-color: var(--accent); }
	.subnav a.placeholder { opacity: 0.45; cursor: default; }
	.subnav a.placeholder:hover { color: var(--text-dim); }

	.no-pad { padding: 0; }
	.nname { font-weight: 500; }
	tr.expanded td { background: var(--bg-elev-2); }
	tr { cursor: pointer; }
	tr:hover td { background: var(--bg-elev-2); }
	.actions { display: flex; gap: 4px; flex-wrap: wrap; cursor: default; }
	.center { text-align: center; padding: 32px; cursor: default; }
	.err { color: var(--bad); border-color: var(--bad); display: flex; justify-content: space-between; align-items: center; }

	.inspect-row td { cursor: default; }
	.inspect-row:hover td { background: var(--bg-elev-2); }
	.inspect-cell { padding: 16px 20px; background: var(--bg-elev-2); border-top: 1px solid var(--border); }
	.inspect-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
	@media (max-width: 900px) { .inspect-grid { grid-template-columns: 1fr; } }
	.inspect-col { display: flex; flex-direction: column; gap: 16px; }
	.inspect-section { display: flex; flex-direction: column; gap: 6px; }
	.section-label {
		font-size: 10px;
		font-weight: 700;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-dim);
		padding-bottom: 4px;
		border-bottom: 1px solid var(--border);
		margin-bottom: 2px;
	}

	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-dim); }
	.v { font-family: var(--mono); word-break: break-all; }

	.env-list {
		max-height: 200px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.container-list {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.container-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 8px;
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 4px;
	}
	.cname { font-weight: 500; font-size: 13px; }

	/* Modals */
	.modal-bg {
		position: fixed; inset: 0;
		background: rgba(0,0,0,0.6);
		display: flex; align-items: center; justify-content: center;
		z-index: 100;
	}
	.modal {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 6px;
		width: min(90vw, 600px);
		max-height: 85vh;
		display: flex; flex-direction: column;
	}
	.modal-head {
		padding: 10px 16px;
		display: flex; justify-content: space-between; align-items: center;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.create-form {
		padding: 16px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.create-form .row { display: grid; grid-template-columns: 130px 1fr; gap: 10px; align-items: center; }
	.create-form .row > span { font-size: 12px; color: var(--text-dim); }
	.create-form .row.checkbox { grid-template-columns: 130px auto 1fr; }
	.create-form input[type="text"],
	.create-form select {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 6px 8px;
		font: inherit;
	}
	.err-text { color: var(--bad); font-size: 11px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 10px; }
</style>
