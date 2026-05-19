<script lang="ts">
	import { page } from '$app/state';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
</script>

<section class="shell-stub">
	<div class="icon"><Icon name="shell" size={32} /></div>
	<h2>Host shell coming soon</h2>
	<p>
		Container-level shells are available from the
		<a href={`/hosts/${id}/containers`}>Containers</a> tab.
	</p>
</section>

<style>
	.shell-stub {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 50vh;
		gap: 10px;
		color: var(--text-faint);
		text-align: center;
	}
	.icon { opacity: 0.4; }
	h2 { font-size: 16px; color: var(--text-dim); margin: 0; }
	p { font-size: 13px; margin: 0; }
	p a { color: var(--accent); }
</style>
