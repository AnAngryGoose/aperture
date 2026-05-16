<script lang="ts">
	import type { MonitoringBundle, AlertEvent } from '$lib/types';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import { fmtRelative, fmtAbsolute } from '$lib/format';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	let events = $state<AlertEvent[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	async function load() {
		loading = true;
		try {
			events = await api.alertEvents({ hostID: bundle.host.id, limit: 100 });
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load events';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function levelColor(level: string): string {
		if (level === 'critical') return 'var(--crit)';
		if (level === 'warning') return 'var(--warn)';
		return 'var(--info)';
	}
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Alert events</h3>
			<span class="card-sub mono">{events.length} recent</span>
		</header>
		{#if loading}
			<div class="empty">Loading events…</div>
		{:else if error}
			<div class="empty err">{error}</div>
		{:else if events.length === 0}
			<div class="empty">No alerts have fired for this host.</div>
		{:else}
			<div class="events">
				{#each events as e}
					<div class="event">
						<div class="event-icon" style="color:{e.resolved_at ? 'var(--ok)' : levelColor('warning')}">
							<Icon name={e.resolved_at ? 'ok' : 'warn'} size={14} />
						</div>
						<div class="event-body">
							<div class="event-text">
								Rule <span class="mono">#{e.rule_id}</span>
								{e.resolved_at ? 'resolved' : 'fired'} ·
								value <span class="mono">{e.value.toFixed(2)}</span>
							</div>
							<div class="event-meta mono" title={fmtAbsolute(e.fired_at)}>
								fired {fmtRelative(e.fired_at)}
								{#if e.resolved_at}
									· resolved {fmtRelative(e.resolved_at)}
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>
</div>

<style>
	.tab { display: flex; flex-direction: column; gap: 14px; }
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 14px 16px;
	}
	.card-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 12px;
	}
	.card-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text); }
	.card-sub { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }

	.empty { font-size: 12px; color: var(--text-faint); padding: 12px 0; }
	.empty.err { color: var(--crit); }

	.events { display: flex; flex-direction: column; gap: 2px; }

	.event {
		display: grid;
		grid-template-columns: 24px 1fr;
		gap: 10px;
		padding: 10px 0;
		border-bottom: 1px solid var(--line);
		align-items: center;
	}
	.event:last-child { border-bottom: none; }

	.event-icon { display: flex; justify-content: center; }

	.event-body { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.event-text { font-size: 13px; color: var(--text); }
	.event-meta { font-size: 11px; color: var(--text-faint); }
</style>
