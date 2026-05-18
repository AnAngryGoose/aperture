<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { hostStore, type HostEntry } from '$lib/stores/hosts.svelte';
	import { fmtBytes, fmtDuration, fmtRelative } from '$lib/format';
	import StatusIndicator from '$lib/components/primitives/StatusIndicator.svelte';
	import HostKindIcon from '$lib/components/primitives/HostKindIcon.svelte';
	import Tag from '$lib/components/primitives/Tag.svelte';

	let loading = $state(true);
	let error = $state<string | null>(null);
	let pollTimer: ReturnType<typeof setInterval>;

	async function load() {
		try {
			const overview = await api.monitoring.overview();
			hostStore.hydrate(overview);
			error = null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load hosts';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		// SSE on dashboard handles live updates; this page is a dense table
		// view where 30s reconciliation is sufficient.
		pollTimer = setInterval(load, 30_000);
	});

	onDestroy(() => clearInterval(pollTimer));

	function open(entry: HostEntry) {
		goto(`/hosts/${entry.host.id}/overview`);
	}

	function kindOf(entry: HostEntry): 'docker' | 'linux' | 'edge' {
		return (entry.host.kind as 'docker' | 'linux' | 'edge') || 'linux';
	}
</script>

<svelte:head><title>Aperture — Hosts</title></svelte:head>

<div class="page">
	<header class="head">
		<div>
			<h1>Hosts</h1>
			<p class="sub">
				{hostStore.list.length} {hostStore.list.length === 1 ? 'host' : 'hosts'} ·
				system-level management surface
			</p>
		</div>
	</header>

	{#if error}
		<div class="err">{error}</div>
	{/if}

	{#if loading && hostStore.list.length === 0}
		<div class="empty mono">Loading…</div>
	{:else if hostStore.list.length === 0}
		<div class="empty">No hosts yet. Add one from the Dashboard.</div>
	{:else}
		<div class="table-card">
			<div class="thead label-mono">
				<span>Host</span>
				<span>Kind</span>
				<span>OS</span>
				<span>Arch</span>
				<span>CPU</span>
				<span>Memory</span>
				<span>Agent</span>
				<span>Uptime</span>
				<span>Last sync</span>
				<span class="right">Alerts</span>
			</div>

			{#each hostStore.list as entry (entry.host.id)}
				{@const h = entry.host}
				{@const s = entry.latest}
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="trow"
					role="button"
					tabindex="0"
					onclick={() => open(entry)}
					onkeydown={(e) => e.key === 'Enter' && open(entry)}
				>
					<div class="name-cell">
						<StatusIndicator status={entry.status} />
						<span class="name">{h.name}</span>
						<div class="tags">
							{#each (h.tags ?? []).slice(0, 3) as tag}<Tag label={tag} />{/each}
						</div>
					</div>
					<div class="kind-cell">
						<HostKindIcon kind={kindOf(entry)} size={12} />
						<span class="mono">{kindOf(entry)}</span>
					</div>
					<span class="mono dim">{h.platform || h.os || '—'}</span>
					<span class="mono dim">{h.arch || '—'}</span>
					<span class="mono dim">{h.cpu_count ? `${h.cpu_count}c` : '—'}</span>
					<span class="mono dim">{h.mem_total ? fmtBytes(h.mem_total) : '—'}</span>
					<span class="mono dim">{h.agent_version || h.source || '—'}</span>
					<span class="mono dim">{s?.uptime_secs ? fmtDuration(s.uptime_secs) : '—'}</span>
					<span class="mono dim">{h.last_seen ? fmtRelative(h.last_seen) : '—'}</span>
					<span class="alerts right mono">
						{#if (h.open_alerts ?? 0) > 0}
							<span class="alert-badge">{h.open_alerts}</span>
						{:else}
							<span class="dim">0</span>
						{/if}
					</span>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.head {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
	}

	h1 {
		margin: 0;
		font-size: 22px;
		font-weight: 600;
		letter-spacing: -0.01em;
		color: var(--text);
	}

	.sub {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--text-faint);
	}

	.err {
		padding: 10px 14px;
		font-size: 12px;
		color: var(--crit);
		background: var(--bg-elev);
		border: 1px solid var(--crit);
		border-radius: var(--r-md);
	}

	.empty {
		padding: 48px 0;
		text-align: center;
		color: var(--text-faint);
		font-size: 13px;
	}

	.table-card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
	}

	.thead,
	.trow {
		display: grid;
		grid-template-columns:
			minmax(220px, 1.4fr) 100px minmax(120px, 1fr) 80px 60px 80px
			minmax(80px, 0.8fr) 90px 110px 70px;
		align-items: center;
		gap: 14px;
		padding: 10px 18px;
	}

	.thead {
		color: var(--text-faint);
		letter-spacing: 0.12em;
		border-bottom: 1px solid var(--line);
		background: var(--bg-elev-2, var(--bg-elev));
	}

	.trow {
		font-size: 13px;
		color: var(--text);
		border-bottom: 1px solid var(--line);
		cursor: pointer;
		transition: background 120ms;
	}

	.trow:last-child { border-bottom: none; }
	.trow:hover { background: var(--bg-hover); }
	.trow:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }

	.name-cell {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}

	.name {
		font-weight: 500;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.tags {
		display: flex;
		gap: 4px;
		margin-left: 4px;
		flex-wrap: wrap;
	}

	.kind-cell {
		display: flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: var(--text-dim);
	}

	.dim { color: var(--text-faint); font-size: 12px; }
	.right { text-align: right; }

	.alert-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 22px;
		height: 18px;
		padding: 0 6px;
		background: var(--crit);
		color: #fff;
		font-family: var(--font-mono);
		font-size: 11px;
		border-radius: var(--r-pill);
	}
</style>
