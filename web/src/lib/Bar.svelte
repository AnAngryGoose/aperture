<script lang="ts">
	let { value, warn = 75, bad = 90 }: { value: number; warn?: number; bad?: number } = $props();
	let level = $derived(value >= bad ? 'bad' : value >= warn ? 'warn' : '');
	let pct = $derived(Math.max(0, Math.min(100, value)));
</script>

<div class="bar {level}">
	<div class="fill" style="width: {pct}%"></div>
</div>
