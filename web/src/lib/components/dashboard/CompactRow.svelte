<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts';
	import Sparkline from '$lib/components/primitives/Sparkline.svelte';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import HostKindIcon from '$lib/components/primitives/HostKindIcon.svelte';
	import { fmtRate, fmtDuration } from '$lib/format';

	interface Props {
		entry: HostEntry;
		onclick?: () => void;
	}

	let { entry, onclick }: Props = $props();

	const s = $derived(entry.latest);
	const kind = $derived((entry.host.kind as 'docker' | 'linux' | 'edge') || 'linux');
	const statusColor = $derived(
		entry.status === 'crit' ? 'var(--crit)' :
		entry.status === 'warn' ? 'var(--warn)' :
		entry.status === 'ok' ? 'var(--ok)' : 'var(--offline)'
	);
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="compact-row" onclick={onclick} role="button" tabindex="0"
     onkeydown={(e) => e.key === 'Enter' && onclick?.()}>
	<div class="rail" style="background: {statusColor};"></div>

	<!-- Identity -->
	<div class="identity">
		<HostKindIcon {kind} size={12} />
		<span class="name">{entry.host.name}</span>
		<StatusIndicator status={entry.status} size={6} />
		<span class="status-txt mono" style="color:var(--text-faint); font-size:11px;">{entry.status}</span>
	</div>

	<!-- Address -->
	<span class="addr mono">{entry.host.platform || entry.host.os || '—'}</span>

	<!-- CPU -->
	<div class="metric-cell">
		<Sparkline data={entry.cpuSeries} width={80} height={20} color="var(--accent)" />
		<span class="pct mono">{(s?.cpu_pct ?? 0).toFixed(0)}%</span>
	</div>

	<!-- MEM -->
	<div class="metric-cell">
		<Sparkline data={entry.memSeries} width={80} height={20} color="var(--accent)" />
		<span class="pct mono">{(s?.mem_percent ?? 0).toFixed(0)}%</span>
	</div>

	<!-- NET -->
	<div class="metric-cell">
		<Sparkline data={entry.netInSeries} width={80} height={20} color="var(--info)" />
		<span class="pct mono">{fmtRate(s?.net_rx ?? 0)}</span>
	</div>

	<!-- Uptime -->
	<span class="uptime mono">{s?.uptime_secs ? fmtDuration(s.uptime_secs) : '—'}</span>

	<!-- More -->
	<button class="more" onclick={(e) => e.stopPropagation()} aria-label="More">⋯</button>
</div>

<style>
	.compact-row {
		position: relative;
		display: grid;
		grid-template-columns: 220px 1fr 160px 160px 160px 100px 32px;
		align-items: center;
		gap: 16px;
		padding: 10px 16px 10px 18px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		overflow: hidden;
		transition: background 120ms, border-color 120ms;
	}

	.compact-row:hover {
		background: var(--bg-hover);
		border-color: var(--line-strong);
	}

	.rail {
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 2px;
	}

	.identity {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}

	.name {
		font-size: 13.5px;
		font-weight: 500;
		color: var(--text);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.addr {
		font-size: 12px;
		color: var(--text-faint);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.metric-cell {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.pct {
		font-size: 12px;
		color: var(--text-dim);
		white-space: nowrap;
	}

	.uptime {
		font-size: 12px;
		color: var(--text-faint);
	}

	.more {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		font-size: 16px;
		color: var(--text-faint);
		background: none;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
		line-height: 1;
	}

	.more:hover { background: var(--bg-hover); color: var(--text); }
</style>
