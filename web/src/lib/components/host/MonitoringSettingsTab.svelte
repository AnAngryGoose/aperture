<script lang="ts">
	import type { MonitoringBundle, HostConfig, MonitoringCatalog } from '$lib/types';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import Button from '$lib/components/primitives/Button.svelte';

	interface Props {
		bundle: MonitoringBundle;
		onsaved?: () => void;
	}

	let { bundle, onsaved }: Props = $props();

	// Snapshot the bundle's config into local mutable state. Form bindings
	// edit this copy; Save calls PUT /api/hosts/{id}/config which persists
	// + pushes to the running collector/agent.
	//
	// `bundle.config` arrives wrapped in Svelte 5's reactive Proxy because the
	// parent page declared it via $state. Passing a Proxy to structuredClone
	// throws DataCloneError, so we use $state.snapshot to deep-copy the plain
	// underlying data first.
	let cfg = $state<HostConfig>(structuredClone($state.snapshot(bundle.config)) as HostConfig);
	let saving = $state(false);
	let saveError = $state<string | null>(null);
	let saveOk = $state(false);
	let catalog = $state<MonitoringCatalog | null>(null);

	onMount(async () => {
		try {
			catalog = await api.monitoring.catalog();
		} catch {
			// Catalog is optional — form still works without it (just no family
			// labels, only raw keys).
		}
	});

	function toggleFamily(key: string) {
		if (cfg.enabled_families.includes(key)) {
			cfg.enabled_families = cfg.enabled_families.filter((f) => f !== key);
		} else {
			cfg.enabled_families = [...cfg.enabled_families, key];
		}
	}

	function isEnabled(key: string): boolean {
		return cfg.enabled_families.includes(key);
	}

	async function save() {
		saving = true;
		saveError = null;
		saveOk = false;
		try {
			const res = await api.hostConfig.put(bundle.host.id, cfg);
			if (res.warning) saveError = `Saved with warning: ${res.warning}`;
			saveOk = true;
			onsaved?.();
		} catch (e) {
			saveError = e instanceof Error ? e.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	function reset() {
		cfg = structuredClone($state.snapshot(bundle.config)) as HostConfig;
		saveOk = false;
		saveError = null;
	}

	const families = $derived(catalog?.families ?? [
		{ key: 'cpu', label: 'CPU' },
		{ key: 'cpu_per_core', label: 'Per-core CPU' },
		{ key: 'mem', label: 'Memory' },
		{ key: 'disk', label: 'Disk usage' },
		{ key: 'mounts', label: 'Mounts' },
		{ key: 'disk_io', label: 'Disk I/O' },
		{ key: 'net', label: 'Network' },
		{ key: 'load', label: 'Load average' },
		{ key: 'temps', label: 'Temperatures' },
		{ key: 'processes', label: 'Processes' },
		{ key: 'containers', label: 'Containers' }
	]);
</script>

<div class="tab">
	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Sample interval & retention</h3>
		</header>
		<div class="fields">
			<label class="field">
				<span class="field-label">Sample interval (seconds)</span>
				<input type="number" min="1" max="3600" bind:value={cfg.sample_interval_s} />
			</label>
			<label class="field">
				<span class="field-label">Retention (days)</span>
				<input type="number" min="1" max="3650" bind:value={cfg.retention_days} />
			</label>
			<label class="field">
				<span class="field-label">Memory calculation</span>
				<select bind:value={cfg.mem_calc}>
					<option value="used">used (default — total − cached − free)</option>
					<option value="avail">avail (uses MemAvailable; matches htop)</option>
				</select>
			</label>
		</div>
	</section>

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Enabled collectors</h3>
			<span class="card-sub mono">{cfg.enabled_families.length} of {families.length}</span>
		</header>
		<div class="family-grid">
			{#each families as f}
				<label class="family">
					<input type="checkbox" checked={isEnabled(f.key)} onchange={() => toggleFamily(f.key)} />
					<span class="family-label">
						{f.label}
						{#if f.experimental}<span class="experimental">experimental</span>{/if}
					</span>
				</label>
			{/each}
		</div>
	</section>

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Status thresholds</h3>
			<span class="card-sub mono">cards flip warn/crit when these are crossed</span>
		</header>
		<div class="thresh-grid">
			<div class="thresh-row">
				<span class="thresh-name">CPU %</span>
				<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={cfg.warn_cpu} /></label>
				<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={cfg.crit_cpu} /></label>
			</div>
			<div class="thresh-row">
				<span class="thresh-name">Memory %</span>
				<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={cfg.warn_mem} /></label>
				<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={cfg.crit_mem} /></label>
			</div>
			<div class="thresh-row">
				<span class="thresh-name">Disk %</span>
				<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={cfg.warn_disk} /></label>
				<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={cfg.crit_disk} /></label>
			</div>
			<div class="thresh-row">
				<span class="thresh-name">Temperature °C</span>
				<label class="thresh-input"><span class="thresh-cap">warn ≥</span><input type="number" min="0" max="200" bind:value={cfg.warn_temp} /></label>
				<label class="thresh-input"><span class="thresh-cap">crit ≥</span><input type="number" min="0" max="200" bind:value={cfg.crit_temp} /></label>
			</div>
		</div>
	</section>

	<section class="card">
		<header class="card-head">
			<h3 class="card-title">Filters</h3>
			<span class="card-sub mono">comma-separated lists; deny wins over allow</span>
		</header>
		<div class="fields">
			<label class="field">
				<span class="field-label">NIC allow</span>
				<input
					type="text"
					placeholder="eth0, wlan0"
					value={(cfg.filters.nic_allow ?? []).join(', ')}
					oninput={(e) => (cfg.filters.nic_allow = (e.currentTarget as HTMLInputElement).value.split(',').map((s) => s.trim()).filter(Boolean))}
				/>
			</label>
			<label class="field">
				<span class="field-label">NIC deny</span>
				<input
					type="text"
					placeholder="docker0, br-*"
					value={(cfg.filters.nic_deny ?? []).join(', ')}
					oninput={(e) => (cfg.filters.nic_deny = (e.currentTarget as HTMLInputElement).value.split(',').map((s) => s.trim()).filter(Boolean))}
				/>
			</label>
			<label class="field">
				<span class="field-label">Sensor deny</span>
				<input
					type="text"
					placeholder="acpitz"
					value={(cfg.filters.sensor_deny ?? []).join(', ')}
					oninput={(e) => (cfg.filters.sensor_deny = (e.currentTarget as HTMLInputElement).value.split(',').map((s) => s.trim()).filter(Boolean))}
				/>
			</label>
			<label class="field">
				<span class="field-label">Mount deny</span>
				<input
					type="text"
					placeholder="/mnt/temp"
					value={(cfg.filters.mount_deny ?? []).join(', ')}
					oninput={(e) => (cfg.filters.mount_deny = (e.currentTarget as HTMLInputElement).value.split(',').map((s) => s.trim()).filter(Boolean))}
				/>
			</label>
		</div>
	</section>

	<div class="bar">
		{#if saveOk}<span class="ok mono">saved ✓</span>{/if}
		{#if saveError}<span class="err">{saveError}</span>{/if}
		<Button variant="ghost" onclick={reset} disabled={saving}>Reset</Button>
		<Button variant="primary" onclick={save} loading={saving}>
			{saving ? 'Saving…' : 'Save defaults'}
		</Button>
	</div>
</div>

<style>
	.tab { display: flex; flex-direction: column; gap: 14px; }
	.card {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		padding: 14px 16px;
	}
	.card-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 12px;
	}
	.card-title { margin: 0; font-size: 14px; font-weight: 600; color: var(--text); }
	.card-sub { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); }

	.fields {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 12px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-label {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		font-family: var(--font-mono);
	}

	.field input, .field select {
		padding: 7px 10px;
		font-size: 12px;
		font-family: var(--font-mono);
		color: var(--text);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}

	.field input:focus, .field select:focus {
		outline: none;
		border-color: var(--accent-line);
	}

	.family-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
		gap: 8px 14px;
	}

	.family {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		cursor: pointer;
		padding: 4px 0;
	}

	.family-label {
		font-size: 13px;
		color: var(--text);
		display: inline-flex;
		align-items: center;
		gap: 8px;
	}

	.experimental {
		font-size: 9px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		padding: 1px 6px;
		background: var(--warn-soft);
		color: var(--warn);
		border-radius: var(--r-sm);
		font-family: var(--font-mono);
	}

	.thresh-grid { display: flex; flex-direction: column; gap: 8px; }

	.thresh-row {
		display: grid;
		grid-template-columns: 1fr auto auto;
		gap: 14px;
		align-items: center;
	}

	.thresh-name { font-size: 13px; color: var(--text); }

	.thresh-input {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}

	.thresh-cap {
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-family: var(--font-mono);
		color: var(--text-faint);
	}

	.thresh-input input {
		width: 70px;
		padding: 5px 8px;
		font-size: 12px;
		font-family: var(--font-mono);
		color: var(--text);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		text-align: right;
	}

	.bar {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
		padding: 8px 0;
	}

	.ok { color: var(--ok); font-size: 11px; font-family: var(--font-mono); }
	.err { color: var(--crit); font-size: 11px; }
</style>
