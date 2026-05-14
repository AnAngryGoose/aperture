<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
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
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="tile-card" onclick={onclick} role="button" tabindex="0"
     onkeydown={(e) => e.key === 'Enter' && onclick?.()}>
	<div class="rail" style="background: {entry.status === 'crit' ? 'var(--crit)' : entry.status === 'warn' ? 'var(--warn)' : entry.status === 'ok' ? 'var(--ok)' : 'var(--offline)'};"></div>

	<div class="head">
		<HostKindIcon {kind} />
		<div class="head-text">
			<span class="name">{entry.host.name}</span>
			<div class="sub-row">
				<StatusIndicator status={entry.status} size={6} />
				<span class="status-text mono" style="font-size:11px; color:var(--text-faint)">
					{entry.status}
				</span>
			</div>
		</div>
	</div>

	<div class="metrics">
		<div class="metric">
			<span class="metric-label label-mono">CPU</span>
			<span class="metric-val mono">{(s?.cpu_percent ?? 0).toFixed(0)}%</span>
			<Sparkline data={entry.cpuSeries} width={120} height={22} color="var(--accent)" />
		</div>
		<div class="metric">
			<span class="metric-label label-mono">MEM</span>
			<span class="metric-val mono">{(s?.mem_percent ?? 0).toFixed(0)}%</span>
			<Sparkline data={entry.memSeries} width={120} height={22} color="var(--accent)" />
		</div>
		<div class="metric">
			<span class="metric-label label-mono">NET ↓</span>
			<span class="metric-val mono">{fmtRate(s?.net_rx_bytes ?? 0)}</span>
			<Sparkline data={entry.netInSeries} width={120} height={22} color="var(--info)" />
		</div>
		<div class="metric">
			<span class="metric-label label-mono">TEMP</span>
			<span class="metric-val mono">—°</span>
			<div style="height:22px;"></div>
		</div>
	</div>

	<div class="footer mono">
		{s?.uptime_secs ? `up ${fmtDuration(s.uptime_secs)}` : '—'}
		{#if (entry.host.open_alerts ?? 0) > 0}
			<span style="color:var(--warn)"> · {entry.host.open_alerts} alerts</span>
		{/if}
	</div>
</div>

<style>
	.tile-card {
		position: relative;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
		cursor: pointer;
		padding: 14px;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	@media (prefers-reduced-motion: no-preference) {
		.tile-card:hover {
			border-color: var(--line-strong);
			transform: translateY(-1px);
			box-shadow: 0 6px 24px -12px rgba(0,0,0,.35);
		}
	}

	.rail {
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 2px;
	}

	.head {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.head-text { display: flex; flex-direction: column; gap: 2px; }

	.name { font-size: 14px; font-weight: 600; color: var(--text); }

	.sub-row { display: flex; align-items: center; gap: 4px; }

	.metrics {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 10px;
	}

	.metric {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.metric-label { font-size: 10px; }
	.metric-val { font-size: 16px; font-weight: 500; letter-spacing: -0.02em; }

	.footer {
		font-size: 11px;
		color: var(--text-faint);
		border-top: 1px solid var(--line);
		padding-top: 8px;
	}
</style>
