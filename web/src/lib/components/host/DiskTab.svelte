<script lang="ts">
	import type { MonitoringBundle } from '$lib/types';
	import { fmtBytes, fmtRate } from '$lib/format';
	import Chart from '$lib/Chart.svelte';

	interface Props {
		bundle: MonitoringBundle;
	}

	let { bundle }: Props = $props();

	const latest = $derived(bundle.latest);
	const mounts = $derived(latest?.disk_mounts ?? []);
	const diskIO = $derived(latest?.disk_io ?? []);
	const mountsHistory = $derived(bundle.history.mounts);
	const diskIOHistory = $derived(bundle.history.diskio);
	const config = $derived(bundle.config);

	function pctColor(pct: number): string {
		if (pct >= config.crit_disk) return 'var(--crit)';
		if (pct >= config.warn_disk) return 'var(--warn)';
		return 'var(--accent)';
	}

	function fmtGiB(bytes: number): string {
		const g = bytes / 1073741824;
		return g >= 10 ? `${g.toFixed(1)} GiB` : `${g.toFixed(2)} GiB`;
	}

	function fmtBytesRate(bps: number): string {
		if (bps < 1024) return `${bps.toFixed(0)} B/s`;
		if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
		return `${(bps / 1048576).toFixed(1)} MiB/s`;
	}

	function deriveRates(timestamps: number[], bytes: number[]): number[] {
		return bytes.map((b, i) => {
			if (i === 0) return 0;
			const dt = timestamps[i] - timestamps[i - 1];
			if (dt <= 0) return 0;
			return Math.max(0, (b - bytes[i - 1]) / dt);
		});
	}

	// Per-mount history → one chart card per mount with used vs total.
	const mountCharts = $derived.by(() => {
		if (!mountsHistory) return [];
		return Object.entries(mountsHistory.mounts ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([mount, series]) => ({
				mount,
				x: mountsHistory.timestamps,
				series: [
					{ label: 'used',  values: series.used.map((v) => v / 1073741824),  stroke: 'var(--accent)' },
					{ label: 'total', values: series.total.map((v) => v / 1073741824), stroke: 'var(--text-faint)', fill: false as const }
				]
			}));
	});

	// Per-device I/O — all devices on one chart, color-coded.
	const devColors = ['#5cc8ff', '#7ce38b', '#ffcb6b', '#c792ea', '#ff6b6b', '#f78c6c'];
	const diskIOChart = $derived.by(() => {
		if (!diskIOHistory || !diskIOHistory.timestamps.length) {
			return { x: [] as number[], series: [] as Array<{label: string; values: number[]; stroke: string}> };
		}
		const x = diskIOHistory.timestamps;
		const series: Array<{label: string; values: number[]; stroke: string}> = [];
		Object.entries(diskIOHistory.devices ?? {})
			.sort(([a], [b]) => a.localeCompare(b))
			.forEach(([device, s], i) => {
				const col = devColors[i % devColors.length];
				series.push({ label: `${device} read`,  values: deriveRates(x, s.read_bytes),  stroke: col });
				series.push({ label: `${device} write`, values: deriveRates(x, s.write_bytes), stroke: col + '99' });
			});
		return { x, series };
	});
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Mounts</h3>
			<span class="card-sub mono">{mounts.length} mounted</span>
		</header>
		{#if mounts.length === 0}
			<div class="empty">No mounts reported.</div>
		{:else}
			<table>
				<thead>
					<tr>
						<th>Mount</th>
						<th>Device</th>
						<th>FS</th>
						<th class="num">Used</th>
						<th class="num">Total</th>
						<th>Usage</th>
					</tr>
				</thead>
				<tbody>
					{#each mounts as m}
						<tr>
							<td class="mono">{m.mount}</td>
							<td class="mono dim">{m.device}</td>
							<td class="mono dim">{m.fstype}</td>
							<td class="num mono">{fmtBytes(m.used)}</td>
							<td class="num mono">{fmtBytes(m.total)}</td>
							<td>
								<div class="meter">
									<div class="meter-fill" style="width:{m.percent}%; background:{pctColor(m.percent)}"></div>
								</div>
								<span class="meter-label mono" style="color:{pctColor(m.percent)}">{m.percent.toFixed(1)}%</span>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</section>

	{#if diskIO.length > 0}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Disk I/O (live)</h3>
				<span class="card-sub mono">{diskIO.length} devices</span>
			</header>
			<table>
				<thead>
					<tr>
						<th>Device</th>
						<th class="num">Read rate</th>
						<th class="num">Write rate</th>
						<th class="num">Read total</th>
						<th class="num">Write total</th>
					</tr>
				</thead>
				<tbody>
					{#each diskIO as d}
						<tr>
							<td class="mono">{d.device}</td>
							<td class="num mono">{fmtBytesRate(d.read_rate)}</td>
							<td class="num mono">{fmtBytesRate(d.write_rate)}</td>
							<td class="num mono dim">{fmtBytes(d.read_bytes)}</td>
							<td class="num mono dim">{fmtBytes(d.write_bytes)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</section>
	{/if}

	{#if diskIOChart.x.length > 0}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title">Disk I/O history</h3>
			</header>
			<Chart x={diskIOChart.x} series={diskIOChart.series} height={200} valueFormatter={fmtBytesRate} />
		</section>
	{/if}

	{#each mountCharts as mc}
		<section class="card">
			<header class="card-head">
				<h3 class="card-title mono">{mc.mount}</h3>
			</header>
			<Chart x={mc.x} series={mc.series} height={150} valueFormatter={fmtGiB} />
		</section>
	{/each}
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

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
	}

	th {
		text-align: left;
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-family: var(--font-mono);
		color: var(--text-faint);
		font-weight: 400;
		padding: 6px 10px 8px;
		border-bottom: 1px solid var(--line);
	}

	th.num, td.num { text-align: right; }

	td {
		padding: 8px 10px;
		border-bottom: 1px solid var(--line);
		color: var(--text);
	}

	tr:last-child td { border-bottom: none; }

	td.mono { font-family: var(--font-mono); }
	td.dim { color: var(--text-dim); }

	.meter {
		display: inline-block;
		width: 100px;
		height: 6px;
		background: var(--bg-elev-2);
		border-radius: var(--r-pill);
		overflow: hidden;
		vertical-align: middle;
	}
	.meter-fill { height: 100%; transition: width 200ms, background 200ms; }
	.meter-label { font-size: 11px; margin-left: 8px; vertical-align: middle; }
</style>
