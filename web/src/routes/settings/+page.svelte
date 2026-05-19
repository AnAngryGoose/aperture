<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import type { AgentToken } from '$lib/types';
	import { toast } from '$lib/toast';
	import { absTime, relTime } from '$lib/format';
	import { theme, type ThemeMode } from '$lib/stores/theme.svelte';
	import { accent, ACCENTS, type AccentKey } from '$lib/stores/accent.svelte';

	const THEME_OPTIONS: ReadonlyArray<{ key: ThemeMode; label: string }> = [
		{ key: 'dark',          label: 'Dark' },
		{ key: 'light',         label: 'Light' },
		{ key: 'gruvbox-dark',  label: 'Gruvbox dark' },
		{ key: 'gruvbox-light', label: 'Gruvbox light' },
		{ key: 'system',        label: 'System' }
	];
	import type { HostConfig } from '$lib/types';
	import Button from '$lib/components/primitives/Button.svelte';
	import ConfirmDialog from '$lib/components/primitives/ConfirmDialog.svelte';

	// ── password change ───────────────────────────────────────────────────────
	let pwCurrent  = $state('');
	let pwNew      = $state('');
	let pwConfirm  = $state('');
	let pwError    = $state('');
	let pwSaving   = $state(false);
	let pwMismatch = $derived(pwConfirm.length > 0 && pwNew !== pwConfirm);
	let pwCanSave  = $derived(pwCurrent.length > 0 && pwNew.length >= 8 && pwNew === pwConfirm && !pwSaving);

	async function changePassword(e: SubmitEvent) {
		e.preventDefault();
		if (!pwCanSave) return;
		pwSaving = true; pwError = '';
		try {
			await api.auth.changePassword(pwCurrent, pwNew);
			toast.success('Password updated');
			pwCurrent = pwNew = pwConfirm = '';
		} catch (err) {
			pwError = (err instanceof Error) ? err.message : 'Failed to update password';
		} finally {
			pwSaving = false;
		}
	}


	let tokens = $state<AgentToken[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// ── add-agent wizard ─────────────────────────────────────────────────────
	let wizardOpen  = $state(false);
	let wizardStep  = $state<'name' | 'command'>('name');
	let agentName   = $state('');
	let nameErr     = $state('');
	let generating  = $state(false);
	let newToken    = $state<AgentToken | null>(null);
	let copied      = $state(false);
	let showDocker  = $state(false);

	// The hub URL is whatever origin serves this SPA.
	let hubOrigin = $state('');
	onMount(() => { hubOrigin = window.location.origin; });

	let binaryCmd = $derived(
		newToken ? `aperture-agent \\\n  --hub ${hubOrigin} \\\n  --token ${newToken.token}` : ''
	);
	let dockerCmd = $derived(
		newToken
			? `docker run -d \\\n  --name aperture-agent \\\n  --restart unless-stopped \\\n  -v /var/run/docker.sock:/var/run/docker.sock \\\n  ghcr.io/aperture/agent:latest \\\n  --hub ${hubOrigin} \\\n  --token ${newToken.token}`
			: ''
	);

	async function loadTokens() {
		try {
			tokens = await api.agentTokens();
			error = null;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	function openWizard() {
		agentName = '';
		nameErr = '';
		newToken = null;
		copied = false;
		showDocker = false;
		wizardStep = 'name';
		wizardOpen = true;
	}

	async function generateToken() {
		nameErr = '';
		const name = agentName.trim();
		if (!name) { nameErr = 'Name is required'; return; }
		generating = true;
		try {
			newToken = await api.createAgentToken(name);
			await loadTokens();
			wizardStep = 'command';
		} catch (e) {
			nameErr = (e as Error).message;
		} finally {
			generating = false;
		}
	}

	async function copyCmd() {
		const cmd = showDocker ? dockerCmd : binaryCmd;
		await navigator.clipboard.writeText(cmd.replace(/\\\n  /g, ' '));
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	let revokeTarget = $state<AgentToken | null>(null);
	let revokeBusy = $state(false);

	async function doRevoke() {
		if (!revokeTarget) return;
		revokeBusy = true;
		try {
			await api.revokeAgentToken(revokeTarget.id);
			tokens = tokens.filter(tk => tk.id !== revokeTarget!.id);
			toast.success(`Token "${revokeTarget.name}" revoked`);
			revokeTarget = null;
		} catch (e) {
			toast.error((e as Error).message);
		} finally {
			revokeBusy = false;
		}
	}

	let signOutOpen = $state(false);
	let signOutBusy = $state(false);
	async function doSignOut() {
		signOutBusy = true;
		await api.auth.logout().catch(() => {});
		signOutOpen = false;
		signOutBusy = false;
		await goto('/login');
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && wizardOpen) wizardOpen = false;
	}

	onMount(loadTokens);

	// ── global monitoring defaults ───────────────────────────────────────────
	// Applied to hosts that don't have their own host_config row. The hub
	// reads from user_settings['monitoring.defaults'] when answering
	// GET /api/hosts/{id}/config for hosts without a row, so changes here
	// take effect for any non-customized host immediately.
	let defaults = $state<HostConfig | null>(null);
	let defaultsLoading = $state(false);
	let defaultsSaving  = $state(false);
	let defaultsError   = $state<string | null>(null);
	let defaultsSaved   = $state(false);

	async function loadDefaults() {
		defaultsLoading = true;
		try {
			defaults = await api.monitoringDefaults.get();
			defaultsError = null;
		} catch (e) {
			defaultsError = (e as Error).message;
		} finally {
			defaultsLoading = false;
		}
	}

	async function saveDefaults() {
		if (!defaults) return;
		defaultsSaving = true;
		defaultsError = null;
		defaultsSaved = false;
		try {
			await api.monitoringDefaults.put(defaults);
			defaultsSaved = true;
			setTimeout(() => (defaultsSaved = false), 2500);
		} catch (e) {
			defaultsError = (e as Error).message;
		} finally {
			defaultsSaving = false;
		}
	}

	onMount(loadDefaults);
</script>

<svelte:head><title>Aperture — Settings</title></svelte:head>
<svelte:window onkeydown={handleKeydown} />

<div class="page-header">
	<h1>Settings</h1>
</div>

<!-- ── Appearance ────────────────────────────────────────────────────────── -->
<div class="section-header">
	<div>
		<div class="section-title">Appearance</div>
		<div class="section-sub muted">Theme and accent color. Changes apply immediately.</div>
	</div>
</div>

<div class="appear-card">
	<div class="appear-row">
		<span class="appear-label">Theme</span>
		<div class="theme-btns">
			{#each THEME_OPTIONS as t}
				<button
					class="theme-btn"
					class:active={theme.mode === t.key}
					onclick={() => theme.set(t.key)}
				>{t.label}</button>
			{/each}
		</div>
	</div>

	<div class="appear-row">
		<span class="appear-label">Accent</span>
		<div class="accent-swatches">
			{#each Object.entries(ACCENTS) as [key, val]}
				<button
					class="swatch"
					class:active={accent.key === key}
					style="--sw: {val.hex}"
					title={val.label}
					onclick={() => accent.set(key as AccentKey)}
					aria-label={val.label}
				></button>
			{/each}
		</div>
	</div>
</div>

<!-- ── Monitoring defaults ────────────────────────────────────────────────── -->
<div class="section-header">
	<div>
		<div class="section-title">Monitoring defaults</div>
		<div class="section-sub muted">Applied to every host that doesn't have its own monitoring config. Per-host settings always win.</div>
	</div>
</div>

{#if defaultsLoading && !defaults}
	<div class="card muted" style="text-align:center;padding:24px">Loading…</div>
{:else if defaults}
	<div class="card defaults-card">
		<div class="defaults-grid">
			<label class="d-field">
				<span class="d-label">Sample interval (seconds)</span>
				<input type="number" min="1" max="3600" bind:value={defaults.sample_interval_s} />
			</label>
			<label class="d-field">
				<span class="d-label">Retention (days)</span>
				<input type="number" min="1" max="3650" bind:value={defaults.retention_days} />
			</label>
			<label class="d-field">
				<span class="d-label">Memory calculation</span>
				<select bind:value={defaults.mem_calc}>
					<option value="used">used (gopsutil default)</option>
					<option value="avail">avail (matches htop)</option>
				</select>
			</label>
		</div>

		<div class="thresh-block">
			<div class="thresh-head muted">Status thresholds</div>
			<div class="thresh-rows">
				<div class="thresh-row">
					<span class="thresh-name">CPU %</span>
					<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={defaults.warn_cpu} /></label>
					<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={defaults.crit_cpu} /></label>
				</div>
				<div class="thresh-row">
					<span class="thresh-name">Memory %</span>
					<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={defaults.warn_mem} /></label>
					<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={defaults.crit_mem} /></label>
				</div>
				<div class="thresh-row">
					<span class="thresh-name">Disk %</span>
					<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={defaults.warn_disk} /></label>
					<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={defaults.crit_disk} /></label>
				</div>
				<div class="thresh-row">
					<span class="thresh-name">Temperature °C</span>
					<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={defaults.warn_temp} /></label>
					<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={defaults.crit_temp} /></label>
				</div>
			</div>
		</div>

		<div class="defaults-bar">
			{#if defaultsSaved}<span class="ok-msg">saved ✓</span>{/if}
			{#if defaultsError}<span class="err-msg">{defaultsError}</span>{/if}
			<Button variant="primary" onclick={saveDefaults} loading={defaultsSaving}>
				{defaultsSaving ? 'Saving…' : 'Save defaults'}
			</Button>
		</div>
	</div>
{/if}

<!-- ── Agent tokens ───────────────────────────────────────────────────────── -->
<div class="section-header">
	<div>
		<div class="section-title">Agent tokens</div>
		<div class="section-sub muted">Tokens authorise remote <code>aperture-agent</code> instances to connect to this hub.</div>
	</div>
	<Button variant="primary" onclick={openWizard}>+ Add Agent</Button>
</div>

{#if error}
	<div class="card err">Error: {error}</div>
{/if}

{#if loading}
	<div class="card muted" style="text-align:center;padding:24px">Loading…</div>
{:else if tokens.length === 0}
	<div class="card empty">
		<div class="empty-icon">⬡</div>
		<div class="empty-title">No agent tokens yet</div>
		<div class="empty-sub muted">Click <strong>+ Add Agent</strong> to generate a token and get the connection command for a remote host.</div>
		<div style="margin-top:12px"><Button variant="primary" onclick={openWizard}>+ Add Agent</Button></div>
	</div>
{:else}
	<div class="card no-pad">
		<table>
			<thead>
				<tr>
					<th>Name</th>
					<th>Created</th>
					<th>Last used</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each tokens as t (t.id)}
					<tr>
						<td><span class="token-name">{t.name}</span></td>
						<td class="muted small" title={absTime(t.created_at)}>{relTime(t.created_at)}</td>
						<td class="muted small">
							{#if t.last_used}
								<span title={absTime(t.last_used)}>{relTime(t.last_used)}</span>
							{:else}
								<span class="muted">never</span>
							{/if}
						</td>
						<td class="actions">
							<Button variant="danger" size="sm" onclick={() => (revokeTarget = t)}>Revoke</Button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- ── Add Agent Wizard ───────────────────────────────────────────────────── -->
{#if wizardOpen}
	<div class="modal-bg" onclick={() => (wizardOpen = false)} role="presentation">
		<div class="modal" onclick={e => e.stopPropagation()} role="dialog" aria-modal="true">

			{#if wizardStep === 'name'}
				<!-- Step 1: name the agent -->
				<div class="modal-head">
					<h2>Add agent</h2>
					<button class="modal-close" onclick={() => (wizardOpen = false)}>✕</button>
				</div>
				<div class="modal-body">
					<p class="step-hint muted">
						An agent is a lightweight process that runs on a remote host and streams metrics
						back to this hub. Give this token a name that identifies the machine it'll run on.
					</p>
					<label class="field">
						<span class="field-label">Agent name</span>
						<input
							type="text"
							placeholder="e.g. nas-box, media-server, pi4"
							bind:value={agentName}
							onkeydown={e => e.key === 'Enter' && generateToken()}
							autofocus
						/>
						{#if nameErr}<div class="field-err">{nameErr}</div>{/if}
					</label>
				</div>
				<div class="modal-foot">
					<button onclick={() => (wizardOpen = false)}>Cancel</button>
					<button disabled={generating} onclick={generateToken}>
						{generating ? 'Generating…' : 'Generate token →'}
					</button>
				</div>

			{:else}
				<!-- Step 2: copy the command -->
				<div class="modal-head">
					<h2>Run on <em>{newToken?.name}</em></h2>
					<button class="modal-close" onclick={() => (wizardOpen = false)}>✕</button>
				</div>
				<div class="modal-body">
					<p class="step-hint muted">
						Copy one of the commands below and run it on your remote host.
						The agent will appear in the dashboard automatically once it connects.
					</p>

					<div class="cmd-tabs">
						<button class:active={!showDocker} onclick={() => (showDocker = false)}>Binary</button>
						<button class:active={showDocker} onclick={() => (showDocker = true)}>Docker</button>
					</div>

					<div class="cmd-block">
						<pre class="cmd-pre">{showDocker ? dockerCmd : binaryCmd}</pre>
						<button class="copy-btn" class:copied onclick={copyCmd}>
							{copied ? '✓ Copied' : 'Copy'}
						</button>
					</div>

					{#if !showDocker}
						<div class="hint muted small">
							Download the <code>aperture-agent</code> binary for your platform from the
							releases page, or build it: <code>go build ./cmd/agent</code>
						</div>
					{:else}
						<div class="hint muted small">
							The container needs access to the Docker socket to manage containers on this host.
						</div>
					{/if}

					<div class="token-warning">
						<span class="warn-icon">⚠</span>
						This token is shown <strong>once</strong>. Copy it now — it cannot be retrieved after
						you close this dialog.
					</div>
				</div>
				<div class="modal-foot">
					<button onclick={() => (wizardOpen = false)}>Done</button>
				</div>
			{/if}

		</div>
	</div>
{/if}

<!-- ── Security ──────────────────────────────────────────────────────────── -->
<div class="section-header" style="margin-top:32px">
	<div>
		<div class="section-title">Security</div>
		<div class="section-sub muted">Manage admin password and session.</div>
	</div>
</div>

<div class="security-card">
	<form onsubmit={changePassword}>
		<div class="pw-row">
			<label for="pw-current">Current password</label>
			<input id="pw-current" type="password" bind:value={pwCurrent} disabled={pwSaving} autocomplete="current-password" />
		</div>
		<div class="pw-row" style="margin-top:12px">
			<label for="pw-new">New password <span style="font-weight:400;text-transform:none">(min 8 chars)</span></label>
			<input id="pw-new" type="password" bind:value={pwNew} disabled={pwSaving} autocomplete="new-password" />
		</div>
		<div class="pw-row" style="margin-top:12px">
			<label for="pw-confirm">Confirm new password</label>
			<input id="pw-confirm" type="password" bind:value={pwConfirm} disabled={pwSaving}
				class:bad={pwMismatch} autocomplete="new-password" />
			{#if pwMismatch}<p class="pw-hint">Passwords do not match.</p>{/if}
		</div>
		{#if pwError}<p class="pw-hint" style="margin-top:6px">{pwError}</p>{/if}
		<div class="pw-actions" style="margin-top:16px">
			<Button variant="primary" type="submit" disabled={!pwCanSave} loading={pwSaving}>
				{pwSaving ? 'Saving…' : 'Change password'}
			</Button>
			<Button variant="ghost" type="button" onclick={() => (signOutOpen = true)}>Sign out</Button>
		</div>
	</form>
</div>

<ConfirmDialog
	open={revokeTarget !== null}
	tone="danger"
	title="Revoke agent token"
	message="Revoke this token and disconnect any agent using it?"
	detail={revokeTarget?.name ?? ''}
	consequences={[
		'Any agent currently connected with this token will be kicked off immediately.',
		'You will need to issue a new token to reconnect the agent.'
	]}
	confirmLabel="Revoke token"
	busy={revokeBusy}
	onconfirm={doRevoke}
	oncancel={() => (revokeTarget = null)}
/>

<ConfirmDialog
	open={signOutOpen}
	tone="warning"
	title="Sign out"
	message="Sign out of Aperture on this browser?"
	consequences={['You will need to sign back in to access the hub.']}
	confirmLabel="Sign out"
	busy={signOutBusy}
	onconfirm={doSignOut}
	oncancel={() => (signOutOpen = false)}
/>

<style>
	.page-header { margin-bottom: 20px; }
	h1 { margin: 0; font-size: 20px; font-weight: 600; }

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: 12px;
		gap: 12px;
	}
	.section-title { font-size: 14px; font-weight: 600; margin-bottom: 3px; }
	.section-sub { font-size: 12px; }
	.section-sub code { font-family: var(--mono); font-size: 11px; }

	/* Token table */
	.no-pad { padding: 0; }
	.token-name { font-weight: 500; }
	.small { font-size: 11px; }
	.actions { display: flex; gap: 4px; }
	.err { color: var(--bad); border-color: var(--bad); }

	/* Empty state */
	.empty {
		text-align: center;
		padding: 40px 24px;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 6px;
	}
	.empty-icon { font-size: 32px; opacity: 0.3; margin-bottom: 4px; }
	.empty-title { font-weight: 600; font-size: 15px; }
	.empty-sub { font-size: 13px; max-width: 380px; }

	/* Modal */
	.modal-bg {
		position: fixed; inset: 0;
		background: rgba(0,0,0,0.55);
		display: flex; align-items: center; justify-content: center;
		z-index: 200;
	}
	.modal {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		width: min(92vw, 560px);
		display: flex; flex-direction: column;
		box-shadow: 0 8px 40px rgba(0,0,0,0.5);
	}
	.modal-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px 14px;
		border-bottom: 1px solid var(--border);
	}
	.modal-head h2 { margin: 0; font-size: 15px; font-weight: 600; }
	.modal-head h2 em { font-style: normal; color: var(--accent); }
	.modal-close { background: none; border: none; color: var(--text-dim); cursor: pointer; font-size: 16px; }
	.modal-body { padding: 16px 20px; display: flex; flex-direction: column; gap: 14px; }
	.modal-foot {
		padding: 12px 20px;
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: flex-end;
		gap: 8px;
	}

	.step-hint { font-size: 13px; line-height: 1.5; margin: 0; }

	.field { display: flex; flex-direction: column; gap: 6px; }
	.field-label { font-size: 12px; color: var(--text-dim); }
	.field input {
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 5px;
		color: var(--text);
		padding: 8px 10px;
		font: inherit;
		font-size: 13px;
		outline: none;
	}
	.field input:focus { border-color: var(--accent); }
	.field-err { font-size: 11px; color: var(--bad); }

	/* Command display */
	.cmd-tabs { display: flex; gap: 4px; margin-bottom: 2px; }
	.cmd-tabs button { font-size: 12px; padding: 4px 12px; }
	.cmd-tabs button.active { border-color: var(--accent); color: var(--accent); background: var(--bg-elev-2); }

	.cmd-block {
		position: relative;
		background: var(--bg-elev-2);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 12px 14px;
		padding-right: 70px;
	}
	.cmd-pre {
		margin: 0;
		font-family: var(--mono);
		font-size: 11.5px;
		white-space: pre-wrap;
		word-break: break-all;
		color: var(--text);
		line-height: 1.6;
	}
	.copy-btn {
		position: absolute;
		top: 10px;
		right: 10px;
		font-size: 11px;
		padding: 3px 10px;
		white-space: nowrap;
		transition: background 0.15s, color 0.15s;
	}
	.copy-btn.copied { border-color: var(--ok); color: var(--ok); }

	.hint { line-height: 1.5; }
	.hint code { font-family: var(--mono); font-size: 10.5px; }

	.token-warning {
		background: rgba(255,203,107,0.08);
		border: 1px solid rgba(255,203,107,0.3);
		border-radius: 6px;
		padding: 10px 14px;
		font-size: 12px;
		color: var(--warn);
		display: flex;
		gap: 8px;
		align-items: flex-start;
		line-height: 1.5;
	}
	.warn-icon { flex-shrink: 0; }

	/* security section */
	.security-card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 20px 24px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		max-width: 480px;
	}
	.pw-row {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.pw-row label { font-size: 12px; color: var(--text-dim); text-transform: uppercase; letter-spacing: 0.05em; }
	.pw-row input {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		color: var(--text);
		font-size: 14px;
		padding: 8px 12px;
		width: 100%;
		box-sizing: border-box;
		outline: none;
		transition: border-color 0.15s;
	}
	.pw-row input:focus { border-color: var(--accent); }
	.pw-row input.bad   { border-color: var(--bad); }
	.pw-hint { font-size: 12px; color: var(--bad); }
	.pw-actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }

	/* Appearance */
	.appear-card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 16px 20px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		max-width: 480px;
		margin-bottom: 28px;
	}

	.appear-row {
		display: flex;
		align-items: center;
		gap: 16px;
	}

	.appear-label {
		font-size: 12px;
		color: var(--text-dim);
		width: 56px;
		flex-shrink: 0;
	}

	.theme-btns {
		display: flex;
		gap: 4px;
		flex-wrap: wrap;
	}

	.theme-btn {
		padding: 5px 14px;
		font-size: 12px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-pill);
		cursor: pointer;
		transition: background 120ms, color 120ms, border-color 120ms;
	}

	.theme-btn:hover { background: var(--bg-hover); color: var(--text); }
	.theme-btn.active { background: var(--accent-soft); border-color: var(--accent-line); color: var(--accent); }

	.accent-swatches {
		display: flex;
		gap: 8px;
		align-items: center;
	}

	.swatch {
		width: 22px;
		height: 22px;
		border-radius: 50%;
		background: var(--sw);
		border: 2px solid transparent;
		cursor: pointer;
		transition: transform 120ms, border-color 120ms;
		padding: 0;
	}

	.swatch:hover { transform: scale(1.15); }
	.swatch.active { border-color: var(--text); }

	/* Monitoring defaults */
	.defaults-card { display: flex; flex-direction: column; gap: 16px; padding: 16px; }

	.defaults-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 12px;
	}

	.d-field { display: flex; flex-direction: column; gap: 4px; }
	.d-label {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		font-family: var(--font-mono);
	}
	.d-field input, .d-field select {
		padding: 7px 10px;
		font-size: 12px;
		font-family: var(--font-mono);
		color: var(--text);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}
	.d-field input:focus, .d-field select:focus {
		outline: none;
		border-color: var(--accent-line);
	}

	.thresh-block {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 12px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}
	.thresh-head {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-family: var(--font-mono);
	}
	.thresh-rows { display: flex; flex-direction: column; gap: 6px; }
	.thresh-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 14px;
		align-items: center;
	}
	.thresh-name { font-size: 13px; color: var(--text); }
	.thresh-input { display: inline-flex; align-items: center; gap: 6px; }
	.thresh-cap {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-family: var(--font-mono);
		color: var(--text-faint);
	}
	.thresh-input input {
		width: 70px;
		padding: 5px 8px;
		font-size: 12px;
		font-family: var(--font-mono);
		color: var(--text);
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		text-align: right;
	}

	.defaults-bar {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 12px;
	}
	.ok-msg { color: var(--ok); font-size: 11px; font-family: var(--font-mono); }
	.err-msg { color: var(--crit); font-size: 11px; }
</style>
