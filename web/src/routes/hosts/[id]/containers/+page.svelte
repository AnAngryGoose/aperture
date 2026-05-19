<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container, ContainerInspect, CreateSpec, CreatePortBinding, CreateVolumeBinding } from '$lib/types';
	import Terminal from '$lib/components/Terminal.svelte';
	import Button from '$lib/components/primitives/Button.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';
	import Modal from '$lib/components/primitives/Modal.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import { formatBytes, formatPct, relTime, absTime } from '$lib/format';

	let id = $derived(page.params.id ?? '');
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

	const STATE_FILTERS: Array<typeof stateFilter> = ['all', 'running', 'exited', 'paused'];

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
			inspectData = await api.containerInspect(id, inspectCid);
		} catch (e) {
			resError = (e as Error).message;
		} finally {
			resBusy = false;
		}
	}

	// --- Logs modal ---
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

	// --- Action confirmations ---
	type Pending = {
		title: string;
		message: string;
		detail: string;
		consequences: string[];
		tone: 'warning' | 'danger';
		confirmLabel: string;
		run: () => Promise<void>;
	} | null;
	let pending = $state<Pending>(null);
	let pendingBusy = $state(false);

	async function runPending() {
		if (!pending) return;
		pendingBusy = true;
		try {
			await pending.run();
		} finally {
			pendingBusy = false;
			pending = null;
		}
	}

	function containerLabel(c: Container) {
		const name = c.name?.replace(/^\//, '') || c.id.slice(0, 12);
		return `${name} (${c.id.slice(0, 12)})`;
	}

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

	function confirmStop(c: Container) {
		pending = {
			title: 'Stop container',
			message: 'Stop this container?',
			detail: containerLabel(c),
			tone: 'warning',
			confirmLabel: 'Stop container',
			consequences: ['Processes inside the container will receive SIGTERM and then exit.', 'Any data held only in memory will be lost.'],
			run: () => act(c.id, 'stop')
		};
	}

	function confirmRestart(c: Container) {
		pending = {
			title: 'Restart container',
			message: 'Restart this container?',
			detail: containerLabel(c),
			tone: 'warning',
			confirmLabel: 'Restart container',
			consequences: ['The container will briefly stop and start again.', 'In-flight requests may be dropped.'],
			run: () => act(c.id, 'restart')
		};
	}

	function confirmRecreate(c: Container) {
		pending = {
			title: 'Recreate container',
			message: 'Stop and recreate this container from its current image and config?',
			detail: containerLabel(c),
			tone: 'warning',
			confirmLabel: 'Recreate container',
			consequences: ['The existing container will be removed.', 'A new container will be created with the same image and configuration.'],
			run: async () => {
				try {
					const res = await api.containerRecreate(id, c.id);
					if (inspectCid === c.id) { inspectCid = null; inspectData = null; }
					await refresh();
					if (res.warning) error = `recreated but: ${res.warning}`;
				} catch (e) {
					error = (e as Error).message;
				}
			}
		};
	}

	function confirmRemove(c: Container, force: boolean) {
		pending = {
			title: force ? 'Force remove container' : 'Remove container',
			message: force
				? 'Force remove this container? It will be stopped and deleted immediately.'
				: 'Remove this container?',
			detail: containerLabel(c),
			tone: 'danger',
			confirmLabel: force ? 'Force remove' : 'Remove',
			consequences: force
				? ['The container is stopped without grace period.', 'Anonymous volumes attached to it are deleted.']
				: ['The container must already be stopped; otherwise this will fail.', 'Named volumes are preserved; anonymous volumes are deleted.'],
			run: async () => {
				try {
					await api.containerRemove(id, c.id, force);
					if (inspectCid === c.id) { inspectCid = null; inspectData = null; }
					await refresh();
				} catch (e) {
					error = (e as Error).message;
				}
			}
		};
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
		if (pending) { pending = null; return; }
		if (inspectCid) { inspectCid = null; inspectData = null; return; }
	}

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 5000);
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

	// Row action menu (More menu)
	let moreFor = $state<string | null>(null);
	function toggleMore(cid: string) {
		moreFor = moreFor === cid ? null : cid;
	}
	function closeMore() { moreFor = null; }
</script>

<svelte:window onkeydown={handleKeydown} onclick={closeMore} />

<section class="containers-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Containers</h2>
			<span class="lead-sub mono">{containers.length} total · {running} running</span>
		</div>
		<Button variant="primary" onclick={openCreate}>+ New Container</Button>
	</header>

	{#if error}
		<div class="error-banner">
			<span>{error}</span>
			<Button variant="icon" size="sm" ariaLabel="Dismiss" onclick={() => (error = null)}>
				<Icon name="x" size={12} />
			</Button>
		</div>
	{/if}

	<div class="toolbar">
		<div class="filters">
			{#each STATE_FILTERS as f}
				<Button
					variant="outline"
					size="sm"
					active={stateFilter === f}
					onclick={() => (stateFilter = f)}
				>
					{f}
				</Button>
			{/each}
		</div>

		<input
			class="search"
			type="search"
			placeholder="Filter by name or image…"
			bind:value={nameFilter}
		/>

		<div class="sorts mono">
			<span class="sort-label">Sort</span>
			{#each [['name','Name'],['state','State'],['cpu','CPU'],['mem','Mem']] as [k,label]}
				<Button
					variant="outline"
					size="sm"
					active={sortKey === k}
					onclick={() => toggleSort(k as SortKey)}
				>
					{label} {sortIcon(k as SortKey)}
				</Button>
			{/each}
		</div>
	</div>

	<div class="table-wrap">
		<table>
			<thead>
				<tr>
					<th>Name / ID</th>
					<th>Image</th>
					<th>State</th>
					<th>CPU</th>
					<th>Memory</th>
					<th>Ports</th>
					<th class="actions-col">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered() as c (c.id)}
					<tr class:expanded={inspectCid === c.id} onclick={() => openInspect(c.id)}>
						<td>
							<div class="cname">{c.name || c.id.slice(0, 12)}</div>
							<div class="muted mono micro">{c.id.slice(0, 12)} · <span title={absTime(c.created_at)}>{relTime(c.created_at)}</span></div>
						</td>
						<td class="mono micro muted">{c.image}</td>
						<td><span class="pill {c.state}">{c.state}</span></td>
						<td>
							{#if c.state === 'running'}
								<div class="mono micro">{formatPct(c.cpu_percent)}</div>
								<div class="meter"><div class="meter-fill" style="width:{Math.min(100,c.cpu_percent)}%"></div></div>
							{:else}<span class="muted">–</span>{/if}
						</td>
						<td>
							{#if c.state === 'running'}
								<div class="mono micro">{formatBytes(c.mem_usage)} / {formatBytes(c.mem_limit)}</div>
								<div class="meter"><div class="meter-fill" style="width:{Math.min(100,c.mem_percent)}%"></div></div>
							{:else}<span class="muted">–</span>{/if}
						</td>
						<td class="mono micro">{portLabel(c) || '—'}</td>
						<td class="actions" onclick={(e) => e.stopPropagation()}>
							<Button variant="mini" size="sm" onclick={() => showLogs(c.id)}>Logs</Button>
							{#if c.state === 'running'}
								<Button variant="mini" size="sm" onclick={() => openTerminal(c.id)}>Terminal</Button>
							{/if}
							<div class="more-wrap">
								<Button
									variant="icon"
									size="sm"
									ariaLabel="More actions"
									onclick={(e) => { e.stopPropagation(); toggleMore(c.id); }}
								>
									<Icon name="more" size={14} />
								</Button>
								{#if moreFor === c.id}
									<!-- svelte-ignore a11y_no_static_element_interactions -->
									<!-- svelte-ignore a11y_click_events_have_key_events -->
									<div class="more-menu" onclick={(e) => e.stopPropagation()}>

										{#if c.state === 'running'}
											<button onclick={() => { closeMore(); confirmRestart(c); }}>Restart…</button>
											<button onclick={() => { closeMore(); confirmStop(c); }}>Stop…</button>
											<button disabled={busy[c.id]} onclick={() => { closeMore(); act(c.id, 'pause'); }}>Pause</button>
										{:else if c.state === 'paused'}
											<button disabled={busy[c.id]} onclick={() => { closeMore(); act(c.id, 'unpause'); }}>Unpause</button>
											<button onclick={() => { closeMore(); confirmStop(c); }}>Stop…</button>
										{:else}
											<button disabled={busy[c.id]} onclick={() => { closeMore(); act(c.id, 'start'); }}>Start</button>
										{/if}
										<button onclick={() => { closeMore(); confirmRecreate(c); }}>Recreate…</button>
										<button onclick={() => { closeMore(); openInspect(c.id); }}>Inspect</button>
										<div class="menu-sep"></div>
										<button class="menu-danger" onclick={() => { closeMore(); confirmRemove(c, false); }}>Remove…</button>
										<button class="menu-danger" onclick={() => { closeMore(); confirmRemove(c, true); }}>Force Remove…</button>
									</div>
								{/if}
							</div>
						</td>
					</tr>

					{#if inspectCid === c.id}
						<tr class="inspect-row">
							<td colspan="7" class="inspect-cell">
								{#if inspectLoading}
									<div class="muted micro">Loading…</div>
								{:else if inspectData}
									<div class="inspect-grid">
										<div class="inspect-col">
											<div class="inspect-section">
												<div class="section-label">Configuration</div>
												<div class="kv">
													<span class="k">Image</span><span class="v mono">{inspectData.image}</span>
													<span class="k">State</span><span class="v"><span class="pill {inspectData.state}">{inspectData.state}</span></span>
													<span class="k">Restart</span><span class="v mono">{inspectData.restart_policy || 'no'}</span>
													{#if inspectData.cmd?.length}
														<span class="k">Cmd</span><span class="v mono micro">{inspectData.cmd.join(' ')}</span>
													{/if}
													{#if inspectData.entrypoint?.length}
														<span class="k">Entrypoint</span><span class="v mono micro">{inspectData.entrypoint.join(' ')}</span>
													{/if}
												</div>
											</div>

											{#if inspectData.ports.length > 0}
												<div class="inspect-section">
													<div class="section-label">Port bindings</div>
													{#each inspectData.ports.filter(p => p.public_port) as p}
														<div class="mono micro">{p.public_port} → {p.private_port}/{p.type}</div>
													{/each}
												</div>
											{/if}

											{#if inspectData.mounts.length > 0}
												<div class="inspect-section">
													<div class="section-label">Mounts</div>
													{#each inspectData.mounts as m}
														<div class="mount-row">
															<span class="pill micro">{m.type}</span>
															<span class="mono micro">{m.source} → {m.destination}</span>
															{#if !m.rw}<span class="muted mono micro">ro</span>{/if}
														</div>
													{/each}
												</div>
											{/if}

											{#if inspectData.env.length > 0}
												<div class="inspect-section">
													<div class="section-label env-head">
														<span>Environment ({inspectData.env.length})</span>
														<Button variant="mini" size="sm" onclick={() => (showEnv = !showEnv)}>{showEnv ? 'Hide' : 'Show'}</Button>
													</div>
													{#if showEnv}
														<div class="env-list">
															{#each inspectData.env as kv}
																<div class="mono micro env-line">{kv}</div>
															{/each}
														</div>
													{/if}
												</div>
											{/if}
										</div>

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
														<Button variant="mini" size="sm" onclick={() => (resEditing = true)}>Edit</Button>
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
															<span class="muted micro">CPU cores (0 = unlimited)</span>
															<input type="number" min="0" step="0.1" placeholder="e.g. 2.0" bind:value={resNanoCPUs} />
														</label>
														<label class="res-row">
															<span class="muted micro">Memory GiB (0 = unlimited)</span>
															<input type="number" min="0" step="0.1" placeholder="e.g. 1.5" bind:value={resMemGB} />
														</label>
														{#if resError}<div class="err-text">{resError}</div>{/if}
														<div class="res-actions">
															<Button variant="ghost" size="sm" onclick={() => (resEditing = false)}>Cancel</Button>
															<Button variant="primary" size="sm" loading={resBusy} onclick={saveResources}>Save</Button>
														</div>
													</div>
												{/if}
											</div>

											{#if Object.keys(inspectData.labels).length > 0}
												<div class="inspect-section">
													<div class="section-label">Labels</div>
													<div class="env-list">
														{#each Object.entries(inspectData.labels).slice(0,10) as [k,v]}
															<div class="mono micro">{k}={v}</div>
														{/each}
														{#if Object.keys(inspectData.labels).length > 10}
															<div class="muted micro">…{Object.keys(inspectData.labels).length - 10} more</div>
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
					<tr><td colspan="7" class="empty-row">No containers{stateFilter !== 'all' ? ` in state "${stateFilter}"` : ''}{nameFilter.trim() ? ` matching "${nameFilter.trim()}"` : ''}.</td></tr>
				{/if}
			</tbody>
		</table>
	</div>
</section>

<!-- Logs modal -->
<Modal open={logsFor !== null} onclose={() => (logsFor = null)} title="Container logs" width="900px">
	<div class="logs-modal">
		<div class="logs-head">
			<span class="mono micro">{logsFor ? logsFor.slice(0, 12) : ''}</span>
			<input class="search" type="search" placeholder="Filter…" bind:value={logsSearch} />
		</div>
		<pre class="logs-pre">{logsFiltered}</pre>
	</div>
</Modal>

<!-- Terminal modal -->
{#if terminalOpen}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<div class="modal-backdrop" onclick={() => (terminalOpen = false)}>
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div class="terminal-modal" onclick={(e) => e.stopPropagation()}>
			<Terminal hostId={id} cid={terminalCid} onClose={() => (terminalOpen = false)} />
		</div>
	</div>
{/if}

<!-- Create modal -->
<Modal open={showCreate} onclose={() => (showCreate = false)} title="New container" width="640px">
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
			<Button variant="mini" size="sm" onclick={() => form.envRows.push({ key: '', value: '' })}>+ Add</Button>
		</div>
		{#each form.envRows as row, i}
			<div class="kvpair">
				<input type="text" placeholder="KEY" bind:value={row.key} />
				<input type="text" placeholder="value" bind:value={row.value} />
				<Button variant="icon" size="sm" ariaLabel="Remove" onclick={() => form.envRows.splice(i, 1)}>
					<Icon name="x" size={12} />
				</Button>
			</div>
		{/each}

		<div class="section-head">
			<span>Port mappings</span>
			<Button variant="mini" size="sm" onclick={() => form.ports.push({ host_port: 0, container_port: 0, protocol: 'tcp' })}>+ Add</Button>
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
				<Button variant="icon" size="sm" ariaLabel="Remove" onclick={() => form.ports.splice(i, 1)}>
					<Icon name="x" size={12} />
				</Button>
			</div>
		{/each}

		<div class="section-head">
			<span>Volume mounts</span>
			<Button variant="mini" size="sm" onclick={() => form.volumes.push({ host_path: '', container_path: '', read_only: false })}>+ Add</Button>
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
				<Button variant="icon" size="sm" ariaLabel="Remove" onclick={() => form.volumes.splice(i, 1)}>
					<Icon name="x" size={12} />
				</Button>
			</div>
		{/each}

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
	open={pending !== null}
	title={pending?.title ?? ''}
	message={pending?.message ?? ''}
	detail={pending?.detail}
	consequences={pending?.consequences ?? []}
	tone={pending?.tone ?? 'danger'}
	confirmLabel={pending?.confirmLabel ?? 'Confirm'}
	busy={pendingBusy}
	onconfirm={runPending}
	oncancel={() => (pending = null)}
/>

<style>
	.containers-tab { display: flex; flex-direction: column; gap: 12px; }

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

	.toolbar {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}

	.filters, .sorts {
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.sort-label {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		margin-right: 4px;
	}

	.search {
		flex: 1;
		min-width: 160px;
		max-width: 280px;
		font-size: 12px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 6px 10px;
		color: var(--text);
	}
	.search:focus { outline: none; border-color: var(--accent-line); }

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

	.cname { font-weight: 500; color: var(--text); font-size: 12px; }
	.muted { color: var(--text-faint); }
	.micro { font-size: 10px; }

	.actions-col { text-align: right; }
	.actions {
		display: flex;
		gap: 4px;
		flex-wrap: nowrap;
		justify-content: flex-end;
		cursor: default;
		align-items: center;
		position: relative;
	}

	.more-wrap { position: relative; }
	.more-menu {
		position: absolute;
		right: 0;
		top: calc(100% + 4px);
		min-width: 160px;
		background: var(--bg-elev);
		border: 1px solid var(--line-strong);
		border-radius: var(--r-md);
		padding: 4px;
		z-index: 20;
		display: flex;
		flex-direction: column;
		box-shadow: 0 12px 32px -16px rgba(0,0,0,0.5);
	}
	.more-menu button {
		text-align: left;
		font: inherit;
		font-size: 12px;
		padding: 6px 10px;
		background: transparent;
		border: none;
		color: var(--text);
		cursor: pointer;
		border-radius: var(--r-sm);
	}
	.more-menu button:hover:not(:disabled) { background: var(--bg-hover); }
	.more-menu button:disabled { opacity: 0.4; cursor: not-allowed; }
	.menu-danger { color: var(--crit) !important; }
	.menu-sep { height: 1px; background: var(--line); margin: 4px 0; }

	.meter {
		position: relative;
		height: 4px;
		background: var(--bg-elev-2);
		border-radius: 2px;
		overflow: hidden;
		margin-top: 2px;
	}
	.meter-fill {
		position: absolute;
		left: 0; top: 0; bottom: 0;
		background: var(--accent);
		transition: width 0.4s ease;
	}

	.empty-row {
		text-align: center;
		padding: 32px 12px;
		color: var(--text-faint);
		font-size: 12px;
		cursor: default;
	}

	/* Inspect panel */
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
	.env-head { display: flex; justify-content: space-between; align-items: center; }

	.kv { display: grid; grid-template-columns: 80px 1fr; gap: 4px 12px; font-size: 12px; }
	.k { color: var(--text-faint); }
	.v { font-family: var(--font-mono); word-break: break-all; color: var(--text); }

	.mount-row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 11px; }
	.pill.micro { font-size: 10px; padding: 1px 5px; }

	.env-list {
		max-height: 200px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.env-line { word-break: break-all; color: var(--text-dim); }

	.res-form { display: flex; flex-direction: column; gap: 8px; }
	.res-row { display: flex; flex-direction: column; gap: 3px; }
	.res-row input { font-size: 12px; padding: 5px 8px; }
	.res-actions { display: flex; gap: 6px; justify-content: flex-end; }
	.err-text { color: var(--crit); font-size: 11px; }

	/* Modals */
	.logs-modal { display: flex; flex-direction: column; gap: 8px; max-height: 70vh; }
	.logs-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
	.logs-pre {
		margin: 0;
		padding: 12px;
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		overflow: auto;
		font-family: var(--font-mono);
		font-size: 11px;
		white-space: pre-wrap;
		word-break: break-all;
		flex: 1;
		min-height: 240px;
		max-height: 60vh;
	}

	.modal-backdrop {
		position: fixed;
		inset: 0;
		z-index: 100;
		display: flex;
		align-items: center;
		justify-content: center;
		background: rgba(0, 0, 0, 0.55);
		backdrop-filter: blur(6px) saturate(1.2);
	}
	.terminal-modal {
		width: min(90vw, 1000px);
		height: 80vh;
		display: flex;
		flex-direction: column;
		background: #0a0a0a;
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
	}

	.create-form { display: flex; flex-direction: column; gap: 10px; }
	.create-form .row { display: grid; grid-template-columns: 130px 1fr; gap: 10px; align-items: center; }
	.create-form .row > span { font-size: 12px; color: var(--text-dim); }
	.create-form .row.checkbox { grid-template-columns: 130px auto 1fr; }
	.create-form input[type="text"],
	.create-form input[type="number"],
	.create-form select {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		color: var(--text);
		padding: 6px 8px;
		font-size: 12px;
	}
	.section-head {
		display: flex; justify-content: space-between; align-items: center;
		margin-top: 6px; padding-top: 8px;
		border-top: 1px solid var(--line);
		font-size: 12px; color: var(--text-dim);
	}
	.kvpair, .portrow, .volrow { display: grid; gap: 6px; align-items: center; }
	.kvpair { grid-template-columns: 1fr 2fr 28px; }
	.portrow { grid-template-columns: 1fr 16px 1fr 90px 28px; }
	.volrow { grid-template-columns: 1fr 16px 1fr auto 28px; }
	.arrow { color: var(--text-faint); text-align: center; font-family: var(--font-mono); }
	.checkbox.inline { display: flex; align-items: center; gap: 4px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }

	/* Pills inherited from global styles for running/exited/paused */
</style>
