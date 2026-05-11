<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { ComposeStack, ComposeService } from '$lib/types';
	import { toast } from '$lib/toast';

	let id = $derived(page.params.id);
	let hostName = $state('');
	let stacks = $state<ComposeStack[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let composeAvailable = $state(true);

	// Per-stack expanded state and active tab
	let expanded = $state<Record<string, boolean>>({});
	let activeTab = $state<Record<string, 'services' | 'file' | 'logs'>>({});
	let actionBusy = $state<Record<string, boolean>>({});
	let actionOutput = $state<Record<string, string>>({});

	// Per-stack file editor state
	let fileContent = $state<Record<string, string>>({});
	let fileLoading = $state<Record<string, boolean>>({});
	let fileDirty = $state<Record<string, boolean>>({});
	let fileSaving = $state<Record<string, boolean>>({});

	// Per-stack log state
	let logContent = $state<Record<string, string>>({});
	let logLoading = $state<Record<string, boolean>>({});
	let logService = $state<Record<string, string>>({});
	let logTail = $state<Record<string, number>>({});

	// New stack modal
	let newModal = $state(false);
	let newDir = $state('');
	let newContent = $state(`services:
  app:
    image: nginx:alpine
    ports:
      - "8080:80"
    restart: unless-stopped
`);
	let newStart = $state(true);
	let newSaving = $state(false);
	let newError = $state('');

	// Delete confirm
	let deleteTarget = $state<string | null>(null);
	let deleteVolumes = $state(false);

	let timer: ReturnType<typeof setInterval> | null = null;

	async function load(quiet = false) {
		if (!quiet) loading = true;
		try {
			const [h, s] = await Promise.all([
				api.host(id).catch(() => null),
				api.composeStacks(id).catch((e: Error) => {
					if (e.message.includes('503') || e.message.includes('not available')) {
						composeAvailable = false;
					}
					return [] as ComposeStack[];
				})
			]);

			// Fetch detailed services for expanded stacks
			const expandedProjects = s.filter(st => expanded[st.project]).map(st => st.project);
			if (expandedProjects.length > 0) {
				const details = await Promise.all(
					expandedProjects.map(proj => api.composeStack(id, proj).catch(() => null))
				);
				for (const d of details) {
					if (d) {
						const idx = s.findIndex(st => st.project === d.project);
						if (idx !== -1) s[idx] = d;
					}
				}
			}

			hostName = h?.name ?? id;
			stacks = s;
			composeAvailable = true;
			error = null;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		timer = setInterval(() => load(true), 8000);
	});
	onDestroy(() => { if (timer) clearInterval(timer); });

	async function loadServices(project: string) {
		try {
			const res = await api.composeStack(id, project);
			const idx = stacks.findIndex(s => s.project === project);
			if (idx !== -1) {
				stacks[idx] = res;
			}
		} catch (e) {
			toast.error(`Failed to load services: ${(e as Error).message}`);
		}
	}

	function toggleExpand(project: string) {
		expanded[project] = !expanded[project];
		if (expanded[project]) {
			if (!activeTab[project]) {
				activeTab[project] = 'services';
			}
			loadServices(project);
		}
	}

	function setTab(project: string, tab: 'services' | 'file' | 'logs') {
		activeTab[project] = tab;
		if (tab === 'services') loadServices(project);
		if (tab === 'file' && !fileContent[project]) loadFile(project);
		if (tab === 'logs') loadLogs(project);
	}

	async function loadFile(project: string) {
		const st = stacks.find(s => s.project === project);
		fileLoading[project] = true;
		try {
			const res = await api.composeFile(id, project, st?.working_dir);
			fileContent[project] = res.content;
			fileDirty[project] = false;
		} catch (e) {
			toast.error(`File load failed: ${(e as Error).message}`);
		} finally {
			fileLoading[project] = false;
		}
	}

	async function saveFile(project: string, deploy = false) {
		const st = stacks.find(s => s.project === project);
		fileSaving[project] = true;
		try {
			await api.composeWriteFile(id, project, {
				content: fileContent[project],
				working_dir: st?.working_dir,
				deploy
			});
			fileDirty[project] = false;
			toast.success(deploy ? 'Saved and deployed' : 'Compose file saved');
			if (deploy) load(true);
		} catch (e) {
			toast.error(`Save failed: ${(e as Error).message}`);
		} finally {
			fileSaving[project] = false;
		}
	}

	async function loadLogs(project: string) {
		const st = stacks.find(s => s.project === project);
		logLoading[project] = true;
		const svc = logService[project] ?? '';
		const tail = logTail[project] ?? 200;
		try {
			const res = await api.composeLogs(id, project, {
				working_dir: st?.working_dir,
				service: svc,
				tail
			});
			logContent[project] = res.logs || '(no output)';
		} catch (e) {
			logContent[project] = `Error: ${(e as Error).message}`;
		} finally {
			logLoading[project] = false;
		}
	}

	async function stackAction(project: string, action: string, service = '', extra: Record<string, unknown> = {}) {
		const st = stacks.find(s => s.project === project);
		const key = `${project}:${action}:${service}`;
		actionBusy[key] = true;
		actionOutput[project] = '';
		try {
			const res = await api.composeAction(id, project, action, {
				working_dir: st?.working_dir,
				service: service || undefined,
				...extra
			});
			actionOutput[project] = res.output || '';
			toast.success(`${action} completed`);
			load(true);
		} catch (e) {
			const msg = (e as Error).message;
			actionOutput[project] = msg;
			toast.error(`${action} failed`);
		} finally {
			actionBusy[key] = false;
		}
	}

	function isBusy(project: string, action: string, service = '') {
		return !!actionBusy[`${project}:${action}:${service}`];
	}

	function anyBusy(project: string) {
		return Object.keys(actionBusy).some(k => k.startsWith(project + ':') && actionBusy[k]);
	}

	async function doDelete() {
		if (!deleteTarget) return;
		try {
			await api.deleteComposeStack(id, deleteTarget, deleteVolumes);
			toast.success(`Stack "${deleteTarget}" stopped`);
			deleteTarget = null;
			load(true);
		} catch (e) {
			toast.error(`Down failed: ${(e as Error).message}`);
		}
	}

	async function createStack() {
		if (!newDir.trim() || !newContent.trim()) {
			newError = 'Directory and compose content are required.';
			return;
		}
		newSaving = true;
		newError = '';
		try {
			await api.createComposeStack(id, {
				working_dir: newDir.trim(),
				content: newContent,
				start: newStart
			});
			newModal = false;
			newDir = '';
			toast.success('Stack created');
			load(true);
		} catch (e) {
			newError = (e as Error).message;
		} finally {
			newSaving = false;
		}
	}

	function statusColor(status: string) {
		if (status === 'running') return 'status-running';
		if (status === 'partial') return 'status-partial';
		return 'status-stopped';
	}

	function serviceStateColor(state: string) {
		if (state === 'running') return 'svc-running';
		if (state === 'paused') return 'svc-paused';
		if (state === 'dead') return 'svc-dead';
		return 'svc-exited';
	}

	function healthBadge(health: string | undefined) {
		if (!health || health === '') return '';
		if (health === 'healthy') return 'health-ok';
		if (health === 'unhealthy') return 'health-bad';
		return 'health-starting';
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (deleteTarget) { deleteTarget = null; return; }
			if (newModal) { newModal = false; return; }
		}
	}
</script>

<svelte:head><title>Aperture — Compose · {hostName}</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page">
	<header class="page-header">
		<div class="header-left">
			<a href={`/hosts/${id}`} class="back-link">← {hostName}</a>
			<h1>Compose Stacks</h1>
		</div>
		<button class="btn-primary" onclick={() => { newModal = true; newError = ''; }}>+ New Stack</button>
	</header>

	<!-- Sub-nav mirrors containers page -->
	<nav class="sub-nav">
		<a href={`/hosts/${id}`}>Overview</a>
		<a href={`/hosts/${id}/containers`}>Containers</a>
		<a href={`/hosts/${id}/compose`} class="active">Compose</a>
		<a href={`/hosts/${id}/networks`} class="">Networks</a>
		<a href={`/hosts/${id}/volumes`} class="">Volumes</a>
		<a href={`/hosts/${id}/images`} class="placeholder">Images</a>
		<a href={`/hosts/${id}/logs`} class="placeholder">Logs</a>
	</nav>

	{#if loading}
		<p class="muted centre">Loading stacks…</p>
	{:else if !composeAvailable}
		<div class="unavailable-banner">
			<strong>Docker Compose not available</strong>
			<p>This host is offline, or <code>docker compose</code> is not installed.</p>
		</div>
	{:else if error}
		<p class="error-msg">{error}</p>
	{:else if stacks.length === 0}
		<div class="empty-state">
			<p>No compose stacks found on this host.</p>
			<p class="muted">Start a stack with <code>docker compose up -d</code> or create one below.</p>
			<button class="btn-primary" onclick={() => { newModal = true; }}>+ New Stack</button>
		</div>
	{:else}
		<div class="stack-list">
			{#each stacks as st (st.project)}
				{@const isExpanded = expanded[st.project]}
				{@const busy = anyBusy(st.project)}
				<div class="stack-card" class:expanded={isExpanded}>
					<!-- Stack header row -->
					<div class="stack-header" role="button" tabindex="0"
						onclick={() => toggleExpand(st.project)}
						onkeydown={(e) => e.key === 'Enter' && toggleExpand(st.project)}>
						<span class="status-dot {statusColor(st.status)}" title={st.status}></span>
						<div class="stack-identity">
							<span class="stack-name">{st.project}</span>
							{#if st.working_dir}
								<span class="stack-dir" title={st.working_dir}>{st.working_dir}</span>
							{/if}
						</div>
						<div class="stack-meta">
							<span class="svc-count {statusColor(st.status)}">
								{st.running_count ?? 0}/{st.total_count ?? 0} running
							</span>
							<span class="status-pill {statusColor(st.status)}">{st.status}</span>
						</div>
						<div class="stack-actions" role="none" onclick={(e) => e.stopPropagation()}>
							<button class="act-btn" title="Start / up" disabled={busy}
								onclick={() => stackAction(st.project, 'up')}>▶</button>
							<button class="act-btn" title="Stop / down" disabled={busy}
								onclick={() => stackAction(st.project, 'down')}>⏹</button>
							<button class="act-btn" title="Restart" disabled={busy}
								onclick={() => stackAction(st.project, 'restart')}>↺</button>
							<button class="act-btn" title="Pull images" disabled={busy}
								onclick={() => stackAction(st.project, 'pull')}>⬇</button>
						</div>
						<span class="chevron" class:open={isExpanded}>▾</span>
					</div>

					{#if actionOutput[st.project]}
						<pre class="action-output">{actionOutput[st.project]}</pre>
					{/if}

					{#if isExpanded}
						<div class="stack-detail">
							<div class="tab-bar">
								<button class:active={activeTab[st.project] === 'services'}
									onclick={() => setTab(st.project, 'services')}>Services</button>
								<button class:active={activeTab[st.project] === 'file'}
									onclick={() => setTab(st.project, 'file')}>Compose File</button>
								<button class:active={activeTab[st.project] === 'logs'}
									onclick={() => setTab(st.project, 'logs')}>Logs</button>
								<div class="tab-spacer"></div>
								<button class="danger-sm" title="Stop and remove"
									onclick={() => { deleteTarget = st.project; deleteVolumes = false; }}>
									Down…
								</button>
							</div>

							<!-- Services tab -->
							{#if activeTab[st.project] === 'services' || !activeTab[st.project]}
								{#if st.services && st.services.length > 0}
									<table class="svc-table">
										<thead>
											<tr>
												<th>Service</th>
												<th>State</th>
												<th>Status</th>
												<th>Ports</th>
												<th>Actions</th>
											</tr>
										</thead>
										<tbody>
											{#each st.services as svc (svc.name)}
												{@const hb = healthBadge(svc.health)}
												<tr>
													<td class="svc-name">
														{svc.name}
														{#if svc.container_id}
															<span class="cid">{svc.container_id}</span>
														{/if}
													</td>
													<td>
														<span class="svc-state {serviceStateColor(svc.state)}">{svc.state}</span>
														{#if hb}
															<span class="health-badge {hb}">{svc.health}</span>
														{/if}
													</td>
													<td class="svc-status">{svc.status}</td>
													<td class="svc-ports">
														{#if svc.ports && svc.ports.length > 0}
															{#each svc.ports as p}
																<span class="port-tag">{p.public_port}:{p.private_port}/{p.type}</span>
															{/each}
														{:else}
															<span class="muted">—</span>
														{/if}
													</td>
													<td class="svc-actions">
														{#if svc.state === 'running'}
															<button class="sm-btn" title="Restart service"
																disabled={isBusy(st.project, 'restart', svc.name)}
																onclick={() => stackAction(st.project, 'restart', svc.name)}>
																↺ restart
															</button>
															<button class="sm-btn" title="Stop service"
																disabled={isBusy(st.project, 'stop', svc.name)}
																onclick={() => stackAction(st.project, 'stop', svc.name)}>
																⏹ stop
															</button>
														{:else}
															<button class="sm-btn" title="Start service"
																disabled={isBusy(st.project, 'start', svc.name)}
																onclick={() => stackAction(st.project, 'start', svc.name)}>
																▶ start
															</button>
														{/if}
														<button class="sm-btn" title="View service logs"
															onclick={() => { setTab(st.project, 'logs'); logService[st.project] = svc.name; loadLogs(st.project); }}>
															logs
														</button>
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								{:else}
									<p class="muted pad">No containers found — stack may be stopped or loading. Use ▶ to start it.</p>
								{/if}
							{/if}

							<!-- File tab -->
							{#if activeTab[st.project] === 'file'}
								<div class="file-panel">
									{#if fileLoading[st.project]}
										<p class="muted">Loading compose file…</p>
									{:else}
										<div class="file-toolbar">
											<span class="file-path muted">{st.working_dir}/compose.yml</span>
											<div class="file-actions">
												<button class="sm-btn" onclick={() => loadFile(st.project)}>↻ Reload</button>
												<button class="sm-btn" disabled={fileSaving[st.project] || !fileDirty[st.project]}
													onclick={() => saveFile(st.project)}>Save</button>
												<button class="btn-primary sm" disabled={fileSaving[st.project]}
													onclick={() => saveFile(st.project, true)}>Save + Deploy</button>
											</div>
										</div>
										<textarea class="yaml-editor"
											value={fileContent[st.project] ?? ''}
											oninput={(e) => { fileContent[st.project] = (e.target as HTMLTextAreaElement).value; fileDirty[st.project] = true; }}
											spellcheck={false}
											placeholder="Compose YAML will appear here…"></textarea>
										{#if fileDirty[st.project]}
											<p class="dirty-hint">Unsaved changes — Save to write, Save + Deploy to write and restart.</p>
										{/if}
									{/if}
								</div>
							{/if}

							<!-- Logs tab -->
							{#if activeTab[st.project] === 'logs'}
								<div class="logs-panel">
									<div class="logs-toolbar">
										<label>
											Service
											<select value={logService[st.project] ?? ''}
												onchange={(e) => { logService[st.project] = (e.target as HTMLSelectElement).value; }}>
												<option value="">All services</option>
												{#each (st.services ?? []) as svc}
													<option value={svc.name}>{svc.name}</option>
												{/each}
											</select>
										</label>
										<label>
											Lines
											<select value={logTail[st.project] ?? 200}
												onchange={(e) => { logTail[st.project] = Number((e.target as HTMLSelectElement).value); }}>
												<option value={50}>50</option>
												<option value={200}>200</option>
												<option value={500}>500</option>
												<option value={1000}>1000</option>
											</select>
										</label>
										<button class="sm-btn" onclick={() => loadLogs(st.project)}
											disabled={logLoading[st.project]}>
											{logLoading[st.project] ? 'Loading…' : '↻ Refresh'}
										</button>
									</div>
									<pre class="log-output">{logLoading[st.project] ? 'Loading…' : (logContent[st.project] ?? '')}</pre>
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<!-- New Stack Modal -->
{#if newModal}
	<div class="modal-backdrop" onclick={() => { newModal = false; }} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="New Compose Stack">
			<h2>New Compose Stack</h2>
			<div class="form-group">
				<label for="new-dir">Directory path on host</label>
				<input id="new-dir" type="text" bind:value={newDir}
					placeholder="/opt/myapp or ~/stacks/nginx" />
				<span class="hint">The compose.yml will be written here. Directory is created if it doesn't exist.</span>
			</div>
			<div class="form-group">
				<label for="new-yaml">compose.yml content</label>
				<textarea id="new-yaml" class="yaml-editor tall" bind:value={newContent} spellcheck={false}></textarea>
			</div>
			<div class="form-check">
				<label>
					<input type="checkbox" bind:checked={newStart} />
					Start stack immediately after creating
				</label>
			</div>
			{#if newError}
				<p class="error-msg">{newError}</p>
			{/if}
			<div class="modal-footer">
				<button onclick={() => { newModal = false; }}>Cancel</button>
				<button class="btn-primary" onclick={createStack} disabled={newSaving}>
					{newSaving ? 'Creating…' : 'Create Stack'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete / Down Confirm Modal -->
{#if deleteTarget}
	<div class="modal-backdrop" onclick={() => { deleteTarget = null; }} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Confirm stack down">
			<h2>Stop stack "{deleteTarget}"?</h2>
			<p>This runs <code>docker compose down</code>, stopping and removing all containers for this stack.</p>
			<label class="form-check">
				<input type="checkbox" bind:checked={deleteVolumes} />
				Also remove named volumes (<code>--volumes</code>)
			</label>
			<div class="modal-footer">
				<button onclick={() => { deleteTarget = null; }}>Cancel</button>
				<button class="danger" onclick={doDelete}>Stop Stack</button>
			</div>
		</div>
	</div>
{/if}

<style>
.page { max-width: 1100px; margin: 0 auto; padding: 1.5rem; }

.page-header {
	display: flex; align-items: center; justify-content: space-between;
	margin-bottom: 1rem; gap: 1rem;
}
.header-left { display: flex; align-items: center; gap: 1rem; }
.back-link { color: var(--accent); text-decoration: none; font-size: 0.85rem; }
.back-link:hover { text-decoration: underline; }
h1 { font-size: 1.3rem; font-weight: 600; margin: 0; }

.sub-nav {
	display: flex; gap: 0.25rem; margin-bottom: 1.5rem;
	border-bottom: 1px solid var(--border); padding-bottom: 0.5rem; flex-wrap: wrap;
}
.sub-nav a {
	padding: 0.3rem 0.8rem; border-radius: 4px; text-decoration: none;
	color: var(--fg-muted); font-size: 0.85rem;
}
.sub-nav a:hover { background: var(--bg-hover); color: var(--fg); }
.sub-nav a.active { background: var(--accent); color: #fff; }
.sub-nav a.placeholder { opacity: 0.45; cursor: default; pointer-events: none; }

.unavailable-banner {
	background: var(--bg-card); border: 1px solid var(--border);
	border-radius: 8px; padding: 2rem; text-align: center; color: var(--fg-muted);
}
.unavailable-banner strong { display: block; margin-bottom: 0.4rem; font-size: 1rem; color: var(--fg); }
.unavailable-banner code { background: var(--bg-hover); padding: 0.1rem 0.3rem; border-radius: 3px; }

.empty-state { text-align: center; padding: 3rem 1rem; color: var(--fg-muted); }
.empty-state p { margin: 0.4rem 0; }
.empty-state button { margin-top: 1rem; }

.centre { text-align: center; margin-top: 3rem; }
.muted { color: var(--fg-muted); }
.pad { padding: 1rem 1.25rem; }
.error-msg { color: var(--danger); font-size: 0.88rem; }

/* ── Stack cards ── */
.stack-list { display: flex; flex-direction: column; gap: 0.75rem; }

.stack-card {
	background: var(--bg-card); border: 1px solid var(--border);
	border-radius: 8px; overflow: hidden;
	transition: border-color 0.15s;
}
.stack-card.expanded { border-color: var(--accent); }

.stack-header {
	display: flex; align-items: center; gap: 0.75rem;
	padding: 0.85rem 1rem; cursor: pointer; user-select: none;
}
.stack-header:hover { background: var(--bg-hover); }

.status-dot {
	width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0;
}
.status-dot.status-running { background: #2ecc71; }
.status-dot.status-partial { background: #f39c12; }
.status-dot.status-stopped { background: var(--fg-muted); }

.stack-identity { flex: 1; min-width: 0; }
.stack-name { font-weight: 600; font-size: 0.95rem; display: block; }
.stack-dir { font-size: 0.78rem; color: var(--fg-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block; max-width: 400px; }

.stack-meta { display: flex; align-items: center; gap: 0.5rem; flex-shrink: 0; }

.svc-count { font-size: 0.82rem; font-weight: 500; }
.svc-count.status-running { color: #2ecc71; }
.svc-count.status-partial { color: #f39c12; }
.svc-count.status-stopped { color: var(--fg-muted); }

.status-pill {
	font-size: 0.72rem; padding: 0.15rem 0.5rem; border-radius: 99px;
	text-transform: uppercase; letter-spacing: 0.04em; font-weight: 600;
}
.status-pill.status-running { background: #2ecc7120; color: #2ecc71; }
.status-pill.status-partial { background: #f39c1220; color: #f39c12; }
.status-pill.status-stopped { background: var(--bg-hover); color: var(--fg-muted); }

.stack-actions { display: flex; gap: 0.3rem; flex-shrink: 0; }
.act-btn {
	background: var(--bg-hover); border: 1px solid var(--border);
	border-radius: 4px; padding: 0.2rem 0.5rem; cursor: pointer;
	font-size: 0.9rem; color: var(--fg); line-height: 1;
}
.act-btn:hover:not(:disabled) { background: var(--accent); color: #fff; border-color: var(--accent); }
.act-btn:disabled { opacity: 0.4; cursor: not-allowed; }

.chevron { font-size: 1rem; color: var(--fg-muted); transition: transform 0.2s; flex-shrink: 0; }
.chevron.open { transform: rotate(180deg); }

.action-output {
	margin: 0; padding: 0.6rem 1rem; font-size: 0.78rem;
	background: #111; color: #ccc; border-top: 1px solid var(--border);
	white-space: pre-wrap; max-height: 120px; overflow-y: auto;
}

/* ── Stack detail ── */
.stack-detail { border-top: 1px solid var(--border); }

.tab-bar {
	display: flex; gap: 0.25rem; padding: 0.5rem 0.75rem;
	border-bottom: 1px solid var(--border); background: var(--bg-hover);
	align-items: center;
}
.tab-bar button {
	padding: 0.25rem 0.75rem; border-radius: 4px; border: none;
	background: transparent; color: var(--fg-muted); cursor: pointer; font-size: 0.83rem;
}
.tab-bar button:hover { background: var(--bg-card); color: var(--fg); }
.tab-bar button.active { background: var(--accent); color: #fff; }
.tab-spacer { flex: 1; }
.danger-sm {
	padding: 0.2rem 0.6rem; border-radius: 4px; border: 1px solid var(--danger);
	color: var(--danger); background: transparent; cursor: pointer; font-size: 0.78rem;
}
.danger-sm:hover { background: var(--danger); color: #fff; }

/* ── Services table ── */
.svc-table { width: 100%; border-collapse: collapse; font-size: 0.84rem; }
.svc-table th {
	text-align: left; padding: 0.4rem 0.75rem; font-weight: 500;
	color: var(--fg-muted); border-bottom: 1px solid var(--border); font-size: 0.78rem;
}
.svc-table td { padding: 0.45rem 0.75rem; border-bottom: 1px solid var(--border); vertical-align: middle; }
.svc-table tr:last-child td { border-bottom: none; }
.svc-table tr:hover td { background: var(--bg-hover); }

.svc-name { font-weight: 500; }
.cid { font-family: monospace; font-size: 0.75rem; color: var(--fg-muted); margin-left: 0.4rem; }

.svc-state {
	display: inline-block; font-size: 0.72rem; padding: 0.1rem 0.45rem;
	border-radius: 99px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em;
}
.svc-running { background: #2ecc7120; color: #2ecc71; }
.svc-paused  { background: #f39c1220; color: #f39c12; }
.svc-exited  { background: #e74c3c20; color: #e74c3c; }
.svc-dead    { background: #e74c3c30; color: #e74c3c; }

.health-badge {
	display: inline-block; font-size: 0.68rem; padding: 0.1rem 0.4rem;
	border-radius: 99px; font-weight: 500; margin-left: 0.3rem;
}
.health-ok      { background: #2ecc7115; color: #2ecc71; }
.health-bad     { background: #e74c3c15; color: #e74c3c; }
.health-starting{ background: #f39c1215; color: #f39c12; }

.svc-status { color: var(--fg-muted); font-size: 0.8rem; }

.svc-ports { font-size: 0.78rem; }
.port-tag {
	display: inline-block; background: var(--bg-hover); border: 1px solid var(--border);
	border-radius: 3px; padding: 0.05rem 0.3rem; margin: 0.1rem 0.1rem 0 0;
	font-family: monospace;
}

.svc-actions { display: flex; gap: 0.3rem; flex-wrap: wrap; }
.sm-btn {
	padding: 0.15rem 0.5rem; border-radius: 4px; border: 1px solid var(--border);
	background: var(--bg-hover); color: var(--fg); cursor: pointer; font-size: 0.78rem;
}
.sm-btn:hover:not(:disabled) { background: var(--accent); color: #fff; border-color: var(--accent); }
.sm-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* ── File editor ── */
.file-panel { padding: 0.75rem 1rem; display: flex; flex-direction: column; gap: 0.5rem; }
.file-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; flex-wrap: wrap; }
.file-path { font-family: monospace; font-size: 0.78rem; }
.file-actions { display: flex; gap: 0.4rem; align-items: center; }
.yaml-editor {
	width: 100%; box-sizing: border-box;
	font-family: monospace; font-size: 0.82rem; line-height: 1.5;
	background: #0d1117; color: #c9d1d9; border: 1px solid var(--border);
	border-radius: 4px; padding: 0.75rem; resize: vertical; min-height: 300px;
	tab-size: 2;
}
.yaml-editor.tall { min-height: 250px; }
.dirty-hint { color: #f39c12; font-size: 0.8rem; margin: 0; }
.btn-primary.sm { padding: 0.15rem 0.6rem; font-size: 0.78rem; }

/* ── Logs ── */
.logs-panel { padding: 0.75rem 1rem; display: flex; flex-direction: column; gap: 0.5rem; }
.logs-toolbar { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
.logs-toolbar label { display: flex; align-items: center; gap: 0.4rem; font-size: 0.83rem; }
.logs-toolbar select {
	background: var(--bg-hover); border: 1px solid var(--border); border-radius: 4px;
	color: var(--fg); padding: 0.15rem 0.4rem; font-size: 0.82rem;
}
.log-output {
	background: #0d1117; color: #c9d1d9; border: 1px solid var(--border);
	border-radius: 4px; padding: 0.75rem; font-size: 0.78rem; line-height: 1.5;
	overflow: auto; max-height: 400px; white-space: pre; tab-size: 2; margin: 0;
}

/* ── Modals ── */
.modal-backdrop {
	position: fixed; inset: 0; background: rgba(0,0,0,0.6);
	display: flex; align-items: center; justify-content: center; z-index: 500;
}
.modal {
	background: var(--bg-card); border: 1px solid var(--border); border-radius: 10px;
	padding: 1.5rem; width: 560px; max-width: 95vw; max-height: 90vh; overflow-y: auto;
	display: flex; flex-direction: column; gap: 1rem;
}
.modal h2 { margin: 0; font-size: 1.1rem; }

.form-group { display: flex; flex-direction: column; gap: 0.3rem; }
.form-group label { font-size: 0.85rem; font-weight: 500; }
.form-group input {
	background: var(--bg-hover); border: 1px solid var(--border); border-radius: 4px;
	color: var(--fg); padding: 0.4rem 0.6rem; font-size: 0.9rem;
}
.form-group input:focus { outline: 2px solid var(--accent); }
.hint { font-size: 0.78rem; color: var(--fg-muted); }

.form-check { display: flex; align-items: center; gap: 0.5rem; font-size: 0.85rem; }
.form-check input { accent-color: var(--accent); width: 14px; height: 14px; }
.form-check label { display: flex; align-items: center; gap: 0.5rem; cursor: pointer; }
.form-check code { background: var(--bg-hover); padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.78rem; }

.modal-footer { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.5rem; }
.modal-footer button {
	padding: 0.4rem 0.9rem; border-radius: 5px; border: 1px solid var(--border);
	background: var(--bg-hover); color: var(--fg); cursor: pointer; font-size: 0.88rem;
}
.modal-footer button.btn-primary {
	background: var(--accent); color: #fff; border-color: var(--accent);
}
.modal-footer button.danger {
	background: var(--danger, #e74c3c); color: #fff; border-color: transparent;
}
.modal-footer button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
