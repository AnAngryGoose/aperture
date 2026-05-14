<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container, ContainerInspect, CreateSpec, CreatePortBinding, CreateVolumeBinding } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import Terminal from '$lib/components/Terminal.svelte';
	import { formatBytes, formatPct, relTime, absTime } from '$lib/format';

	let id = $derived(page.params.id);
	let hostName = $state('');
	let containers = $state<Container[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let timer: ReturnType<typeof setInterval> | null = null;

	// --- Sort + filter ---
	type SortKey = 'name' | 'state' | 'cpu' | 'mem';
	let sortKey = $state<SortKey>('name');
	let sortAsc = $state(true);
	let stateFilter = $state<'all' | 'running' | 'exited' | 'paused'>('all');
	let nameFilter = $state('');

	let filtered = $derived(() => {
		const needle = nameFilter.toLowerCase().trim();
		let list = containers.filter((c) => {
			if (stateFilter !== 'all' && c.state !== stateFilter) return false;
			if (needle && !c.name.toLowerCase().includes(needle) && !c.image.toLowerCase().includes(needle)) return false;
			return true;
		});
		list = [...list].sort((a, b) => {
			let cmp = 0;
			if (sortKey === 'name') cmp = a.name.localeCompare(b.name);
			else if (sortKey === 'state') cmp = a.state.localeCompare(b.state);
			else if (sortKey === 'cpu') cmp = b.cpu_percent - a.cpu_percent;
			else if (sortKey === 'mem') cmp = b.mem_percent - a.mem_percent;
			return sortAsc ? cmp : -cmp;
		});
		return list;
	});

	function toggleSort(key: SortKey) {
		if (sortKey === key) sortAsc = !sortAsc;
		else { sortKey = key; sortAsc = true; }
	}
	function sortIcon(key: SortKey) {
		if (sortKey !== key) return '↕';
		return sortAsc ? '↑' : '↓';
	}

	// --- Deep inspect panel ---
	let inspectCid = $state<string | null>(null);
	let inspectData = $state<ContainerInspect | null>(null);
	let inspectLoading = $state(false);
	let showEnv = $state(false);

	// Resource update state (within inspect panel)
	let resEditing = $state(false);
	let resNanoCPUs = $state('');
	let resMemGB = $state('');
	let resBusy = $state(false);
	let resError = $state<string | null>(null);

	async function openInspect(cid: string) {
		if (inspectCid === cid) { inspectCid = null; inspectData = null; return; }
		inspectCid = cid;
		inspectData = null;
		inspectLoading = true;
		showEnv = false;
		resEditing = false;
		resError = null;
		try {
			inspectData = await api.containerInspect(id, cid);
			// Pre-fill resource fields from current config.
			resNanoCPUs = inspectData.nano_cpus > 0
				? (inspectData.nano_cpus / 1e9).toFixed(2)
				: '';
			resMemGB = inspectData.mem_limit_bytes > 0
				? (inspectData.mem_limit_bytes / 1073741824).toFixed(2)
				: '';
		} catch (e) {
			error = (e as Error).message;
			inspectCid = null;
		} finally {
			inspectLoading = false;
		}
	}

	async function saveResources() {
		if (!inspectCid) return;
		resBusy = true;
		resError = null;
		try {
			const update: { nano_cpus?: number; memory_bytes?: number } = {};
			const cpuVal = parseFloat(resNanoCPUs);
			if (resNanoCPUs.trim() !== '') {
				update.nano_cpus = isNaN(cpuVal) || cpuVal <= 0 ? 0 : Math.round(cpuVal * 1e9);
			}
			const memVal = parseFloat(resMemGB);
			if (resMemGB.trim() !== '') {
				update.memory_bytes = isNaN(memVal) || memVal <= 0 ? 0 : Math.round(memVal * 1073741824);
			}
			await api.containerUpdateResources(id, inspectCid, update);
			resEditing = false;
			// Re-fetch to show updated limits.
			inspectData = await api.containerInspect(id, inspectCid);
		} catch (e) {
			resError = (e as Error).message;
		} finally {
			resBusy = false;
		}
	}

	// --- Logs ---
	let logsFor = $state<string | null>(null);
	let logsText = $state<string>('');
	let logsSearch = $state('');
	let logsFiltered = $derived(
		logsSearch.trim()
			? logsText.split('\n').filter((l) => l.toLowerCase().includes(logsSearch.toLowerCase())).join('\n')
			: logsText
	);

	async function showLogs(cid: string) {
		logsFor = cid;
		logsText = 'loading…';
		logsSearch = '';
		try {
			logsText = await api.containerLogs(id, cid, { tail: 1000 });
		} catch (e) {
			logsText = `error: ${(e as Error).message}`;
		}
	}

	// --- Terminal ---
	let terminalOpen = $state(false);
	let terminalCid = $state('');

	function openTerminal(cid: string) {
		terminalCid = cid;
		terminalOpen = true;
	}

	// --- Actions ---
	async function act(cid: string, action: string) {
		busy[cid] = true;
		try {
			await api.containerAction(id, cid, action);
			await refresh();
			if (inspectCid === cid) {
				inspectData = await api.containerInspect(id, cid);
			}
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[cid] = false;
		}
	}

	async function remove(cid: string, force = false) {
		if (!confirm(`Remove container ${cid.slice(0, 12)}${force ? ' (force)' : ''}?`)) return;
		busy[cid] = true;
		try {
			await api.containerRemove(id, cid, force);
			if (inspectCid === cid) { inspectCid = null; inspectData = null; }
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[cid] = false;
		}
	}

	async function recreate(cid: string) {
		if (!confirm(`Recreate container ${cid.slice(0, 12)}? This will stop and remove it, then create a new one from the same image.`)) return;
		busy[cid] = true;
		error = null;
		try {
			const res = await api.containerRecreate(id, cid);
			if (inspectCid === cid) { inspectCid = null; inspectData = null; }
			await refresh();
			if (res.warning) error = `recreated but: ${res.warning}`;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[cid] = false;
		}
	}

	async function refresh() {
		try {
			containers = await api.containers(id, true);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key !== 'Escape') return;
		if (logsFor) { logsFor = null; return; }
		if (showCreate) { showCreate = false; return; }
		if (inspectCid) { inspectCid = null; inspectData = null; return; }
	}

	onMount(async () => {
		refresh();
		timer = setInterval(refresh, 5000);
		try { const h = await api.host(id); hostName = h.name; } catch { /* best-effort */ }
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	function portLabel(c: Container): string {
		return c.ports
			.filter((p) => p.public_port)
			.map((p) => `${p.public_port}→${p.private_port}/${p.type}`)
			.join(' ');
	}

	// --- Create modal ---
	let showCreate = $state(false);
	let creating = $state(false);
	let createError = $state<string | null>(null);
	type EnvRow = { key: string; value: string };
	const blankForm = () => ({
		image: '', name: '',
		restart_policy: '' as CreateSpec['restart_policy'],
		auto_start: true,
		envRows: [] as EnvRow[],
		ports: [] as CreatePortBinding[],
		volumes: [] as CreateVolumeBinding[]
	});
	let form = $state(blankForm());

	function openCreate() { form = blankForm(); createError = null; showCreate = true; }

	async function submitCreate(ev: Event) {
		ev.preventDefault();
		createError = null;
		const env: Record<string, string> = {};
		for (const r of form.envRows) { if (r.key.trim()) env[r.key.trim()] = r.value; }
		const spec: CreateSpec = {
			image: form.image.trim(),
			name: form.name.trim() || undefined,
			restart_policy: form.restart_policy || undefined,
			env: Object.keys(env).length ? env : undefined,
			ports: form.ports.length ? form.ports : undefined,
			volumes: form.volumes.length ? form.volumes : undefined,
			auto_start: form.auto_start
		};
		if (!spec.image) { createError = 'image is required'; return; }
		creating = true;
		try {
			const res = await api.createContainer(id, spec);
			showCreate = false;
			await refresh();
			if (res.warning) error = `created ${res.id.slice(0, 12)} but: ${res.warning}`;
		} catch (e) {
			createError = (e as Error).message;
		} finally {
			creating = false;
		}
	}

	let running = $derived(containers.filter((c) => c.state === 'running').length);
</script>

<svelte:head><title>Aperture — {hostName || id} — Containers</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<div>
		<a href={`/hosts/${id}`} class="back">← back to host</a>
		<h1>Containers</h1>
		<div class="muted small">{containers.length} total · {running} running</div>
	</div>
	<button onclick={openCreate}>+ New container</button>
</div>

<!-- Sub-navigation -->
<nav class="subnav">
	<a href={`/hosts/${id}`}>Overview</a>
	<a href={`/hosts/${id}/containers`} class="active">Containers</a>
	<a href={`/hosts/${id}/compose`}>Compose</a>
	<a href={`/hosts/${id}/networks`}>Networks</a>
	<a href={`/hosts/${id}/logs`}>Logs</a>
	<a href={`/hosts/${id}/volumes`}>Volumes</a>
	<a href={`/hosts/${id}/images`}>Images</a>
</nav>

{#if error}
	<div class="card err">{error} <button class="x" onclick={() => (error = null)}>×</button></div>
{/if}

<!-- Filter + sort toolbar -->
<div class="toolbar">
	<div class="filters">
		{#each ['all', 'running', 'exited', 'paused'] as f}
			<button class:active={stateFilter === f} onclick={() => (stateFilter = f as typeof stateFilter)}>{f}</button>
		{/each}
	</div>
	<input class="name-search" type="text" placeholder="search name / image…" bind:value={nameFilter} />
	<div class="sorts muted small">
		Sort:
		{#each [['name','Name'],['state','State'],['cpu','CPU'],['mem','Mem']] as [k,label]}
			<button class="sort-btn" class:active={sortKey === k} onclick={() => toggleSort(k as SortKey)}>
				{label} {sortIcon(k as SortKey)}
			</button>
		{/each}
	</div>
</div>

<div class="card no-pad">
	<table>
		<thead>
			<tr>
				<th>Name / ID</th>
				<th>Image</th>
				<th>State</th>
				<th>CPU</th>
				<th>Memory</th>
				<th>Ports</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>
			{#each filtered() as c (c.id)}
				<tr class:expanded={inspectCid === c.id} onclick={() => openInspect(c.id)}>
					<td>
						<div class="cname">{c.name || c.id.slice(0, 12)}</div>
						<div class="muted mono small">{c.id.slice(0, 12)} · <span title={absTime(c.created_at)}>{relTime(c.created_at)}</span></div>
					</td>
					<td class="mono small muted">{c.image}</td>
					<td><span class="pill {c.state}">{c.state}</span></td>
					<td>
						{#if c.state === 'running'}
							<div class="mono small">{formatPct(c.cpu_percent)}</div>
							<div class="bar"><div class="fill" style="width:{Math.min(100,c.cpu_percent)}%"></div></div>
						{:else}<span class="muted">–</span>{/if}
					</td>
					<td>
						{#if c.state === 'running'}
							<div class="mono small">{formatBytes(c.mem_usage)} / {formatBytes(c.mem_limit)}</div>
							<Bar value={c.mem_percent} />
						{:else}<span class="muted">–</span>{/if}
					</td>
					<td class="mono small">{portLabel(c) || '—'}</td>
					<td class="actions" onclick={(e) => e.stopPropagation()}>
						{#if c.state === 'running'}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'pause')}>Pause</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'restart')}>Restart</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'stop')}>Stop</button>
						{:else if c.state === 'paused'}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'unpause')}>Unpause</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'stop')}>Stop</button>
						{:else}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'start')}>Start</button>
							<button class="danger" disabled={busy[c.id]} onclick={() => remove(c.id)}>Remove</button>
						{/if}
						<button onclick={() => showLogs(c.id)}>Logs</button>
						{#if c.state === 'running'}
							<button onclick={() => openTerminal(c.id)}>Terminal</button>
						{/if}
					</td>
				</tr>

				<!-- Expand: deep inspect panel -->
				{#if inspectCid === c.id}
					<tr class="inspect-row">
						<td colspan="7" class="inspect-cell">
							{#if inspectLoading}
								<div class="muted small">Loading…</div>
							{:else if inspectData}
								<div class="inspect-grid">

									<!-- Left: config -->
									<div class="inspect-col">
										<div class="inspect-section">
											<div class="section-label">Configuration</div>
											<div class="kv">
												<span class="k">Image</span><span class="v mono">{inspectData.image}</span>
												<span class="k">State</span><span class="v"><span class="pill {inspectData.state}">{inspectData.state}</span></span>
												<span class="k">Restart</span><span class="v mono">{inspectData.restart_policy || 'no'}</span>
												{#if inspectData.cmd?.length}
													<span class="k">Cmd</span><span class="v mono small">{inspectData.cmd.join(' ')}</span>
												{/if}
												{#if inspectData.entrypoint?.length}
													<span class="k">Entrypoint</span><span class="v mono small">{inspectData.entrypoint.join(' ')}</span>
												{/if}
											</div>
										</div>

										{#if inspectData.ports.length > 0}
											<div class="inspect-section">
												<div class="section-label">Port bindings</div>
												{#each inspectData.ports.filter(p => p.public_port) as p}
													<div class="mono small">{p.public_port} → {p.private_port}/{p.type}</div>
												{/each}
											</div>
										{/if}

										{#if inspectData.mounts.length > 0}
											<div class="inspect-section">
												<div class="section-label">Mounts</div>
												{#each inspectData.mounts as m}
													<div class="mount-row">
														<span class="pill small">{m.type}</span>
														<span class="mono small">{m.source} → {m.destination}</span>
														{#if !m.rw}<span class="muted mono small">ro</span>{/if}
													</div>
												{/each}
											</div>
										{/if}

										{#if inspectData.env.length > 0}
											<div class="inspect-section">
												<div class="section-label env-head">
													<span>Environment ({inspectData.env.length})</span>
													<button class="x" onclick={() => (showEnv = !showEnv)}>{showEnv ? 'hide' : 'show'}</button>
												</div>
												{#if showEnv}
													<div class="env-list">
														{#each inspectData.env as kv}
															<div class="mono small env-line">{kv}</div>
														{/each}
													</div>
												{/if}
											</div>
										{/if}
									</div>

									<!-- Right: stats + resources + actions -->
									<div class="inspect-col">
										{#if inspectData.state === 'running'}
											<div class="inspect-section">
												<div class="section-label">Live stats</div>
												<div class="kv">
													<span class="k">CPU</span><span class="v mono">{formatPct(inspectData.cpu_percent)}</span>
													<span class="k">Memory</span><span class="v mono">{formatBytes(inspectData.mem_usage)} / {formatBytes(inspectData.mem_limit)} ({formatPct(inspectData.mem_percent)})</span>
													<span class="k">Net Rx</span><span class="v mono">{formatBytes(inspectData.net_rx_bytes)}</span>
													<span class="k">Net Tx</span><span class="v mono">{formatBytes(inspectData.net_tx_bytes)}</span>
												</div>
											</div>
										{/if}

										<div class="inspect-section">
											<div class="section-label env-head">
												<span>Resource limits</span>
												{#if !resEditing}
													<button class="x" onclick={() => (resEditing = true)}>edit</button>
												{/if}
											</div>
											{#if !resEditing}
												<div class="kv">
													<span class="k">CPU</span>
													<span class="v mono">{inspectData.nano_cpus > 0 ? (inspectData.nano_cpus/1e9).toFixed(2)+' cores' : 'unlimited'}</span>
													<span class="k">Memory</span>
													<span class="v mono">{inspectData.mem_limit_bytes > 0 ? formatBytes(inspectData.mem_limit_bytes) : 'unlimited'}</span>
												</div>
											{:else}
												<div class="res-form">
													<label class="res-row">
														<span class="muted small">CPU cores (0 = unlimited)</span>
														<input type="number" min="0" step="0.1" placeholder="e.g. 2.0" bind:value={resNanoCPUs} />
													</label>
													<label class="res-row">
														<span class="muted small">Memory GiB (0 = unlimited)</span>
														<input type="number" min="0" step="0.1" placeholder="e.g. 1.5" bind:value={resMemGB} />
													</label>
													{#if resError}<div class="err-text">{resError}</div>{/if}
													<div class="res-actions">
														<button onclick={() => (resEditing = false)}>Cancel</button>
														<button disabled={resBusy} onclick={saveResources}>{resBusy ? 'Saving…' : 'Save'}</button>
													</div>
												</div>
											{/if}
										</div>

										<div class="inspect-section">
											<div class="section-label">Actions</div>
											<div class="inspect-actions">
												{#if inspectData.state === 'running'}
													<button disabled={busy[c.id]} onclick={() => act(c.id, 'restart')}>Restart</button>
													<button disabled={busy[c.id]} onclick={() => act(c.id, 'stop')}>Stop</button>
													<button disabled={busy[c.id]} onclick={() => act(c.id, 'pause')}>Pause</button>
												{:else if inspectData.state === 'paused'}
													<button disabled={busy[c.id]} onclick={() => act(c.id, 'unpause')}>Unpause</button>
												{:else}
													<button disabled={busy[c.id]} onclick={() => act(c.id, 'start')}>Start</button>
												{/if}
												<button disabled={busy[c.id]} onclick={() => recreate(c.id)}>Recreate</button>
												<button onclick={() => showLogs(c.id)}>Logs</button>
												{#if inspectData.state === 'running'}
													<button onclick={() => openTerminal(c.id)}>Terminal</button>
												{/if}
												<button class="danger" disabled={busy[c.id]} onclick={() => remove(c.id, true)}>Force remove</button>
											</div>
										</div>

										{#if Object.keys(inspectData.labels).length > 0}
											<div class="inspect-section">
												<div class="section-label">Labels</div>
												<div class="env-list">
													{#each Object.entries(inspectData.labels).slice(0,10) as [k,v]}
														<div class="mono small">{k}={v}</div>
													{/each}
													{#if Object.keys(inspectData.labels).length > 10}
														<div class="muted small">…{Object.keys(inspectData.labels).length - 10} more</div>
													{/if}
												</div>
											</div>
										{/if}
									</div>
								</div>
							{/if}
						</td>
					</tr>
				{/if}
			{/each}
			{#if filtered().length === 0 && !error}
				<tr><td colspan="7" class="muted center">no containers{stateFilter !== 'all' ? ` in state "${stateFilter}"` : ''}{nameFilter.trim() ? ` matching "${nameFilter.trim()}"` : ''}</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<!-- Logs modal -->
{#if logsFor}
	<div class="modal-bg" onclick={() => (logsFor = null)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
			<div class="modal-head">
				<div class="mono small">logs · {logsFor.slice(0, 12)}</div>
				<div class="modal-head-right">
					<input class="search-input" type="text" placeholder="filter…" bind:value={logsSearch} />
					<button onclick={() => (logsFor = null)}>close</button>
				</div>
			</div>
			<pre class="logs">{logsFiltered}</pre>
		</div>
	</div>
{/if}

{#if terminalOpen}
	<div class="modal-backdrop" onclick={() => (terminalOpen = false)}>
		<div class="modal terminal-modal" onclick={(e) => e.stopPropagation()}>
			<Terminal hostId={id} cid={terminalCid} onClose={() => (terminalOpen = false)} />
		</div>
	</div>
{/if}

<!-- Create modal -->
{#if showCreate}
	<div class="modal-bg" onclick={() => (showCreate = false)} role="presentation">
		<div class="modal create" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
			<div class="modal-head">
				<div>New container</div>
				<button onclick={() => (showCreate = false)}>close</button>
			</div>
			<form onsubmit={submitCreate} class="create-form">
				<label class="row">
					<span>Image *</span>
					<input type="text" placeholder="nginx:alpine" bind:value={form.image} />
				</label>
				<label class="row">
					<span>Name</span>
					<input type="text" placeholder="(auto-generated)" bind:value={form.name} />
				</label>
				<label class="row">
					<span>Restart</span>
					<select bind:value={form.restart_policy}>
						<option value="">(no)</option>
						<option value="no">no</option>
						<option value="on-failure">on-failure</option>
						<option value="always">always</option>
						<option value="unless-stopped">unless-stopped</option>
					</select>
				</label>
				<label class="row checkbox">
					<input type="checkbox" bind:checked={form.auto_start} />
					<span>Start after create</span>
				</label>

				<div class="section-head">
					<span>Environment variables</span>
					<button type="button" onclick={() => form.envRows.push({ key: '', value: '' })}>+ add</button>
				</div>
				{#each form.envRows as row, i}
					<div class="kvpair">
						<input type="text" placeholder="KEY" bind:value={row.key} />
						<input type="text" placeholder="value" bind:value={row.value} />
						<button type="button" class="x" onclick={() => form.envRows.splice(i, 1)}>×</button>
					</div>
				{/each}

				<div class="section-head">
					<span>Port mappings</span>
					<button type="button" onclick={() => form.ports.push({ host_port: 0, container_port: 0, protocol: 'tcp' })}>+ add</button>
				</div>
				{#each form.ports as p, i}
					<div class="portrow">
						<input type="number" placeholder="host" min="0" bind:value={p.host_port} />
						<span class="arrow">→</span>
						<input type="number" placeholder="container *" min="1" bind:value={p.container_port} />
						<select bind:value={p.protocol}>
							<option value="tcp">tcp</option>
							<option value="udp">udp</option>
						</select>
						<button type="button" class="x" onclick={() => form.ports.splice(i, 1)}>×</button>
					</div>
				{/each}

				<div class="section-head">
					<span>Volume mounts</span>
					<button type="button" onclick={() => form.volumes.push({ host_path: '', container_path: '', read_only: false })}>+ add</button>
				</div>
				{#each form.volumes as v, i}
					<div class="volrow">
						<input type="text" placeholder="/host/path *" bind:value={v.host_path} />
						<span class="arrow">→</span>
						<input type="text" placeholder="/container/path *" bind:value={v.container_path} />
						<label class="checkbox inline">
							<input type="checkbox" bind:checked={v.read_only} />
							<span>ro</span>
						</label>
						<button type="button" class="x" onclick={() => form.volumes.splice(i, 1)}>×</button>
					</div>
				{/each}

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

<!-- Terminal Modal -->
{#if terminalOpen}
	<div class="modal-backdrop" onclick={() => (terminalOpen = false)}>
		<div class="modal terminal-modal" onclick={(e) => e.stopPropagation()}>
			<Terminal hostId={id} cid={terminalCid} onClose={() => (terminalOpen = false)} />
		</div>
	</div>
{/if}

<style>
	.page-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px; }
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }
	.small { font-size: 11px; }

	/* Sub-nav (same as host overview) */
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

	/* Toolbar */
	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 8px;
		flex-wrap: wrap;
		gap: 8px;
	}
	.filters { display: flex; gap: 4px; }
	.filters button.active { border-color: var(--accent); color: var(--accent); }
	.name-search {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 4px 10px;
		font: inherit;
		font-size: 12px;
		width: 200px;
	}
	.name-search:focus { outline: none; border-color: var(--accent); }
	.sorts { display: flex; align-items: center; gap: 4px; }
	.sort-btn { font-size: 11px; padding: 3px 8px; }
	.sort-btn.active { border-color: var(--accent); color: var(--accent); }

	/* Table */
	.no-pad { padding: 0; }
	.cname { font-weight: 500; }
	tr.expanded td { background: var(--bg-elev-2); }
	tr { cursor: pointer; }
	tr:hover td { background: var(--bg-elev-2); }
	.actions { display: flex; gap: 4px; flex-wrap: wrap; cursor: default; }
	.center { text-align: center; padding: 32px; cursor: default; }
	.err { color: var(--bad); border-color: var(--bad); display: flex; justify-content: space-between; align-items: center; }

	/* Inspect panel */
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
	.env-head { display: flex; justify-content: space-between; align-items: center; }

	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-dim); }
	.v { font-family: var(--mono); word-break: break-all; }

	.mount-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 11px; }
	.pill.small { font-size: 10px; padding: 1px 5px; }

	.env-list {
		max-height: 200px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.env-line { word-break: break-all; color: var(--text-dim); }

	/* Resource editing */
	.res-form { display: flex; flex-direction: column; gap: 8px; }
	.res-row { display: flex; flex-direction: column; gap: 3px; }
	.res-row input {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 5px 8px;
		font: inherit;
		font-size: 12px;
	}
	.res-actions { display: flex; gap: 6px; }
	.err-text { color: var(--bad); font-size: 11px; }

	.inspect-actions { display: flex; gap: 6px; flex-wrap: wrap; }

	/* Logs modal */
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
		width: min(90vw, 900px);
		max-height: 85vh;
		display: flex; flex-direction: column;
	}
	.modal-head {
		padding: 10px 16px;
		display: flex; justify-content: space-between; align-items: center;
		border-bottom: 1px solid var(--border);
		flex-shrink: 0;
	}
	.modal-head-right { display: flex; gap: 8px; align-items: center; }
	.search-input {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 4px 8px;
		font: inherit;
		font-size: 12px;
		width: 180px;
	}
	.logs {
		margin: 0; padding: 14px 16px;
		overflow: auto;
		font-family: var(--mono);
		font-size: 11px;
		white-space: pre-wrap;
		word-break: break-all;
		flex: 1;
	}

	.terminal-modal {
		width: min(90vw, 1000px);
		height: 80vh;
		display: flex;
		flex-direction: column;
		padding: 0;
		background: #1e1e1e;
	}

	/* Create modal */
	.modal.create { width: min(90vw, 640px); }
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
	.create-form input[type="number"],
	.create-form select {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 6px 8px;
		font: inherit;
	}
	.section-head {
		display: flex; justify-content: space-between; align-items: center;
		margin-top: 6px; padding-top: 8px;
		border-top: 1px solid var(--border);
		font-size: 12px; color: var(--text-dim);
	}
	.kvpair, .portrow, .volrow { display: grid; gap: 6px; align-items: center; }
	.kvpair { grid-template-columns: 1fr 2fr 32px; }
	.portrow { grid-template-columns: 1fr 16px 1fr 100px 32px; }
	.volrow { grid-template-columns: 1fr 16px 1fr auto 32px; }
	.arrow { color: var(--text-dim); text-align: center; font-family: var(--mono); }
	.x { padding: 3px 7px; background: transparent; border: 1px solid transparent; color: var(--text-dim); cursor: pointer; }
	.x:hover { color: var(--bad); border-color: var(--bad); }
	.checkbox.inline { display: flex; align-items: center; gap: 4px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
