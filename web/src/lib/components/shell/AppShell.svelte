<script lang="ts">
	import type { Snippet } from 'svelte';
	import Sidebar from './Sidebar.svelte';
	import Topbar from './Topbar.svelte';
	import { theme } from '$lib/stores/theme';
	import { accent } from '$lib/stores/accent';
	import { onMount } from 'svelte';

	interface Props {
		children: Snippet;
		onrefresh?: () => void;
	}

	let { children, onrefresh }: Props = $props();

	onMount(() => {
		theme.init();
		accent.init();
	});
</script>

<div class="shell">
	<Sidebar />
	<div class="main-col">
		<Topbar {onrefresh} />
		<main class="content">
			{@render children()}
		</main>
	</div>
</div>

<style>
	.shell {
		display: grid;
		grid-template-columns: 220px 1fr;
		min-height: 100vh;
	}

	.main-col {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.content {
		flex: 1;
	}
</style>
