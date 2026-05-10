<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import type { AlertRule, AlertEvent, AlertMetadata, AlertChannel, Host } from '$lib/types';
	import { relTime } from '$lib/format';

	// ── data state ──────────────────────────────────────────────────────────────
	let rules     = $state<AlertRule[]>([]);
	let events    = $state<AlertEvent[]>([]);
	let hosts     = $state<Host[]>([]);
	let channels  = $state<AlertChannel[]>([]);
	let meta      = $state<AlertMetadata | null>(null);
	let error     = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	let hostsByID  = $derived(Object.fromEntries(hosts.map(h => [h.id, h])));
	let rulesByID  = $derived(Object.fromEntries(rules.map(r => [r.id, r])));
	let openEvents = $derived(events.filter(e => !e.resolved_at));

	// ── tabs ────────────────────────────────────────────────────────────────────
	let tab = $state<'rules' | 'events' | 'channels'>('rules');

	// ── rule form ───────────────────────────────────────────────────────────────
	let ruleForm = $state({
		host_id: '' as string,
		metric: 'cpu_pct',
		op: '>',
		threshold: 90,
		duration_s: 60,
		severity: 'warning',
		enabled: true
	});
	let ruleFormError = $state<string | null>(null);
	let ruleFormSaving = $state(false);

	// ── channel modal ────────────────────────────────────────────────────────────
	type HeaderRow = { key: string; value: string };

	type ChForm = {
		id: number | null;
		name: string;
		type: string;
		enabled: boolean;
		min_severity: string;
		notify_resolve: boolean;
		// discord / slack
		webhook_url: string;
		// ntfy
		ntfy_url: string;
		ntfy_topic: string;
		ntfy_token: string;
		ntfy_priority: string;
		// gotify
		gotify_url: string;
		gotify_token: string;
		gotify_priority: string;
		// webhook
		wh_url: string;
		wh_method: string;
		wh_headers: HeaderRow[];
	};

	function emptyChForm(): ChForm {
		return {
			id: null, name: '', type: 'discord', enabled: true,
			min_severity: 'info', notify_resolve: true,
			webhook_url: '',
			ntfy_url: 'https://ntfy.sh', ntfy_topic: '', ntfy_token: '', ntfy_priority: '',
			gotify_url: '', gotify_token: '', gotify_priority: '',
			wh_url: '', wh_method: 'POST', wh_headers: []
		};
	}

	let chModalOpen  = $state(false);
	let chForm       = $state<ChForm>(emptyChForm());
	let chFormError  = $state<string | null>(null);
	let chFormSaving = $state(false);
	let chTestResult = $state<string | null>(null);

	const CHANNEL_TYPES = [
		{ id: 'discord',  label: 'Discord' },
		{ id: 'slack',    label: 'Slack' },
		{ id: 'ntfy',     label: 'ntfy' },
		{ id: 'gotify',   label: 'Gotify' },
		{ id: 'webhook',  label: 'Webhook' }
	];

	// Build config JSON from form fields
	function buildConfig(f: ChForm): Record<string, unknown> {
		switch (f.type) {
			case 'discord':
			case 'slack':
				return { webhook_url: f.webhook_url };
			case 'ntfy': {
				const c: Record<string, unknown> = { url: f.ntfy_url, topic: f.ntfy_topic };
				if (f.ntfy_token)    c.token    = f.ntfy_token;
				if (f.ntfy_priority) c.priority = f.ntfy_priority;
				return c;
			}
			case 'gotify': {
				const c: Record<string, unknown> = { url: f.gotify_url, token: f.gotify_token };
				if (f.gotify_priority) c.priority = Number(f.gotify_priority);
				return c;
			}
			case 'webhook': {
				const c: Record<string, unknown> = { url: f.wh_url, method: f.wh_method };
				if (f.wh_headers.length) {
					const headers: Record<string, string> = {};
					for (const h of f.wh_headers) if (h.key) headers[h.key] = h.value;
					c.headers = headers;
				}
				return c;
			}
		}
		return {};
	}

	// Populate form from an existing channel (for edit)
	function populateChForm(ch: AlertChannel) {
		const f = emptyChForm();
		f.id           = ch.id;
		f.name         = ch.name;
		f.type         = ch.type;
		f.enabled      = ch.enabled;
		f.min_severity = ch.min_severity;
		f.notify_resolve = ch.notify_resolve;
		const c = (ch.config ?? {}) as Record<string, unknown>;
		switch (ch.type) {
			case 'discord':
			case 'slack':
				f.webhook_url = String(c.webhook_url ?? '');
				break;
			case 'ntfy':
				f.ntfy_url      = String(c.url ?? 'https://ntfy.sh');
				f.ntfy_topic    = String(c.topic ?? '');
				f.ntfy_token    = String(c.token ?? '');
				f.ntfy_priority = String(c.priority ?? '');
				break;
			case 'gotify':
				f.gotify_url      = String(c.url ?? '');
				f.gotify_token    = String(c.token ?? '');
				f.gotify_priority = c.priority != null ? String(c.priority) : '';
				break;
			case 'webhook':
				f.wh_url    = String(c.url ?? '');
				f.wh_method = String(c.method ?? 'POST');
				if (c.headers && typeof c.headers === 'object') {
					f.wh_headers = Object.entries(c.headers as Record<string, string>)
						.map(([key, value]) => ({ key, value }));
				}
				break;
		}
		return f;
	}

	// ── data loading ─────────────────────────────────────────────────────────────
	async function refresh() {
		try {
			const [rs, es, hs, chs] = await Promise.all([
				api.alertRules(),
				api.alertEvents({ limit: 100 }),
				api.hosts(),
				api.alertChannels()
			]);
			rules    = rs;
			events   = es;
			hosts    = hs;
			channels = chs;
			error    = null;
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

	// ── rule actions ─────────────────────────────────────────────────────────────
	async function submitRule(ev: Event) {
		ev.preventDefault();
		ruleFormError = null;
		ruleFormSaving = true;
		try {
			await api.createAlertRule({
				host_id:    ruleForm.host_id || undefined,
				metric:     ruleForm.metric,
				op:         ruleForm.op,
				threshold:  Number(ruleForm.threshold),
				duration_s: Number(ruleForm.duration_s),
				severity:   ruleForm.severity,
				enabled:    ruleForm.enabled
			});
			ruleForm.threshold = 90;
			ruleForm.duration_s = 60;
			await refresh();
		} catch (e) {
			ruleFormError = (e as Error).message;
		} finally {
			ruleFormSaving = false;
		}
	}

	async function toggleRule(r: AlertRule) {
		try {
			await api.updateAlertRule(r.id, { ...r, enabled: !r.enabled });
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function removeRule(r: AlertRule) {
		if (!confirm(`Delete rule #${r.id} (${r.metric} ${r.op} ${r.threshold})?`)) return;
		try {
			await api.deleteAlertRule(r.id);
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	function ruleSummary(r: AlertRule): string {
		const target = r.host_id ? (hostsByID[r.host_id]?.name ?? r.host_id) : 'all hosts';
		const dur = r.duration_s > 0 ? ` for ${r.duration_s}s` : '';
		return `${r.metric} ${r.op} ${r.threshold}${dur} on ${target}`;
	}

	// ── channel actions ───────────────────────────────────────────────────────────
	function openAddChannel() {
		chForm      = emptyChForm();
		chFormError = null;
		chTestResult = null;
		chModalOpen = true;
	}

	function openEditChannel(ch: AlertChannel) {
		chForm      = populateChForm(ch);
		chFormError = null;
		chTestResult = null;
		chModalOpen = true;
	}

	async function saveChannel() {
		chFormError  = null;
		chTestResult = null;
		chFormSaving = true;
		try {
			const payload = {
				name:           chForm.name,
				type:           chForm.type,
				config:         buildConfig(chForm),
				enabled:        chForm.enabled,
				min_severity:   chForm.min_severity,
				notify_resolve: chForm.notify_resolve
			};
			if (chForm.id != null) {
				await api.updateAlertChannel(chForm.id, payload);
			} else {
				await api.createAlertChannel(payload);
			}
			chModalOpen = false;
			await refresh();
		} catch (e) {
			chFormError = (e as Error).message;
		} finally {
			chFormSaving = false;
		}
	}

	async function testChannel() {
		chFormError  = null;
		chTestResult = null;
		if (chForm.id == null) {
			chFormError = 'Save the channel first, then test it.';
			return;
		}
		chFormSaving = true;
		try {
			await api.testAlertChannel(chForm.id);
			chTestResult = 'Test sent successfully.';
		} catch (e) {
			chFormError = (e as Error).message;
		} finally {
			chFormSaving = false;
		}
	}

	async function toggleChannel(ch: AlertChannel) {
		try {
			await api.updateAlertChannel(ch.id, { ...ch, enabled: !ch.enabled });
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function removeChannel(ch: AlertChannel) {
		if (!confirm(`Delete channel "${ch.name}"?`)) return;
		try {
			await api.deleteAlertChannel(ch.id);
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function testExistingChannel(ch: AlertChannel) {
		try {
			await api.testAlertChannel(ch.id);
			alert(`Test sent to "${ch.name}".`);
		} catch (e) {
			alert(`Test failed: ${(e as Error).message}`);
		}
	}

	function addHeader() {
		chForm.wh_headers = [...chForm.wh_headers, { key: '', value: '' }];
	}
	function removeHeader(i: number) {
		chForm.wh_headers = chForm.wh_headers.filter((_, j) => j !== i);
	}

	// ── lifecycle ─────────────────────────────────────────────────────────────────
	onMount(() => {
		loadMeta();
		refresh();
		timer = setInterval(refresh, 5000);
	});
	onDestroy(() => { if (timer) clearInterval(timer); });

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && chModalOpen) chModalOpen = false;
	}

	const SEV_COLORS: Record<string, string> = {
		info: '#3498db', warning: '#f39c12', critical: '#e74c3c'
	};
</script>

<svelte:head><title>Aperture — Alerts</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<h1>Alerts</h1>
	<span class="muted">{rules.length} rule{rules.length === 1 ? '' : 's'} · {openEvents.length} firing</span>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

<!-- Tab bar -->
<div class="tabs">
	<button class:active={tab === 'rules'}    onclick={() => tab = 'rules'}>Rules</button>
	<button class:active={tab === 'events'}   onclick={() => tab = 'events'}>Events ({events.length})</button>
	<button class:active={tab === 'channels'} onclick={() => tab = 'channels'}>Channels ({channels.length})</button>
</div>

<!-- ── RULES TAB ─────────────────────────────────────────────────────────── -->
{#if tab === 'rules'}

<div class="card">
	<h2>New rule</h2>
	<form onsubmit={submitRule} class="rule-form">
		<label>
			<span>Host</span>
			<select bind:value={ruleForm.host_id}>
				<option value="">All hosts</option>
				{#each hosts as h}
					<option value={h.id}>{h.name}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Metric</span>
			<select bind:value={ruleForm.metric}>
				{#each meta?.metrics ?? [] as m}
					<option value={m}>{m}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Op</span>
			<select bind:value={ruleForm.op}>
				{#each meta?.ops ?? [] as o}
					<option value={o}>{o}</option>
				{/each}
			</select>
		</label>
		<label>
			<span>Threshold</span>
			<input type="number" step="any" bind:value={ruleForm.threshold} />
		</label>
		<label>
			<span>Duration (s)</span>
			<input type="number" min="0" bind:value={ruleForm.duration_s} />
		</label>
		<label>
			<span>Severity</span>
			<select bind:value={ruleForm.severity}>
				<option value="info">info</option>
				<option value="warning">warning</option>
				<option value="critical">critical</option>
			</select>
		</label>
		<label class="checkbox">
			<input type="checkbox" bind:checked={ruleForm.enabled} />
			<span>Enabled</span>
		</label>
		<button type="submit" disabled={ruleFormSaving}>{ruleFormSaving ? 'Saving…' : 'Add rule'}</button>
	</form>
	{#if ruleFormError}
		<div class="form-err">{ruleFormError}</div>
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
				<th>Severity</th>
				<th>Duration</th>
				<th>Enabled</th>
				<th>Created</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			{#each rules as r (r.id)}
				<tr class:firing={openEvents.some(e => e.rule_id === r.id)}>
					<td class="mono small">#{r.id}</td>
					<td>{r.host_id ? (hostsByID[r.host_id]?.name ?? r.host_id) : 'all'}</td>
					<td class="mono">{r.metric} {r.op} {r.threshold}</td>
					<td>
						<span class="sev-pill" style="background:{SEV_COLORS[r.severity ?? 'warning']}22;color:{SEV_COLORS[r.severity ?? 'warning']}">
							{r.severity ?? 'warning'}
						</span>
					</td>
					<td>{r.duration_s}s</td>
					<td>
						<button onclick={() => toggleRule(r)}>{r.enabled ? 'on' : 'off'}</button>
					</td>
					<td class="muted small">{relTime(r.created_at)}</td>
					<td><button class="danger" onclick={() => removeRule(r)}>delete</button></td>
				</tr>
			{/each}
			{#if rules.length === 0}
				<tr><td colspan="8" class="muted center">no rules yet</td></tr>
			{/if}
		</tbody>
	</table>
</div>

<!-- ── EVENTS TAB ─────────────────────────────────────────────────────────── -->
{:else if tab === 'events'}

<div class="card no-pad">
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

<!-- ── CHANNELS TAB ───────────────────────────────────────────────────────── -->
{:else if tab === 'channels'}

<div class="ch-header">
	<button class="btn-primary" onclick={openAddChannel}>+ Add Channel</button>
</div>

<div class="ch-list">
	{#each channels as ch (ch.id)}
		<div class="ch-card">
			<div class="ch-card-left">
				<div class="ch-name">{ch.name}</div>
				<div class="ch-meta">
					<span class="ch-type">{ch.type}</span>
					<span class="sev-pill" style="background:{SEV_COLORS[ch.min_severity]}22;color:{SEV_COLORS[ch.min_severity]}">
						{ch.min_severity}+
					</span>
					<span class="muted small">resolve: {ch.notify_resolve ? 'yes' : 'no'}</span>
				</div>
			</div>
			<div class="ch-card-right">
				<button onclick={() => testExistingChannel(ch)}>Test</button>
				<button onclick={() => openEditChannel(ch)}>Edit</button>
				<button onclick={() => toggleChannel(ch)} class:active-toggle={ch.enabled}>
					{ch.enabled ? 'enabled' : 'disabled'}
				</button>
				<button class="danger" onclick={() => removeChannel(ch)}>delete</button>
			</div>
		</div>
	{/each}
	{#if channels.length === 0}
		<div class="card muted center" style="padding: 32px">
			No channels yet. Add one to start receiving notifications.
		</div>
	{/if}
</div>

{/if}

<!-- ── CHANNEL MODAL ──────────────────────────────────────────────────────── -->
{#if chModalOpen}
<div class="modal-backdrop" onclick={() => (chModalOpen = false)} role="button" tabindex="-1" aria-label="Close"></div>
<div class="modal" role="dialog" aria-modal="true">
	<div class="modal-header">
		<h2>{chForm.id != null ? 'Edit' : 'Add'} Channel</h2>
		<button class="modal-close" onclick={() => (chModalOpen = false)}>✕</button>
	</div>

	<div class="modal-body">
		<!-- Type selector -->
		<div class="field">
			<label class="field-label">Type</label>
			<div class="type-btns">
				{#each CHANNEL_TYPES as ct}
					<button
						class="type-btn"
						class:selected={chForm.type === ct.id}
						onclick={() => { chForm.type = ct.id; }}
						type="button"
					>{ct.label}</button>
				{/each}
			</div>
		</div>

		<!-- Name -->
		<div class="field">
			<label class="field-label" for="ch-name">Name</label>
			<input id="ch-name" type="text" bind:value={chForm.name} placeholder="e.g. homelab-discord" />
		</div>

		<!-- Type-specific fields -->
		{#if chForm.type === 'discord' || chForm.type === 'slack'}
			<div class="field">
				<label class="field-label" for="ch-wh-url">Webhook URL</label>
				<input id="ch-wh-url" type="url" bind:value={chForm.webhook_url} placeholder="https://discord.com/api/webhooks/…" />
			</div>
		{:else if chForm.type === 'ntfy'}
			<div class="field">
				<label class="field-label" for="ntfy-url">Server URL</label>
				<input id="ntfy-url" type="url" bind:value={chForm.ntfy_url} placeholder="https://ntfy.sh" />
			</div>
			<div class="field">
				<label class="field-label" for="ntfy-topic">Topic</label>
				<input id="ntfy-topic" type="text" bind:value={chForm.ntfy_topic} placeholder="my-alerts" />
			</div>
			<div class="field">
				<label class="field-label" for="ntfy-token">Token <span class="optional">(optional)</span></label>
				<input id="ntfy-token" type="text" bind:value={chForm.ntfy_token} placeholder="tk_…" />
			</div>
			<div class="field">
				<label class="field-label" for="ntfy-prio">Priority <span class="optional">(auto if empty)</span></label>
				<select id="ntfy-prio" bind:value={chForm.ntfy_priority}>
					<option value="">auto</option>
					<option value="min">min</option>
					<option value="low">low</option>
					<option value="default">default</option>
					<option value="high">high</option>
					<option value="urgent">urgent</option>
				</select>
			</div>
		{:else if chForm.type === 'gotify'}
			<div class="field">
				<label class="field-label" for="gotify-url">Server URL</label>
				<input id="gotify-url" type="url" bind:value={chForm.gotify_url} placeholder="https://gotify.example.com" />
			</div>
			<div class="field">
				<label class="field-label" for="gotify-token">App Token</label>
				<input id="gotify-token" type="text" bind:value={chForm.gotify_token} placeholder="A…" />
			</div>
			<div class="field">
				<label class="field-label" for="gotify-prio">Priority <span class="optional">(0 = auto)</span></label>
				<input id="gotify-prio" type="number" min="0" max="10" bind:value={chForm.gotify_priority} placeholder="0" />
			</div>
		{:else if chForm.type === 'webhook'}
			<div class="field">
				<label class="field-label" for="wh-url">URL</label>
				<input id="wh-url" type="url" bind:value={chForm.wh_url} placeholder="https://…" />
			</div>
			<div class="field">
				<label class="field-label" for="wh-method">Method</label>
				<select id="wh-method" bind:value={chForm.wh_method}>
					<option value="POST">POST</option>
					<option value="GET">GET</option>
					<option value="PUT">PUT</option>
				</select>
			</div>
			<div class="field">
				<label class="field-label">Headers</label>
				{#each chForm.wh_headers as h, i}
					<div class="header-row">
						<input type="text" bind:value={h.key}   placeholder="X-Header-Name" />
						<input type="text" bind:value={h.value} placeholder="value" />
						<button type="button" onclick={() => removeHeader(i)} class="danger small-btn">✕</button>
					</div>
				{/each}
				<button type="button" onclick={addHeader} class="small-btn">+ Header</button>
			</div>
		{/if}

		<!-- Min severity -->
		<div class="field">
			<span class="field-label">Min severity</span>
			<div class="radio-row">
				{#each ['info', 'warning', 'critical'] as sev}
					<label class="radio-label">
						<input type="radio" name="min_sev" bind:group={chForm.min_severity} value={sev} />
						<span class="sev-pill" style="background:{SEV_COLORS[sev]}22;color:{SEV_COLORS[sev]}">{sev}</span>
					</label>
				{/each}
			</div>
		</div>

		<!-- Notify resolve -->
		<div class="field">
			<label class="field-label checkbox-label">
				<input type="checkbox" bind:checked={chForm.notify_resolve} />
				Notify when alert resolves
			</label>
		</div>

		<!-- Enabled -->
		<div class="field">
			<label class="field-label checkbox-label">
				<input type="checkbox" bind:checked={chForm.enabled} />
				Enabled
			</label>
		</div>

		{#if chFormError}
			<div class="form-err">{chFormError}</div>
		{/if}
		{#if chTestResult}
			<div class="form-ok">{chTestResult}</div>
		{/if}
	</div>

	<div class="modal-footer">
		<button onclick={() => (chModalOpen = false)}>Cancel</button>
		{#if chForm.id != null}
			<button onclick={testChannel} disabled={chFormSaving}>Test</button>
		{/if}
		<button class="btn-primary" onclick={saveChannel} disabled={chFormSaving}>
			{chFormSaving ? 'Saving…' : 'Save'}
		</button>
	</div>
</div>
{/if}

<style>
	.page-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 16px;
	}
	h1 { margin: 0; font-size: 20px; font-weight: 600; }
	h2 { margin: 0 0 12px; font-size: 14px; font-weight: 600; color: var(--text-dim); text-transform: uppercase; letter-spacing: 0.05em; }

	/* tabs */
	.tabs {
		display: flex;
		gap: 2px;
		margin-bottom: 16px;
		border-bottom: 1px solid var(--border);
	}
	.tabs button {
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		color: var(--text-dim);
		cursor: pointer;
		padding: 8px 14px;
		font: inherit;
		font-size: 13px;
		margin-bottom: -1px;
		transition: color 0.15s, border-color 0.15s;
	}
	.tabs button.active { color: var(--text); border-bottom-color: var(--accent, #5cc8ff); }
	.tabs button:hover:not(.active) { color: var(--text); }

	/* rule form */
	.rule-form {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
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

	/* severity pill */
	.sev-pill {
		display: inline-block;
		border-radius: 3px;
		padding: 1px 6px;
		font-size: 10px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	/* channels tab */
	.ch-header {
		display: flex;
		justify-content: flex-end;
		margin-bottom: 12px;
	}
	.ch-list { display: flex; flex-direction: column; gap: 8px; }
	.ch-card {
		background: var(--bg-elev-1);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 12px 16px;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
	}
	.ch-card-left { display: flex; flex-direction: column; gap: 4px; }
	.ch-name { font-size: 14px; font-weight: 500; }
	.ch-meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
	.ch-type {
		font-size: 11px;
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 3px;
		padding: 1px 6px;
		color: var(--text-dim);
	}
	.ch-card-right { display: flex; gap: 6px; flex-shrink: 0; flex-wrap: wrap; }
	.active-toggle { color: var(--good, #7ce38b); border-color: var(--good, #7ce38b); }

	/* misc table */
	.no-pad { padding: 0; }
	.no-pad h2 { margin-top: 12px; }
	.small { font-size: 11px; }
	.center { text-align: center; padding: 24px; }
	.firing { background: rgba(255, 107, 107, 0.06); }
	.err { color: var(--bad); border-color: var(--bad); }
	.form-err { color: var(--bad); margin-top: 8px; font-size: 12px; }
	.form-ok  { color: var(--good, #7ce38b); margin-top: 8px; font-size: 12px; }
	.btn-primary {
		background: var(--accent, #5cc8ff);
		color: #000;
		border: none;
		border-radius: 4px;
		padding: 7px 14px;
		font: inherit;
		font-size: 12px;
		cursor: pointer;
		font-weight: 600;
	}
	.btn-primary:hover { opacity: 0.85; }

	/* modal */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0,0,0,0.55);
		z-index: 100;
		cursor: default;
	}
	.modal {
		position: fixed;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		z-index: 101;
		background: var(--bg-elev-1);
		border: 1px solid var(--border);
		border-radius: 10px;
		width: min(520px, 95vw);
		max-height: 85vh;
		display: flex;
		flex-direction: column;
		box-shadow: 0 16px 48px rgba(0,0,0,0.6);
	}
	.modal-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 14px 18px;
		border-bottom: 1px solid var(--border);
	}
	.modal-header h2 { margin: 0; font-size: 15px; text-transform: none; letter-spacing: 0; color: var(--text); }
	.modal-close { background: none; border: none; color: var(--text-dim); cursor: pointer; font-size: 16px; }
	.modal-body { overflow-y: auto; padding: 16px 18px; display: flex; flex-direction: column; gap: 14px; }
	.modal-footer {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		padding: 12px 18px;
		border-top: 1px solid var(--border);
	}

	/* modal fields */
	.field { display: flex; flex-direction: column; gap: 6px; }
	.field-label { font-size: 12px; color: var(--text-dim); }
	.optional { opacity: 0.6; }
	.field input[type="text"],
	.field input[type="url"],
	.field input[type="number"],
	.field select {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 4px;
		color: var(--text);
		padding: 7px 10px;
		font: inherit;
		font-size: 13px;
	}
	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		color: var(--text);
		cursor: pointer;
	}
	.type-btns { display: flex; gap: 6px; flex-wrap: wrap; }
	.type-btn {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 5px;
		color: var(--text-dim);
		padding: 6px 12px;
		font: inherit;
		font-size: 12px;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.type-btn.selected {
		border-color: var(--accent, #5cc8ff);
		color: var(--accent, #5cc8ff);
		background: rgba(92,200,255,0.08);
	}
	.radio-row { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
	.radio-label { display: flex; align-items: center; gap: 6px; cursor: pointer; }
	.header-row { display: flex; gap: 6px; align-items: center; margin-bottom: 4px; }
	.header-row input { flex: 1; }
	.small-btn {
		font-size: 11px;
		padding: 4px 8px;
	}
</style>
