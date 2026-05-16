<script lang="ts">
	import type { Host, HostStatus } from '$lib/types';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import HostKindIcon from '$lib/components/primitives/HostKindIcon.svelte';
	import Tag from '$lib/components/primitives/Tag.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import { fmtDuration, fmtBytes, fmtRelative } from '$lib/format';

	interface Props {
		host: Host;
		status: HostStatus;
		uptimeSecs?: number;
		onaction?: (action: 'restart' | 'ssh' | 'update' | 'stop') => void;
	}

	let { host, status, uptimeSecs, onaction }: Props = $props();

	const kind = $derived((host.kind as 'docker' | 'linux' | 'edge') || 'linux');
	const platform = $derived(host.platform || host.os || '—');
	const uptimeLabel = $derived(uptimeSecs ? fmtDuration(uptimeSecs) : '—');
</script>

<header class="host-head">
	<a href="/" class="back" aria-label="Back to dashboard">
		<Icon name="arrow-left" size={14} />
		<span>all hosts</span>
	</a>

	<div class="identity">
		<HostKindIcon {kind} size={32} />
		<div class="info">
			<div class="name-row">
				<h1 class="name">{host.name}</h1>
				<StatusIndicator {status} />
				{#each (host.tags ?? []) as tag}
					<Tag label={tag} />
				{/each}
			</div>
			<div class="meta mono">
				<span>{platform}</span>
				<span class="sep">·</span>
				<span>{host.arch || '—'}</span>
				<span class="sep">·</span>
				<span>{host.cpu_count} vCPU</span>
				<span class="sep">·</span>
				<span>{fmtBytes(host.mem_total)} RAM</span>
				<span class="sep">·</span>
				<span>up {uptimeLabel}</span>
				<span class="sep">·</span>
				<span title={host.last_seen}>last sync {fmtRelative(host.last_seen)}</span>
			</div>
		</div>
	</div>

	<div class="actions">
		<button class="action" onclick={() => onaction?.('restart')}>Restart</button>
		<button class="action" onclick={() => onaction?.('ssh')}>SSH</button>
		<button class="action" onclick={() => onaction?.('update')}>Update</button>
		<button class="action danger" onclick={() => onaction?.('stop')}>Stop</button>
	</div>
</header>

<style>
	.host-head {
		display: grid;
		grid-template-columns: auto 1fr auto;
		grid-template-rows: auto 1fr;
		align-items: center;
		gap: 8px 14px;
		padding: 4px 0 16px;
		border-bottom: 1px solid var(--line);
		margin-bottom: 18px;
	}

	.back {
		grid-row: 1;
		grid-column: 1 / -1;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 11px;
		font-family: var(--font-mono);
		color: var(--text-faint);
		text-decoration: none;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		width: fit-content;
	}
	.back:hover { color: var(--text-dim); }

	.identity {
		grid-row: 2;
		grid-column: 1 / 3;
		display: flex;
		align-items: center;
		gap: 14px;
		min-width: 0;
	}

	.info {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.name-row {
		display: flex;
		align-items: center;
		gap: 10px;
		flex-wrap: wrap;
	}

	.name {
		font-size: 22px;
		font-weight: 600;
		letter-spacing: -0.02em;
		color: var(--text);
		margin: 0;
	}

	.meta {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		font-size: 11px;
		color: var(--text-faint);
	}

	.sep {
		opacity: 0.5;
	}

	.actions {
		grid-row: 2;
		grid-column: 3;
		display: flex;
		gap: 6px;
		flex-shrink: 0;
	}

	.action {
		padding: 6px 12px;
		font-size: 12px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 120ms, color 120ms, border-color 120ms;
	}

	.action:hover {
		background: var(--bg-hover);
		color: var(--text);
		border-color: var(--line-strong);
	}

	.action.danger {
		color: var(--warn);
		background: var(--warn-soft);
		border-color: var(--warn);
	}

	.action.danger:hover {
		background: var(--warn);
		color: #fff;
	}
</style>
