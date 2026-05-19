<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import type { Container } from '$lib/types';
	import Button from '$lib/components/primitives/Button.svelte';
	import Icon from '$lib/components/primitives/Icon.svelte';

	let id = $derived(page.params.id ?? '');
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
		return s.replace(/\x1b\[[0-9;]*[mGKHFJABCDsu]/g, '').replace(/\x1b[()][0-9A-Z]/g, '');
	}

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

	function onScroll() {
		if (!logEl) return;
		const atBottom = logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 60;
		follow = atBottom;
	}

	onMount(async () => {
		try {
			const cs = await api.containers(id, true);
			allContainers = cs;
			const first = cs.find((c) => c.state === 'running') ?? cs[0];
			if (first) selectedIDs = [first.id];
		} catch {}
		await loadAll();
	});

	onDestroy(stopPolling);
</script>

<section class="logs-tab">
	<header class="tab-head">
		<div class="lead">
			<h2>Logs</h2>
			<span class="lead-sub mono">
				{filteredLines.length}{searchText.trim() ? ` / ${lines.length}` : ''} line{filteredLines.length === 1 ? '' : 's'}
				{#if polling} · live (2s){/if}
			</span>
		</div>
	</header>

	<div class="toolbar">
		<div class="row">
			<span class="label-mono">Containers</span>
			<div class="picker">
				{#each allContainers as c (c.id)}
					<button
						class="pick-btn"
						class:selected={selectedIDs.includes(c.id)}
						onclick={() => toggleContainer(c.id)}
						title={c.image}
					>
						<span class="state-dot" class:running={c.state === 'running'}></span>
						{c.name.replace(/^\//, '')}
					</button>
				{/each}
				{#if allContainers.length === 0}
					<span class="muted">No containers found</span>
				{/if}
			</div>
			<Button variant="ghost" size="sm" onclick={loadAll} disabled={selectedIDs.length === 0 || loading}>
				{loading ? 'Loading…' : 'Reload'}
			</Button>
		</div>

		<div class="row opts-row">
			<label class="opt">
				<span class="label-mono">Tail</span>
				<select bind:value={tail}>
					<option value={50}>50</option>
					<option value={200}>200</option>
					<option value={500}>500</option>
					<option value={1000}>1000</option>
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
			<Button variant="ghost" size="sm" onclick={clearDisplay} title="Clear display">Clear</Button>
			<Button variant="ghost" size="sm" onclick={exportLogs} disabled={lines.length === 0} title="Export as text">
				Export
			</Button>
		</div>
	</div>

	<div class="log-wrap" bind:this={logEl} onscroll={onScroll}>
		{#if loading}
			<div class="empty">Loading…</div>
		{:else if selectedIDs.length === 0}
			<div class="empty">Select one or more containers above to view logs.</div>
		{:else if filteredLines.length === 0 && lines.length > 0}
			<div class="empty">No lines match "{searchText}"</div>
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

	{#if !follow && lines.length > 0}
		<div class="jump-row">
			<Button variant="ghost" size="sm" onclick={() => { follow = true; scheduleScroll(); }}>
				<Icon name="arrow-down" size={12} /> Jump to bottom
			</Button>
		</div>
	{/if}
</section>

<style>
	.logs-tab {
		display: flex;
		flex-direction: column;
		gap: 10px;
		min-height: calc(100vh - 220px);
	}

	.tab-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
	}
	.lead { display: flex; align-items: baseline; gap: 10px; }
	.lead h2 { margin: 0; font-size: 16px; font-weight: 600; color: var(--text); letter-spacing: -0.01em; }
	.lead-sub { font-size: 11px; color: var(--text-faint); }

	.toolbar {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 10px 12px;
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
	}
	.row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
	.opts-row { gap: 14px; }

	.label-mono {
		font-family: var(--font-mono);
		font-size: 10px;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--text-faint);
		white-space: nowrap;
	}

	.picker { display: flex; gap: 4px; flex-wrap: wrap; }

	.pick-btn {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 3px 10px;
		border-radius: var(--r-pill);
		border: 1px solid var(--line);
		background: var(--bg-elev-2);
		color: var(--text-dim);
		font-size: 11px;
		font-family: var(--font-sans);
		cursor: pointer;
		transition: background 120ms, color 120ms, border-color 120ms;
	}
	.pick-btn:hover { border-color: var(--line-strong); color: var(--text); }
	.pick-btn.selected {
		border-color: var(--accent);
		color: var(--accent);
		background: var(--accent-soft);
	}
	.state-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--offline); flex-shrink: 0; }
	.state-dot.running { background: var(--ok); }

	.opt {
		display: flex; align-items: center; gap: 6px;
		font-size: 12px; color: var(--text-dim);
		cursor: pointer; user-select: none; white-space: nowrap;
	}
	.opt select {
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		color: var(--text); border-radius: var(--r-md);
		padding: 3px 8px; font-size: 11px;
	}
	.opt input[type='checkbox'] { accent-color: var(--accent); }

	.flex-spacer { flex: 1; }

	.search {
		width: 200px;
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 4px 10px;
		color: var(--text);
		font-size: 12px;
	}
	.search:focus { outline: none; border-color: var(--accent-line); }

	.log-wrap {
		flex: 1;
		min-height: 320px;
		overflow-y: auto;
		background: var(--bg);
		border-radius: var(--r-md);
		border: 1px solid var(--line);
		padding: 8px 12px;
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.55;
	}

	.log-line {
		display: block;
		white-space: pre-wrap;
		word-break: break-all;
		padding: 1px 0;
		color: var(--text);
	}
	.log-line:hover { background: var(--bg-hover); }

	.ctag {
		font-weight: 600;
		margin-right: 0.5em;
		min-width: 10ch;
		display: inline-block;
		font-size: 10px;
	}
	.ts {
		color: var(--text-faint);
		margin-right: 0.6em;
		font-size: 10px;
		white-space: nowrap;
	}
	.msg { color: var(--text); }

	.empty {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		min-height: 80px;
		color: var(--text-faint);
		font-size: 12px;
	}

	.muted { color: var(--text-faint); font-size: 12px; }

	.jump-row { display: flex; justify-content: flex-end; }
</style>
