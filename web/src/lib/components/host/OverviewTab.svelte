<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import { fmtBytes, fmtRate, fmtDuration } from '$lib/format';
	import BigMetric from './BigMetric.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const config = $derived(bundle.config);
	const openAlerts = $derived(bundle.openAlerts ?? []);

	// CPU + Mem aggregate series from the bundle history for sparklines on the
	// big-metric cards. Trimmed to the most recent ~60 points so the
	// glanceable display feels like the dashboard cards.
	const cpuSeries = $derived(
		(bundle.history.metrics ?? []).slice(-60).map((s) => s.cpu_percent)
	);
	const memSeries = $derived(
		(bundle.history.metrics ?? []).slice(-60).map((s) => s.mem_percent)
	);

	// Network rate derived from successive cumulative byte counters in the
	// aggregate history. First sample rates at 0.
	const netInSeries = $derived.by(() => {
		const ms = (bundle.history.metrics ?? []).slice(-60);
		const out: number[] = [];
		for (let i = 0; i < ms.length; i++) {
			if (i === 0) { out.push(0); continue; }
			const prev = ms[i - 1];
			const dt = (new Date(ms[i].timestamp).getTime() - new Date(prev.timestamp).getTime()) / 1000;
			if (dt <= 0) { out.push(0); continue; }
			out.push(Math.max(0, (ms[i].net_rx_bytes - prev.net_rx_bytes) / dt));
		}
		return out;
	});

	// Live network rate from the latest sample's net_rate-derived fields is
	// not available — derive from netInSeries' tail.
	const netInRate = $derived(netInSeries.at(-1) ?? 0);

	// Max temperature across all live sensors — matches dashboard card and
	// the per-host warn/crit thresholds.
	const maxTemp = $derived.by(() => {
		const temps = latest?.temps ?? [];
		return temps.reduce((acc, t) => Math.max(acc, t.temp_celsius ?? 0), 0);
	});
	const tempColor = $derived(
		maxTemp >= config.crit_temp ? 'var(--crit)' :
		maxTemp >= config.warn_temp ? 'var(--warn)' :
		'var(--info)'
	);

	const cpuColor = $derived(
		(latest?.cpu_percent ?? 0) >= config.crit_cpu ? 'var(--crit)' :
		(latest?.cpu_percent ?? 0) >= config.warn_cpu ? 'var(--warn)' :
		'var(--accent)'
	);
	const memColor = $derived(
		(latest?.mem_percent ?? 0) >= config.crit_mem ? 'var(--crit)' :
		(latest?.mem_percent ?? 0) >= config.warn_mem ? 'var(--warn)' :
		'var(--accent)'
	);

	// Top processes peek — 3 by CPU%.
	const topProcs = $derived(
		[...(latest?.processes ?? [])]
			.sort((a, b) => b.cpu_pct - a.cpu_pct)
			.slice(0, 3)
	);
</script>

{#if openAlerts.length > 0}
	<div class="alert-banner" role="alert">
		<Icon name="warn" size={16} />
		<div class="alert-text">
			<strong>{openAlerts.length}</strong>
			open {openAlerts.length === 1 ? 'alert' : 'alerts'} on this host
		</div>
		<a class="alert-link" href="#events">view →</a>
	</div>
{/if}

<div class="big-metrics">
	<BigMetric
		label="CPU"
		value="{(latest?.cpu_percent ?? 0).toFixed(1)}%"
		sub="{bundle.host.cpu_count} cores · load {(latest?.load_avg_1 ?? 0).toFixed(2)}"
		data={cpuSeries}
		color={cpuColor}
	/>
	<BigMetric
		label="Memory"
		value="{(latest?.mem_percent ?? 0).toFixed(1)}%"
		sub="{fmtBytes(latest?.mem_used ?? 0)} / {fmtBytes(bundle.host.mem_total)}"
		data={memSeries}
		color={memColor}
	/>
	<BigMetric
		label="Network ↓"
		value={fmtRate(netInRate)}
		sub={(latest?.disk_percent ?? 0).toFixed(1) + '% disk · up ' + fmtDuration(latest?.uptime_secs ?? 0)}
		data={netInSeries}
		color="var(--info)"
	/>
	<BigMetric
		label="Temperature"
		value={maxTemp ? maxTemp.toFixed(1) + '°C' : '—'}
		sub={(latest?.temps?.length ?? 0) + ' sensor' + ((latest?.temps?.length ?? 0) === 1 ? '' : 's')}
		data={[]}
		color={tempColor}
	/>
</div>

{#if topProcs.length > 0}
	<section class="peek">
		<header class="peek-head">
			<span class="label-mono">Top processes by CPU</span>
		</header>
		<div class="peek-body">
			{#each topProcs as p}
				<div class="proc-row">
					<span class="proc-name">{p.name}</span>
					<span class="proc-pid mono">{p.pid}</span>
					<span class="proc-cpu mono">{p.cpu_pct.toFixed(1)}%</span>
					<span class="proc-mem mono">{fmtBytes(p.mem_rss)}</span>
				</div>
			{/each}
		</div>
	</section>
{/if}

<style>
	.alert-banner {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 14px;
		background: var(--warn-soft);
		border: 1px solid var(--warn);
		border-radius: var(--r-md);
		color: var(--warn);
		font-size: 13px;
		margin-bottom: 14px;
	}
	.alert-text { flex: 1; }
	.alert-link { color: var(--warn); font-family: var(--font-mono); font-size: 11px; text-decoration: none; }
	.alert-link:hover { text-decoration: underline; }

	.big-metrics {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 12px;
	}

	.peek {
		margin-top: 16px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 14px 16px;
	}

	.peek-head { margin-bottom: 10px; }

	.label-mono {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		font-family: var(--font-mono);
	}

	.peek-body { display: flex; flex-direction: column; gap: 4px; }

	.proc-row {
		display: grid;
		grid-template-columns: 1fr auto auto auto;
		gap: 12px;
		font-size: 12px;
		padding: 4px 0;
		align-items: center;
	}
	.proc-name { color: var(--text); }
	.proc-pid { color: var(--text-faint); font-size: 11px; }
	.proc-cpu { color: var(--text); }
	.proc-mem { color: var(--text-dim); }
</style>
