<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerVolume, VolumeCreateSpec } from '$lib/types';
	import { formatBytes, relTime, absTime } from '$lib/format';
	import Button from '$lib/components/primitives/Button.svelte';
	import Modal from '$lib/components/primitives/Modal.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
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

	// Confirmation
	type Pending = { vol: DockerVolume; force: boolean } | null;
	let pending = $state<Pending>(null);
	let pendingBusy = $state(false);

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

	async function runPending() {
		if (!pending) return;
		const { vol, force } = pending;
		busy[vol.name] = true;
		pendingBusy = true;
		try {
			await api.removeVolume(id, vol.name, force);
			if (inspectVolName === vol.name) { inspectVolName = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[vol.name] = false;
			pendingBusy = false;
			pending = null;
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
		if (!form.name.trim()) { createError = 'Name is required'; return; }
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
		if (pending) { pending = null; return; }
		if (inspectVolName) { inspectVolName = null; inspectData = null; return; }
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

<section class="volumes-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Volumes</h2>
			<span class="lead-sub mono">{volumes.length} total</span>
		</div>
		<Button variant="primary" onclick={openCreate}>+ New Volume</Button>
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
					<th>Name</th>
					<th>Driver</th>
					<th>Size</th>
					<th>Usage</th>
					<th>Created</th>
					<th class="actions-col">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each volumes as v (v.name)}
					<tr class:expanded={inspectVolName === v.name} onclick={() => openInspect(v.name)}>
						<td>
							<div class="vname">{v.name}</div>
						</td>
						<td><span class="pill">{v.driver}</span></td>
						<td class="mono micro">{formatBytes(v.size_bytes)}</td>
						<td>
							{#if v.ref_count > 0}
								<span class="pill in-use">In use ({v.ref_count})</span>
							{:else}
								<span class="pill unused">Unused</span>
							{/if}
						</td>
						<td class="mono micro muted" title={absTime(v.created_at)}>{relTime(v.created_at)}</td>
						<td class="actions" onclick={(e) => e.stopPropagation()}>
							<Button variant="danger" size="sm" disabled={busy[v.name]} onclick={() => { pending = { vol: v, force: false }; }}>Remove</Button>
						</td>
					</tr>

					{#if inspectVolName === v.name}
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
													<span class="k">Driver</span><span class="v mono">{inspectData.driver}</span>
													<span class="k">Scope</span><span class="v mono">{inspectData.scope}</span>
													<span class="k">Mountpoint</span><span class="v mono micro">{inspectData.mountpoint}</span>
												</div>
											</div>

											{#if inspectData.options && Object.keys(inspectData.options).length > 0}
												<div class="inspect-section">
													<div class="section-label">Driver options</div>
													<div class="env-list">
														{#each Object.entries(inspectData.options) as [k, val]}
															<div class="mono micro">{k}={val}</div>
														{/each}
													</div>
												</div>
											{/if}

											{#if Object.keys(inspectData.labels || {}).length > 0}
												<div class="inspect-section">
													<div class="section-label">Labels</div>
													<div class="env-list">
														{#each Object.entries(inspectData.labels || {}) as [k, val]}
															<div class="mono micro">{k}={val}</div>
														{/each}
													</div>
												</div>
											{/if}
										</div>

										<div class="inspect-col">
											<div class="inspect-section">
												<div class="section-label">Usage</div>
												<div class="kv">
													<span class="k">Size</span><span class="v mono">{formatBytes(inspectData.size_bytes)}</span>
													<span class="k">Containers</span><span class="v mono">{inspectData.ref_count} reference{inspectData.ref_count === 1 ? '' : 's'}</span>
												</div>
											</div>
											<div class="inspect-section">
												<div class="section-label">Actions</div>
												<div class="action-buttons">
													<Button variant="danger" size="md" disabled={busy[v.name]} onclick={() => { pending = { vol: v, force: false }; }}>Remove volume</Button>
													<Button variant="danger" size="md" disabled={busy[v.name]} onclick={() => { pending = { vol: v, force: true }; }}>Force remove</Button>
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
					<tr><td colspan="6" class="empty-row">No volumes.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>
</section>

<Modal open={showCreate} onclose={() => (showCreate = false)} title="New volume" width="480px">
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
			<div class="section-label">Driver options</div>
			{#each Object.entries(form.driver_opts || {}) as [k, val]}
				<div class="env-row">
					<input type="text" readonly value={k} />
					<input type="text" readonly value={val} />
					<Button variant="icon" size="sm" ariaLabel="Remove" onclick={() => { const o = { ...form.driver_opts }; delete o[k]; form.driver_opts = o; }}>
						<Icon name="x" size={12} />
					</Button>
				</div>
			{/each}
			<div class="env-row new-env">
				<input type="text" placeholder="key" bind:value={optKey} />
				<input type="text" placeholder="value" bind:value={optVal} />
				<Button variant="ghost" size="sm" onclick={() => { if (optKey) { form.driver_opts = { ...form.driver_opts, [optKey]: optVal }; optKey = ''; optVal = ''; } }}>Add</Button>
			</div>
		</div>

		<div class="env-editor mt">
			<div class="section-label">Labels</div>
			{#each Object.entries(form.labels || {}) as [k, val]}
				<div class="env-row">
					<input type="text" readonly value={k} />
					<input type="text" readonly value={val} />
					<Button variant="icon" size="sm" ariaLabel="Remove" onclick={() => { const l = { ...form.labels }; delete l[k]; form.labels = l; }}>
						<Icon name="x" size={12} />
					</Button>
				</div>
			{/each}
			<div class="env-row new-env">
				<input type="text" placeholder="key" bind:value={labelKey} />
				<input type="text" placeholder="value" bind:value={labelVal} />
				<Button variant="ghost" size="sm" onclick={() => { if (labelKey) { form.labels = { ...form.labels, [labelKey]: labelVal }; labelKey = ''; labelVal = ''; } }}>Add</Button>
			</div>
		</div>

		{#if createError}
			<div class="err-text">{createError}</div>
		{/if}
		<div class="form-actions">
			<Button variant="ghost" onclick={() => (showCreate = false)}>Cancel</Button>
			<Button variant="primary" type="submit" loading={creating} disabled={!form.name.trim()}>{creating ? 'Creating…' : 'Create'}</Button>
		</div>
	</form>
</Modal>

<ConfirmDialog
	open={pending !== null}
	tone="danger"
	title={pending?.force ? 'Force remove volume' : 'Remove volume'}
	message={pending?.force
		? 'Force remove this volume even if it is referenced by a container?'
		: 'Remove this Docker volume?'}
	detail={pending?.vol.name ?? ''}
	consequences={pending?.force
		? [
			'All data stored in this volume will be permanently deleted.',
			'Containers referencing this volume may fail until reconfigured.'
		]
		: [
			'All data stored in this volume will be permanently deleted.',
			'The volume cannot be in use by any container; otherwise this will fail.'
		]}
	confirmLabel={pending?.force ? 'Force remove' : 'Remove'}
	busy={pendingBusy}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<style>
	.volumes-tab { display: flex; flex-direction: column; gap: 12px; }

	.tab-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
	.lead { display: flex; align-items: baseline; gap: 10px; }
	.lead h2 { margin: 0; font-size: 16px; font-weight: 600; color: var(--text); letter-spacing: -0.01em; }
	.lead-sub { font-size: 11px; color: var(--text-faint); }

	.error-banner {
		display: flex; justify-content: space-between; align-items: center; gap: 8px;
		padding: 8px 12px;
		background: var(--crit-soft);
		border: 1px solid color-mix(in srgb, var(--crit) 40%, transparent);
		border-radius: var(--r-md);
		color: var(--crit);
		font-size: 12px;
	}

	.table-wrap { background: var(--bg-elev); border: 1px solid var(--line); border-radius: var(--r-lg); overflow: hidden; }
	table { width: 100%; border-collapse: collapse; font-size: 12px; }
	th {
		padding: 8px 12px;
		font-family: var(--font-mono); font-size: 10px;
		font-weight: 500; letter-spacing: 0.08em; text-transform: uppercase;
		text-align: left; color: var(--text-faint);
		border-bottom: 1px solid var(--line);
		background: var(--bg-elev);
	}
	td { padding: 8px 12px; border-bottom: 1px solid var(--line); color: var(--text-dim); vertical-align: middle; }
	tbody tr:last-child td { border-bottom: none; }
	tbody tr { cursor: pointer; transition: background 120ms; }
	tbody tr:hover:not(.inspect-row) td { background: var(--bg-hover); color: var(--text); }
	tbody tr.expanded > td { background: var(--bg-elev-2); }

	.vname { font-weight: 500; color: var(--text); font-size: 12px; }
	.muted { color: var(--text-faint); }
	.micro { font-size: 10px; }
	.pill.in-use { background: var(--accent-soft); color: var(--accent); }
	.pill.unused { background: var(--bg-elev-2); color: var(--text-faint); border: 1px solid var(--line); }

	.actions-col { text-align: right; }
	.actions { display: flex; gap: 4px; justify-content: flex-end; cursor: default; }

	.empty-row { text-align: center; padding: 32px 12px; color: var(--text-faint); font-size: 12px; cursor: default; }

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

	.kv { display: grid; grid-template-columns: 100px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-faint); }
	.v { font-family: var(--font-mono); word-break: break-all; color: var(--text); }

	.env-list { max-height: 200px; overflow: auto; display: flex; flex-direction: column; gap: 2px; }
	.action-buttons { display: flex; gap: 6px; flex-wrap: wrap; }

	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.create-form .row { display: grid; grid-template-columns: 110px 1fr; gap: 10px; align-items: center; }
	.create-form .row > span { font-size: 12px; color: var(--text-dim); }
	.create-form input[type="text"] {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text);
		padding: 6px 10px;
		font-size: 12px;
	}
	.env-editor.mt { margin-top: 8px; }
	.env-row { display: grid; grid-template-columns: 1fr 1fr auto; gap: 6px; margin-bottom: 4px; align-items: center; }
	.env-row input {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text);
		padding: 5px 8px;
		font-size: 11px;
		font-family: var(--font-mono);
	}
	.new-env { margin-top: 6px; }
	.err-text { color: var(--crit); font-size: 11px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
