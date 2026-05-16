<script lang="ts">
	import type { HostEntry } from '$lib/stores/monitoring.svelte';
	import { dashboardLayout } from '$lib/stores/dashboardLayout.svelte';
	import { SCALAR_METRICS, DEFAULT_WIDGETS, metricsByCategory } from '$lib/monitoring/metricCatalog';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		entry: HostEntry;
		onclose: () => void;
	}

	let { entry, onclose }: Props = $props();

	// Working copy of the host's widget selection. Falls back to defaults
	// when the host hasn't been customized. 2-4 selections allowed.
	let selected = $state<string[]>(
		dashboardLayout.getCardWidgets(entry.host.id) ?? [...DEFAULT_WIDGETS]
	);

	const groups = metricsByCategory();
	const groupOrder: Array<keyof typeof groups> = ['cpu', 'mem', 'disk', 'net', 'load', 'temp', 'misc'];
	const groupLabel: Record<string, string> = {
		cpu: 'CPU',
		mem: 'Memory',
		disk: 'Disk',
		net: 'Network',
		load: 'Load',
		temp: 'Temperature',
		misc: 'Other'
	};

	function toggle(key: string) {
		if (selected.includes(key)) {
			if (selected.length <= 1) return; // keep at least one
			selected = selected.filter((k) => k !== key);
		} else {
			if (selected.length >= 4) {
				// Replace the least-recently-added (first) when at the cap.
				selected = [...selected.slice(1), key];
			} else {
				selected = [...selected, key];
			}
		}
	}

	function save() {
		dashboardLayout.setCardWidgets(entry.host.id, selected);
		onclose();
	}

	function reset() {
		selected = [...DEFAULT_WIDGETS];
	}

	function onBackdrop(e: MouseEvent) {
		if (e.currentTarget === e.target) onclose();
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onclose()} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onBackdrop}>
	<div class="modal">
		<header class="modal-head">
			<div>
				<h2>Configure card widgets</h2>
				<p class="sub mono">{entry.host.name}</p>
			</div>
			<button class="close" onclick={onclose} aria-label="Close">
				<Icon name="x" size={14} />
			</button>
		</header>

		<div class="modal-body">
			<p class="instr">Pick 2–4 metrics to surface on this host's card.</p>

			{#each groupOrder as g}
				{#if groups[g].length > 0}
					<section class="group">
						<div class="label-mono">{groupLabel[g]}</div>
						<div class="picks">
							{#each groups[g] as m}
								{@const on = selected.includes(m.key)}
								<button
									type="button"
									class="pick"
									class:on
									onclick={() => toggle(m.key)}
									aria-pressed={on}
								>
									<span class="dot" style="background:{on ? m.color : 'var(--bg-hover)'}"></span>
									<div class="info">
										<span class="name">{m.label}</span>
										<span class="desc mono">{m.unit || ''}</span>
									</div>
								</button>
							{/each}
						</div>
					</section>
				{/if}
			{/each}
		</div>

		<footer class="modal-foot">
			<div class="count mono">
				{selected.length} of 4 selected
			</div>
			<div class="actions">
				<button class="btn ghost" onclick={reset}>Reset to defaults</button>
				<button class="btn ghost" onclick={onclose}>Cancel</button>
				<button class="btn primary" onclick={save} disabled={selected.length === 0}>
					Save
				</button>
			</div>
		</footer>
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 90;
		background: rgba(0, 0, 0, 0.55);
		backdrop-filter: blur(6px) saturate(1.2);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 40px 20px;
	}

	.modal {
		width: min(640px, 100%);
		max-height: 100%;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		box-shadow: 0 24px 60px -20px rgba(0, 0, 0, 0.5);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	@media (prefers-reduced-motion: no-preference) {
		.modal { animation: scale-in var(--dur-modal) var(--ease-card) both; }
		@keyframes scale-in {
			from { opacity: 0; transform: scale(0.97); }
			to   { opacity: 1; transform: scale(1); }
		}
	}

	.modal-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		padding: 18px 20px 12px;
		border-bottom: 1px solid var(--line);
	}

	.modal-head h2 { margin: 0; font-size: 16px; font-weight: 600; color: var(--text); letter-spacing: -0.01em; }
	.sub { margin: 4px 0 0; font-size: 11px; color: var(--text-faint); }

	.close {
		width: 28px; height: 28px;
		display: flex; align-items: center; justify-content: center;
		background: none; border: none; cursor: pointer;
		color: var(--text-faint); border-radius: var(--r-sm);
	}
	.close:hover { background: var(--bg-hover); color: var(--text); }

	.modal-body {
		flex: 1;
		overflow-y: auto;
		padding: 14px 20px;
	}

	.instr { font-size: 12px; color: var(--text-dim); margin: 0 0 14px; }

	.group { margin-bottom: 14px; }

	.label-mono {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		font-family: var(--font-mono);
		margin-bottom: 6px;
	}

	.picks {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: 6px;
	}

	.pick {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 10px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		cursor: pointer;
		text-align: left;
		transition: background 120ms, border-color 120ms;
	}

	.pick:hover { background: var(--bg-hover); border-color: var(--line-strong); }
	.pick.on {
		background: var(--accent-soft);
		border-color: var(--accent-line);
	}

	.dot {
		width: 10px; height: 10px;
		border-radius: var(--r-sm);
		flex-shrink: 0;
	}

	.info { display: flex; flex-direction: column; gap: 1px; min-width: 0; }
	.name { font-size: 12px; color: var(--text); }
	.desc { font-size: 10px; color: var(--text-faint); }

	.modal-foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 12px 20px;
		border-top: 1px solid var(--line);
	}

	.count { font-size: 11px; color: var(--text-faint); }
	.actions { display: flex; gap: 8px; }

	.btn {
		padding: 7px 14px;
		font-size: 12px;
		border-radius: var(--r-md);
		cursor: pointer;
		font-family: var(--font-sans);
		border: 1px solid transparent;
	}
	.btn.ghost { background: transparent; color: var(--text-dim); border-color: var(--line); }
	.btn.ghost:hover { background: var(--bg-hover); color: var(--text); }
	.btn.primary { background: var(--accent); color: #fff; border-color: var(--accent); }
	.btn.primary:hover { filter: brightness(1.05); }
	.btn.primary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
