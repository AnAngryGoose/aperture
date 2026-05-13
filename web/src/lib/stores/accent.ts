const STORAGE_KEY = 'aperture-accent';

export type AccentKey = 'teal' | 'indigo' | 'amber' | 'violet' | 'lime' | 'rose';

export const ACCENTS: Record<AccentKey, { hex: string; soft: string; line: string; label: string }> = {
	teal:   { hex: '#14b8a6', soft: 'rgba(20,184,166,.14)',  line: 'rgba(20,184,166,.4)',  label: 'Teal' },
	indigo: { hex: '#6366f1', soft: 'rgba(99,102,241,.14)',  line: 'rgba(99,102,241,.4)',  label: 'Indigo' },
	amber:  { hex: '#f59e0b', soft: 'rgba(245,158,11,.14)',  line: 'rgba(245,158,11,.4)',  label: 'Amber' },
	violet: { hex: '#a855f7', soft: 'rgba(168,85,247,.14)',  line: 'rgba(168,85,247,.4)',  label: 'Violet' },
	lime:   { hex: '#84cc16', soft: 'rgba(132,204,22,.14)',  line: 'rgba(132,204,22,.4)',  label: 'Lime' },
	rose:   { hex: '#f43f5e', soft: 'rgba(244,63,94,.14)',   line: 'rgba(244,63,94,.4)',   label: 'Rose' }
};

function applyAccent(key: AccentKey) {
	if (typeof document === 'undefined') return;
	const a = ACCENTS[key];
	const root = document.documentElement;
	root.style.setProperty('--accent', a.hex);
	root.style.setProperty('--accent-soft', a.soft);
	root.style.setProperty('--accent-line', a.line);
}

function createAccentStore() {
	let key = $state<AccentKey>('teal');

	function init() {
		const saved = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null;
		key = (saved as AccentKey) ?? 'teal';
		applyAccent(key);
	}

	function set(next: AccentKey) {
		key = next;
		if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, next);
		applyAccent(next);
	}

	return {
		get key() { return key; },
		get hex() { return ACCENTS[key].hex; },
		init,
		set
	};
}

export const accent = createAccentStore();
