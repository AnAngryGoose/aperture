<script lang="ts">
	import { hostStore } from '$lib/stores/hosts';
	import { fmtAbsolute } from '$lib/format';

	const totals = $derived.by(() => {
		let healthy = 0, warn = 0, crit = 0, offline = 0;
		let running = 0, total = 0, unhealthy = 0, openAlerts = 0;

		for (const e of Object.values(hostStore.entries)) {
			if (e.status === 'ok') healthy++;
			else if (e.status === 'warn') warn++;
			else if (e.status === 'crit') crit++;
			else offline++;

			openAlerts += e.host.open_alerts ?? 0;
		}

		return { healthy, warn, crit, offline, running, total, unhealthy, openAlerts };
	});

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
			<span class="stat-value mono">—</span>
		</div>
		<div class="stat">
			<span class="stat-label label-mono">Unhealthy</span>
			<span class="stat-value mono" style="color: {totals.unhealthy > 0 ? 'var(--crit)' : 'var(--text-dim)'}">
				{totals.unhealthy}
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
</style>
