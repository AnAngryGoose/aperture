<script lang="ts">
	import type { HostEntry } from '$lib/stores/hosts.svelte';
	import type { OverviewAlertEvent } from '$lib/types';
	import { deriveIssues, type Issue, type IssueKind } from '$lib/monitoring/issues';
	import Icon from '$lib/components/primitives/Icon.svelte';

	interface Props {
		entries: HostEntry[];
		/** Open alert events from the overview, joined with their rule. */
		events: OverviewAlertEvent[];
	}

	let { entries, events }: Props = $props();

	// Single derivation: the same function that PageHeader's summarize()
	// calls internally. Guarantees the two displays cannot disagree on what
	// counts as an issue.
	const issues = $derived<Issue[]>(deriveIssues(entries, events));

	const critCount = $derived(issues.filter((i) => i.severity === 'crit').length);
	const warnCount = $derived(issues.length - critCount);

	// Cap visible rows at 5; surface "View all" when more exist. 5 keeps the
	// panel from dominating vertical space on first paint while still showing
	// the most urgent items (the sort puts critical first).
	const VISIBLE_CAP = 5;
	const visibleIssues = $derived(issues.slice(0, VISIBLE_CAP));
	const hiddenCount = $derived(Math.max(0, issues.length - VISIBLE_CAP));

	const KIND_ICON: Record<IssueKind, string> = {
		offline: 'offline',
		unhealthy_containers: 'container',
		alert: 'alerts',
		cpu: 'cpu',
		mem: 'mem',
		temp: 'temp',
		disk: 'disk'
	};
</script>

<section class="needs-attention" class:has-crit={critCount > 0}>
	<header class="head">
		<div class="title-row">
			<span class="title-icon" style="color: {issues.length === 0 ? 'var(--ok)' : critCount > 0 ? 'var(--crit)' : 'var(--warn)'}">
				<Icon name={issues.length === 0 ? 'ok' : 'warn'} size={14} />
			</span>
			<h2 class="title">Needs Attention</h2>
			{#if issues.length > 0}
				<span class="counts mono">
					{#if critCount > 0}<span class="crit-count">{critCount} critical</span>{/if}
					{#if critCount > 0 && warnCount > 0}<span class="sep">·</span>{/if}
					{#if warnCount > 0}<span class="warn-count">{warnCount} warning{warnCount === 1 ? '' : 's'}</span>{/if}
				</span>
			{/if}
		</div>
	</header>

	{#if issues.length === 0}
		<div class="empty mono">No issues detected.</div>
	{:else}
		<ul class="issue-list">
			{#each visibleIssues as issue (issue.hostId + '|' + issue.kind + '|' + issue.reason)}
				<li>
					<a
						class="issue-row"
						class:crit={issue.severity === 'crit'}
						href={issue.href}
					>
						<span class="sev-rail" style="background:{issue.severity === 'crit' ? 'var(--crit)' : 'var(--warn)'}"></span>
						<span class="sev-icon" style="color:{issue.severity === 'crit' ? 'var(--crit)' : 'var(--warn)'}">
							<Icon name={KIND_ICON[issue.kind]} size={14} />
						</span>
						<span class="host-name mono">{issue.hostName}</span>
						<span class="reason">{issue.reason}</span>
						<span class="detail mono">{issue.detail}</span>
						<span class="chev"><Icon name="chevron-right" size={12} /></span>
					</a>
				</li>
			{/each}
		</ul>
		{#if hiddenCount > 0}
			<a class="view-all" href="/alerts">
				View all issues
				<span class="view-all-count mono">+{hiddenCount} more</span>
				<Icon name="chevron-right" size={12} />
			</a>
		{/if}
	{/if}
</section>

<style>
	.needs-attention {
		background: var(--bg-elev);
		border: 1px solid var(--line);
		border-radius: var(--r-lg);
		overflow: hidden;
	}

	/* Subtle accent on the whole card when something is critical, so the panel
	   reads as "this is the urgent thing" without shouting. */
	.needs-attention.has-crit { border-color: color-mix(in srgb, var(--crit) 35%, var(--line)); }

	.head {
		padding: 10px 14px;
		border-bottom: 1px solid var(--line);
	}

	.title-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.title-icon { display: inline-flex; }

	.title {
		margin: 0;
		font-size: 13px;
		font-weight: 600;
		color: var(--text);
		letter-spacing: -0.005em;
	}

	.counts {
		margin-left: 4px;
		font-size: 11px;
		color: var(--text-faint);
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}

	.crit-count { color: var(--crit); font-weight: 500; }
	.warn-count { color: var(--warn); font-weight: 500; }
	.sep { color: var(--text-faint); }

	.empty {
		padding: 14px 16px;
		font-size: 12px;
		color: var(--text-faint);
		text-align: center;
	}

	.issue-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	li { display: block; }

	.issue-row {
		display: grid;
		/* rail | icon | host | reason (flex) | detail | chev */
		grid-template-columns: 3px 22px minmax(120px, 220px) minmax(0, 1fr) auto 14px;
		align-items: center;
		gap: 12px;
		width: 100%;
		padding: 8px 14px;
		background: none;
		border: none;
		border-bottom: 1px solid var(--line);
		color: var(--text);
		font-family: inherit;
		font-size: inherit;
		text-align: left;
		text-decoration: none;
		cursor: pointer;
		transition: background 120ms;
	}

	.issue-row:hover { background: var(--bg-hover); }
	li:last-child .issue-row { border-bottom: none; }

	.sev-rail {
		width: 3px;
		height: 18px;
		border-radius: var(--r-pill);
	}

	.sev-icon {
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.host-name {
		font-size: 12px;
		color: var(--text);
		font-weight: 500;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.reason {
		font-size: 12px;
		color: var(--text-dim);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.detail {
		font-size: 11px;
		color: var(--text-faint);
		white-space: nowrap;
	}

	.chev {
		color: var(--text-faint);
		display: flex;
		align-items: center;
		justify-content: flex-end;
	}

	/* "View all issues" footer: full-width affordance only rendered when the
	   issue count exceeds the 5-row cap. Matches issue-row hover behavior so
	   it feels like the next row rather than a CTA pill. */
	.view-all {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		width: 100%;
		padding: 9px 14px;
		font-size: 12px;
		color: var(--accent);
		background: var(--bg-elev-2);
		border-top: 1px solid var(--line);
		text-decoration: none;
		transition: background 120ms;
	}
	.view-all:hover { background: var(--bg-hover); }
	.view-all-count {
		color: var(--text-faint);
		font-size: 11px;
	}
</style>
