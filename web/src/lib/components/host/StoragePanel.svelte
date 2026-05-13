<script lang="ts">
	import type { MetricSample } from '$lib/types';
	import Meter from '$lib/components/primitives/Meter.svelte';
	import { fmtBytes } from '$lib/format';

	interface Props {
		sample: MetricSample | null;
	}

	let { sample }: Props = $props();
</script>

<div class="panel">
	<div class="panel-head label-mono">Storage</div>
	{#if sample?.disk_mounts && sample.disk_mounts.length > 0}
		{#each sample.disk_mounts as mount}
			<div class="mount-row">
				<div class="mount-info">
					<span class="mount-name mono">{mount.mount}</span>
					<span class="mount-size mono text-faint">
						{fmtBytes(mount.used)} / {fmtBytes(mount.total)}
					</span>
				</div>
				<Meter value={mount.percent} max={100} />
			</div>
		{/each}
	{:else if sample}
		<div class="mount-row">
			<div class="mount-info">
				<span class="mount-name mono">/</span>
				<span class="mount-size mono text-faint">
					{fmtBytes(sample.disk_used ?? 0)} / {fmtBytes(sample.disk_total ?? 0)}
				</span>
			</div>
			<Meter value={sample.disk_pct ?? 0} max={100} />
		</div>
	{:else}
		<span class="empty text-faint">No data</span>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.panel-head { color: var(--text-dim); margin-bottom: 4px; }

	.mount-row {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.mount-info {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}

	.mount-name { font-size: 12px; color: var(--text); }
	.mount-size { font-size: 11px; }

	.empty { font-size: 12px; }
</style>
