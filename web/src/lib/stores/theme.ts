type ThemeMode = 'dark' | 'light' | 'system';

const STORAGE_KEY = 'aperture-theme';

function getSystemTheme(): 'dark' | 'light' {
	if (typeof window === 'undefined') return 'dark';
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function resolveTheme(mode: ThemeMode): 'dark' | 'light' {
	if (mode === 'system') return getSystemTheme();
	return mode;
}

function applyTheme(mode: ThemeMode) {
	if (typeof document === 'undefined') return;
	document.documentElement.dataset.theme = resolveTheme(mode);
}

function createThemeStore() {
	let mode = $state<ThemeMode>('dark');

	function init() {
		const saved = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null;
		mode = (saved as ThemeMode) ?? 'dark';
		applyTheme(mode);

		if (mode === 'system' && typeof window !== 'undefined') {
			window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => applyTheme('system'));
		}
	}

	function set(next: ThemeMode) {
		mode = next;
		if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, next);
		applyTheme(next);
	}

	function toggle() {
		const current = resolveTheme(mode);
		set(current === 'dark' ? 'light' : 'dark');
	}

	return {
		get mode() { return mode; },
		get resolved() { return resolveTheme(mode); },
		init,
		set,
		toggle
	};
}

export const theme = createThemeStore();
