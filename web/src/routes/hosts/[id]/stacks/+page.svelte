<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { ComposeStack, ComposeVersion } from '$lib/types';
	import { toast } from '$lib/toast';
	import Button from '$lib/components/primitives/Button.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';
	import Modal from '$lib/components/primitives/Modal.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
	let stacks = $state<ComposeStack[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let composeAvailable = $state(true);

	let expanded = $state<Record<string, boolean>>({});
	let activeTab = $state<Record<string, 'services' | 'file' | 'logs'>>({});
	let actionBusy = $state<Record<string, boolean>>({});
	let actionOutput = $state<Record<string, string>>({});

	let fileContent = $state<Record<string, string>>({});
	let fileLoading = $state<Record<string, boolean>>({});
	let fileDirty = $state<Record<string, boolean>>({});
	let fileSaving = $state<Record<string, boolean>>({});

	let logContent = $state<Record<string, string>>({});
	let logLoading = $state<Record<string, boolean>>({});
	let logService = $state<Record<string, string>>({});
	let logTail = $state<Record<string, number>>({});

	let historyOpen = $state<Record<string, boolean>>({});
	let historyLoading = $state<Record<string, boolean>>({});
	let versions = $state<Record<string, ComposeVersion[]>>({});

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

	let deleteTarget = $state<string | null>(null);
	let deleteVolumes = $state(false);
	let deleteBusy = $state(false);

	let timer: ReturnType<typeof setInterval> | null = null;

	async function load(quiet = false) {
		if (!quiet) loading = true;
		try {
			const s = await api.composeStacks(id).catch((e: Error) => {
				if (e.message.includes('503') || e.message.includes('not available')) {
					composeAvailable = false;
				}
				return [] as ComposeStack[];
			});

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
			if (idx !== -1) stacks[idx] = res;
		} catch (e) {
			toast.error(`Failed to load services: ${(e as Error).message}`);
		}
	}

	function toggleExpand(project: string) {
		expanded[project] = !expanded[project];
		if (expanded[project]) {
			if (!activeTab[project]) activeTab[project] = 'services';
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

	async function loadHistory(project: string) {
		historyOpen[project] = true;
		historyLoading[project] = true;
		try {
			versions[project] = await api.composeVersions(id, project);
		} catch (e) {
			toast.error(`History error: ${(e as Error).message}`);
		} finally {
			historyLoading[project] = false;
		}
	}

	async function restoreVersion(project: string, vid: number) {
		try {
			const v = await api.composeVersionContent(id, vid);
			if (v && v.content) {
				fileContent[project] = v.content;
				fileDirty[project] = true;
				historyOpen[project] = false;
				toast.info('Version loaded into editor — Save or Save + Deploy to apply.');
			}
		} catch(e) {
			toast.error(`Restore error: ${(e as Error).message}`);
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
			actionOutput[project] = (e as Error).message;
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
		deleteBusy = true;
		try {
			await api.deleteComposeStack(id, deleteTarget, deleteVolumes);
			toast.success(`Stack "${deleteTarget}" stopped`);
			deleteTarget = null;
			load(true);
		} catch (e) {
			toast.error(`Down failed: ${(e as Error).message}`);
		} finally {
			deleteBusy = false;
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

	// Humanized stack health: instead of "0/0 running RUNNING", we read the
	// running/total counts and the backend-reported status string to derive
	// one of: running, partial, stopped, empty, unknown, error.
	type Health = 'running' | 'partial' | 'stopped' | 'empty' | 'unknown' | 'error';

	function healthOf(st: ComposeStack): Health {
		const status = (st.status || '').toLowerCase();
		const total = st.total_count ?? st.services?.length ?? 0;
		const running = st.running_count ?? 0;
		if (status === 'error') return 'error';
		if (total === 0) return 'empty';
		if (running === total && total > 0) return 'running';
		if (running > 0 && running < total) return 'partial';
		if (running === 0) return 'stopped';
		return 'unknown';
	}

	function healthLabel(h: Health): string {
		switch (h) {
			case 'running':  return 'Running';
			case 'partial':  return 'Partial';
			case 'stopped':  return 'Stopped';
			case 'empty':    return 'No services';
			case 'error':    return 'Error';
			default:         return 'Unknown';
		}
	}

	function healthSummary(st: ComposeStack): string {
		const total = st.total_count ?? st.services?.length ?? 0;
		const running = st.running_count ?? 0;
		if (total === 0) return 'no services defined';
		return `${running} of ${total} running`;
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

	// Confirmation for stack actions
	type Pending = {
		project: string;
		action: 'down' | 'restart' | 'pull';
		title: string;
		message: string;
		detail: string;
		consequences: string[];
		tone: 'warning' | 'danger';
		confirmLabel: string;
	} | null;
	let pending = $state<Pending>(null);

	function confirmStackAction(project: string, action: 'restart' | 'pull') {
		const labels: Record<typeof action, Omit<NonNullable<Pending>, 'project' | 'action'>> = {
			restart: {
				title: 'Restart stack',
				message: 'Restart all services in this stack?',
				detail: project,
				tone: 'warning',
				confirmLabel: 'Restart stack',
				consequences: ['Containers will briefly stop and start again.', 'In-flight requests may be dropped.']
			},
			pull: {
				title: 'Pull stack images',
				message: 'Pull the latest images for every service in this stack?',
				detail: project,
				tone: 'warning',
				confirmLabel: 'Pull images',
				consequences: ['Newer images will be downloaded.', 'You will still need to recreate the stack to use them.']
			}
		};
		pending = { project, action, ...labels[action] };
	}

	async function runPending() {
		if (!pending) return;
		const { project, action } = pending;
		pending = null;
		await stackAction(project, action);
	}
</script>

<svelte:window onkeydown={handleKeydown} />

<section class="stacks-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Compose Stacks</h2>
			<span class="lead-sub mono">{stacks.length} stack{stacks.length === 1 ? '' : 's'}</span>
		</div>
		<Button variant="primary" onclick={() => { newModal = true; newError = ''; }}>+ New Stack</Button>
	</header>

	{#if loading}
		<div class="state-msg">Loading stacks…</div>
	{:else if !composeAvailable}
		<div class="banner">
			<strong>Docker Compose not available</strong>
			<p>This host is offline, or <code>docker compose</code> is not installed.</p>
		</div>
	{:else if error}
		<div class="error-banner">{error}</div>
	{:else if stacks.length === 0}
		<div class="empty-state">
			<p>No compose stacks found on this host.</p>
			<p class="muted">Start one with <code>docker compose up -d</code> or create a new stack.</p>
			<Button variant="primary" onclick={() => { newModal = true; }}>+ New Stack</Button>
		</div>
	{:else}
		<div class="stack-list">
			{#each stacks as st (st.project)}
				{@const isExpanded = expanded[st.project]}
				{@const busy = anyBusy(st.project)}
				{@const h = healthOf(st)}
				<div class="stack-card" class:expanded={isExpanded}>
					<div class="stack-header" role="button" tabindex="0"
						onclick={() => toggleExpand(st.project)}
						onkeydown={(e) => e.key === 'Enter' && toggleExpand(st.project)}>
						<span class="status-dot status-{h}" title={healthLabel(h)}></span>
						<div class="stack-identity">
							<span class="stack-name">{st.project}</span>
							{#if st.working_dir}
								<span class="stack-dir mono" title={st.working_dir}>{st.working_dir}</span>
							{/if}
						</div>
						<div class="stack-meta">
							<span class="svc-count mono">{healthSummary(st)}</span>
							<span class="pill {h === 'running' ? 'ok' : h === 'partial' ? 'warn' : h === 'error' ? 'crit' : 'offline'}">{healthLabel(h)}</span>
						</div>
						<div class="stack-actions" role="none" onclick={(e) => e.stopPropagation()}>
							<Button variant="icon" size="sm" ariaLabel="Start / up" title="Start / up" disabled={busy}
								onclick={() => stackAction(st.project, 'up')}>
								<Icon name="play" size={12} />
							</Button>
							<Button variant="icon" size="sm" ariaLabel="Stop / down" title="Stop / down" disabled={busy}
								onclick={() => { deleteTarget = st.project; deleteVolumes = false; }}>
								<Icon name="stop" size={12} />
							</Button>
							<Button variant="icon" size="sm" ariaLabel="Restart" title="Restart" disabled={busy}
								onclick={() => confirmStackAction(st.project, 'restart')}>
								<Icon name="restart" size={12} />
							</Button>
							<Button variant="icon" size="sm" ariaLabel="Pull images" title="Pull images" disabled={busy}
								onclick={() => confirmStackAction(st.project, 'pull')}>
								<Icon name="arrow-down" size={12} />
							</Button>
						</div>
						<span class="chev" class:open={isExpanded}>
							<Icon name="chevron-down" size={14} />
						</span>
					</div>

					{#if actionOutput[st.project]}
						<pre class="action-output">{actionOutput[st.project]}</pre>
					{/if}

					{#if isExpanded}
						<div class="stack-detail">
							<div class="tab-bar">
								<Button variant="outline" size="sm" active={activeTab[st.project] === 'services'}
									onclick={() => setTab(st.project, 'services')}>Services</Button>
								<Button variant="outline" size="sm" active={activeTab[st.project] === 'file'}
									onclick={() => setTab(st.project, 'file')}>Compose File</Button>
								<Button variant="outline" size="sm" active={activeTab[st.project] === 'logs'}
									onclick={() => setTab(st.project, 'logs')}>Logs</Button>
								<div class="tab-spacer"></div>
								<Button variant="danger" size="sm" onclick={() => { deleteTarget = st.project; deleteVolumes = false; }}>
									Down…
								</Button>
							</div>

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
															<span class="cid mono">{svc.container_id.slice(0, 12)}</span>
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
																<span class="port-tag mono">{p.public_port}:{p.private_port}/{p.type}</span>
															{/each}
														{:else}
															<span class="muted">—</span>
														{/if}
													</td>
													<td class="svc-actions">
														{#if svc.state === 'running'}
															<Button variant="mini" size="sm"
																disabled={isBusy(st.project, 'restart', svc.name)}
																onclick={() => stackAction(st.project, 'restart', svc.name)}>
																Restart
															</Button>
															<Button variant="mini" size="sm"
																disabled={isBusy(st.project, 'stop', svc.name)}
																onclick={() => stackAction(st.project, 'stop', svc.name)}>
																Stop
															</Button>
														{:else}
															<Button variant="mini" size="sm"
																disabled={isBusy(st.project, 'start', svc.name)}
																onclick={() => stackAction(st.project, 'start', svc.name)}>
																Start
															</Button>
														{/if}
														<Button variant="mini" size="sm"
															onclick={() => { setTab(st.project, 'logs'); logService[st.project] = svc.name; loadLogs(st.project); }}>
															Logs
														</Button>
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								{:else}
									<p class="muted pad">No containers found — stack may be stopped or loading.</p>
								{/if}
							{/if}

							{#if activeTab[st.project] === 'file'}
								<div class="file-panel">
									{#if fileLoading[st.project]}
										<p class="muted">Loading compose file…</p>
									{:else}
										<div class="file-toolbar">
											<span class="file-path mono">{st.working_dir}/compose.yml</span>
											<div class="file-actions">
												<Button variant="ghost" size="sm" onclick={() => loadHistory(st.project)}>History</Button>
												<Button variant="ghost" size="sm" onclick={() => loadFile(st.project)}>Reload</Button>
												<Button variant="ghost" size="sm" disabled={fileSaving[st.project] || !fileDirty[st.project]}
													onclick={() => saveFile(st.project)}>Save</Button>
												<Button variant="primary" size="sm" loading={fileSaving[st.project]}
													onclick={() => saveFile(st.project, true)}>Save + Deploy</Button>
											</div>
										</div>
										<textarea class="yaml-editor"
											value={fileContent[st.project] ?? ''}
											oninput={(e) => { fileContent[st.project] = (e.target as HTMLTextAreaElement).value; fileDirty[st.project] = true; }}
											spellcheck={false}
											placeholder="Compose YAML will appear here…"></textarea>
										{#if fileDirty[st.project]}
											<p class="dirty-hint mono">Unsaved changes — Save to write, Save + Deploy to write and restart.</p>
										{/if}

										<Modal open={!!historyOpen[st.project]} onclose={() => historyOpen[st.project] = false} title="Version history" width="440px">
											<div class="history-body">
												{#if historyLoading[st.project]}
													<p class="muted">Loading history…</p>
												{:else if !versions[st.project] || versions[st.project].length === 0}
													<p class="muted">No backup versions found.</p>
												{:else}
													<ul class="version-list">
														{#each versions[st.project] as v}
															<li>
																<div class="v-time mono">{new Date(v.created_at).toLocaleString()}</div>
																<Button variant="ghost" size="sm" onclick={() => restoreVersion(st.project, v.id)}>Restore</Button>
															</li>
														{/each}
													</ul>
												{/if}
											</div>
										</Modal>
									{/if}
								</div>
							{/if}

							{#if activeTab[st.project] === 'logs'}
								<div class="logs-panel">
									<div class="logs-toolbar">
										<label>
											<span class="label-mono">Service</span>
											<select value={logService[st.project] ?? ''}
												onchange={(e) => { logService[st.project] = (e.target as HTMLSelectElement).value; }}>
												<option value="">All services</option>
												{#each (st.services ?? []) as svc}
													<option value={svc.name}>{svc.name}</option>
												{/each}
											</select>
										</label>
										<label>
											<span class="label-mono">Lines</span>
											<select value={logTail[st.project] ?? 200}
												onchange={(e) => { logTail[st.project] = Number((e.target as HTMLSelectElement).value); }}>
												<option value={50}>50</option>
												<option value={200}>200</option>
												<option value={500}>500</option>
												<option value={1000}>1000</option>
											</select>
										</label>
										<Button variant="ghost" size="sm" onclick={() => loadLogs(st.project)}
											loading={logLoading[st.project]}>
											{logLoading[st.project] ? 'Loading…' : 'Refresh'}
										</Button>
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
</section>

<!-- New stack modal -->
<Modal open={newModal} onclose={() => (newModal = false)} title="New Compose Stack" width="620px">
	<div class="form-stack">
		<div class="form-group">
			<label for="new-dir" class="label-mono">Directory path on host</label>
			<input id="new-dir" type="text" bind:value={newDir}
				placeholder="/opt/myapp or ~/stacks/nginx" />
			<span class="hint">The compose.yml will be written here. Directory is created if it doesn't exist.</span>
		</div>
		<div class="form-group">
			<label for="new-yaml" class="label-mono">compose.yml content</label>
			<textarea id="new-yaml" class="yaml-editor tall" bind:value={newContent} spellcheck={false}></textarea>
		</div>
		<label class="form-check">
			<input type="checkbox" bind:checked={newStart} />
			Start stack immediately after creating
		</label>
		{#if newError}
			<p class="error-banner">{newError}</p>
		{/if}
		<div class="modal-footer">
			<Button variant="ghost" onclick={() => (newModal = false)}>Cancel</Button>
			<Button variant="primary" loading={newSaving} onclick={createStack}>
				{newSaving ? 'Creating…' : 'Create Stack'}
			</Button>
		</div>
	</div>
</Modal>

<ConfirmDialog
	open={!!deleteTarget}
	tone="danger"
	title="Bring stack down"
	message="Stop and remove all containers for this stack?"
	detail={deleteTarget ?? ''}
	consequences={[
		'Equivalent to `docker compose down`.',
		'All services in this stack will stop and their containers will be deleted.',
		deleteVolumes ? 'Named volumes for this stack will also be removed.' : 'Named volumes are preserved.'
	]}
	confirmLabel="Bring down"
	busy={deleteBusy}
	onconfirm={doDelete}
	oncancel={() => (deleteTarget = null)}
/>

<ConfirmDialog
	open={pending !== null}
	tone={pending?.tone ?? 'warning'}
	title={pending?.title ?? ''}
	message={pending?.message ?? ''}
	detail={pending?.detail}
	consequences={pending?.consequences ?? []}
	confirmLabel={pending?.confirmLabel ?? 'Confirm'}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<style>
	.stacks-tab { display: flex; flex-direction: column; gap: 12px; }

	.tab-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.lead { display: flex; align-items: baseline; gap: 10px; }
	.lead h2 { margin: 0; font-size: 16px; font-weight: 600; color: var(--text); letter-spacing: -0.01em; }
	.lead-sub { font-size: 11px; color: var(--text-faint); }

	.state-msg { padding: 24px; text-align: center; color: var(--text-faint); font-size: 13px; }
	.banner, .empty-state {
		background: var(--bg-elev); border: 1px solid var(--line);
		border-radius: var(--r-lg); padding: 32px; text-align: center;
		color: var(--text-dim);
	}
	.empty-state p { margin: 6px 0; }
	.empty-state code { background: var(--bg-elev-2); padding: 1px 6px; border-radius: 3px; font-family: var(--font-mono); font-size: 12px; }
	.banner strong { display: block; margin-bottom: 6px; font-size: 14px; color: var(--text); }
	.banner code { background: var(--bg-elev-2); padding: 1px 6px; border-radius: 3px; font-family: var(--font-mono); font-size: 12px; }
	.error-banner {
		padding: 8px 12px;
		background: var(--crit-soft);
		border: 1px solid color-mix(in srgb, var(--crit) 40%, transparent);
		border-radius: var(--r-md);
		color: var(--crit);
		font-size: 12px;
		margin: 0;
	}

	.muted { color: var(--text-faint); }
	.pad { padding: 16px; margin: 0; }

	.stack-list { display: flex; flex-direction: column; gap: 8px; }

	.stack-card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
		transition: border-color 120ms;
	}
	.stack-card.expanded { border-color: var(--line-strong); }

	.stack-header {
		display: flex; align-items: center; gap: 10px;
		padding: 10px 14px; cursor: pointer; user-select: none;
		transition: background 120ms;
	}
	.stack-header:hover { background: var(--bg-hover); }

	.status-dot {
		width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
	}
	.status-dot.status-running { background: var(--ok); }
	.status-dot.status-partial { background: var(--warn); }
	.status-dot.status-stopped { background: var(--offline); }
	.status-dot.status-empty { background: var(--offline); }
	.status-dot.status-error { background: var(--crit); }
	.status-dot.status-unknown { background: var(--text-faint); }

	.stack-identity {
		flex: 1; min-width: 0;
		display: flex; flex-direction: column; gap: 2px;
	}
	.stack-name { font-weight: 500; font-size: 13px; color: var(--text); }
	.stack-dir { font-size: 11px; color: var(--text-faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 400px; }

	.stack-meta { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
	.svc-count { font-size: 11px; color: var(--text-dim); }

	.stack-actions { display: flex; gap: 4px; flex-shrink: 0; }

	.chev { color: var(--text-faint); transition: transform 200ms; flex-shrink: 0; display: inline-flex; }
	.chev.open { transform: rotate(180deg); }

	.action-output {
		margin: 0; padding: 10px 14px;
		font-family: var(--font-mono); font-size: 11px;
		background: var(--bg);
		color: var(--text-dim);
		border-top: 1px solid var(--line);
		white-space: pre-wrap; max-height: 120px; overflow-y: auto;
	}

	.stack-detail { border-top: 1px solid var(--line); }

	.tab-bar {
		display: flex; gap: 6px; padding: 8px 14px;
		border-bottom: 1px solid var(--line);
		background: var(--bg-elev-2);
		align-items: center;
	}
	.tab-spacer { flex: 1; }

	.svc-table { width: 100%; border-collapse: collapse; font-size: 12px; }
	.svc-table th {
		text-align: left; padding: 6px 14px; font-weight: 500;
		color: var(--text-faint); font-family: var(--font-mono);
		font-size: 10px; letter-spacing: 0.08em; text-transform: uppercase;
		border-bottom: 1px solid var(--line);
	}
	.svc-table td { padding: 8px 14px; border-bottom: 1px solid var(--line); vertical-align: middle; color: var(--text-dim); }
	.svc-table tr:last-child td { border-bottom: none; }
	.svc-table tr:hover td { background: var(--bg-hover); color: var(--text); }

	.svc-name { font-weight: 500; color: var(--text); }
	.cid { font-size: 10px; color: var(--text-faint); margin-left: 6px; }

	.svc-state {
		display: inline-block; font-size: 10px; padding: 1px 7px;
		border-radius: var(--r-pill); font-weight: 500;
		text-transform: uppercase; letter-spacing: 0.04em;
		font-family: var(--font-mono);
	}
	.svc-running { background: var(--ok-soft); color: var(--ok); }
	.svc-paused  { background: var(--warn-soft); color: var(--warn); }
	.svc-exited  { background: rgba(107,114,128,0.14); color: var(--offline); }
	.svc-dead    { background: var(--crit-soft); color: var(--crit); }

	.health-badge {
		display: inline-block; font-size: 9px; padding: 1px 6px;
		border-radius: var(--r-pill); font-weight: 500; margin-left: 4px;
		font-family: var(--font-mono);
	}
	.health-ok       { background: var(--ok-soft); color: var(--ok); }
	.health-bad      { background: var(--crit-soft); color: var(--crit); }
	.health-starting { background: var(--warn-soft); color: var(--warn); }

	.svc-status { color: var(--text-faint); font-size: 11px; }
	.port-tag {
		display: inline-block; background: var(--bg-elev-2); border: 1px solid var(--line);
		border-radius: 3px; padding: 1px 6px; margin: 1px 2px 1px 0;
		font-size: 10px;
	}

	.svc-actions { display: flex; gap: 4px; flex-wrap: wrap; }

	.file-panel { padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; }
	.file-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
	.file-path { font-size: 11px; color: var(--text-faint); }
	.file-actions { display: flex; gap: 6px; align-items: center; }
	.yaml-editor {
		width: 100%; box-sizing: border-box;
		font-family: var(--font-mono); font-size: 12px; line-height: 1.5;
		background: var(--bg); color: var(--text); border: 1px solid var(--line);
		border-radius: var(--r-md); padding: 10px; resize: vertical; min-height: 300px;
		tab-size: 2;
	}
	.yaml-editor.tall { min-height: 250px; }
	.dirty-hint { color: var(--warn); font-size: 11px; margin: 0; }

	.logs-panel { padding: 12px 14px; display: flex; flex-direction: column; gap: 8px; }
	.logs-toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
	.logs-toolbar label { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text-dim); }
	.logs-toolbar select {
		background: var(--bg-elev-2); border: 1px solid var(--line); border-radius: var(--r-md);
		color: var(--text); padding: 4px 8px; font-size: 12px;
	}
	.log-output {
		background: var(--bg); color: var(--text);
		border: 1px solid var(--line);
		border-radius: var(--r-md); padding: 10px;
		font-family: var(--font-mono); font-size: 11px; line-height: 1.5;
		overflow: auto; max-height: 400px; white-space: pre; tab-size: 2; margin: 0;
	}

	.label-mono {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
	}

	.form-stack { display: flex; flex-direction: column; gap: 12px; }
	.form-group { display: flex; flex-direction: column; gap: 4px; }
	.form-group input {
		background: var(--bg-elev-2); border: 1px solid var(--line);
		color: var(--text); padding: 6px 10px; border-radius: var(--r-md); font-size: 12px;
	}
	.form-group input:focus { outline: none; border-color: var(--accent-line); }
	.hint { font-size: 11px; color: var(--text-faint); }
	.form-check { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--text); cursor: pointer; }
	.form-check input { accent-color: var(--accent); }
	.modal-footer { display: flex; justify-content: flex-end; gap: 8px; }

	.history-body { display: flex; flex-direction: column; gap: 8px; }
	.version-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
	.version-list li {
		display: flex; align-items: center; justify-content: space-between;
		padding: 8px 10px; background: var(--bg-elev-2);
		border: 1px solid var(--line); border-radius: var(--r-md);
	}
	.v-time { color: var(--text-dim); font-size: 11px; }
</style>
