<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import Sparkline from '$lib/components/primitives/Sparkline.svelte';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import HostKindIcon from '$lib/components/primitives/HostKindIcon.svelte';
	import Tag from '$lib/components/primitives/Tag.svelte';
	import CardMenu from './CardMenu.svelte';
	import CardConfigModal from './CardConfigModal.svelte';
	import { fmtBytes, fmtRate, fmtDuration } from '$lib/format';
	import { getMetric, DEFAULT_WIDGETS } from '$lib/monitoring/metricCatalog';
	import { dashboardLayout } from '$lib/stores/dashboardLayout.svelte';

	function fmtCount(n: number | undefined): string {
		return typeof n === 'number' ? String(n) : '—';
	}

	interface Props {
		entry: HostEntry;
		onclick?: () => void;
	}

	let { entry, onclick }: Props = $props();

	let menuOpen = $state(false);
	let configOpen = $state(false);

	const s = $derived(entry.latest);
	const kind = $derived((entry.host.kind as 'docker' | 'linux' | 'edge') || 'linux');

	const cpuColor = $derived(
		(s?.cpu_percent ?? 0) >= 85 ? 'var(--crit)' :
		(s?.cpu_percent ?? 0) >= 70 ? 'var(--warn)' :
		'var(--accent)'
	);

	const memColor = $derived(
		(s?.mem_percent ?? 0) >= 90 ? 'var(--crit)' :
		(s?.mem_percent ?? 0) >= 80 ? 'var(--warn)' :
		'var(--accent)'
	);

	const statusColor = $derived(
		entry.status === 'crit' ? 'var(--crit)' :
		entry.status === 'warn' ? 'var(--warn)' :
		entry.status === 'ok' ? 'var(--ok)' :
		'var(--offline)'
	);

	function toggleMenu(e: MouseEvent) {
		e.stopPropagation();
		menuOpen = !menuOpen;
	}

	function closeMenu() { menuOpen = false; }

	// Resolve the per-host widget selection, falling back to the catalog
	// defaults when the user hasn't configured this host.
	const widgetKeys = $derived(
		dashboardLayout.getCardWidgets(entry.host.id) ?? [...DEFAULT_WIDGETS]
	);

	// Build the runtime widget row data: spec + series + value + color.
	// Each row knows how to render itself based on the metric's category.
	// Series-bearing metrics (cpu, mem, net) render a sparkline; others
	// render just label + value.
	interface WidgetRow {
		key: string;
		label: string;
		value: string;
		color: string;
		series?: number[];
		ts?: number[];
	}

	const widgetRows = $derived.by<WidgetRow[]>(() => {
		const rows: WidgetRow[] = [];
		for (const key of widgetKeys) {
			const spec = getMetric(key);
			if (!spec || !s) continue;
			let series: number[] | undefined;
			let value: number;
			let color = spec.color;

			switch (key) {
				case 'cpu_pct':
					series = entry.cpuSeries;
					value = s.cpu_percent;
					color = cpuColor;
					break;
				case 'mem_pct':
					series = entry.memSeries;
					value = s.mem_percent;
					color = memColor;
					break;
				case 'net_rx_rate':
					series = entry.netInSeries;
					value = entry.netInRate;
					break;
				case 'net_tx_rate':
					series = entry.netOutSeries;
					value = entry.netOutRate;
					break;
				default:
					value = spec.resolve(s);
					break;
			}

			rows.push({
				key,
				label: spec.label.toUpperCase(),
				value: spec.format ? spec.format(value) : value.toFixed(1),
				color,
				series,
				ts: series ? entry.tsSeries : undefined
			});
		}
		return rows;
	});
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="rich-card"
	class:crit={entry.status === 'crit'}
	onclick={() => { closeMenu(); onclick?.(); }}
	role="button"
	tabindex="0"
	onkeydown={(e) => e.key === 'Enter' && onclick?.()}
>
	<!-- Status rail -->
	<div class="rail" style="background: {statusColor};"></div>

	<!-- Header -->
	<div class="card-head">
		<div class="identity">
			<HostKindIcon {kind} />
			<div class="id-text">
				<div class="name-row">
					<span class="name">{entry.host.name}</span>
					<StatusIndicator status={entry.status} />
				</div>
				<span class="addr mono">{entry.host.platform || entry.host.os || '—'}</span>
			</div>
		</div>
		<div class="head-right">
			{#each (entry.host.tags ?? []).slice(0, 2) as tag}
				<Tag label={tag} />
			{/each}
			<div class="more-wrap" style="position:relative;">
				<button class="more-btn" onclick={toggleMenu} aria-label="More actions">⋯</button>
				{#if menuOpen}
					<CardMenu {entry} onclose={closeMenu} onconfigure={() => { menuOpen = false; configOpen = true; }} />
				{/if}
			</div>
		</div>
	</div>

	<!-- Meta row -->
	<div class="meta mono">
		{entry.host.os}{entry.host.arch ? ` · ${entry.host.arch}` : ''}
		{entry.host.cpu_model ? ` · ${entry.host.cpu_count}c` : ''}
		{s?.uptime_secs ? ` · up ${fmtDuration(s.uptime_secs)}` : ''}
	</div>

	<!-- Body: metrics + side panel -->
	<div class="body">
		<!-- Left: metric rows (driven by per-host widget config) -->
		<div class="metrics">
			{#each widgetRows as row (row.key)}
				<div class="metric-row">
					<span class="metric-label label-mono">{row.label}</span>
					{#if row.series && row.series.length > 1}
						<Sparkline data={row.series} xs={row.ts} color={row.color} height={26} />
					{:else}
						<span class="metric-spacer"></span>
					{/if}
					<span class="metric-val mono" style="color:{row.color}">{row.value}</span>
				</div>
			{/each}
		</div>

		<!-- Right: side panel -->
		<div class="side-panel">
			<div class="panel-head label-mono">
				{kind === 'docker' ? 'Containers' : 'System'}
			</div>
			{#if kind === 'docker'}
				<div class="container-stats">
					<div class="cstat">
						<span class="cstat-val mono" style="color:var(--ok)">{fmtCount(entry.containers?.running)}</span>
						<span class="cstat-label">Running</span>
					</div>
					<div class="cstat">
						<span class="cstat-val mono text-faint">{fmtCount(entry.containers?.stopped)}</span>
						<span class="cstat-label">Stopped</span>
					</div>
					<div class="cstat">
						<span
							class="cstat-val mono"
							style="color:{(entry.containers?.unhealthy ?? 0) > 0 ? 'var(--crit)' : 'var(--text-dim)'}"
						>{fmtCount(entry.containers?.unhealthy)}</span>
						<span class="cstat-label">Unhealthy</span>
					</div>
				</div>
			{:else}
				<div class="sys-info">
					<div class="mono text-faint" style="font-size:12px;">
						{s ? fmtBytes(s.mem_used ?? 0) + ' / ' + fmtBytes(s.mem_total ?? 0) : '—'}
					</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- Alert footer -->
	{#if (entry.host.open_alerts ?? 0) > 0}
		<div class="alert-footer" style="background: var(--warn-soft);">
			⚠ {entry.host.open_alerts} open alert{(entry.host.open_alerts ?? 0) > 1 ? 's' : ''}
		</div>
	{/if}
</div>

{#if configOpen}
	<CardConfigModal {entry} onclose={() => (configOpen = false)} />
{/if}

<style>
	.rich-card {
		position: relative;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
		cursor: pointer;
		transition: border-color var(--dur-card) var(--ease-card),
		            box-shadow var(--dur-card) var(--ease-card);
	}

	@media (prefers-reduced-motion: no-preference) {
		.rich-card:hover {
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
		transition: width var(--dur-card) var(--ease-card);
	}

	.rich-card:hover .rail { width: 3px; }

	.card-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		padding: 14px 16px 6px 18px;
		gap: 8px;
	}

	.identity {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.id-text { display: flex; flex-direction: column; gap: 2px; }

	.name-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.name {
		font-size: 16px;
		font-weight: 600;
		letter-spacing: -0.01em;
		color: var(--text);
	}

	.addr { font-size: 11px; color: var(--text-faint); }

	.head-right {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-shrink: 0;
	}

	.more-btn {
		width: 24px;
		height: 24px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 16px;
		color: var(--text-faint);
		background: none;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
		line-height: 1;
	}

	.more-btn:hover { background: var(--bg-hover); color: var(--text); }

	.meta {
		padding: 0 16px 12px 18px;
		font-size: 11px;
		color: var(--text-faint);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.body {
		display: grid;
		grid-template-columns: 1fr 1fr;
		border-top: 1px solid var(--line);
	}

	.metric-spacer {
		/* Placeholder used when a widget metric has no series (e.g. swap_pct,
		   load_1) — keeps the row's column alignment with sparkline-bearing
		   metrics. */
		width: 140px;
		height: 26px;
		flex-shrink: 0;
	}

	.metrics {
		display: flex;
		flex-direction: column;
	}

	.metric-row {
		display: grid;
		grid-template-columns: 36px 1fr auto;
		align-items: center;
		gap: 8px;
		padding: 8px 12px 8px 14px;
		border-bottom: 1px solid var(--line);
	}

	.metric-row:last-child { border-bottom: none; }

	.metric-label { font-size: 10px; }

	.metric-val {
		font-size: 13px;
		font-weight: 500;
		white-space: nowrap;
	}

	.arr { color: var(--text-dim); }

	.side-panel {
		border-left: 1px solid var(--line);
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.panel-head { color: var(--text-dim); }

	.container-stats {
		display: grid;
		grid-template-columns: 1fr 1fr 1fr;
		gap: 4px;
	}

	.cstat {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
	}

	.cstat-val {
		font-size: 22px;
		font-weight: 500;
		letter-spacing: -0.02em;
		line-height: 1;
	}

	.cstat-label {
		font-size: 11px;
		color: var(--text-faint);
	}

	.alert-footer {
		padding: 8px 16px 8px 18px;
		font-size: 12px;
		color: var(--warn);
		border-top: 1px solid var(--line);
	}
</style>
