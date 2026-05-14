<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import type { CardLayout } from '$lib/stores/dashboardLayout.svelte';
	import RichCard from './RichCard.svelte';
	import TileCard from './TileCard.svelte';
	import CompactRow from './CompactRow.svelte';

	interface Props {
		entry: HostEntry;
		layout: CardLayout;
		onclick?: () => void;
	}

	let { entry, layout, onclick }: Props = $props();
</script>

{#if layout === 'rich'}
	<RichCard {entry} {onclick} />
{:else if layout === 'tile'}
	<TileCard {entry} {onclick} />
{:else}
	<CompactRow {entry} {onclick} />
{/if}
