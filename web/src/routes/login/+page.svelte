<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!password) return;
		loading = true;
		error = '';
		try {
			await api.auth.login(password);
			await goto('/');
		} catch {
			error = 'Invalid password.';
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
	<h1>Sign in</h1>
	<form onsubmit={submit}>
		<label for="pw">Password</label>
		<input
			id="pw"
			type="password"
			bind:value={password}
			placeholder="Admin password"
			autocomplete="current-password"
			disabled={loading}
		/>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<button type="submit" disabled={loading || !password}>
			{loading ? 'Signing in…' : 'Sign in'}
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
		max-width: 360px;
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
	h1 {
		font-size: 20px;
		font-weight: 600;
		color: var(--text);
		margin: 0;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}
	label {
		font-size: 12px;
		color: var(--text-dim);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}
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
	.error {
		color: var(--bad);
		font-size: 13px;
		margin: 0;
	}
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
		margin-top: 4px;
	}
	button:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
