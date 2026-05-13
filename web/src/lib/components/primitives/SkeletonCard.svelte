<script lang="ts">
	interface Props {
		height?: string;
	}
	let { height = '200px' }: Props = $props();
</script>

<div class="skel-card card" style="height: {height};">
	<div class="skel-head">
		<div class="skeleton skel-circle"></div>
		<div class="skel-lines">
			<div class="skeleton skel-line wide"></div>
			<div class="skeleton skel-line narrow"></div>
		</div>
	</div>
	<div class="skel-body">
		<div class="skeleton skel-block"></div>
		<div class="skeleton skel-block short"></div>
	</div>
</div>

<style>
	.skel-card {
		padding: 16px;
		overflow: hidden;
	}

	.skel-head {
		display: flex;
		gap: 10px;
		align-items: center;
		margin-bottom: 16px;
	}

	.skel-circle {
		width: 32px;
		height: 32px;
		border-radius: var(--r-md);
		flex-shrink: 0;
	}

	.skel-lines {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.skel-line {
		height: 12px;
		border-radius: var(--r-sm);
	}

	.wide { width: 60%; }
	.narrow { width: 35%; }

	.skel-body {
		display: flex;
		flex-direction: column;
		gap: 10px;
	}

	.skel-block {
		height: 48px;
		border-radius: var(--r-md);
	}

	.skel-block.short { height: 32px; width: 70%; }
</style>
