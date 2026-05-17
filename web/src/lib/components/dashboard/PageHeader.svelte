<script lang="ts">
	import { hostStore } from '$lib/stores/hosts.svelte';
	import { fmtAbsolute } from '$lib/format';
	import { summarize } from '$lib/monitoring/issues';

	// Single derivation: same helper that NeedsAttention's deriveIssues backs.
	// The OPEN ALERTS column literally counts the events array that drives
	// the per-alert rows in Needs Attention — they cannot disagree.
	const totals = $derived(summarize(hostStore.list, hostStore.events));

	const hostCount = $derived(Object.keys(hostStore.entries).length);
	const syncLabel = $derived(
		hostStore.lastSync
			? `last sync ${fmtAbsolute(hostStore.lastSync.toISOString())}`
			: 'connecting…'
	);
</script>

<div class="page-header">
	<div class="left">
		<h1>Dashboard</h1>
		<p class="sub mono">{hostCount} host{hostCount !== 1 ? 's' : ''} · {syncLabel}</p>
	</div>
	<div class="stat-strip">
		<div class="stat">
			<span class="stat-label label-mono">Healthy</span>
			<span class="stat-value mono" style="color: var(--ok)">{totals.healthy}</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Warning</span>
			<span class="stat-value mono" style="color: var(--warn)">{totals.warn}</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Critical</span>
			<span class="stat-value mono" style="color: var(--crit)">{totals.crit}</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Containers</span>
			<span class="stat-value mono">
				{#if totals.containers.total > 0}
					<span style="color: var(--ok)">{totals.containers.running}</span><span class="ctr-sep">/{totals.containers.total}</span>
				{:else}
					—
				{/if}
			</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Unhealthy</span>
			<span class="stat-value mono" style="color: {totals.containers.unhealthy > 0 ? 'var(--crit)' : 'var(--text-dim)'}">
				{totals.containers.unhealthy}
			</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Open Alerts</span>
			<span class="stat-value mono" style="color: {totals.openAlerts > 0 ? 'var(--warn)' : 'var(--text-dim)'}">
				{totals.openAlerts}
			</span>
		</div>
	</div>
</div>

<style>
	.page-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 24px;
		flex-wrap: wrap;
	}

	h1 { font-size: 22px; font-weight: 600; letter-spacing: -0.02em; }

	.sub {
		font-size: 12px;
		color: var(--text-faint);
		margin-top: 2px;
	}

	.stat-strip {
		display: flex;
		align-items: center;
		gap: 24px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 8px 18px;
	}

	.stat {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
	}

	.stat-label {
		color: var(--text-faint);
	}

	.stat-value {
		font-size: 18px;
		font-weight: 500;
		letter-spacing: -0.02em;
		line-height: 1;
	}

	.ctr-sep {
		color: var(--text-faint);
		font-size: 13px;
	}
</style>
