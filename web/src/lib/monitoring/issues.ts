// Single source of truth for "what needs attention on the dashboard."
//
// Both the PageHeader summary strip and the NeedsAttention panel consume this
// module — without it the two displays could drift (and previously did: the
// header read a stale `host.open_alerts` field while the panel read the new
// `entry.openAlerts`, so the strip showed 0 while the panel showed 1).
//
// Inputs:
//   entries  — the dashboard's host list with live ring buffers and last sample
//   events   — currently-open alert events pre-joined with their rule (from
//              /api/monitoring/overview.events)
//
// Outputs are derived deterministically; calling deriveIssues twice with the
// same inputs returns the same rows. summarize() reduces the issue list and
// entries into the top-strip totals so both displays count the same things.

import type { HostEntry } from '$lib/stores/monitoring.svelte';
import type { OverviewAlertEvent } from '$lib/types';
import { fmtBytes, fmtRelative } from '$lib/format';
import { humanizeAlert, formatMetricValue } from './humanizeAlert';

export type Severity = 'crit' | 'warn';

export type IssueKind =
	| 'offline'
	| 'unhealthy_containers'
	| 'alert'
	| 'cpu' | 'mem' | 'temp' | 'disk';

export interface Issue {
	hostId: string;
	hostName: string;
	severity: Severity;
	kind: IssueKind;
	/** Human-readable reason — appears in the middle column. */
	reason: string;
	/** Concrete value when available (CPU%, °C, count, etc.) — right column. */
	detail: string;
	/** Anchor route the click target should navigate to. */
	href: string;
}

/**
 * Thresholds for the metric-breach issue kinds. Match the
 * DefaultHostConfig fallbacks in internal/store/store.go so the panel calls
 * out the same hosts the server-side status flips on. Per-host alert rules
 * can still be tuned in /alerts; this is a dashboard-level summary only.
 */
const T = {
	cpu:  { warn: 70, crit: 90 },
	mem:  { warn: 80, crit: 90 },
	temp: { warn: 70, crit: 85 },
	disk: { warn: 80, crit: 90 }
};

function severityFor(value: number, t: { warn: number; crit: number }): Severity | null {
	if (value >= t.crit) return 'crit';
	if (value >= t.warn) return 'warn';
	return null;
}

/**
 * Map an alert event's rule severity onto the issue severity vocabulary.
 * The events come in with the rule's severity string ("info" / "warning" /
 * "critical"). "info" rows are rendered as warning to keep the panel
 * actionable; the host-detail Events tab has the full breakdown.
 */
function eventSeverity(s: string): Severity {
	return s === 'critical' ? 'crit' : 'warn';
}

/**
 * Path-based deep link into the host detail page. Each monitoring tab is a
 * real route (e.g. /hosts/{id}/cpu) so refreshing preserves the selection
 * and the browser's back/forward navigation moves between tabs.
 */
function hostHref(hostId: string, tab: string = 'overview'): string {
	return `/hosts/${hostId}/${tab}`;
}

export function deriveIssues(entries: HostEntry[], events: OverviewAlertEvent[] = []): Issue[] {
	const out: Issue[] = [];
	const byHost = new Map<string, HostEntry>();
	for (const e of entries) byHost.set(e.host.id, e);

	for (const e of entries) {
		const h = e.host;
		const s = e.latest;

		// Offline trumps everything else for this host. Skip the metric checks
		// since the sample is stale by definition.
		if (e.status === 'offline') {
			out.push({
				hostId: h.id, hostName: h.name,
				severity: 'crit', kind: 'offline',
				reason: 'Host offline',
				detail: h.last_seen ? `last seen ${fmtRelative(h.last_seen)}` : '',
				href: hostHref(h.id)
			});
			continue;
		}

		// Unhealthy containers (docker hosts only). 3+ unhealthy → critical;
		// 1–2 → warning. Docker reports unhealthy via healthcheck failures, so
		// one or two isn't necessarily a fire.
		if (e.containers && e.containers.unhealthy > 0) {
			const n = e.containers.unhealthy;
			out.push({
				hostId: h.id, hostName: h.name,
				severity: n >= 3 ? 'crit' : 'warn',
				kind: 'unhealthy_containers',
				reason: `${n} unhealthy container${n === 1 ? '' : 's'}`,
				detail: `${e.containers.running}/${e.containers.total} running`,
				href: `/hosts/${h.id}/containers`
			});
		}

		if (!s) continue;

		const cpuSev = severityFor(s.cpu_percent, T.cpu);
		if (cpuSev) {
			out.push({
				hostId: h.id, hostName: h.name,
				severity: cpuSev, kind: 'cpu',
				reason: cpuSev === 'crit' ? 'High CPU usage' : 'Elevated CPU usage',
				detail: `${s.cpu_percent.toFixed(1)}%`,
				href: hostHref(h.id, 'cpu')
			});
		}

		const memSev = severityFor(s.mem_percent, T.mem);
		if (memSev) {
			out.push({
				hostId: h.id, hostName: h.name,
				severity: memSev, kind: 'mem',
				reason: memSev === 'crit' ? 'High memory usage' : 'Elevated memory usage',
				detail: `${s.mem_percent.toFixed(1)}% · ${fmtBytes(s.mem_used)}`,
				href: hostHref(h.id, 'memory')
			});
		}

		let maxTemp = 0;
		let hotSensorName = '';
		for (const t of (s.temps ?? [])) {
			if (t.temp_celsius > maxTemp) {
				maxTemp = t.temp_celsius;
				hotSensorName = t.name;
			}
		}
		const tempSev = severityFor(maxTemp, T.temp);
		if (tempSev) {
			out.push({
				hostId: h.id, hostName: h.name,
				severity: tempSev, kind: 'temp',
				reason: tempSev === 'crit' ? 'High temperature' : 'Elevated temperature',
				detail: `${maxTemp.toFixed(1)}°C${hotSensorName ? ' · ' + hotSensorName : ''}`,
				href: hostHref(h.id, 'sensors')
			});
		}

		const diskSev = severityFor(s.disk_percent, T.disk);
		if (diskSev) {
			out.push({
				hostId: h.id, hostName: h.name,
				severity: diskSev, kind: 'disk',
				reason: diskSev === 'crit' ? 'Disk nearly full' : 'Disk filling up',
				detail: `${s.disk_percent.toFixed(1)}%`,
				href: hostHref(h.id, 'disk')
			});
		}
	}

	// One row per open alert event (from the overview's events array). Each
	// row shows the rule's metric/op/threshold as the title plus the current
	// value as the detail. Click → host detail Events tab.
	const eventHostCounts = new Map<string, number>();
	for (const ev of events) {
		eventHostCounts.set(ev.host_id, (eventHostCounts.get(ev.host_id) ?? 0) + 1);
		const host = byHost.get(ev.host_id);
		const hostName = host?.host.name ?? ev.host_id;
		const h = humanizeAlert({
			metric: ev.metric, op: ev.op, threshold: ev.threshold, value: ev.value
		});
		out.push({
			hostId: ev.host_id,
			hostName,
			severity: eventSeverity(ev.severity),
			kind: 'alert',
			reason: h.title,
			detail: typeof ev.value === 'number'
				? `Current: ${formatMetricValue(ev.metric, ev.value)} · Threshold: ${formatMetricValue(ev.metric, ev.threshold)}`
				: `Threshold: ${formatMetricValue(ev.metric, ev.threshold)}`,
			href: hostHref(ev.host_id, 'events')
		});
	}

	// Fallback alert row: if a host has openAlerts > 0 but we don't have a
	// matching detail in `events` (SSE bumped the counter before reconcile
	// refilled events, or the backend capped events at 50 while a host has
	// more open), surface a generic row so the count and the panel agree.
	for (const e of entries) {
		const detailed = eventHostCounts.get(e.host.id) ?? 0;
		const missing = (e.openAlerts ?? 0) - detailed;
		if (missing > 0) {
			out.push({
				hostId: e.host.id,
				hostName: e.host.name,
				severity: 'warn',
				kind: 'alert',
				reason: `${missing} open alert${missing === 1 ? '' : 's'}`,
				detail: '',
				href: hostHref(e.host.id, 'events')
			});
		}
	}

	// Sort: critical first, then warning. Within a severity tier, group by
	// host so a single failing host's rows cluster, then by kind for stable
	// ordering when nothing else changes.
	out.sort((a, b) => {
		if (a.severity !== b.severity) return a.severity === 'crit' ? -1 : 1;
		if (a.hostName !== b.hostName) return a.hostName.localeCompare(b.hostName);
		return a.kind.localeCompare(b.kind);
	});
	return out;
}

/**
 * Reduce the issue list and entries into the values the PageHeader summary
 * strip needs. By going through the same `events` array that deriveIssues
 * uses for alert rows, the strip's OPEN ALERTS count is guaranteed to match
 * the number of "alert" rows in NeedsAttention.
 */
export interface Summary {
	healthy: number;
	warn: number;
	crit: number;
	offline: number;
	containers: { running: number; total: number; unhealthy: number };
	openAlerts: number;
	totalIssues: number;
	critIssues: number;
	warnIssues: number;
}

export function summarize(entries: HostEntry[], events: OverviewAlertEvent[] = []): Summary {
	let healthy = 0, warn = 0, crit = 0, offline = 0;
	let running = 0, total = 0, unhealthy = 0;
	let openAlertsLive = 0;

	for (const e of entries) {
		if (e.status === 'ok') healthy++;
		else if (e.status === 'warn') warn++;
		else if (e.status === 'crit') crit++;
		else offline++;

		const c = e.containers;
		if (c) {
			running += c.running;
			total += c.total;
			unhealthy += c.unhealthy;
		}

		openAlertsLive += e.openAlerts ?? 0;
	}

	// Severity counts on the issue list itself — exposed so the NeedsAttention
	// header can render "N critical · M warnings" from the same source.
	const issues = deriveIssues(entries, events);
	const critIssues = issues.filter((i) => i.severity === 'crit').length;
	const warnIssues = issues.length - critIssues;

	// OPEN ALERTS reflects the SSE-bumped per-host counter, not just the
	// (capped) events array. An incoming alert SSE updates entry.openAlerts
	// immediately while the events array only refreshes on the next reconcile
	// — using the sum keeps the top-strip count in lockstep with the alert
	// rows NeedsAttention renders (real rows + fallback rows). Floor at
	// events.length so the strip never UNDERreports rows that are visible.
	const openAlerts = Math.max(openAlertsLive, events.length);

	return {
		healthy, warn, crit, offline,
		containers: { running, total, unhealthy },
		openAlerts,
		totalIssues: issues.length,
		critIssues,
		warnIssues
	};
}
