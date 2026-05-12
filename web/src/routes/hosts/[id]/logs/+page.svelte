<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container } from '$lib/types';

	let id = $derived(page.params.id);
	let hostName = $state('');
	let allContainers = $state<Container[]>([]);
	let selectedIDs = $state<string[]>([]);
	let tail = $state(200);
	let follow = $state(true);
	let showTimestamps = $state(true);
	let searchText = $state('');
	let lines = $state<LogLine[]>([]);
	let loading = $state(false);
	let polling = $state<ReturnType<typeof setInterval> | null>(null);
	let logEl: HTMLElement | undefined;

	// Track the latest log timestamp seen per container (ms precision for dedup).
	const lastSeenMs = new Map<string, number>();

	const COLORS = [
		'#61dafb', '#f7df1e', '#ff7c7c', '#7dff7d',
		'#c87cff', '#ffa07a', '#98fb98', '#dda0dd',
	];

	type LogLine = {
		key: string;
		cid: string;
		name: string;
		tsMs: number;
		tsStr: string;
		text: string;
		color: string;
	};

	let lineSeq = 0;

	function colorFor(cid: string): string {
		const idx = selectedIDs.indexOf(cid);
		return COLORS[idx < 0 ? 0 : idx % COLORS.length];
	}

	function stripAnsi(s: string): string {
		// Remove CSI sequences and a few common escape codes
		return s.replace(/\x1b\[[0-9;]*[mGKHFJABCDsu]/g, '').replace(/\x1b[()][0-9A-Z]/g, '');
	}

	// Docker log lines with timestamps look like:
	//   2024-01-15T14:30:01.123456789Z rest of line
	function parseLine(raw: string): { tsMs: number; tsStr: string; text: string } {
		const m = raw.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z)\s(.*)$/s);
		if (m) {
			const d = new Date(m[1]);
			const ms = d.getTime();
			const hms = d.toLocaleTimeString('en-GB', { hour12: false });
			const msPad = String(d.getMilliseconds()).padStart(3, '0');
			return { tsMs: ms, tsStr: `${hms}.${msPad}`, text: m[2] };
		}
		return { tsMs: 0, tsStr: '', text: raw };
	}

	async function fetchFor(cid: string, initial: boolean) {
		const c = allContainers.find((c) => c.id === cid);
		if (!c) return;

		const name = c.name.replace(/^\//, '');
		const color = colorFor(cid);
		const prevMs = lastSeenMs.get(cid) ?? 0;

		let raw: string;
		try {
			if (initial) {
				raw = await api.containerLogs(id, cid, { tail, timestamps: true });
			} else {
				if (!prevMs) return;
				// Fetch from 1 second before last seen to avoid missing sub-second logs;
				// dedup client-side by tsMs.
				const sinceUnix = Math.max(0, Math.floor(prevMs / 1000) - 1);
				raw = await api.containerLogs(id, cid, { since: sinceUnix, timestamps: true });
			}
		} catch {
			return;
		}

		if (!raw.trim()) return;

		const newLines: LogLine[] = [];
		for (const rawLine of raw.split('\n')) {
			const trimmed = rawLine.trimEnd();
			if (!trimmed) continue;
			const { tsMs, tsStr, text } = parseLine(trimmed);
			// Skip lines we've already displayed.
			if (tsMs > 0 && tsMs <= prevMs) continue;
			newLines.push({
				key: `${cid}-${lineSeq++}`,
				cid,
				name,
				tsMs,
				tsStr,
				text: stripAnsi(text),
				color,
			});
			if (tsMs > (lastSeenMs.get(cid) ?? 0)) {
				lastSeenMs.set(cid, tsMs);
			}
		}

		if (newLines.length > 0) {
			lines = [...lines, ...newLines].slice(-5000);
			if (follow) scheduleScroll();
		}
	}

	function scheduleScroll() {
		requestAnimationFrame(() => {
			if (logEl) logEl.scrollTop = logEl.scrollHeight;
		});
	}

	async function loadAll() {
		stopPolling();
		loading = true;
		lines = [];
		lastSeenMs.clear();
		lineSeq = 0;
		for (const cid of selectedIDs) {
			await fetchFor(cid, true);
		}
		loading = false;
		if (follow) scheduleScroll();
		startPolling();
	}

	async function pollAll() {
		for (const cid of selectedIDs) {
			await fetchFor(cid, false);
		}
	}

	function startPolling() {
		stopPolling();
		if (selectedIDs.length > 0) {
			polling = setInterval(pollAll, 2000);
		}
	}

	function stopPolling() {
		if (polling) { clearInterval(polling); polling = null; }
	}

	function toggleContainer(cid: string) {
		if (selectedIDs.includes(cid)) {
			selectedIDs = selectedIDs.filter((c) => c !== cid);
		} else {
			selectedIDs = [...selectedIDs, cid];
		}
	}

	function clearDisplay() {
		lines = [];
		lastSeenMs.clear();
		lineSeq = 0;
	}

	function exportLogs() {
		const multi = selectedIDs.length > 1;
		const text = filteredLines.map((l) => {
			const ts = l.tsStr && showTimestamps ? `[${l.tsStr}] ` : '';
			const prefix = multi ? `${l.name} | ` : '';
			return `${ts}${prefix}${l.text}`;
		}).join('\n');
		const blob = new Blob([text], { type: 'text/plain' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		const firstName = allContainers.find((c) => c.id === selectedIDs[0])?.name?.replace(/^\//, '') ?? 'logs';
		a.download = `${multi ? 'multi' : firstName}-${Date.now()}.txt`;
		a.click();
		URL.revokeObjectURL(url);
	}

	let filteredLines = $derived(
		searchText.trim()
			? lines.filter((l) => l.text.toLowerCase().includes(searchText.toLowerCase()))
			: lines
	);

	// Detect manual scroll-up to pause follow.
	function onScroll() {
		if (!logEl) return;
		const atBottom = logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 60;
		follow = atBottom;
	}

	onMount(async () => {
		try {
			const host = await api.host(id);
			hostName = host.name;
		} catch {}
		try {
			const cs = await api.containers(id, true);
			allContainers = cs;
			// Default: first running container.
			const first = cs.find((c) => c.state === 'running') ?? cs[0];
			if (first) selectedIDs = [first.id];
		} catch {}
		await loadAll();
	});

	onDestroy(stopPolling);
</script>

<svelte:head><title>Aperture — {hostName || id} — Logs</title></svelte:head>

<div class="page">
	<header class="page-header">
		<h1>Logs</h1>
		{#if hostName}<span class="host-name">{hostName}</span>{/if}
	</header>

	<nav class="subnav">
		<a href={`/hosts/${id}`}>Overview</a>
		<a href={`/hosts/${id}/containers`}>Containers</a>
		<a href={`/hosts/${id}/compose`}>Compose</a>
		<a href={`/hosts/${id}/networks`}>Networks</a>
		<a href={`/hosts/${id}/logs`} class="active">Logs</a>
		<a href={`/hosts/${id}/volumes`}>Volumes</a>
		<a href={`/hosts/${id}/images`}>Images</a>
	</nav>

	<!-- Toolbar -->
	<div class="toolbar card">
		<div class="row">
			<span class="label">Containers</span>
			<div class="picker">
				{#each allContainers as c (c.id)}
					<button
						class="pick-btn"
						class:selected={selectedIDs.includes(c.id)}
						onclick={() => toggleContainer(c.id)}
						title={c.image}
					>
						<span
							class="state-dot"
							style="background:{c.state === 'running' ? '#2ecc71' : '#888'}"
						></span>
						{c.name.replace(/^\//, '')}
					</button>
				{/each}
				{#if allContainers.length === 0}
					<span class="dim">No containers found</span>
				{/if}
			</div>
			<button class="btn-sm" onclick={loadAll} disabled={selectedIDs.length === 0 || loading}>
				{loading ? 'Loading…' : 'Reload'}
			</button>
		</div>

		<div class="row opts-row">
			<label class="opt">
				<span>Tail</span>
				<select bind:value={tail}>
					<option value={50}>50</option>
					<option value={200}>200</option>
					<option value={500}>500</option>
					<option value={1000}>1 000</option>
				</select>
			</label>

			<label class="opt toggle">
				<input type="checkbox" bind:checked={showTimestamps} />
				Timestamps
			</label>

			<label class="opt toggle">
				<input type="checkbox" bind:checked={follow} />
				Follow
			</label>

			<div class="flex-spacer"></div>

			<input class="search" type="search" placeholder="Filter lines…" bind:value={searchText} />
			<button class="btn-sm" onclick={clearDisplay} title="Clear display">Clear</button>
			<button class="btn-sm" onclick={exportLogs} disabled={lines.length === 0} title="Export as text">
				Export
			</button>
		</div>
	</div>

	<!-- Log pane -->
	<div class="log-wrap" bind:this={logEl} onscroll={onScroll}>
		{#if loading}
			<div class="empty">Loading…</div>
		{:else if selectedIDs.length === 0}
			<div class="empty">Select one or more containers above to view logs.</div>
		{:else if filteredLines.length === 0 && lines.length > 0}
			<div class="empty">No lines match "<em>{searchText}</em>"</div>
		{:else if filteredLines.length === 0}
			<div class="empty">No log output yet.</div>
		{:else}
			{#each filteredLines as line (line.key)}
				<div class="log-line">
					{#if selectedIDs.length > 1}
						<span class="ctag" style="color:{line.color}">{line.name}</span>
					{/if}
					{#if showTimestamps && line.tsStr}
						<span class="ts">{line.tsStr}</span>
					{/if}
					<span class="msg">{line.text}</span>
				</div>
			{/each}
		{/if}
	</div>

	<!-- Status bar -->
	<div class="statusbar">
		<span class="live-dot" class:active={polling !== null}></span>
		<span>
			{filteredLines.length}{searchText.trim() ? ` / ${lines.length}` : ''} lines
		</span>
		{#if polling}
			<span class="dim">· live (2s)</span>
		{/if}
		{#if selectedIDs.length > 1}
			<span class="dim">· {selectedIDs.length} containers</span>
		{/if}
		{#if !follow && lines.length > 0}
			<button class="btn-jump" onclick={() => { follow = true; scheduleScroll(); }}>
				↓ Jump to bottom
			</button>
		{/if}
	</div>
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		height: calc(100vh - 60px);
		padding: 0.75rem 1rem;
		gap: 0.5rem;
		box-sizing: border-box;
		overflow: hidden;
	}

	.page-header {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
	}
	h1 { margin: 0; font-size: 1.3rem; }
	.host-name { font-size: 0.9rem; color: var(--text-dim); }

	/* Sub-nav */
	.subnav {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border);
		flex-wrap: wrap;
		flex-shrink: 0;
	}
	.subnav a {
		padding: 0.3rem 0.75rem;
		color: var(--text-dim);
		text-decoration: none;
		font-size: 0.85rem;
		border-bottom: 2px solid transparent;
		margin-bottom: -1px;
	}
	.subnav a:hover { color: var(--text); }
	.subnav a.active { color: var(--accent); border-bottom-color: var(--accent); }

	/* Toolbar */
	.toolbar {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.6rem 0.9rem;
		flex-shrink: 0;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		flex-wrap: wrap;
	}
	.opts-row { gap: 0.75rem; }
	.label { font-size: 0.8rem; color: var(--text-dim); white-space: nowrap; }
	.picker { display: flex; gap: 0.3rem; flex-wrap: wrap; }

	.pick-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.2rem 0.6rem;
		border-radius: 999px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text-dim);
		font-size: 0.78rem;
		cursor: pointer;
	}
	.pick-btn:hover { border-color: var(--accent); color: var(--text); }
	.pick-btn.selected {
		border-color: var(--accent);
		color: var(--text);
		background: color-mix(in srgb, var(--accent) 14%, transparent);
	}
	.state-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }

	.opt {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.82rem;
		color: var(--text-dim);
		cursor: pointer;
		user-select: none;
		white-space: nowrap;
	}
	.opt select {
		background: var(--bg-card);
		border: 1px solid var(--border);
		color: var(--text);
		border-radius: 4px;
		padding: 0.15rem 0.4rem;
		font-size: 0.8rem;
	}
	.opt input[type='checkbox'] { accent-color: var(--accent); }
	.opt.toggle { cursor: pointer; }

	.flex-spacer { flex: 1; }

	.search {
		width: 200px;
		background: var(--bg-card);
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 0.25rem 0.5rem;
		color: var(--text);
		font-size: 0.82rem;
	}
	.search:focus { outline: none; border-color: var(--accent); }

	.btn-sm {
		padding: 0.25rem 0.7rem;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: transparent;
		color: var(--text-dim);
		font-size: 0.8rem;
		cursor: pointer;
		white-space: nowrap;
	}
	.btn-sm:hover:not(:disabled) { border-color: var(--accent); color: var(--text); }
	.btn-sm:disabled { opacity: 0.4; cursor: default; }

	/* Log pane */
	.log-wrap {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		background: #0d0d0d;
		border-radius: 6px;
		border: 1px solid var(--border);
		padding: 0.4rem 0.6rem;
		font-family: 'Fira Code', 'Cascadia Code', Consolas, 'Courier New', monospace;
		font-size: 0.78rem;
		line-height: 1.55;
	}

	.log-line {
		display: block;
		white-space: pre-wrap;
		word-break: break-all;
		padding: 0.05rem 0;
	}
	.log-line:hover { background: rgba(255, 255, 255, 0.04); }

	.ctag {
		font-weight: 600;
		margin-right: 0.5em;
		min-width: 10ch;
		display: inline-block;
		font-size: 0.74rem;
	}
	.ts {
		color: #555;
		margin-right: 0.6em;
		font-size: 0.73rem;
		white-space: nowrap;
	}
	.msg { color: #d4d4d4; }

	.empty {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		min-height: 80px;
		color: var(--text-dim);
		font-size: 0.9rem;
	}

	/* Status bar */
	.statusbar {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		font-size: 0.78rem;
		color: var(--text-dim);
		flex-shrink: 0;
		padding: 0.1rem 0;
	}
	.live-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--border);
		transition: background 0.3s;
		flex-shrink: 0;
	}
	.live-dot.active { background: #2ecc71; box-shadow: 0 0 5px #2ecc71; }
	.dim { opacity: 0.55; }
	.btn-jump {
		margin-left: auto;
		padding: 0.15rem 0.6rem;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--bg-card);
		color: var(--accent);
		font-size: 0.78rem;
		cursor: pointer;
	}
	.btn-jump:hover { border-color: var(--accent); }
</style>
