import { api } from '$lib/api';

export type CardLayout = 'rich' | 'tile' | 'list';

export interface DashboardLayout {
	cardLayout: CardLayout;
	pinnedHostIds: string[];
	cardOrder: string[];
	activeFilter: string;
	/**
	 * Per-host metric widget configuration: which metric keys to render on
	 * the rich card. Empty / undefined entries fall back to DEFAULT_WIDGETS
	 * from metricCatalog.ts. Users edit this via the per-card "Configure
	 * widget" modal.
	 */
	cardWidgets: Record<string, string[]>;
}

const DEFAULTS: DashboardLayout = {
	cardLayout: 'rich',
	pinnedHostIds: [],
	cardOrder: [],
	activeFilter: 'all',
	cardWidgets: {}
};

const STORAGE_KEY = 'aperture-dashboard-layout';

function createDashboardLayoutStore() {
	let layout = $state<DashboardLayout>({ ...DEFAULTS });

	function init() {
		try {
			const saved = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null;
			if (saved) layout = { ...DEFAULTS, ...JSON.parse(saved) };
		} catch { /* ignore */ }
	}

	function save() {
		if (typeof localStorage !== 'undefined') {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
		}
		// best-effort persist to backend
		api.settings.saveDashboardLayout(layout).catch(() => {});
	}

	function setCardLayout(v: CardLayout) {
		layout = { ...layout, cardLayout: v };
		save();
	}

	function setFilter(v: string) {
		layout = { ...layout, activeFilter: v };
		save();
	}

	function pinHost(id: string) {
		if (layout.pinnedHostIds.includes(id)) return;
		layout = { ...layout, pinnedHostIds: [...layout.pinnedHostIds, id] };
		save();
	}

	function unpinHost(id: string) {
		layout = { ...layout, pinnedHostIds: layout.pinnedHostIds.filter((x) => x !== id) };
		save();
	}

	function setOrder(ids: string[]) {
		layout = { ...layout, cardOrder: ids };
		save();
	}

	function setCardWidgets(hostId: string, widgets: string[]) {
		layout = { ...layout, cardWidgets: { ...layout.cardWidgets, [hostId]: widgets } };
		save();
	}

	function getCardWidgets(hostId: string): string[] | undefined {
		return layout.cardWidgets[hostId];
	}

	return {
		get layout() { return layout; },
		get cardLayout() { return layout.cardLayout; },
		get activeFilter() { return layout.activeFilter; },
		get cardWidgets() { return layout.cardWidgets; },
		init,
		setCardLayout,
		setFilter,
		pinHost,
		unpinHost,
		setOrder,
		setCardWidgets,
		getCardWidgets
	};
}

export const dashboardLayout = createDashboardLayoutStore();
