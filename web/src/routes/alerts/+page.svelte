<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import type { AlertRule, AlertEvent, AlertMetadata, Host } from '$lib/types';
	import { relTime } from '$lib/format';

	let rules = $state<AlertRule[]>([]);
	let events = $state<AlertEvent[]>([]);
	let hosts = $state<Host[]>([]);
	let meta = $state<AlertMetadata | null>(null);
	let error = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	let form = $state({
		host_id: '' as string,
		metric: 'cpu_pct',
		op: '>',
		threshold: 90,
		duration_s: 60,
		enabled: true
	});
	let formError = $state<string | null>(null);
	let saving = $state(false);

	let hostsByID = $derived(Object.fromEntries(hosts.map((h) => [h.id, h])));
	let rulesByID = $derived(Object.fromEntries(rules.map((r) => [r.id, r])));
	let openEvents = $derived(events.filter((e) => !e.resolved_at));

	async function refresh() {
		try {
			const [rs, es, hs] = await Promise.all([
				api.alertRules(),
				api.alertEvents({ limit: 100 }),
				api.hosts()
			]);
			rules = rs;
			events = es;
			hosts = hs;
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function loadMeta() {
		try {
			meta = await api.alertMetadata();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function submit(ev: Event) {
		ev.preventDefault();
		formError = null;
		saving = true;
		try {
			await api.createAlertRule({
				host_id: form.host_id || undefined,
				metric: form.metric,
				op: form.op,
				threshold: Number(form.threshold),
				duration_s: Number(form.duration_s),
				enabled: form.enabled
			});
			form.threshold = 90;
			form.duration_s = 60;
			await refresh();
		} catch (e) {
			formError = (e as Error).message;
		} finally {
			saving = false;
		}
	}

	async function toggle(r: AlertRule) {
		try {
			await api.updateAlertRule(r.id, { ...r, enabled: !r.enabled });
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function remove(r: AlertRule) {
		if (!confirm(`Delete rule #${r.id} (${r.metric} ${r.op} ${r.threshold})?`)) return;
		try {
			await api.deleteAlertRule(r.id);
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	onMount(() => {
		loadMeta();
		refresh();
		timer = setInterval(refresh, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});

	function ruleSummary(r: AlertRule): string {
		const target = r.host_id ? (hostsByID[r.host_id]?.name ?? r.host_id) : 'all hosts';
		const dur = r.duration_s > 0 ? ` for ${r.duration_s}s` : '';
		return `${r.metric} ${r.op} ${r.threshold}${dur} on ${target}`;
	}
</script>

<div class="page-header">
	<h1>Alerts</h1>
	<span class="muted">{rules.length} rule{rules.length === 1 ? '' : 's'} · {openEvents.length} firing</span>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

<div class="card">
	<h2>New rule</h2>
	<form onsubmit={submit} class="rule-form">
		<label>
			<span>Host</span>
			<select bind:value={form.host_id}>
				<option value="">All hosts</option>
				{#each hosts as h}
					<option value={h.id}>{h.name}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Metric</span>
			<select bind:value={form.metric}>
				{#each meta?.metrics ?? [] as m}
					<option value={m}>{m}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Op</span>
			<select bind:value={form.op}>
				{#each meta?.ops ?? [] as o}
					<option value={o}>{o}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Threshold</span>
			<input type="number" step="any" bind:value={form.threshold} />
		</label>
		<label>
			<span>Duration (s)</span>
			<input type="number" min="0" bind:value={form.duration_s} />
		</label>
		<label class="checkbox">
			<input type="checkbox" bind:checked={form.enabled} />
			<span>Enabled</span>
		</label>
		<button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Add rule'}</button>
	</form>
	{#if formError}
		<div class="form-err">{formError}</div>
	{/if}
</div>

<div class="card no-pad">
	<h2 style="padding: 12px 16px 0">Rules</h2>
	<table>
		<thead>
			<tr>
				<th>ID</th>
				<th>Host</th>
				<th>Condition</th>
				<th>Duration</th>
				<th>Enabled</th>
				<th>Created</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#each rules as r (r.id)}
				<tr class:firing={openEvents.some((e) => e.rule_id === r.id)}>
					<td class="mono small">#{r.id}</td>
					<td>{r.host_id ? (hostsByID[r.host_id]?.name ?? r.host_id) : 'all'}</td>
					<td class="mono">{r.metric} {r.op} {r.threshold}</td>
					<td>{r.duration_s}s</td>
					<td>
						<button onclick={() => toggle(r)}>{r.enabled ? 'on' : 'off'}</button>
					</td>
					<td class="muted small">{relTime(r.created_at)}</td>
					<td><button class="danger" onclick={() => remove(r)}>delete</button></td>
				</tr>
			{/each}
			{#if rules.length === 0}
				<tr><td colspan="7" class="muted center">no rules yet</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<div class="card no-pad" style="margin-top: 16px">
	<h2 style="padding: 12px 16px 0">Events ({events.length})</h2>
	<table>
		<thead>
			<tr>
				<th>Status</th>
				<th>Rule</th>
				<th>Host</th>
				<th>Value</th>
				<th>Fired</th>
				<th>Resolved</th>
			</tr>
		</thead>
		<tbody>
			{#each events as e (e.id)}
				{@const r = rulesByID[e.rule_id]}
				<tr>
					<td>
						{#if e.resolved_at}
							<span class="pill exited">resolved</span>
						{:else}
							<span class="pill paused">firing</span>
						{/if}
					</td>
					<td class="mono small">{r ? ruleSummary(r) : `#${e.rule_id} (deleted)`}</td>
					<td>{hostsByID[e.host_id]?.name ?? e.host_id}</td>
					<td class="mono">{e.value.toFixed(2)}</td>
					<td class="muted small">{relTime(e.fired_at)}</td>
					<td class="muted small">{e.resolved_at ? relTime(e.resolved_at) : '—'}</td>
				</tr>
			{/each}
			{#if events.length === 0}
				<tr><td colspan="6" class="muted center">no events yet</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<style>
	.page-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 16px;
	}
	h1 { margin: 0; font-size: 20px; font-weight: 600; }
	h2 { margin: 0 0 12px; font-size: 14px; font-weight: 600; color: var(--text-dim); text-transform: uppercase; letter-spacing: 0.05em; }
	.rule-form {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: 12px;
		align-items: end;
	}
	.rule-form label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--text-dim); }
	.rule-form label.checkbox { flex-direction: row; align-items: center; gap: 6px; }
	.rule-form select, .rule-form input[type="number"] {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 6px 8px;
		font: inherit;
	}
	.form-err { color: var(--bad); margin-top: 8px; font-size: 12px; }
	.no-pad { padding: 0; }
	.no-pad h2 { margin-top: 12px; }
	.small { font-size: 11px; }
	.center { text-align: center; padding: 24px; }
	.firing { background: rgba(255, 107, 107, 0.06); }
	.err { color: var(--bad); border-color: var(--bad); }
</style>
