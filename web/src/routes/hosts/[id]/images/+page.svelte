<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { DockerImage, ImageUpdateStatus } from '$lib/types';
	import { formatBytes, relTime, absTime } from '$lib/format';

	let id = $derived(page.params.id);
	let hostName = $state('');
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
			// use a tag if available for inspection, else the ID
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
			error = "Cannot check update for untagged image";
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
			updateStatus[imgId] = null; // reset status
			await refresh();
		} catch(e) {
			error = `Pull failed: ${(e as Error).message}`;
		} finally {
			busy[imgId] = false;
		}
	}

	async function remove(imgId: string, nameToInspect: string, force = false) {
		if (!confirm(`Remove image ${nameToInspect || imgId.slice(0, 12)}?`)) return;
		busy[imgId] = true;
		try {
			await api.removeImage(id, nameToInspect || imgId, force);
			if (inspectImgId === imgId) { inspectImgId = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[imgId] = false;
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
		if (!pullTarget.trim()) { pullError = 'image name is required'; return; }
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
		if (inspectImgId) { inspectImgId = null; inspectData = null; return; }
	}

	onMount(async () => {
		refresh();
		timer = setInterval(refresh, 8000);
		try { const h = await api.host(id); hostName = h.name; } catch { /* best-effort */ }
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

<svelte:head><title>Aperture — {hostName || id} — Images</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<div>
		<a href={`/hosts/${id}`} class="back">← back to host</a>
		<h1>Images</h1>
		<div class="muted small">{images.length} total</div>
	</div>
	<button onclick={openPull}>+ Pull Image</button>
</div>

<!-- Sub-navigation -->
<nav class="subnav">
	<a href={`/hosts/${id}`}>Overview</a>
	<a href={`/hosts/${id}/containers`}>Containers</a>
	<a href={`/hosts/${id}/compose`}>Compose</a>
	<a href={`/hosts/${id}/networks`}>Networks</a>
	<a href={`/hosts/${id}/volumes`}>Volumes</a>
	<a href={`/hosts/${id}/images`} class="active">Images</a>
</nav>

{#if error}
	<div class="card err">{error} <button class="x" onclick={() => (error = null)}>×</button></div>
{/if}

<div class="card no-pad">
	<table>
		<thead>
			<tr>
				<th>Image</th>
				<th>ID</th>
				<th>Size</th>
				<th>Usage</th>
				<th>Created</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>
			{#each images as img (img.id)}
				{@const primaryTag = getPrimaryTag(img.repo_tags)}
				<tr class:expanded={inspectImgId === img.id} onclick={() => openInspect(img.id, primaryTag)}>
					<td>
						<div class="vname">{primaryTag.split(':')[0]}</div>
						<div class="muted small">{primaryTag.split(':')[1] || ''}</div>
					</td>
					<td class="mono small">{img.id.replace('sha256:', '').slice(0, 12)}</td>
					<td class="mono small">{formatBytes(img.size_bytes)}</td>
					<td>
						{#if img.containers > 0}
							<span class="pill usage">In use ({img.containers})</span>
						{:else}
							<span class="pill unused">Unused</span>
						{/if}
					</td>
					<td class="mono small muted" title={absTime(img.created)}>{relTime(img.created)}</td>
					<td class="actions" onclick={(e) => e.stopPropagation()}>
						<button class="danger" disabled={busy[img.id]} onclick={() => remove(img.id, primaryTag)}>Remove</button>
					</td>
				</tr>

				<!-- Expand: deep inspect panel -->
				{#if inspectImgId === img.id}
					<tr class="inspect-row">
						<td colspan="6" class="inspect-cell">
							{#if inspectLoading}
								<div class="muted small">Loading…</div>
							{:else if inspectData}
								<div class="inspect-grid">
									<!-- Left: config -->
									<div class="inspect-col">
										<div class="inspect-section">
											<div class="section-label">Details</div>
											<div class="kv">
												<span class="k">ID</span><span class="v mono small">{inspectData.id}</span>
												<span class="k">Tags</span><span class="v mono small">{(inspectData.repo_tags || []).join(', ') || '<none>'}</span>
												<span class="k">Digests</span><span class="v mono small">{(inspectData.repo_digests || []).map(formatDigest).join(', ') || '<none>'}</span>
											</div>
										</div>

										<div class="inspect-section">
											<div class="section-label">Registry Update Check</div>
											{#if primaryTag !== '<none>:<none>'}
												{#if checkingUpdate[img.id]}
													<div class="muted small">Checking registry...</div>
												{:else if updateStatus[img.id]}
													{@const us = updateStatus[img.id]}
													{#if us.error}
														<div class="err-box">{us.error}</div>
													{:else if us.up_to_date}
														<div class="update-box success">
															<strong>✓ Up to date</strong><br/>
															<span class="small muted mono">Local: {formatDigest(us.local_digest)}</span><br/>
															<span class="small muted mono">Remote: {formatDigest(us.remote_digest)}</span>
														</div>
													{:else}
														<div class="update-box warning">
															<strong>Update Available!</strong><br/>
															<span class="small muted mono">Local: {formatDigest(us.local_digest)}</span><br/>
															<span class="small mono" style="color: var(--accent);">Remote: {formatDigest(us.remote_digest)}</span>
															<div style="margin-top: 10px;">
																<button class="primary small-btn" disabled={busy[img.id]} onclick={() => pullAndUpdate(img.id, primaryTag)}>Pull Update</button>
															</div>
														</div>
													{/if}
												{/if}
												{#if !updateStatus[img.id] && !checkingUpdate[img.id]}
													<button onclick={() => checkUpdate(img.id, primaryTag)}>Check for updates</button>
												{/if}
											{:else}
												<div class="muted small">Untagged image cannot be checked for updates.</div>
											{/if}
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

									<!-- Right: usage / actions -->
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
												<button class="danger" disabled={busy[img.id]} onclick={() => remove(img.id, primaryTag)}>Remove Image</button>
												<button class="danger force" disabled={busy[img.id]} onclick={() => remove(img.id, primaryTag, true)} title="Force remove the image, even if it is referenced">Force Remove</button>
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
				<tr><td colspan="6" class="muted center">no images</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<!-- Pull modal -->
{#if showPull}
	<div class="modal-bg" onclick={() => (showPull = false)} role="presentation">
		<div class="modal create" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
			<div class="modal-head">
				<div>Pull Image</div>
				<button onclick={() => (showPull = false)}>close</button>
			</div>
			<form onsubmit={submitPull} class="create-form">
				<label class="row">
					<span>Image Name (e.g. nginx:latest) *</span>
					<input type="text" placeholder="nginx:latest" bind:value={pullTarget} required autofocus />
				</label>
				
				<p class="muted small mt">Pulling images can take some time depending on network speed and image size.</p>

				{#if pullError}
					<div class="err-box">{pullError}</div>
				{/if}

				<div class="foot">
					<button type="button" onclick={() => (showPull = false)}>Cancel</button>
					<button type="submit" class="primary" disabled={pulling || !pullTarget.trim()}>{pulling ? 'Pulling...' : 'Pull Image'}</button>
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

	.update-box { padding: 10px 12px; border-radius: 6px; font-size: 13px; margin-top: 4px; }
	.update-box.success { background: rgba(80,220,130,0.1); border: 1px solid rgba(80,220,130,0.2); }
	.update-box.success strong { color: #50dc82; }
	.update-box.warning { background: rgba(255,200,80,0.1); border: 1px solid rgba(255,200,80,0.2); }
	.update-box.warning strong { color: #ffc850; }
	.small-btn { padding: 4px 8px; font-size: 11px; }

	/* Modal */
	.modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px); z-index: 100; display: flex; align-items: center; justify-content: center; }
	.modal { background: var(--bg-elev); border: 1px solid var(--border); border-radius: 8px; width: 440px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); display: flex; flex-direction: column; max-height: 90vh; }
	.modal-head { padding: 16px 20px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; font-weight: 600; }
	.modal-head button { font-size: 12px; padding: 4px 8px; }
	.create-form { padding: 20px; overflow-y: auto; }
	.row { display: flex; flex-direction: column; gap: 4px; margin-bottom: 16px; }
	.row span { font-size: 12px; color: var(--text-dim); font-weight: 500; }
	.row input { background: var(--bg); border: 1px solid var(--border); padding: 8px 12px; color: var(--text); border-radius: 4px; font-size: 13px; width: 100%; box-sizing: border-box; }
	.row input:focus { outline: none; border-color: var(--accent); }
	
	.mt { margin-top: 16px; }
	.err-box { padding: 10px; margin-top: 16px; background: rgba(255,107,107,0.1); border: 1px solid var(--bad); color: var(--bad); font-size: 13px; border-radius: 4px; }
	.foot { margin-top: 24px; display: flex; justify-content: flex-end; gap: 12px; }
	button.primary { background: var(--accent); color: #fff; border-color: var(--accent); }
	button.primary:hover:not(:disabled) { background: #4bb3e6; }
</style>
