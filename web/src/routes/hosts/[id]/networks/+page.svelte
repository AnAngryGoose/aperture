<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerNetwork, NetworkCreateSpec } from '$lib/types';
	import Button from '$lib/components/primitives/Button.svelte';
	import Modal from '$lib/components/primitives/Modal.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
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

	// Default/protected networks shipped with Docker that should not be removed.
	const PROTECTED = new Set(['bridge', 'host', 'none']);
	function isProtected(n: DockerNetwork): boolean { return PROTECTED.has(n.name); }

	// Pending confirmations
	type Pending =
		| { kind: 'remove'; net: DockerNetwork }
		| { kind: 'disconnect'; netID: string; netName: string; cName: string; cid: string }
		| null;
	let pending = $state<Pending>(null);
	let pendingBusy = $state(false);

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

	async function doRemove(net: DockerNetwork) {
		busy[net.id] = true;
		pendingBusy = true;
		try {
			await api.removeNetwork(id, net.id);
			if (inspectNetID === net.id) { inspectNetID = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[net.id] = false;
			pendingBusy = false;
			pending = null;
		}
	}

	async function doDisconnect(netID: string, cid: string) {
		busy[netID] = true;
		pendingBusy = true;
		try {
			await api.disconnectNetwork(id, netID, cid);
			inspectData = await api.networkInspect(id, netID);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[netID] = false;
			pendingBusy = false;
			pending = null;
		}
	}

	function runPending() {
		if (!pending) return;
		if (pending.kind === 'remove') doRemove(pending.net);
		else doDisconnect(pending.netID, pending.cid);
	}

	function openCreate() { form = blankForm(); createError = null; showCreate = true; }

	async function submitCreate(ev: Event) {
		ev.preventDefault();
		createError = null;
		if (!form.name.trim()) { createError = 'Name is required'; return; }
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

	function handleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (showCreate) { showCreate = false; return; }
		if (pending) { pending = null; return; }
		if (inspectNetID) { inspectNetID = null; inspectData = null; return; }
	}

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

<svelte:window onkeydown={handleKeydown} />

<section class="networks-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Networks</h2>
			<span class="lead-sub mono">{networks.length} total</span>
		</div>
		<Button variant="primary" onclick={openCreate}>+ New Network</Button>
	</header>

	{#if error}
		<div class="error-banner">
			<span>{error}</span>
			<Button variant="icon" size="sm" ariaLabel="Dismiss" onclick={() => (error = null)}>
				<Icon name="x" size={12} />
			</Button>
		</div>
	{/if}

	<div class="table-wrap">
		<table>
			<thead>
				<tr>
					<th>Name / ID</th>
					<th>Driver</th>
					<th>Scope</th>
					<th>Subnet</th>
					<th>Gateway</th>
					<th class="actions-col">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each networks as n (n.id)}
					<tr class:expanded={inspectNetID === n.id} onclick={() => openInspect(n.id)}>
						<td>
							<div class="nname">
								{n.name}
								{#if isProtected(n)}<span class="badge mono">default</span>{/if}
							</div>
							<div class="muted mono micro">{n.id.slice(0, 12)}</div>
						</td>
						<td><span class="pill">{n.driver}</span></td>
						<td><span class="pill">{n.scope}</span></td>
						<td class="mono micro">{n.subnet || '—'}</td>
						<td class="mono micro">{n.gateway || '—'}</td>
						<td class="actions" onclick={(e) => e.stopPropagation()}>
							<Button
								variant="danger"
								size="sm"
								disabled={busy[n.id] || isProtected(n)}
								title={isProtected(n) ? 'Default Docker network — cannot be removed' : undefined}
								onclick={() => { pending = { kind: 'remove', net: n }; }}
							>
								Remove
							</Button>
						</td>
					</tr>

					{#if inspectNetID === n.id}
						<tr class="inspect-row">
							<td colspan="6" class="inspect-cell">
								{#if inspectLoading}
									<div class="muted micro">Loading…</div>
								{:else if inspectData}
									<div class="inspect-grid">
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
														{#each Object.entries(inspectData.labels || {}) as [k, v]}
															<div class="mono micro">{k}={v}</div>
														{/each}
													</div>
												</div>
											{/if}
										</div>

										<div class="inspect-col">
											<div class="inspect-section">
												<div class="section-label">Connected containers ({inspectData.containers?.length || 0})</div>
												{#if inspectData.containers && inspectData.containers.length > 0}
													<div class="container-list">
														{#each inspectData.containers as c}
															<div class="container-item">
																<div>
																	<div class="cname">{c.name}</div>
																	<div class="mono micro muted">{c.id.slice(0, 12)}</div>
																	<div class="mono micro">{c.ipv4_address || c.ipv6_address}</div>
																</div>
																<Button
																	variant="danger"
																	size="sm"
																	disabled={busy[n.id]}
																	onclick={() => { pending = { kind: 'disconnect', netID: n.id, netName: n.name, cName: c.name, cid: c.id }; }}
																>
																	Disconnect
																</Button>
															</div>
														{/each}
													</div>
												{:else}
													<div class="muted micro">No containers connected</div>
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
					<tr><td colspan="6" class="empty-row">No networks.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>
</section>

<Modal open={showCreate} onclose={() => (showCreate = false)} title="New network" width="480px">
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
			<Button variant="ghost" onclick={() => (showCreate = false)}>Cancel</Button>
			<Button variant="primary" type="submit" loading={creating}>{creating ? 'Creating…' : 'Create'}</Button>
		</div>
	</form>
</Modal>

<ConfirmDialog
	open={pending?.kind === 'remove'}
	tone="danger"
	title="Remove network"
	message="Remove this Docker network?"
	detail={pending?.kind === 'remove' ? pending.net.name : ''}
	consequences={[
		'Containers currently connected to this network must be disconnected first; otherwise this will fail.',
		'Any application relying on this network by name will need to be updated.'
	]}
	confirmLabel="Remove"
	busy={pendingBusy}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<ConfirmDialog
	open={pending?.kind === 'disconnect'}
	tone="warning"
	title="Disconnect container"
	message="Disconnect this container from the network?"
	detail={pending?.kind === 'disconnect' ? `${pending.cName} ↛ ${pending.netName}` : ''}
	consequences={[
		'The container loses its IP on this network immediately.',
		'Cross-container traffic over this network will fail until the container is reconnected.'
	]}
	confirmLabel="Disconnect"
	busy={pendingBusy}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<style>
	.networks-tab { display: flex; flex-direction: column; gap: 12px; }

	.tab-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.lead { display: flex; align-items: baseline; gap: 10px; }
	.lead h2 { margin: 0; font-size: 16px; font-weight: 600; color: var(--text); letter-spacing: -0.01em; }
	.lead-sub { font-size: 11px; color: var(--text-faint); }

	.error-banner {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		background: var(--crit-soft);
		border: 1px solid color-mix(in srgb, var(--crit) 40%, transparent);
		border-radius: var(--r-md);
		color: var(--crit);
		font-size: 12px;
	}

	.table-wrap {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
	}

	table { width: 100%; border-collapse: collapse; font-size: 12px; }
	th {
		padding: 8px 12px;
		font-family: var(--font-mono);
		font-size: 10px;
		font-weight: 500;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		text-align: left;
		color: var(--text-faint);
		border-bottom: 1px solid var(--line);
		background: var(--bg-elev);
	}
	td {
		padding: 8px 12px;
		border-bottom: 1px solid var(--line);
		color: var(--text-dim);
		vertical-align: middle;
	}
	tbody tr:last-child td { border-bottom: none; }
	tbody tr { cursor: pointer; transition: background 120ms; }
	tbody tr:hover:not(.inspect-row) td { background: var(--bg-hover); color: var(--text); }
	tbody tr.expanded > td { background: var(--bg-elev-2); }

	.nname { font-weight: 500; color: var(--text); font-size: 12px; display: inline-flex; align-items: center; gap: 6px; }
	.badge {
		font-size: 9px; padding: 1px 6px;
		text-transform: uppercase; letter-spacing: 0.08em;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-pill);
		color: var(--text-faint);
	}
	.muted { color: var(--text-faint); }
	.micro { font-size: 10px; }

	.actions-col { text-align: right; }
	.actions { display: flex; gap: 4px; justify-content: flex-end; cursor: default; }

	.empty-row {
		text-align: center;
		padding: 32px 12px;
		color: var(--text-faint);
		font-size: 12px;
		cursor: default;
	}

	.inspect-row td { cursor: default; background: var(--bg-elev-2); }
	.inspect-cell { padding: 16px 20px; border-top: 1px solid var(--line); }
	.inspect-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
	@media (max-width: 900px) { .inspect-grid { grid-template-columns: 1fr; } }
	.inspect-col { display: flex; flex-direction: column; gap: 16px; }
	.inspect-section { display: flex; flex-direction: column; gap: 6px; }
	.section-label {
		font-family: var(--font-mono);
		font-size: 10px;
		font-weight: 500;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		padding-bottom: 4px;
		border-bottom: 1px solid var(--line);
	}

	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-faint); }
	.v { font-family: var(--font-mono); word-break: break-all; color: var(--text); }

	.env-list {
		max-height: 200px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.container-list { display: flex; flex-direction: column; gap: 8px; }
	.container-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 8px 10px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}
	.cname { font-weight: 500; font-size: 12px; color: var(--text); }

	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.create-form .row { display: grid; grid-template-columns: 110px 1fr; gap: 10px; align-items: center; }
	.create-form .row > span { font-size: 12px; color: var(--text-dim); }
	.create-form .row.checkbox { grid-template-columns: 110px auto 1fr; }
	.create-form input[type="text"],
	.create-form select {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text);
		padding: 6px 10px;
		font-size: 12px;
	}
	.err-text { color: var(--crit); font-size: 11px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
