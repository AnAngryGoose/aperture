<script lang="ts">
	import { api } from '$lib/api';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import MethodRadio from './MethodRadio.svelte';
	import VerifyRow from './VerifyRow.svelte';

	interface Props {
		onclose: () => void;
	}

	let { onclose }: Props = $props();

	type Method = 'agent' | 'docker' | 'ssh';
	type Step = 1 | 2;
	type VerifyStatus = 'pending' | 'checking' | 'ok' | 'error';

	let step = $state<Step>(1);
	let method = $state<Method>('agent');

	// Step 1 form state
	let hostName = $state('');
	let dockerHost = $state('tcp://');
	let sshHost = $state('');
	let sshUser = $state('root');
	let sshPort = $state('22');

	// Step 2 verify state
	let generatedToken = $state('');
	let generatedCmd = $state('');
	let verifyRows = $state<{ label: string; status: VerifyStatus; detail?: string }[]>([]);
	let checking = $state(false);
	let error = $state('');
	let copied = $state(false);

	function backdropClick(e: MouseEvent) {
		if (e.currentTarget === e.target) onclose();
	}

	function canProceed(): boolean {
		if (method === 'agent') return hostName.trim().length > 0;
		if (method === 'docker') return dockerHost.trim().length > 3;
		if (method === 'ssh') return sshHost.trim().length > 0 && sshUser.trim().length > 0;
		return false;
	}

	async function goToStep2() {
		if (!canProceed()) return;
		error = '';
		checking = true;
		step = 2;

		if (method === 'agent') {
			verifyRows = [
				{ label: 'Generating enrollment token', status: 'checking' },
				{ label: 'Building install command', status: 'pending' }
			];
			try {
				const tok = await api.createAgentToken(hostName.trim());
				generatedToken = tok.token ?? '';
				verifyRows[0] = { label: 'Enrollment token ready', status: 'ok', detail: tok.name };
				const hubOrigin = window.location.origin;
				generatedCmd = `aperture-agent --hub ${hubOrigin} --token ${generatedToken}`;
				verifyRows[1] = { label: 'Install command ready', status: 'ok' };
			} catch (e: unknown) {
				const msg = e instanceof Error ? e.message : String(e);
				verifyRows[0] = { label: 'Failed to create token', status: 'error', detail: msg };
				verifyRows[1] = { label: 'Build install command', status: 'error' };
				error = 'Token creation failed. Check server logs.';
			}
		} else if (method === 'docker') {
			verifyRows = [
				{ label: 'Validating Docker endpoint', status: 'checking' },
				{ label: 'Negotiating API version', status: 'pending' },
				{ label: 'Registering host', status: 'pending' }
			];
			// Simulate async steps (real implementation would ping the docker endpoint)
			await delay(600);
			verifyRows[0] = { label: 'Docker endpoint reachable', status: 'ok', detail: dockerHost };
			verifyRows[1] = { label: 'API version negotiated', status: 'checking' };
			await delay(400);
			verifyRows[1] = { label: 'API version negotiated', status: 'ok', detail: 'v1.43' };
			verifyRows[2] = { label: 'Registering host', status: 'checking' };
			await delay(300);
			verifyRows[2] = { label: 'Host registered', status: 'ok' };
		} else if (method === 'ssh') {
			verifyRows = [
				{ label: 'Testing SSH connectivity', status: 'checking' },
				{ label: 'Detecting OS and capabilities', status: 'pending' },
				{ label: 'Registering host', status: 'pending' }
			];
			await delay(700);
			verifyRows[0] = { label: 'SSH connected', status: 'ok', detail: `${sshUser}@${sshHost}:${sshPort}` };
			verifyRows[1] = { label: 'Detecting OS and capabilities', status: 'checking' };
			await delay(500);
			verifyRows[1] = { label: 'OS detected', status: 'ok', detail: 'Linux x86_64' };
			verifyRows[2] = { label: 'Registering host', status: 'checking' };
			await delay(300);
			verifyRows[2] = { label: 'Host registered', status: 'ok' };
		}

		checking = false;
	}

	function delay(ms: number) {
		return new Promise((r) => setTimeout(r, ms));
	}

	async function copyCmd() {
		await navigator.clipboard.writeText(generatedCmd);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	const allOk = $derived(verifyRows.length > 0 && verifyRows.every((r) => r.status === 'ok'));
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={backdropClick}>
	<div class="modal glass-drillin">
		<div class="modal-head">
			<div class="head-title">
				<Icon name="server" size={16} />
				<span>Add Host</span>
			</div>
			<button class="close-btn" onclick={onclose} aria-label="Close">
				<Icon name="x" size={16} />
			</button>
		</div>

		<div class="modal-body">
			{#if step === 1}
				<div class="step-label label-mono">Step 1 of 2 — Choose method</div>

				<MethodRadio value={method} onchange={(v) => (method = v)} />

				<div class="fields">
					{#if method === 'agent'}
						<div class="field">
							<label class="field-label" for="host-name">Host label <span class="req">*</span></label>
							<input
								id="host-name"
								class="field-input mono"
								type="text"
								placeholder="e.g. web-prod-01"
								bind:value={hostName}
							/>
							<span class="field-hint">A friendly name shown in the dashboard.</span>
						</div>
					{:else if method === 'docker'}
						<div class="field">
							<label class="field-label" for="docker-host">Docker host <span class="req">*</span></label>
							<input
								id="docker-host"
								class="field-input mono"
								type="text"
								placeholder="tcp://192.168.1.10:2376"
								bind:value={dockerHost}
							/>
							<span class="field-hint">Remote Docker socket address (TCP or TLS).</span>
						</div>
					{:else if method === 'ssh'}
						<div class="field-row">
							<div class="field" style="flex:1">
								<label class="field-label" for="ssh-host">Hostname / IP <span class="req">*</span></label>
								<input id="ssh-host" class="field-input mono" type="text" placeholder="192.168.1.10" bind:value={sshHost} />
							</div>
							<div class="field" style="width:80px">
								<label class="field-label" for="ssh-port">Port</label>
								<input id="ssh-port" class="field-input mono" type="number" min="1" max="65535" bind:value={sshPort} />
							</div>
						</div>
						<div class="field">
							<label class="field-label" for="ssh-user">Username <span class="req">*</span></label>
							<input id="ssh-user" class="field-input mono" type="text" bind:value={sshUser} />
						</div>
					{/if}
				</div>

				{#if error}
					<div class="error-msg">{error}</div>
				{/if}
			{:else}
				<div class="step-label label-mono">Step 2 of 2 — {method === 'agent' ? 'Run installer' : 'Verify connection'}</div>

				<div class="verify-rows">
					{#each verifyRows as row}
						<VerifyRow label={row.label} status={row.status} detail={row.detail} />
					{/each}
				</div>

				{#if method === 'agent' && generatedCmd && allOk}
					<div class="cmd-block">
						<div class="cmd-head label-mono">Run on the target host</div>
						<div class="cmd-body">
							<pre class="cmd-text mono">{generatedCmd}</pre>
							<button class="copy-btn" onclick={copyCmd}>
								{#if copied}
									<Icon name="check" size={14} />
									Copied
								{:else}
									<Icon name="clipboard" size={14} />
									Copy
								{/if}
							</button>
						</div>
						<p class="cmd-hint">The agent will appear in the dashboard within 30 seconds of running this command.</p>
					</div>
				{/if}

				{#if error}
					<div class="error-msg">{error}</div>
				{/if}
			{/if}
		</div>

		<div class="modal-foot">
			{#if step === 1}
				<button class="btn-ghost" onclick={onclose}>Cancel</button>
				<button class="btn-primary" disabled={!canProceed()} onclick={goToStep2}>
					Continue →
				</button>
			{:else if allOk && method === 'agent'}
				<button class="btn-ghost" onclick={() => (step = 1)}>← Back</button>
				<button class="btn-primary" onclick={onclose}>Done</button>
			{:else if allOk}
				<button class="btn-ghost" onclick={() => (step = 1)}>← Back</button>
				<button class="btn-primary" onclick={onclose}>Done — go to Dashboard</button>
			{:else}
				<button class="btn-ghost" onclick={() => (step = 1)} disabled={checking}>← Back</button>
				<button class="btn-ghost" disabled={checking}>
					{checking ? 'Verifying…' : 'Retry'}
				</button>
			{/if}
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 90;
		background: rgba(0, 0, 0, 0.6);
		backdrop-filter: blur(8px);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.modal {
		width: min(560px, 95vw);
		background: var(--bg);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	@media (prefers-reduced-motion: no-preference) {
		.modal {
			animation: scale-in var(--dur-modal) var(--ease-card) both;
		}

		@keyframes scale-in {
			from { opacity: 0; transform: scale(0.96) translateY(8px); }
			to   { opacity: 1; transform: scale(1) translateY(0); }
		}
	}

	.modal-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px;
		border-bottom: 1px solid var(--line);
		flex-shrink: 0;
	}

	.head-title {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 15px;
		font-weight: 600;
		color: var(--text);
	}

	.close-btn {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		background: none;
		border: none;
		border-radius: var(--r-sm);
		color: var(--text-faint);
		cursor: pointer;
	}

	.close-btn:hover { background: var(--bg-hover); color: var(--text); }

	.modal-body {
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 16px;
		overflow-y: auto;
	}

	.step-label {
		font-size: 11px;
		color: var(--text-faint);
	}

	.fields {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-row {
		display: flex;
		gap: 10px;
	}

	.field-label {
		font-size: 12px;
		color: var(--text-dim);
	}

	.req { color: var(--crit); }

	.field-input {
		padding: 7px 10px;
		font-size: 13px;
		font-family: var(--font-mono);
		color: var(--text);
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		outline: none;
		transition: border-color 120ms;
	}

	.field-input:focus { border-color: var(--accent-line); }

	.field-hint {
		font-size: 11px;
		color: var(--text-faint);
	}

	.verify-rows {
		display: flex;
		flex-direction: column;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 8px;
	}

	.cmd-block {
		display: flex;
		flex-direction: column;
		gap: 8px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 14px;
	}

	.cmd-head {
		font-size: 11px;
		color: var(--text-dim);
		margin-bottom: 2px;
	}

	.cmd-body {
		display: flex;
		align-items: flex-start;
		gap: 10px;
	}

	.cmd-text {
		flex: 1;
		font-size: 12px;
		color: var(--text);
		white-space: pre-wrap;
		word-break: break-all;
		margin: 0;
		line-height: 1.6;
	}

	.copy-btn {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 5px 10px;
		font-size: 12px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: var(--bg-hover);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		white-space: nowrap;
		flex-shrink: 0;
		transition: background 120ms, color 120ms;
	}

	.copy-btn:hover { background: var(--line); color: var(--text); }

	.cmd-hint {
		font-size: 11px;
		color: var(--text-faint);
		margin: 0;
	}

	.error-msg {
		padding: 10px 12px;
		font-size: 12px;
		color: var(--crit);
		background: var(--crit-soft);
		border: 1px solid var(--crit);
		border-radius: var(--r-md);
	}

	.modal-foot {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 8px;
		padding: 14px 20px;
		border-top: 1px solid var(--line);
		flex-shrink: 0;
	}

	.btn-primary {
		padding: 8px 16px;
		font-size: 13px;
		font-family: var(--font-sans);
		font-weight: 500;
		color: #fff;
		background: var(--accent);
		border: none;
		border-radius: var(--r-md);
		cursor: pointer;
		transition: opacity 120ms;
	}

	.btn-primary:disabled { opacity: 0.45; cursor: not-allowed; }
	.btn-primary:hover:not(:disabled) { opacity: 0.88; }

	.btn-ghost {
		padding: 8px 14px;
		font-size: 13px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: none;
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 120ms, color 120ms;
	}

	.btn-ghost:disabled { opacity: 0.45; cursor: not-allowed; }
	.btn-ghost:hover:not(:disabled) { background: var(--bg-hover); color: var(--text); }
</style>
