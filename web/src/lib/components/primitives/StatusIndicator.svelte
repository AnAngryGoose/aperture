<script lang="ts">
	import type { HostStatus } from '$lib/stores/hosts';

	interface Props {
		status: HostStatus;
		size?: number;
	}

	let { status, size = 8 }: Props = $props();

	const color = $derived(
		status === 'ok' ? 'var(--ok)' :
		status === 'warn' ? 'var(--warn)' :
		status === 'crit' ? 'var(--crit)' :
		'var(--offline)'
	);
</script>

<span
	class="dot"
	class:pulse-crit={status === 'crit'}
	style="width:{size}px; height:{size}px; background:{color};"
	aria-label={status}
></span>

<style>
	.dot {
		display: inline-block;
		border-radius: var(--r-pill);
		flex-shrink: 0;
	}
</style>
