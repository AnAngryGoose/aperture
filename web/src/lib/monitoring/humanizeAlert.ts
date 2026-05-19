// Convert raw alert rule syntax ("cpu_pct < 90") into operator-readable
// sentences ("CPU below threshold"). Used wherever an alert event surfaces
// in the UI — dashboard NeedsAttention, host overview alert banner, the
// host Events tab, and the alerts admin page.
//
// The catalog covers the dashboard's scalar metrics from
// `monitoring/metricCatalog.ts`. Anything not in the catalog falls back to
// the raw rule text so unfamiliar metrics still display, just less prettily.

import { getMetric } from './metricCatalog';

export interface HumanAlert {
	/** Headline sentence — what crossed the threshold. */
	title: string;
	/** Optional secondary line — current value vs. threshold, when both are known. */
	detail: string;
}

function isPercentMetric(metric: string): boolean {
	return /(_pct|_percent)$/.test(metric);
}

function isTempMetric(metric: string): boolean {
	return metric.startsWith('temp') || metric.includes('temperature');
}

function isLoadMetric(metric: string): boolean {
	return metric.startsWith('load_') || metric === 'loadavg';
}

function isRateMetric(metric: string): boolean {
	return metric.startsWith('net_') || metric.endsWith('_rate') || metric.endsWith('_bps');
}

function formatBytesPerSec(bps: number): string {
	if (bps < 1024) return `${bps.toFixed(0)} B/s`;
	if (bps < 1048576) return `${(bps / 1024).toFixed(1)} KiB/s`;
	if (bps < 1073741824) return `${(bps / 1048576).toFixed(1)} MiB/s`;
	return `${(bps / 1073741824).toFixed(1)} GiB/s`;
}

function formatNumber(v: number): string {
	if (!Number.isFinite(v)) return String(v);
	if (Number.isInteger(v)) return String(v);
	if (Math.abs(v) >= 100) return v.toFixed(0);
	if (Math.abs(v) >= 10) return v.toFixed(1);
	return v.toFixed(2);
}

/** Render a value in the units appropriate for the given metric. */
export function formatMetricValue(metric: string, value: number): string {
	if (isPercentMetric(metric)) return `${formatNumber(value)}%`;
	if (isTempMetric(metric)) return `${formatNumber(value)}°C`;
	if (isRateMetric(metric)) return formatBytesPerSec(value);
	if (isLoadMetric(metric)) return value.toFixed(2);
	return formatNumber(value);
}

const OP_PHRASES: Record<string, { above: string; below: string }> = {
	'>':  { above: 'above threshold', below: 'above threshold' },
	'>=': { above: 'above threshold', below: 'above threshold' },
	'<':  { above: 'below threshold', below: 'below threshold' },
	'<=': { above: 'below threshold', below: 'below threshold' },
	'==': { above: 'matches threshold', below: 'matches threshold' },
	'!=': { above: 'diverged from threshold', below: 'diverged from threshold' }
};

function direction(op: string): 'above' | 'below' {
	return op.startsWith('<') ? 'below' : 'above';
}

/**
 * Convert an alert rule + observed value into a human-readable title + detail.
 *
 *   humanizeAlert({ metric: 'cpu_pct', op: '<', threshold: 90, value: 11.5 })
 *   →  { title: 'CPU below threshold',
 *        detail: 'Current: 11.5% · Threshold: 90%' }
 *
 * Unknown metrics fall back to the metric key as-is:
 *   { metric: 'foo', op: '>', threshold: 1, value: 2 }
 *   →  { title: 'foo above threshold', detail: 'Current: 2 · Threshold: 1' }
 */
export function humanizeAlert(input: {
	metric: string;
	op: string;
	threshold: number;
	value?: number;
}): HumanAlert {
	const spec = getMetric(input.metric);
	const label = spec?.label ?? input.metric;
	const phrase = OP_PHRASES[input.op]?.[direction(input.op)] ?? `${input.op} ${formatNumber(input.threshold)}`;
	const title = `${label} ${phrase}`;

	const parts: string[] = [];
	if (typeof input.value === 'number' && !Number.isNaN(input.value)) {
		parts.push(`Current: ${formatMetricValue(input.metric, input.value)}`);
	}
	parts.push(`Threshold: ${formatMetricValue(input.metric, input.threshold)}`);

	return { title, detail: parts.join(' · ') };
}

/**
 * Best-effort one-line render. When the value is known we render both
 * Current and Threshold; otherwise just the title.
 */
export function humanizeAlertLine(input: {
	metric: string;
	op: string;
	threshold: number;
	value?: number;
}): string {
	const h = humanizeAlert(input);
	return h.detail ? `${h.title} — ${h.detail}` : h.title;
}
