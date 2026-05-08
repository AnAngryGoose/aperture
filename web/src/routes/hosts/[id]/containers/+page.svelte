<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container, CreateSpec, CreatePortBinding, CreateVolumeBinding } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import { formatBytes, formatPct, relTime } from '$lib/format';

	let id = $derived(page.params.id);
	let containers = $state<Container[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let logsFor = $state<string | null>(null);
	let logsText = $state<string>('');
	let timer: ReturnType<typeof setInterval> | null = null;

	// --- Surface "New container" form state. Deep config (capabilities,
	// healthcheck, ulimits, etc.) is intentionally omitted; that lands with
	// the compose-first work where YAML is the natural surface for it.
	let showCreate = $state(false);
	let creating = $state(false);
	let createError = $state<string | null>(null);
	type EnvRow = { key: string; value: string };
	const blankForm = () => ({
		image: '',
		name: '',
		restart_policy: '' as CreateSpec['restart_policy'],
		auto_start: true,
		envRows: [] as EnvRow[],
		ports: [] as CreatePortBinding[],
		volumes: [] as CreateVolumeBinding[]
	});
	let form = $state(blankForm());

	function openCreate() {
		form = blankForm();
		createError = null;
		showCreate = true;
	}

	async function submitCreate(ev: Event) {
		ev.preventDefault();
		createError = null;
		const env: Record<string, string> = {};
		for (const r of form.envRows) {
			if (r.key.trim()) env[r.key.trim()] = r.value;
		}
		const spec: CreateSpec = {
			image: form.image.trim(),
			name: form.name.trim() || undefined,
			restart_policy: form.restart_policy || undefined,
			env: Object.keys(env).length ? env : undefined,
			ports: form.ports.length ? form.ports : undefined,
			volumes: form.volumes.length ? form.volumes : undefined,
			auto_start: form.auto_start
		};
		if (!spec.image) {
			createError = 'image is required';
			return;
		}
		creating = true;
		try {
			const res = await api.createContainer(id, spec);
			showCreate = false;
			await refresh();
			if (res.warning) {
				error = `created ${res.id.slice(0, 12)} but: ${res.warning}`;
			}
		} catch (e) {
			createError = (e as Error).message;
		} finally {
			creating = false;
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

	async function act(cid: string, action: string) {
		busy[cid] = true;
		try {
			await api.containerAction(id, cid, action);
			await refresh();
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
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			busy[cid] = false;
		}
	}

	async function showLogs(cid: string) {
		logsFor = cid;
		logsText = 'loading…';
		try {
			logsText = await api.containerLogs(id, cid, 500);
		} catch (e) {
			logsText = `error: ${(e as Error).message}`;
		}
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
			.join(', ');
	}
</script>

<div class="page-header">
	<div>
		<a href={`/hosts/${id}`} class="back">← back to host</a>
		<h1>Containers</h1>
		<div class="muted">{containers.length} total · {containers.filter((c) => c.state === 'running').length} running</div>
	</div>
	<div class="actions">
		<button onclick={openCreate}>+ New container</button>
	</div>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

<div class="card no-pad">
	<table>
		<thead>
			<tr>
				<th>Name</th>
				<th>Image</th>
				<th>State</th>
				<th>CPU</th>
				<th>Memory</th>
				<th>Ports</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tbody>
			{#each containers as c (c.id)}
				<tr>
					<td>
						<div class="cname">{c.name || c.id.slice(0, 12)}</div>
						<div class="muted mono small">{c.id.slice(0, 12)} · {relTime(c.created_at)}</div>
					</td>
					<td class="mono small">{c.image}</td>
					<td><span class="pill {c.state}">{c.state}</span></td>
					<td>
						{#if c.state === 'running'}
							<div class="mono small">{formatPct(c.cpu_percent)}</div>
							<div class="bar"><div class="fill" style="width: {Math.min(100, c.cpu_percent)}%"></div></div>
						{:else}
							<span class="muted">–</span>
						{/if}
					</td>
					<td>
						{#if c.state === 'running'}
							<div class="mono small">{formatBytes(c.mem_usage)} / {formatBytes(c.mem_limit)}</div>
							<Bar value={c.mem_percent} />
						{:else}
							<span class="muted">–</span>
						{/if}
					</td>
					<td class="mono small">{portLabel(c) || '—'}</td>
					<td class="actions">
						{#if c.state === 'running'}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'pause')}>Pause</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'restart')}>Restart</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'stop')}>Stop</button>
						{:else if c.state === 'paused'}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'unpause')}>Unpause</button>
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'stop')}>Stop</button>
						{:else}
							<button disabled={busy[c.id]} onclick={() => act(c.id, 'start')}>Start</button>
							<button class="danger" disabled={busy[c.id]} onclick={() => remove(c.id, false)}>Remove</button>
						{/if}
						<button onclick={() => showLogs(c.id)}>Logs</button>
					</td>
				</tr>
			{/each}
			{#if containers.length === 0 && !error}
				<tr><td colspan="7" class="muted center">no containers</td></tr>
			{/if}
		</tbody>
	</table>
</div>

{#if logsFor}
	<div class="modal-bg" onclick={() => (logsFor = null)} role="presentation">
		<div class="modal" onclick={(e) => e.stopPropagation()} role="dialog">
			<div class="modal-head">
				<div class="mono small">logs · {logsFor.slice(0, 12)}</div>
				<button onclick={() => (logsFor = null)}>close</button>
			</div>
			<pre class="logs">{logsText}</pre>
		</div>
	</div>
{/if}

{#if showCreate}
	<div class="modal-bg" onclick={() => (showCreate = false)} role="presentation">
		<div class="modal create" onclick={(e) => e.stopPropagation()} role="dialog">
			<div class="modal-head">
				<div>New container</div>
				<button onclick={() => (showCreate = false)}>close</button>
			</div>
			<form onsubmit={submitCreate} class="create-form">
				<label class="row">
					<span>Image *</span>
					<input type="text" placeholder="nginx:alpine" bind:value={form.image} autofocus />
				</label>
				<label class="row">
					<span>Name</span>
					<input type="text" placeholder="(auto-generated if blank)" bind:value={form.name} />
				</label>
				<label class="row">
					<span>Restart policy</span>
					<select bind:value={form.restart_policy}>
						<option value="">(default — no)</option>
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
					<div class="form-err">{createError}</div>
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
	.page-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
	.back { font-size: 12px; color: var(--text-dim); }
	h1 { margin: 4px 0; font-size: 22px; font-weight: 600; }
	.no-pad { padding: 0; }
	.cname { font-weight: 500; }
	.small { font-size: 11px; }
	.actions { display: flex; gap: 6px; flex-wrap: wrap; }
	.center { text-align: center; padding: 32px; }
	.err { color: var(--bad); border-color: var(--bad); }
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
		padding: 12px 16px;
		display: flex; justify-content: space-between; align-items: center;
		border-bottom: 1px solid var(--border);
	}
	.logs {
		margin: 0; padding: 16px;
		overflow: auto;
		font-family: var(--mono);
		font-size: 12px;
		white-space: pre-wrap;
		word-break: break-all;
	}

	.modal.create { width: min(90vw, 640px); }
	.create-form {
		padding: 16px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.create-form .row { display: grid; grid-template-columns: 140px 1fr; gap: 10px; align-items: center; }
	.create-form .row > span { font-size: 12px; color: var(--text-dim); }
	.create-form .row.checkbox { grid-template-columns: 140px auto 1fr; }
	.create-form input[type="text"], .create-form input[type="number"], .create-form select {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 6px 8px;
		font: inherit;
	}
	.create-form .section-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-top: 6px;
		padding-top: 8px;
		border-top: 1px solid var(--border);
		font-size: 12px;
		color: var(--text-dim);
	}
	.kvpair, .portrow, .volrow {
		display: grid;
		gap: 6px;
		align-items: center;
	}
	.kvpair { grid-template-columns: 1fr 2fr 32px; }
	.portrow { grid-template-columns: 1fr 16px 1fr 100px 32px; }
	.volrow { grid-template-columns: 1fr 16px 1fr auto 32px; }
	.arrow { color: var(--text-dim); text-align: center; font-family: var(--mono); }
	.x {
		padding: 4px 8px;
		background: transparent;
		border: 1px solid transparent;
		color: var(--text-dim);
	}
	.x:hover:not(:disabled) { color: var(--bad); border-color: var(--bad); }
	.checkbox.inline { display: flex; align-items: center; gap: 4px; }
	.form-err { color: var(--bad); font-size: 12px; }
	.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
