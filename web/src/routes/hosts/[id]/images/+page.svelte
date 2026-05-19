<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerImage, ImageUpdateStatus } from '$lib/types';
	import { formatBytes, relTime, absTime } from '$lib/format';
	import Button from '$lib/components/primitives/Button.svelte';
	import Modal from '$lib/components/primitives/Modal.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
	let images = $state<DockerImage[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let timer: ReturnType<typeof setInterval> | null = null;

	let inspectImgId = $state<string | null>(null);
	let inspectData = $state<DockerImage | null>(null);
	let inspectLoading = $state(false);

	let showPull = $state(false);
	let pulling = $state(false);
	let pullError = $state<string | null>(null);
	let pullTarget = $state('');

	let updateStatus = $state<Record<string, ImageUpdateStatus | null>>({});
	let checkingUpdate = $state<Record<string, boolean>>({});

	// Confirmation
	type Pending = { img: DockerImage; tag: string; force: boolean } | null;
	let pending = $state<Pending>(null);
	let pendingBusy = $state(false);

	async function refresh() {
		try {
			images = await api.images(id);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function openInspect(imgId: string, nameToInspect: string) {
		if (inspectImgId === imgId) { inspectImgId = null; inspectData = null; return; }
		inspectImgId = imgId;
		inspectData = null;
		inspectLoading = true;
		try {
			inspectData = await api.imageInspect(id, nameToInspect || imgId);
		} catch (e) {
			error = (e as Error).message;
			inspectImgId = null;
		} finally {
			inspectLoading = false;
		}
	}

	async function checkUpdate(imgId: string, tag: string) {
		if (!tag || tag.includes('<none>')) {
			error = 'Cannot check updates for untagged images';
			return;
		}
		checkingUpdate[imgId] = true;
		try {
			updateStatus[imgId] = await api.checkImageUpdate(id, tag);
		} catch (e) {
			error = `Update check failed: ${(e as Error).message}`;
		} finally {
			checkingUpdate[imgId] = false;
		}
	}

	async function pullAndUpdate(imgId: string, tag: string) {
		if (!tag || tag.includes('<none>')) return;
		busy[imgId] = true;
		try {
			await api.pullImage(id, tag);
			updateStatus[imgId] = null;
			await refresh();
		} catch (e) {
			error = `Pull failed: ${(e as Error).message}`;
		} finally {
			busy[imgId] = false;
		}
	}

	async function runPending() {
		if (!pending) return;
		const { img, tag, force } = pending;
		busy[img.id] = true;
		pendingBusy = true;
		try {
			await api.removeImage(id, tag || img.id, force);
			if (inspectImgId === img.id) { inspectImgId = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[img.id] = false;
			pendingBusy = false;
			pending = null;
		}
	}

	function openPull() {
		pullTarget = '';
		pullError = null;
		showPull = true;
	}

	async function submitPull(ev: Event) {
		ev.preventDefault();
		pullError = null;
		if (!pullTarget.trim()) { pullError = 'Image name is required'; return; }
		pulling = true;
		try {
			await api.pullImage(id, pullTarget.trim());
			showPull = false;
			await refresh();
		} catch (e) {
			pullError = (e as Error).message;
		} finally {
			pulling = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (showPull) { showPull = false; return; }
		if (pending) { pending = null; return; }
		if (inspectImgId) { inspectImgId = null; inspectData = null; return; }
	}

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 8000);
	});

	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	function getPrimaryTag(repoTags: string[] | null): string {
		if (!repoTags || repoTags.length === 0) return '<none>:<none>';
		return repoTags[0];
	}

	function formatDigest(digest: string): string {
		if (!digest) return '';
		const parts = digest.split('@');
		const hash = parts.length > 1 ? parts[1] : digest;
		return hash.replace('sha256:', '').slice(0, 12);
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<section class="images-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Images</h2>
			<span class="lead-sub mono">{images.length} total</span>
		</div>
		<Button variant="primary" onclick={openPull}>+ Pull Image</Button>
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
					<th>Image</th>
					<th>ID</th>
					<th>Size</th>
					<th>Usage</th>
					<th>Created</th>
					<th class="actions-col">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each images as img (img.id)}
					{@const primaryTag = getPrimaryTag(img.repo_tags)}
					{@const untagged = primaryTag === '<none>:<none>'}
					<tr class:expanded={inspectImgId === img.id} onclick={() => openInspect(img.id, primaryTag)}>
						<td>
							<div class="vname">{primaryTag.split(':')[0]}</div>
							<div class="muted mono micro">{primaryTag.split(':')[1] || ''}</div>
						</td>
						<td class="mono micro">{img.id.replace('sha256:', '').slice(0, 12)}</td>
						<td class="mono micro">{formatBytes(img.size_bytes)}</td>
						<td>
							{#if img.containers > 0}
								<span class="pill in-use">In use ({img.containers})</span>
							{:else}
								<span class="pill unused">Unused</span>
							{/if}
						</td>
						<td class="mono micro muted" title={absTime(img.created)}>{relTime(img.created)}</td>
						<td class="actions" onclick={(e) => e.stopPropagation()}>
							{#if !untagged}
								<Button variant="ghost" size="sm" disabled={checkingUpdate[img.id]} onclick={() => checkUpdate(img.id, primaryTag)}>
									{checkingUpdate[img.id] ? 'Checking…' : 'Check for Updates'}
								</Button>
							{/if}
							<Button variant="danger" size="sm" disabled={busy[img.id]} onclick={() => { pending = { img, tag: primaryTag, force: false }; }}>Remove</Button>
						</td>
					</tr>

					{#if inspectImgId === img.id}
						<tr class="inspect-row">
							<td colspan="6" class="inspect-cell">
								{#if inspectLoading}
									<div class="muted micro">Loading…</div>
								{:else if inspectData}
									<div class="inspect-grid">
										<div class="inspect-col">
											<div class="inspect-section">
												<div class="section-label">Details</div>
												<div class="kv">
													<span class="k">ID</span><span class="v mono micro">{inspectData.id}</span>
													<span class="k">Tags</span><span class="v mono micro">{(inspectData.repo_tags || []).join(', ') || '<none>'}</span>
													<span class="k">Digests</span><span class="v mono micro">{(inspectData.repo_digests || []).map(formatDigest).join(', ') || '<none>'}</span>
												</div>
											</div>

											<div class="inspect-section">
												<div class="section-label">Registry update check</div>
												{#if !untagged}
													{#if checkingUpdate[img.id]}
														<div class="muted micro">Checking registry…</div>
													{:else if updateStatus[img.id]}
														{@const us = updateStatus[img.id]!}
														{#if us.error}
															<div class="update-box error-tone">{us.error}</div>
														{:else if us.up_to_date}
															<div class="update-box ok-tone">
																<strong>Up to date</strong>
																<div class="mono micro muted">Local: {formatDigest(us.local_digest)}</div>
																<div class="mono micro muted">Remote: {formatDigest(us.remote_digest)}</div>
															</div>
														{:else}
															<div class="update-box warn-tone">
																<strong>Update available</strong>
																<div class="mono micro muted">Local: {formatDigest(us.local_digest)}</div>
																<div class="mono micro" style="color: var(--accent);">Remote: {formatDigest(us.remote_digest)}</div>
																<div style="margin-top: 8px;">
																	<Button variant="primary" size="sm" disabled={busy[img.id]} onclick={() => pullAndUpdate(img.id, primaryTag)}>Pull update</Button>
																</div>
															</div>
														{/if}
													{:else}
														<Button variant="ghost" size="sm" onclick={() => checkUpdate(img.id, primaryTag)}>Check for updates</Button>
													{/if}
												{:else}
													<div class="muted micro">Untagged image — cannot check for updates.</div>
												{/if}
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
												<div class="section-label">Usage</div>
												<div class="kv">
													<span class="k">Size</span><span class="v mono">{formatBytes(inspectData.size_bytes)}</span>
												</div>
											</div>
											<div class="inspect-section">
												<div class="section-label">Actions</div>
												<div class="action-buttons">
													<Button variant="danger" size="md" disabled={busy[img.id]} onclick={() => { pending = { img, tag: primaryTag, force: false }; }}>Remove image</Button>
													<Button variant="danger" size="md" disabled={busy[img.id]} title="Force remove the image even if it is referenced" onclick={() => { pending = { img, tag: primaryTag, force: true }; }}>Force remove</Button>
												</div>
											</div>
										</div>
									</div>
								{/if}
							</td>
						</tr>
					{/if}
				{/each}
				{#if images.length === 0 && !error}
					<tr><td colspan="6" class="empty-row">No images.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>
</section>

<Modal open={showPull} onclose={() => (showPull = false)} title="Pull image" width="440px">
	<form onsubmit={submitPull} class="create-form">
		<label class="row">
			<span>Image name (e.g. nginx:latest) *</span>
			<input type="text" placeholder="nginx:latest" bind:value={pullTarget} required autofocus />
		</label>
		<p class="hint">Pulling can take a while depending on network speed and image size.</p>
		{#if pullError}
			<div class="err-text">{pullError}</div>
		{/if}
		<div class="form-actions">
			<Button variant="ghost" onclick={() => (showPull = false)}>Cancel</Button>
			<Button variant="primary" type="submit" loading={pulling} disabled={!pullTarget.trim()}>
				{pulling ? 'Pulling…' : 'Pull image'}
			</Button>
		</div>
	</form>
</Modal>

<ConfirmDialog
	open={pending !== null}
	tone="danger"
	title={pending?.force ? 'Force remove image' : 'Remove image'}
	message={pending?.force
		? 'Force remove this image even if containers reference it?'
		: 'Remove this image?'}
	detail={pending?.tag || pending?.img.id.slice(0, 12)}
	consequences={pending?.force
		? [
			'Any container using this image must be removed too; force-removing here may break those containers.',
			'You will need to re-pull the image if you want to use it again.'
		]
		: [
			'The image is deleted only if no container references it; otherwise this will fail.',
			'You will need to re-pull the image if you want to use it again.'
		]}
	confirmLabel={pending?.force ? 'Force remove' : 'Remove'}
	busy={pendingBusy}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<style>
	.images-tab { display: flex; flex-direction: column; gap: 12px; }

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
	.actions { display: flex; gap: 4px; justify-content: flex-end; cursor: default; flex-wrap: wrap; }

	.empty-row { text-align: center; padding: 32px 12px; color: var(--text-faint); font-size: 12px; cursor: default; }

	.inspect-row td { cursor: default; background: var(--bg-elev-2); }
	.inspect-cell { padding: 16px 20px; border-top: 1px solid var(--line); }
	.inspect-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
	@media (max-width: 900px) { .inspect-grid { grid-template-columns: 1fr; } }
	.inspect-col { display: flex; flex-direction: column; gap: 16px; }
	.inspect-section { display: flex; flex-direction: column; gap: 6px; }
	.section-label {
		font-family: var(--font-mono);
		font-size: 10px; font-weight: 500;
		text-transform: uppercase; letter-spacing: 0.08em;
		color: var(--text-faint);
		padding-bottom: 4px; border-bottom: 1px solid var(--line);
	}

	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-faint); }
	.v { font-family: var(--font-mono); word-break: break-all; color: var(--text); }

	.env-list { max-height: 200px; overflow: auto; display: flex; flex-direction: column; gap: 2px; }
	.action-buttons { display: flex; gap: 6px; flex-direction: column; max-width: 180px; }

	.update-box { padding: 10px 12px; border-radius: var(--r-md); font-size: 12px; }
	.update-box strong { display: block; margin-bottom: 4px; }
	.update-box.ok-tone   { background: var(--ok-soft);   border: 1px solid color-mix(in srgb, var(--ok)   40%, transparent); color: var(--ok); }
	.update-box.warn-tone { background: var(--warn-soft); border: 1px solid color-mix(in srgb, var(--warn) 40%, transparent); color: var(--warn); }
	.update-box.error-tone{ background: var(--crit-soft); border: 1px solid color-mix(in srgb, var(--crit) 40%, transparent); color: var(--crit); }

	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.row { display: flex; flex-direction: column; gap: 4px; }
	.row span { font-size: 12px; color: var(--text-dim); }
	.row input {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text); padding: 6px 10px;
		font-size: 12px;
	}
	.hint { font-size: 11px; color: var(--text-faint); margin: 0; }
	.err-text { color: var(--crit); font-size: 11px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
