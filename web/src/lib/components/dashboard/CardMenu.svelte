<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';

	interface Props {
		entry: HostEntry;
		onclose: () => void;
		onconfigure?: () => void;
	}

	let { entry, onclose, onconfigure }: Props = $props();

	function stop(e: MouseEvent) { e.stopPropagation(); }
	function configure(e: MouseEvent) {
		stop(e);
		onconfigure?.();
	}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="menu" onclick={stop}>
	<button class="item" onclick={configure}>Configure widget…</button>
	<button class="item" onclick={onclose}>Pin to dashboard</button>
	<a class="item" href="/hosts/{entry.host.id}" onclick={onclose}>Open full monitoring →</a>
	<div class="sep"></div>
	<button class="item" onclick={onclose}>Open shell</button>
	<button class="item" onclick={onclose}>Restart host</button>
	<div class="sep"></div>
	<button class="item danger" onclick={onclose}>Remove host</button>
</div>

<style>
	.menu {
		position: absolute;
		right: 0;
		top: calc(100% + 4px);
		z-index: 60;
		min-width: 200px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		box-shadow: 0 18px 40px -16px rgba(0,0,0,.4);
		backdrop-filter: blur(14px) saturate(1.2);
		padding: 4px;
		display: flex;
		flex-direction: column;
		animation: menu-in var(--dur-menu) ease-out both;
	}

	@keyframes menu-in {
		from { opacity: 0; transform: translateY(-4px); }
		to   { opacity: 1; transform: translateY(0); }
	}

	.item {
		text-align: left;
		padding: 7px 10px;
		font-size: 13px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: none;
		border: none;
		border-radius: var(--r-md);
		cursor: pointer;
		transition: background 100ms, color 100ms;
	}

	.item:hover { background: var(--bg-hover); color: var(--text); }

	.item.danger { color: var(--crit); }
	.item.danger:hover { background: var(--crit-soft); }

	.sep {
		height: 1px;
		background: var(--line);
		margin: 4px 0;
	}
</style>
