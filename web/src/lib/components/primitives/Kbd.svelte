<script lang="ts">
	interface Props {
		key: string;
	}
	let { key }: Props = $props();
</script>

<kbd>{key}</kbd>

<style>
	kbd {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		height: 18px;
		min-width: 20px;
		padding: 0 4px;
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-faint);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-sm);
	}
</style>
