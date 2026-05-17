<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import type { CardLayout } from '$lib/stores/dashboardLayout.svelte';
	import HostCard from './HostCard.svelte';
	import AddWidgetTile from './AddWidgetTile.svelte';
	import SkeletonCard from '$lib/components/primitives/SkeletonCard.svelte';
	import EmptyBlock from '$lib/components/primitives/EmptyBlock.svelte';
	import ErrorBlock from '$lib/components/primitives/ErrorBlock.svelte';

	interface Props {
		entries: HostEntry[];
		layout: CardLayout;
		loading?: boolean;
		error?: string | null;
		filter?: string;
		onselect?: (entry: HostEntry) => void;
		onaddhost?: () => void;
		onretry?: () => void;
	}

	let {
		entries,
		layout,
		loading = false,
		error = null,
		filter = 'all',
		onselect,
		onaddhost,
		onretry
	}: Props = $props();

	const filtered = $derived.by(() => {
		if (filter === 'all') return entries;
		if (filter === 'alerts') return entries.filter((e) => (e.host.open_alerts ?? 0) > 0);
		return entries.filter((e) => (e.host.tags ?? []).includes(filter));
	});

	const gridClass = $derived(
		layout === 'rich' ? 'grid-rich' :
		layout === 'tile' ? 'grid-tile' :
		'grid-list'
	);
</script>

{#if error}
	<ErrorBlock message={error} onretry={onretry} />
{:else if loading}
	<div class={gridClass}>
		{#each [1, 2, 3, 4] as _}
			<SkeletonCard height={layout === 'list' ? '60px' : '200px'} />
		{/each}
	</div>
{:else if filtered.length === 0}
	<EmptyBlock
		icon="hosts"
		title="No hosts yet"
		description="Add your first host to start monitoring."
		action="+ Add your first host"
		onaction={onaddhost}
	/>
{:else}
	<div class={gridClass}>
		{#each filtered as entry (entry.host.id)}
			<HostCard {entry} {layout} onclick={() => onselect?.(entry)} />
		{/each}
		<AddWidgetTile onclick={onaddhost} />
	</div>
{/if}

<style>
	.grid-rich {
		display: grid;
		/* Rich cards now have a 240px side column for containers + top-by-cpu
		   panels alongside the metric rows; bumped from 560px → 620px so the
		   metric column doesn't squeeze. */
		grid-template-columns: repeat(auto-fit, minmax(620px, 1fr));
		gap: 14px;
	}

	.grid-tile {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
		gap: 14px;
	}

	.grid-list {
		display: grid;
		grid-template-columns: 1fr;
		gap: 6px;
	}
</style>
