<script lang="ts">
	import type { Snippet } from 'svelte';
	import Sidebar from './Sidebar.svelte';
	import Topbar from './Topbar.svelte';

	interface Props {
		children: Snippet;
		onrefresh?: () => void;
	}

	let { children, onrefresh }: Props = $props();
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
		padding: 22px 28px 60px;
		max-width: 1600px;
		width: 100%;
		margin: 0 auto;
	}
</style>
