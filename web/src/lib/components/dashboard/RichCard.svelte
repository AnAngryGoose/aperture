<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import Sparkline from '$lib/components/primitives/Sparkline.svelte';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import Meter from '$lib/components/primitives/Meter.svelte';
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

	const diskColor = $derived(
		(s?.disk_percent ?? 0) >= 90 ? 'var(--crit)' :
		(s?.disk_percent ?? 0) >= 75 ? 'var(--warn)' :
		'var(--accent)'
	);

	const statusColor = $derived(
		entry.status === 'crit' ? 'var(--crit)' :
		entry.status === 'warn' ? 'var(--warn)' :
		entry.status === 'ok' ? 'var(--ok)' :
		'var(--offline)'
	);

	const statusLabel = $derived(
		entry.status === 'ok' ? 'Healthy' :
		entry.status === 'warn' ? 'Warning' :
		entry.status === 'crit' ? 'Critical' :
		'Offline'
	);

	// Hottest temperature reading across all sensors — surfaced on the CPU
	// row as a context sub-label ("16 cores · 58°C") and inside any temp_max
	// widget. Returns 0 (not undefined) so we can branch on `> 0` to decide
	// whether to render the suffix.
	const maxTemp = $derived(
		(s?.temps ?? []).reduce((acc, t) => Math.max(acc, t.temp_celsius), 0)
	);

	// Rich meta line: OS + version, arch, CPU model (truncated by overflow if long),
	// core count, RAM total, agent version (when known), uptime.
	const metaLine = $derived.by(() => {
		const h = entry.host;
		const parts: string[] = [];
		if (h.platform) parts.push(h.platform);
		else if (h.os) parts.push(h.os);
		if (h.os && h.arch) parts.push(`${h.os}/${h.arch}`);
		else if (h.arch) parts.push(h.arch);
		if (h.cpu_model) parts.push(h.cpu_model);
		if (h.cpu_count) parts.push(`${h.cpu_count}c`);
		if (h.mem_total) parts.push(fmtBytes(h.mem_total));
		if (h.agent_version) parts.push(`agent ${h.agent_version}`);
		if (s?.uptime_secs) parts.push(`up ${fmtDuration(s.uptime_secs)}`);
		return parts.join(' · ');
	});

	// Primary mount info for the disk sub-label. Prefer the first disk_mounts
	// entry with the largest total when available, else show the configured
	// root path ("/").
	const primaryMount = $derived.by(() => {
		const mounts = s?.disk_mounts ?? [];
		if (mounts.length === 0) return null;
		let best = mounts[0];
		for (const m of mounts) {
			if (m.total > best.total) best = m;
		}
		return best;
	});

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

	// Per-widget render hint. Sparkline-bearing metrics (cpu/mem/net rates)
	// already track a series in the entry's ring buffers. Percent metrics
	// without series (disk_pct, swap_pct) render as a Meter — the prototype
	// shows DISK as a wide bar, not a sparkline.
	type Render = 'sparkline' | 'meter' | 'none';

	interface WidgetRow {
		key: string;
		label: string;
		value: string;
		sub: string;
		color: string;
		render: Render;
		pct?: number;            // for render === 'meter'
		series?: number[];       // for render === 'sparkline'
		ts?: number[];
	}

	const widgetRows = $derived.by<WidgetRow[]>(() => {
		const rows: WidgetRow[] = [];
		if (!s) return rows;
		const memTotal = entry.host.mem_total ?? s.mem_total;

		for (const key of widgetKeys) {
			const spec = getMetric(key);
			if (!spec) continue;
			let series: number[] | undefined;
			let value: number;
			let color = spec.color;
			let render: Render = 'none';
			let pct = 0;
			let sub = '';

			switch (key) {
				case 'cpu_pct': {
					series = entry.cpuSeries;
					value = s.cpu_percent;
					color = cpuColor;
					render = 'sparkline';
					const cores = entry.host.cpu_count || (s.cpu_per_core?.length ?? 0);
					sub = cores ? `${cores} cores` : '';
					if (maxTemp > 0) sub += (sub ? ' · ' : '') + `${maxTemp.toFixed(0)}°C`;
					break;
				}
				case 'mem_pct': {
					series = entry.memSeries;
					value = s.mem_percent;
					color = memColor;
					render = 'sparkline';
					sub = `${fmtBytes(s.mem_used)} / ${fmtBytes(memTotal)}`;
					break;
				}
				case 'swap_pct': {
					value = s.swap_total ? (s.swap_used / s.swap_total) * 100 : 0;
					render = 'meter';
					pct = value;
					color = value >= 50 ? 'var(--warn)' : 'var(--text-faint)';
					sub = s.swap_total ? `${fmtBytes(s.swap_used)} / ${fmtBytes(s.swap_total)}` : 'no swap';
					break;
				}
				case 'net_rx_rate': {
					series = entry.netInSeries;
					value = entry.netInRate;
					render = 'sparkline';
					sub = `↑ ${fmtRate(entry.netOutRate)}`;
					break;
				}
				case 'net_tx_rate': {
					series = entry.netOutSeries;
					value = entry.netOutRate;
					render = 'sparkline';
					sub = `↓ ${fmtRate(entry.netInRate)}`;
					break;
				}
				case 'disk_pct': {
					value = s.disk_percent;
					render = 'meter';
					pct = value;
					color = diskColor;
					if (s.disk_used && s.disk_total) {
						sub = `${fmtBytes(s.disk_used)} / ${fmtBytes(s.disk_total)}`;
					}
					if (primaryMount) {
						const m = primaryMount;
						const tail = `${m.device || ''}${m.device && m.fstype ? ` · ${m.fstype}` : m.fstype}`.trim();
						if (tail) sub += (sub ? ' · ' : '') + tail;
					}
					break;
				}
				case 'temp_max': {
					value = maxTemp;
					const count = (s.temps ?? []).length;
					sub = count ? `${count} sensor${count === 1 ? '' : 's'}` : 'no sensors';
					color = value >= 85 ? 'var(--crit)' : value >= 70 ? 'var(--warn)' : 'var(--info)';
					break;
				}
				case 'load_1':
				case 'load_5':
					value = spec.resolve(s);
					sub = entry.host.cpu_count ? `${entry.host.cpu_count} cores` : '';
					break;
				case 'mem_used':
					value = s.mem_used;
					sub = `of ${fmtBytes(memTotal)}`;
					break;
				case 'uptime':
					value = s.uptime_secs;
					break;
				default:
					value = spec.resolve(s);
			}

			rows.push({
				key,
				label: spec.label.toUpperCase(),
				value: spec.format ? spec.format(value) : value.toFixed(1),
				sub,
				color,
				render,
				pct,
				series,
				ts: series ? entry.tsSeries : undefined
			});
		}
		return rows;
	});

	// Top-4 processes by CPU% — surfaced as the "TOP BY CPU" side panel
	// matching the prototype. Filtered to processes with non-zero CPU so we
	// don't waste rows on idle services when the host is quiet (the
	// collector emits top 20+ by CPU/RSS even at idle).
	const topProcs = $derived(
		(s?.processes ?? [])
			.filter((p) => p.cpu_pct > 0)
			.sort((a, b) => b.cpu_pct - a.cpu_pct)
			.slice(0, 4)
	);

	function fmtProcMem(b: number): string {
		if (b < 1048576) return `${(b / 1024).toFixed(0)} KB`;
		if (b < 1073741824) return `${(b / 1048576).toFixed(0)} MB`;
		return `${(b / 1073741824).toFixed(1)} GB`;
	}

	const containerTotal = $derived(
		(entry.containers?.running ?? 0)
		+ (entry.containers?.stopped ?? 0)
	);

	// Metric widget key → host detail tab path. Clicking a metric row goes
	// straight to that tab instead of the generic overview. Anything not in
	// this map falls back to opening the drawer (the card's onclick handler).
	const WIDGET_TAB: Record<string, string> = {
		cpu_pct:    'cpu',
		mem_pct:    'memory',
		mem_used:   'memory',
		swap_pct:   'memory',
		disk_pct:   'disk',
		net_rx_rate:'network',
		net_tx_rate:'network',
		temp_max:   'sensors',
		load_1:     'cpu',
		load_5:     'cpu'
	};

	function rowHref(key: string): string | null {
		const tab = WIDGET_TAB[key];
		return tab ? `/hosts/${entry.host.id}/${tab}` : null;
	}

	function rowClick(e: MouseEvent, href: string | null) {
		// Per-metric clicks bypass the card's open-drawer handler so the click
		// target on a CPU row goes to /cpu rather than opening the drawer.
		if (!href) return;
		e.stopPropagation();
		// Honor modifier keys (cmd-click for new tab) by deferring to the
		// browser's default anchor behavior — the wrapping <a> element handles
		// that via href.
	}
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
					<span class="status-label" style="color:{statusColor}">{statusLabel}</span>
				</div>
				<div class="meta mono" title={metaLine}>{metaLine || '—'}</div>
			</div>
		</div>
		<div class="head-right">
			{#each (entry.host.tags ?? []).slice(0, 3) as tag}
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

	<!-- Body: metrics + side panel -->
	<div class="body">
		<!-- Left: metric rows (driven by per-host widget config) -->
		<div class="metrics">
			{#each widgetRows as row (row.key)}
				{@const href = rowHref(row.key)}
				{#if href}
					<a class="metric-row link" {href} onclick={(e) => rowClick(e, href)}>
						<span class="metric-label label-mono">{row.label}</span>
						<div class="metric-vis">
							{#if row.render === 'sparkline' && row.series && row.series.length > 1}
								<Sparkline data={row.series} xs={row.ts} color={row.color} height={26} />
							{:else if row.render === 'meter'}
								<Meter value={row.pct ?? 0} max={100} height={6} />
							{/if}
						</div>
						<div class="metric-vals">
							<span class="metric-val mono" style="color:{row.color}">{row.value}</span>
							{#if row.sub}
								<span class="metric-sub mono">{row.sub}</span>
							{/if}
						</div>
					</a>
				{:else}
					<div class="metric-row">
						<span class="metric-label label-mono">{row.label}</span>
						<div class="metric-vis">
							{#if row.render === 'sparkline' && row.series && row.series.length > 1}
								<Sparkline data={row.series} xs={row.ts} color={row.color} height={26} />
							{:else if row.render === 'meter'}
								<Meter value={row.pct ?? 0} max={100} height={6} />
							{/if}
						</div>
						<div class="metric-vals">
							<span class="metric-val mono" style="color:{row.color}">{row.value}</span>
							{#if row.sub}
								<span class="metric-sub mono">{row.sub}</span>
							{/if}
						</div>
					</div>
				{/if}
			{/each}
		</div>

		<!-- Right: container summary + top by CPU -->
		<aside class="side">
			{#if kind === 'docker'}
				<a class="panel containers-panel link" href="/hosts/{entry.host.id}/containers" onclick={(e) => e.stopPropagation()}>
					<div class="panel-head label-mono">
						<span>Containers</span>
						{#if entry.containers}
							<span class="panel-head-num mono">{containerTotal}<span class="panel-head-num-sub">TOTAL</span></span>
						{/if}
					</div>
					<div class="container-stats">
						<div class="cstat">
							<span class="cstat-val mono" style="color:var(--ok)">{fmtCount(entry.containers?.running)}</span>
							<span class="cstat-label">running</span>
						</div>
						<div class="cstat">
							<span class="cstat-val mono" style="color:var(--text-faint)">{fmtCount(entry.containers?.stopped)}</span>
							<span class="cstat-label">stopped</span>
						</div>
						<div class="cstat">
							<span
								class="cstat-val mono"
								style="color:{(entry.containers?.unhealthy ?? 0) > 0 ? 'var(--crit)' : 'var(--text-faint)'}"
							>{fmtCount(entry.containers?.unhealthy)}</span>
							<span class="cstat-label">unhealthy</span>
						</div>
					</div>
				</a>
			{/if}

			{#if topProcs.length > 0}
				<section class="panel topproc-panel">
					<div class="panel-head label-mono">Top by CPU</div>
					<div class="proc-list">
						{#each topProcs as p}
							<div class="proc-row">
								<span class="proc-name" title={p.name}>{p.name}</span>
								<span class="proc-cpu mono">{p.cpu_pct.toFixed(1)}%</span>
								<span class="proc-mem mono">{fmtProcMem(p.mem_rss)}</span>
							</div>
						{/each}
					</div>
				</section>
			{:else if kind !== 'docker'}
				<section class="panel empty-panel">
					<div class="panel-head label-mono">System</div>
					<div class="empty-note mono">
						{s?.uptime_secs ? `up ${fmtDuration(s.uptime_secs)}` : 'no recent sample'}
					</div>
				</section>
			{/if}
		</aside>
	</div>

	<!-- Alert footer -->
	{#if (entry.host.open_alerts ?? 0) > 0}
		<a
			class="alert-footer"
			style="background: var(--warn-soft);"
			href="/hosts/{entry.host.id}/events"
			onclick={(e) => e.stopPropagation()}
		>
			⚠ {entry.host.open_alerts} open alert{(entry.host.open_alerts ?? 0) > 1 ? 's' : ''}
		</a>
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
		padding: 14px 16px 12px 18px;
		gap: 8px;
	}

	.identity {
		display: flex;
		align-items: center;
		gap: 12px;
		min-width: 0;
		flex: 1;
	}

	.id-text {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.name-row {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}

	.name {
		font-size: 17px;
		font-weight: 600;
		letter-spacing: -0.01em;
		color: var(--text);
	}

	.status-label {
		font-size: 12px;
		font-weight: 500;
	}

	.meta {
		font-size: 11px;
		color: var(--text-faint);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

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

	/* Body layout: metrics on the left take the bulk of the card, side
	   panel is a fixed-width column so the metric column doesn't reflow
	   when containers/processes appear or disappear. */
	.body {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 240px;
		border-top: 1px solid var(--line);
	}

	.metrics { display: flex; flex-direction: column; }

	.metric-row {
		display: grid;
		grid-template-columns: 48px minmax(80px, 1fr) auto;
		align-items: center;
		gap: 12px;
		padding: 9px 14px 9px 16px;
		border-bottom: 1px solid var(--line);
		color: inherit;
		text-decoration: none;
	}
	.metric-row.link {
		cursor: pointer;
		transition: background 100ms;
	}
	.metric-row.link:hover { background: var(--bg-hover, transparent); }

	.metric-row:last-child { border-bottom: none; }

	.metric-label {
		font-size: 10px;
		color: var(--text-faint);
	}

	.metric-vis {
		display: flex;
		align-items: center;
		min-height: 26px;
	}

	.metric-vals {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 2px;
		min-width: 0;
	}

	.metric-val {
		font-size: 14px;
		font-weight: 500;
		white-space: nowrap;
	}

	.metric-sub {
		font-size: 11px;
		color: var(--text-faint);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

	/* Side panel: stacked rich cards (containers + top procs). */
	.side {
		display: flex;
		flex-direction: column;
		border-left: 1px solid var(--line);
	}

	.panel {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 12px 14px;
		border-bottom: 1px solid var(--line);
		color: inherit;
		text-decoration: none;
	}
	.panel.link {
		cursor: pointer;
		transition: background 100ms;
	}
	.panel.link:hover { background: var(--bg-hover, transparent); }

	.panel:last-child { border-bottom: none; }

	.panel-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 8px;
		color: var(--text-dim);
		text-transform: uppercase;
		font-size: 10px;
		letter-spacing: 0.08em;
	}

	.panel-head-num {
		display: inline-flex;
		align-items: baseline;
		gap: 4px;
		font-size: 14px;
		color: var(--text);
		text-transform: none;
		letter-spacing: 0;
	}

	.panel-head-num-sub {
		font-size: 9px;
		color: var(--text-faint);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.container-stats {
		display: grid;
		grid-template-columns: 1fr 1fr 1fr;
		gap: 6px;
	}

	.cstat {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 2px;
	}

	.cstat-val {
		font-size: 22px;
		font-weight: 500;
		letter-spacing: -0.02em;
		line-height: 1;
	}

	.cstat-label {
		font-size: 10px;
		color: var(--text-faint);
		text-transform: lowercase;
	}

	/* Top-by-CPU rows: 3 columns (name | cpu% | mem). The name column
	   ellipsis-truncates when long so the numbers stay aligned. */
	.proc-list { display: flex; flex-direction: column; gap: 2px; }

	.proc-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 48px 56px;
		gap: 8px;
		font-size: 12px;
		align-items: baseline;
	}

	.proc-name {
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.proc-cpu {
		color: var(--text-dim);
		text-align: right;
	}

	.proc-mem {
		color: var(--text-faint);
		text-align: right;
	}

	.empty-note {
		font-size: 11px;
		color: var(--text-faint);
	}

	.alert-footer {
		display: block;
		padding: 8px 16px 8px 18px;
		font-size: 12px;
		color: var(--warn);
		border-top: 1px solid var(--line);
		text-decoration: none;
		transition: filter 100ms;
	}
	.alert-footer:hover { filter: brightness(1.1); }
</style>
