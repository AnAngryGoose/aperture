<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let password = $state('');
	let confirm = $state('');
	let error = $state('');
	let loading = $state(false);

	let mismatch = $derived(confirm.length > 0 && password !== confirm);
	let tooShort = $derived(password.length > 0 && password.length < 8);
	let canSubmit = $derived(password.length >= 8 && password === confirm && !loading);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		loading = true;
		error = '';
		try {
			await api.auth.setup(password);
			await goto('/');
		} catch (err: unknown) {
			error = err instanceof Error ? err.message : 'Setup failed.';
		} finally {
			loading = false;
		}
	}
</script>

<div class="card">
	<div class="brand">
		<span class="dot"></span>
		<span>Aperture</span>
	</div>
	<div class="intro">
		<h1>Set up Aperture</h1>
		<p>Create an admin password to secure your instance. This is only shown once.</p>
	</div>
	<form onsubmit={submit}>
		<label for="pw">Password <span class="hint">(min 8 characters)</span></label>
		<input
			id="pw"
			type="password"
			bind:value={password}
			placeholder="Choose a strong password"
			autocomplete="new-password"
			disabled={loading}
		/>
		{#if tooShort}
			<p class="hint-msg">Password must be at least 8 characters.</p>
		{/if}

		<label for="confirm">Confirm password</label>
		<input
			id="confirm"
			type="password"
			bind:value={confirm}
			placeholder="Confirm password"
			autocomplete="new-password"
			disabled={loading}
			class:bad={mismatch}
		/>
		{#if mismatch}
			<p class="hint-msg bad">Passwords do not match.</p>
		{/if}

		{#if error}
			<p class="error">{error}</p>
		{/if}
		<button type="submit" disabled={!canSubmit}>
			{loading ? 'Setting up…' : 'Create admin account'}
		</button>
	</form>
</div>

<style>
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: 12px;
		padding: 40px;
		width: 100%;
		max-width: 400px;
		display: flex;
		flex-direction: column;
		gap: 24px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 18px;
		font-weight: 600;
		color: var(--text);
	}
	.dot {
		width: 10px; height: 10px;
		border-radius: 50%;
		background: var(--accent);
		box-shadow: 0 0 8px var(--accent);
	}
	.intro h1 { font-size: 20px; font-weight: 600; color: var(--text); margin: 0 0 8px; }
	.intro p  { font-size: 13px; color: var(--text-dim); margin: 0; }
	form {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	label {
		font-size: 12px;
		color: var(--text-dim);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin-top: 4px;
	}
	.hint { font-weight: 400; text-transform: none; letter-spacing: 0; font-size: 11px; }
	input {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 6px;
		color: var(--text);
		font-size: 14px;
		padding: 10px 12px;
		width: 100%;
		box-sizing: border-box;
		outline: none;
		transition: border-color 0.15s;
	}
	input:focus { border-color: var(--accent); }
	input.bad   { border-color: var(--bad); }
	.hint-msg { font-size: 12px; color: var(--text-dim); margin: 0; }
	.hint-msg.bad { color: var(--bad); }
	.error { color: var(--bad); font-size: 13px; margin: 0; }
	button {
		background: var(--accent);
		color: #000;
		border: none;
		border-radius: 6px;
		font-size: 14px;
		font-weight: 600;
		padding: 10px;
		cursor: pointer;
		transition: opacity 0.15s;
		margin-top: 8px;
	}
	button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
