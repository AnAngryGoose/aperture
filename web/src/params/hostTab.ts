// Route param matcher: constrains [tab] inside /hosts/[id]/[tab=hostTab] to
// the host detail tabs. Dedicated routes that need their own +page.svelte
// (containers, stacks, networks, volumes, images, logs, shell) live at
// literal segments which take precedence over this matcher.

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
