<script lang="ts">
	import { onMount } from 'svelte';
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import type { Container, AlertEvent } from '$lib/types';
	import { api } from '$lib/api';
	import { fmtBytes, fmtRate, fmtDuration } from '$lib/format';
	import Icon from '$lib/components/primitives/Icon.svelte';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import HostKindIcon from '$lib/components/primitives/HostKindIcon.svelte';
	import Tag from '$lib/components/primitives/Tag.svelte';
	import BigMetric from './BigMetric.svelte';
	import StoragePanel from './StoragePanel.svelte';
	import ContainersPanel from './ContainersPanel.svelte';
	import EventsPanel from './EventsPanel.svelte';

	interface Props {
		entry: HostEntry;
		onclose: () => void;
	}

	let { entry, onclose }: Props = $props();

	type DrawerTab = 'overview' | 'containers' | 'stacks' | 'logs' | 'shell';
	let activeTab = $state<DrawerTab>('overview');

	// Drawer tab → full host route. Derived so it follows the current entry
	// when the drawer is reused across host selections.
	const TAB_HREF = $derived<Record<DrawerTab, string>>({
		overview:   `/hosts/${entry.host.id}/overview`,
		containers: `/hosts/${entry.host.id}/containers`,
		stacks:     `/hosts/${entry.host.id}/stacks`,
		logs:       `/hosts/${entry.host.id}/logs`,
		shell:      `/hosts/${entry.host.id}/shell`
	});
	let containers = $state<Container[]>([]);
	let alerts = $state<AlertEvent[]>([]);
	let loadingContainers = $state(false);

	const s = $derived(entry.latest);
	const kind = $derived((entry.host.kind as 'docker' | 'linux' | 'edge') || 'linux');

	async function loadContainers() {
		if (kind !== 'docker') return;
		loadingContainers = true;
		try {
			containers = await api.containers(entry.host.id, true);
		} catch { /* silent */ } finally {
			loadingContainers = false;
		}
	}

	async function loadAlerts() {
		try {
			alerts = await api.alertEvents({ hostID: entry.host.id, limit: 20 });
		} catch { /* silent */ }
	}

	onMount(() => {
		loadContainers();
		loadAlerts();
	});

	function onBackdrop(e: MouseEvent) {
		if (e.currentTarget === e.target) onclose();
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onBackdrop}>
	<div class="panel glass-drillin">
		<!-- Sticky header -->
		<div class="panel-head">
			<div class="head-left">
				<button class="close-btn" onclick={onclose} aria-label="Close"><Icon name="x" size={16} /></button>
				<HostKindIcon {kind} />
				<div class="host-info">
					<div class="name-row">
						<span class="name">{entry.host.name}</span>
						<StatusIndicator status={entry.status} />
					</div>
					<span class="meta mono">{entry.host.platform || entry.host.os || '—'} · {entry.host.arch || '—'}</span>
				</div>
				{#each (entry.host.tags ?? []) as tag}
					<Tag label={tag} />
				{/each}
			</div>
			<div class="head-actions">
				<button class="action-btn">Restart</button>
				<button class="action-btn">SSH</button>
				<button class="action-btn">Update</button>
				<button class="action-btn danger">Stop</button>
			</div>
		</div>

		<!-- Tabs -->
		<div class="tabs">
			{#each ['overview', 'containers', 'stacks', 'logs', 'shell'] as tab}
				<button
					class="tab"
					class:active={activeTab === tab}
					onclick={() => (activeTab = tab as typeof activeTab)}
				>
					{tab.charAt(0).toUpperCase() + tab.slice(1)}
				</button>
			{/each}
		</div>

		<!-- Content -->
		<div class="panel-body">
			{#if activeTab === 'overview'}
				<!-- Big metric grid — each tile is a deep link into the matching
				     host detail tab so a click on CPU lands on /cpu, etc. -->
				<div class="big-metrics">
					<BigMetric
						label="CPU"
						value="{(s?.cpu_percent ?? 0).toFixed(1)}%"
						sub="{entry.host.cpu_count} cores · {entry.host.cpu_model || '—'}"
						data={entry.cpuSeries}
						href="/hosts/{entry.host.id}/cpu"
						onclick={onclose}
					/>
					<BigMetric
						label="Memory"
						value="{(s?.mem_percent ?? 0).toFixed(1)}%"
						sub="{fmtBytes(s?.mem_used ?? 0)} / {fmtBytes(entry.host.mem_total ?? 0)}"
						data={entry.memSeries}
						href="/hosts/{entry.host.id}/memory"
						onclick={onclose}
					/>
					<BigMetric
						label="Network ↓"
						value={fmtRate(entry.netInRate)}
						sub="↑ {fmtRate(entry.netOutRate)}"
						data={entry.netInSeries}
						color="var(--info)"
						href="/hosts/{entry.host.id}/network"
						onclick={onclose}
					/>
					{#if (s?.temps ?? []).length > 0}
						{@const temps = s!.temps!}
						{@const maxTemp = temps.reduce((a, t) => Math.max(a, t.temp_celsius), 0)}
						{@const tempColor = maxTemp >= 85 ? 'var(--crit)' : maxTemp >= 70 ? 'var(--warn)' : 'var(--info)'}
						<BigMetric
							label="Temperature"
							value="{maxTemp.toFixed(1)}°C"
							sub="{temps.length} sensor{temps.length === 1 ? '' : 's'}"
							data={[]}
							color={tempColor}
							href="/hosts/{entry.host.id}/sensors"
							onclick={onclose}
						/>
					{:else}
						<BigMetric
							label="Temperature"
							value="—"
							sub="no sensors"
							data={[]}
							color="var(--text-faint)"
							href="/hosts/{entry.host.id}/sensors"
							onclick={onclose}
						/>
					{/if}
				</div>

				<!-- Lower panels -->
				<div class="lower-panels">
					<a class="panel-card link" href="/hosts/{entry.host.id}/disk" onclick={onclose}>
						<StoragePanel sample={s} />
					</a>
					{#if kind === 'docker'}
						<a class="panel-card link" href="/hosts/{entry.host.id}/containers" onclick={onclose}>
							<ContainersPanel {containers} loading={loadingContainers} />
						</a>
					{:else if (s?.processes ?? []).length > 0}
						<a class="panel-card link" href="/hosts/{entry.host.id}/processes" onclick={onclose}>
							<div class="proc-peek">
								<div class="label-mono">Top by CPU</div>
								<div class="proc-list">
									{#each [...(s!.processes!)].sort((a, b) => b.cpu_pct - a.cpu_pct).slice(0, 5) as p}
										<div class="proc-row">
											<span class="proc-name">{p.name}</span>
											<span class="proc-cpu mono">{p.cpu_pct.toFixed(1)}%</span>
										</div>
									{/each}
								</div>
							</div>
						</a>
					{:else}
						<div class="panel-card">
							<div class="text-faint" style="font-size:12px;">
								Uptime: <span class="mono">{s?.uptime_secs ? fmtDuration(s.uptime_secs) : '—'}</span>
							</div>
						</div>
					{/if}
					<a class="panel-card link" href="/hosts/{entry.host.id}/events" onclick={onclose}>
						<EventsPanel events={alerts} hostId={entry.host.id} />
					</a>
				</div>

				{#if (s?.temps ?? []).length > 0}
					<a class="sensors-mini link" href="/hosts/{entry.host.id}/sensors" onclick={onclose}>
						<div class="label-mono">Sensors</div>
						<div class="sensor-grid">
							{#each (s?.temps ?? []) as sensor}
								{@const c = sensor.temp_celsius}
								{@const color = c >= 85 ? 'var(--crit)' : c >= 70 ? 'var(--warn)' : 'var(--info)'}
								<div class="sensor-cell" style="border-color:{color};">
									<span class="sensor-temp mono" style="color:{color}">{c.toFixed(1)}°</span>
									<span class="sensor-name mono">{sensor.name}</span>
								</div>
							{/each}
						</div>
					</a>
				{/if}

				<!-- CTA into the full host monitoring page -->
				<div class="full-cta">
					<a href={TAB_HREF.overview} class="full-link" onclick={onclose}>
						Open full host monitoring →
					</a>
				</div>
			{:else if activeTab === 'containers'}
				<div style="padding:16px;">
					<a href={TAB_HREF.containers} class="goto-link" onclick={onclose}>
						Open full container management →
					</a>
				</div>
			{:else if activeTab === 'stacks'}
				<div style="padding:16px;">
					<a href={TAB_HREF.stacks} class="goto-link" onclick={onclose}>
						Open Compose stacks →
					</a>
				</div>
			{:else if activeTab === 'logs'}
				<div style="padding:16px;">
					<a href={TAB_HREF.logs} class="goto-link" onclick={onclose}>
						Open logs viewer →
					</a>
				</div>
			{:else if activeTab === 'shell'}
				<div style="padding:16px;">
					<a href={TAB_HREF.shell} class="goto-link" onclick={onclose}>
						Open shell →
					</a>
				</div>
			{/if}
		</div>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 80;
		background: rgba(0,0,0,.55);
		backdrop-filter: blur(8px);
		display: flex;
		justify-content: flex-end;
	}

	.panel {
		width: min(1080px, 95vw);
		height: 100%;
		background: var(--bg);
		border-left: 1px solid var(--line);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	@media (prefers-reduced-motion: no-preference) {
		.panel {
			animation: slide-in var(--dur-slide) var(--ease-card) both;
		}

		@keyframes slide-in {
			from { transform: translateX(100%); }
			to   { transform: translateX(0); }
		}
	}

	.panel-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 16px 20px;
		border-bottom: 1px solid var(--line);
		gap: 12px;
		flex-shrink: 0;
	}

	.head-left {
		display: flex;
		align-items: center;
		gap: 10px;
		flex: 1;
		min-width: 0;
	}

	.close-btn {
		width: 28px;
		height: 28px;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-faint);
		background: none;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
		flex-shrink: 0;
	}

	.close-btn:hover { background: var(--bg-hover); color: var(--text); }

	.host-info { display: flex; flex-direction: column; gap: 2px; }

	.name-row { display: flex; align-items: center; gap: 8px; }

	.name { font-size: 20px; font-weight: 600; letter-spacing: -0.01em; color: var(--text); }

	.meta { font-size: 11px; color: var(--text-faint); }

	.head-actions {
		display: flex;
		gap: 6px;
		flex-shrink: 0;
	}

	.action-btn {
		padding: 6px 12px;
		font-size: 12px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 120ms, color 120ms;
	}

	.action-btn:hover { background: var(--bg-hover); color: var(--text); }

	.action-btn.danger { color: var(--warn); background: var(--warn-soft); border-color: var(--warn); }
	.action-btn.danger:hover { background: var(--warn); color: #fff; }

	.tabs {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--line);
		padding: 0 20px;
		flex-shrink: 0;
	}

	.tab {
		padding: 10px 14px;
		font-size: 13px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
		cursor: pointer;
		transition: color 120ms, border-color 120ms;
	}

	.tab:hover { color: var(--text); }

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.panel-body {
		flex: 1;
		overflow-y: auto;
		padding: 20px;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	.big-metrics {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 12px;
	}

	.lower-panels {
		display: grid;
		grid-template-columns: 1fr 1fr 1fr;
		gap: 16px;
	}

	.panel-card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 16px;
		display: block;
		color: inherit;
		text-decoration: none;
	}
	.panel-card.link {
		cursor: pointer;
		transition: border-color 120ms, background 120ms;
	}
	.panel-card.link:hover {
		border-color: var(--line-strong);
		background: var(--bg-hover, var(--bg-elev));
	}

	.goto-link {
		font-size: 13px;
		color: var(--accent);
	}

	.label-mono {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		font-family: var(--font-mono);
		margin-bottom: 8px;
	}

	.proc-peek {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.proc-list {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.proc-row {
		display: flex;
		justify-content: space-between;
		gap: 8px;
		font-size: 12px;
	}

	.proc-name {
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
		flex: 1;
	}

	.proc-cpu {
		color: var(--text-dim);
		flex-shrink: 0;
	}

	.sensors-mini {
		display: block;
		padding: 12px 16px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		color: inherit;
		text-decoration: none;
	}
	.sensors-mini.link {
		cursor: pointer;
		transition: border-color 120ms, background 120ms;
	}
	.sensors-mini.link:hover {
		border-color: var(--line-strong);
		background: var(--bg-hover, var(--bg-elev));
	}

	.sensor-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
		gap: 8px;
	}

	.sensor-cell {
		display: flex;
		flex-direction: column;
		gap: 2px;
		padding: 8px 10px;
		background: var(--bg-elev-2);
		border-left: 2px solid var(--line);
		border-radius: var(--r-md);
	}

	.sensor-temp { font-size: 15px; font-weight: 500; letter-spacing: -0.02em; }
	.sensor-name { font-size: 10px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint); }

	.full-cta {
		display: flex;
		justify-content: center;
		padding: 6px 0;
	}

	.full-link {
		font-size: 13px;
		color: var(--accent);
		font-family: var(--font-mono);
		text-decoration: none;
		padding: 6px 14px;
		border-radius: var(--r-md);
		transition: background 120ms;
	}
	.full-link:hover { background: var(--accent-soft); }
</style>
