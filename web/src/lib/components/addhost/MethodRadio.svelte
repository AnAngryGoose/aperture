<script lang="ts">
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		value: 'agent' | 'docker' | 'ssh';
		onchange: (v: 'agent' | 'docker' | 'ssh') => void;
	}

	let { value, onchange }: Props = $props();

	const methods = [
		{
			id: 'agent' as const,
			icon: 'cpu',
			label: 'Install Agent',
			desc: 'Run aperture-agent on the host. Works for any Linux machine. Recommended for full metric visibility.'
		},
		{
			id: 'docker' as const,
			icon: 'box',
			label: 'Docker API',
			desc: 'Connect to a remote Docker socket over TCP/TLS. No agent install needed — container metrics only.'
		},
		{
			id: 'ssh' as const,
			icon: 'terminal',
			label: 'SSH Probe',
			desc: 'Connect via SSH and collect metrics without an installed agent. Good for quick one-off inspection.'
		}
	] as const;
</script>

<div class="method-grid">
	{#each methods as m}
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="method-card"
			class:selected={value === m.id}
			onclick={() => onchange(m.id)}
			role="radio"
			aria-checked={value === m.id}
			tabindex="0"
			onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && onchange(m.id)}
		>
			<div class="method-icon">
				<Icon name={m.icon} size={20} />
			</div>
			<div class="method-label">{m.label}</div>
			<div class="method-desc">{m.desc}</div>
			<div class="method-check" class:visible={value === m.id}></div>
		</div>
	{/each}
</div>

<style>
	.method-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 10px;
	}

	.method-card {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 14px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		cursor: pointer;
		transition: border-color 120ms, background 120ms;
		outline: none;
	}

	.method-card:hover {
		border-color: var(--line-strong);
		background: var(--bg-hover);
	}

	.method-card.selected {
		border-color: var(--accent-line);
		background: var(--accent-soft);
	}

	.method-card:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}

	.method-icon {
		color: var(--text-dim);
		margin-bottom: 2px;
	}

	.method-card.selected .method-icon {
		color: var(--accent);
	}

	.method-label {
		font-size: 13px;
		font-weight: 500;
		color: var(--text);
	}

	.method-desc {
		font-size: 11px;
		color: var(--text-faint);
		line-height: 1.5;
	}

	.method-check {
		position: absolute;
		top: 10px;
		right: 10px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		border: 1.5px solid var(--line-strong);
		background: var(--bg-elev);
		transition: border-color 120ms, background 120ms;
	}

	.method-check.visible {
		border-color: var(--accent);
		background: var(--accent);
		box-shadow: inset 0 0 0 3px var(--bg-elev);
	}
</style>
