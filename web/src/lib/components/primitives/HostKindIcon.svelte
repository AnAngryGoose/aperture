<script lang="ts">
	import type { HostKind } from '$lib/stores/hosts';
	import Icon from './Icon.svelte';

	interface Props {
		kind: HostKind;
		size?: number;
	}

	let { kind, size = 14 }: Props = $props();
</script>

<span class="kind-chip">
	<Icon name={kind} {size} />
</span>

<style>
	.kind-chip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		flex-shrink: 0;
		color: var(--text-dim);
	}
</style>
