<script lang="ts">
	import { dashboardLayout, type CardLayout } from '$lib/stores/dashboardLayout';
	import { hostStore } from '$lib/stores/hosts';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		onaddhost?: () => void;
	}

	let { onaddhost }: Props = $props();

	// Build tag list from all hosts.
	const allTags = $derived.by(() => {
		const tags = new Set<string>();
		for (const e of Object.values(hostStore.entries)) {
			for (const t of e.host.tags ?? []) tags.add(t);
		}
		return Array.from(tags).sort();
	});

	const hasAlerts = $derived(
		Object.values(hostStore.entries).some((e) => (e.host.open_alerts ?? 0) > 0)
	);

	const filters = $derived([
		{ id: 'all', label: 'all' },
		...allTags.map((t) => ({ id: t, label: t })),
		...(hasAlerts ? [{ id: 'alerts', label: '⚠ alerts' }] : [])
	]);

	const LAYOUTS: { id: CardLayout; icon: string; label: string }[] = [
		{ id: 'rich', icon: 'grid', label: 'Rich' },
		{ id: 'tile', icon: 'rows', label: 'Tile' },
		{ id: 'list', icon: 'list', label: 'List' }
	];
</script>

<div class="filter-bar">
	<!-- Tag filter tabs -->
	<div class="tabs">
		{#each filters as f}
			<button
				class="tab"
				class:active={dashboardLayout.activeFilter === f.id}
				onclick={() => dashboardLayout.setFilter(f.id)}
			>
				{f.label}
			</button>
		{/each}
	</div>

	<!-- Right controls -->
	<div class="right">
		<div class="segmented">
			{#each LAYOUTS as l}
				<button
					class:active={dashboardLayout.cardLayout === l.id}
					onclick={() => dashboardLayout.setCardLayout(l.id)}
					title={l.label}
				>
					<Icon name={l.icon} size={13} />
					{l.label}
				</button>
			{/each}
		</div>

		<button class="add-btn" onclick={onaddhost}>
			<Icon name="plus" size={14} />
			Add host
		</button>
	</div>
</div>

<style>
	.filter-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}

	.tabs {
		display: flex;
		gap: 2px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 3px;
	}

	.tab {
		padding: 5px 12px;
		font-size: 12px;
		font-family: var(--font-sans);
		color: var(--text-dim);
		background: transparent;
		border: none;
		border-radius: var(--r-sm);
		cursor: pointer;
		transition: background 120ms, color 120ms;
		white-space: nowrap;
	}

	.tab:hover { background: var(--bg-hover); color: var(--text); }
	.tab.active { background: var(--bg-hover); color: var(--text); }

	.right {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.add-btn {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 7px 14px;
		font-size: 13px;
		font-weight: 500;
		font-family: var(--font-sans);
		color: #fff;
		border: none;
		border-radius: var(--r-md);
		cursor: pointer;
		background-image: linear-gradient(
			180deg,
			color-mix(in srgb, var(--accent) 100%, white 8%),
			var(--accent)
		);
		box-shadow: 0 1px 0 rgba(255,255,255,.18) inset, 0 4px 16px -8px var(--accent);
		transition: filter 120ms;
	}

	.add-btn:hover { filter: brightness(1.08); }
</style>
