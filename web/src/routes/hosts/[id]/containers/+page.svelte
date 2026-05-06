<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container } from '$lib/types';
	import Bar from '$lib/Bar.svelte';
	import { formatBytes, formatPct, relTime } from '$lib/format';

	let id = $derived(page.params.id);
	let containers = $state<Container[]>([]);
	let error = $state<string | null>(null);
	let busy = $state<Record<string, boolean>>({});
	let logsFor = $state<string | null>(null);
	let logsText = $state<string>('');
	let timer: ReturnType<typeof setInterval> | null = null;

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
</style>
