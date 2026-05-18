<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';

	// The compose UI moved to /stacks. Keep this route as a redirect so any
	// stale links or bookmarks from before the rename still resolve.
	onMount(() => {
		const id = page.params.id;
		if (id) goto(`/hosts/${id}/stacks`, { replaceState: true });
	});
</script>

<div class="redirect-stub">Redirecting to Stacks…</div>

<style>
	.redirect-stub {
		padding: 40px;
		text-align: center;
		color: var(--text-faint);
		font-size: 13px;
	}
</style>
