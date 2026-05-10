export function formatBytes(n: number): string {
	if (!isFinite(n) || n < 0) return '–';
	const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
	let i = 0;
	let v = n;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return `${v.toFixed(v >= 100 || i === 0 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

export function formatBytesRate(bps: number): string {
	if (!isFinite(bps) || bps < 0) return '–';
	if (bps < 1024) return `${bps.toFixed(0)} B/s`;
	if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
	if (bps < 1073741824) return `${(bps / 1048576).toFixed(1)} MiB/s`;
	return `${(bps / 1073741824).toFixed(2)} GiB/s`;
}

export function formatPct(n: number): string {
	if (!isFinite(n)) return '–';
	return `${n.toFixed(1)}%`;
}

export function formatDuration(secs: number): string {
	if (!isFinite(secs) || secs < 0) return '–';
	const d = Math.floor(secs / 86400);
	const h = Math.floor((secs % 86400) / 3600);
	const m = Math.floor((secs % 3600) / 60);
	const s = Math.floor(secs % 60);
	if (d > 0) return `${d}d ${h}h`;
	if (h > 0) return `${h}h ${m}m`;
	if (m > 0) return `${m}m ${s}s`;
	return `${s}s`;
}

export function relTime(iso: string): string {
	const t = new Date(iso).getTime();
	const diff = (Date.now() - t) / 1000;
	if (diff < 5) return 'just now';
	if (diff < 60) return `${Math.floor(diff)}s ago`;
	if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
	if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
	return `${Math.floor(diff / 86400)}d ago`;
}

export function absTime(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		month: 'short', day: 'numeric',
		hour: 'numeric', minute: '2-digit', second: '2-digit'
	});
}
