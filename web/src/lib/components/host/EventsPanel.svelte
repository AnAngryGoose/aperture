<script lang="ts">
	import type { AlertEvent } from '$lib/types';
	import { fmtRelative } from '$lib/format';

	interface Props {
		events: AlertEvent[];
		/**
		 * Optional — accepted for API symmetry with the dashboard drawer's
		 * usage. The drawer wraps this whole panel in a link, so rows here
		 * stay non-interactive to avoid nested anchors.
		 */
		hostId?: string;
	}

	let { events }: Props = $props();

	const recent = $derived(events.slice(0, 8));
</script>

<div class="panel">
	<div class="panel-head label-mono">Recent Events</div>
	{#if recent.length === 0}
		<span class="text-faint" style="font-size:12px">No recent events</span>
	{:else}
		{#each recent as ev}
			<div class="event-row">
				<span class="ev-time mono text-faint">{fmtRelative(ev.fired_at)}</span>
				<span class="ev-text" class:resolved={!!ev.resolved_at}>
					{ev.resolved_at ? 'Resolved' : 'Firing'} — rule #{ev.rule_id}
					{#if ev.value !== undefined}
						({ev.value.toFixed(1)})
					{/if}
				</span>
			</div>
		{/each}
	{/if}
</div>

<style>
	.panel { display: flex; flex-direction: column; gap: 6px; }
	.panel-head { color: var(--text-dim); margin-bottom: 4px; }

	.event-row {
		display: grid;
		grid-template-columns: 48px 1fr;
		gap: 8px;
		align-items: baseline;
		padding: 2px 0;
	}

	.ev-time { font-size: 11px; }

	.ev-text {
		font-size: 12px;
		color: var(--warn);
	}

	.ev-text.resolved { color: var(--ok); }
</style>
