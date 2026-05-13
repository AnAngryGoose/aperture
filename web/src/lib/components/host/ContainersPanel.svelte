<script lang="ts">
	import type { Container } from '$lib/types';

	interface Props {
		containers: Container[];
		loading?: boolean;
	}

	let { containers, loading = false }: Props = $props();

	const running = $derived(containers.filter((c) => c.state === 'running').length);
	const stopped = $derived(containers.filter((c) => c.state === 'exited').length);
	const unhealthy = $derived(containers.filter((c) => c.health === 'unhealthy').length);

	const topByCpu = $derived(
		[...containers]
			.filter((c) => c.state === 'running')
			.sort((a, b) => (b.cpu_pct ?? 0) - (a.cpu_pct ?? 0))
			.slice(0, 4)
	);
</script>

<div class="panel">
	<div class="panel-head label-mono">Containers</div>
	{#if loading}
		<span class="text-faint" style="font-size:12px">Loading…</span>
	{:else}
		<div class="stats">
			<div class="stat">
				<span class="val mono" style="color:var(--ok)">{running}</span>
				<span class="lbl">Running</span>
			</div>
			<div class="stat">
				<span class="val mono text-faint">{stopped}</span>
				<span class="lbl">Stopped</span>
			</div>
			<div class="stat">
				<span class="val mono" style="color:{unhealthy > 0 ? 'var(--crit)' : 'var(--text-dim)'}">{unhealthy}</span>
				<span class="lbl">Unhealthy</span>
			</div>
		</div>
		{#if topByCpu.length > 0}
			<div class="top-head label-mono" style="margin-top:8px;">Top by CPU</div>
			{#each topByCpu as c}
				<div class="top-row mono">
					<span class="top-name">{c.name}</span>
					<span class="top-cpu text-faint">{(c.cpu_pct ?? 0).toFixed(1)}%</span>
					<span class="top-mem text-faint">{((c.mem_usage ?? 0) / 1e9).toFixed(1)} GB</span>
				</div>
			{/each}
		{/if}
	{/if}
</div>

<style>
	.panel { display: flex; flex-direction: column; gap: 8px; }
	.panel-head { color: var(--text-dim); margin-bottom: 4px; }

	.stats {
		display: grid;
		grid-template-columns: 1fr 1fr 1fr;
		gap: 4px;
	}

	.stat {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
	}

	.val {
		font-size: 22px;
		font-weight: 500;
		letter-spacing: -0.02em;
		line-height: 1;
	}

	.lbl { font-size: 11px; color: var(--text-faint); }

	.top-head { color: var(--text-dim); margin-top: 4px; }

	.top-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 8px;
		font-size: 12px;
		padding: 2px 0;
	}

	.top-name {
		color: var(--text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
