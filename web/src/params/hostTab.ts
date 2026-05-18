// Route param matcher: constrains [tab] inside /hosts/[id]/[tab=hostTab] to
// the monitoring tabs that the single-page host detail handles. Dedicated
// routes (containers, stacks, networks, volumes, images, logs, shell) sit at
// literal segments and take precedence over this matcher.

export type HostTab =
	| 'overview'
	| 'cpu'
	| 'memory'
	| 'disk'
	| 'network'
	| 'sensors'
	| 'processes'
	| 'events'
	| 'settings';

const TABS: ReadonlySet<string> = new Set([
	'overview', 'cpu', 'memory', 'disk', 'network',
	'sensors', 'processes', 'events', 'settings'
]);

export function match(param: string): boolean {
	return TABS.has(param);
}
