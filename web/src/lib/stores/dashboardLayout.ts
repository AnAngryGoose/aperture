import { api } from '$lib/api';

export type CardLayout = 'rich' | 'tile' | 'list';

export interface DashboardLayout {
	cardLayout: CardLayout;
	pinnedHostIds: string[];
	cardOrder: string[];
	activeFilter: string;
}

const DEFAULTS: DashboardLayout = {
	cardLayout: 'rich',
	pinnedHostIds: [],
	cardOrder: [],
	activeFilter: 'all'
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

	return {
		get layout() { return layout; },
		get cardLayout() { return layout.cardLayout; },
		get activeFilter() { return layout.activeFilter; },
		init,
		setCardLayout,
		setFilter,
		pinHost,
		unpinHost,
		setOrder
	};
}

export const dashboardLayout = createDashboardLayoutStore();
