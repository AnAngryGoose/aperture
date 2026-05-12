<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import '@xterm/xterm/css/xterm.css';

	let { hostId, cid, onClose } = $props<{
		hostId: string;
		cid: string;
		onClose: () => void;
	}>();

	let containerElem: HTMLElement;
	let term: any;
	let fitAddon: any;
	let ws: WebSocket;
	let resizeObserver: ResizeObserver;

	onMount(async () => {
		const xtermModule = await import('@xterm/xterm');
		const fitAddonModule = await import('@xterm/addon-fit');
		const Terminal = xtermModule.Terminal;
		const FitAddon = fitAddonModule.FitAddon;

		term = new Terminal({
			cursorBlink: true,
			theme: {
				background: '#1e1e1e',
				foreground: '#d4d4d4',
			},
			fontFamily: 'monospace'
		});

		fitAddon = new FitAddon();
		term.loadAddon(fitAddon);

		term.open(containerElem);
		fitAddon.fit();

		// Construct WS URL
		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		let host = window.location.host;
		
		// If running locally with vite, direct to API server on 8080
		if (import.meta.env.DEV) {
			host = 'localhost:8080';
		}

		ws = new WebSocket(`${protocol}//${host}/api/hosts/${hostId}/containers/${cid}/terminal?cmd=/bin/sh`);

		ws.onopen = () => {
			term.focus();
			ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
		};

		ws.onmessage = (ev) => {
			try {
				const msg = JSON.parse(ev.data);
				if (msg.type === 'data') {
					term.write(atob(msg.data));
				}
			} catch(e) {
				console.error("Terminal msg error", e);
			}
		};

		ws.onclose = () => {
			term.write('\r\n\x1b[31m[Session closed]\x1b[0m\r\n');
		};

		term.onData((data) => {
			if (ws.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify({ type: 'input', data: btoa(data) }));
			}
		});

		resizeObserver = new ResizeObserver(() => {
			fitAddon.fit();
			if (ws.readyState === WebSocket.OPEN) {
				ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
			}
		});
		resizeObserver.observe(containerElem);
	});

	onDestroy(() => {
		if (resizeObserver) resizeObserver.disconnect();
		if (ws) ws.close();
		if (term) term.dispose();
	});
</script>

<div class="terminal-wrapper">
	<div class="terminal-head">
		<span class="muted">Terminal: {cid.substring(0,12)}</span>
		<button class="sm-btn" onclick={onClose}>Close</button>
	</div>
	<div class="terminal-body" bind:this={containerElem}></div>
</div>

<style>
	.terminal-wrapper {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: #1e1e1e;
		border: 1px solid var(--border);
		border-radius: 8px;
		overflow: hidden;
	}
	.terminal-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 1rem;
		background: var(--bg-card);
		border-bottom: 1px solid var(--border);
		font-family: monospace;
	}
	.terminal-body {
		flex: 1;
		padding: 0.5rem;
		overflow: hidden;
	}
</style>
