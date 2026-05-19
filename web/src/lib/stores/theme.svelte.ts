/**
 * Theme store. The runtime theme is one of four palettes — `dark`, `light`,
 * `gruvbox-dark`, `gruvbox-light` — plus a `system` mode that maps to the
 * regular dark/light pair based on the OS preference.
 *
 * The resolved value is written to `<html data-theme="…">` and the matching
 * `[data-theme="…"]` block in `tokens.css` provides the design tokens.
 */
type ThemeMode = 'dark' | 'light' | 'gruvbox-dark' | 'gruvbox-light' | 'system';
type ResolvedTheme = Exclude<ThemeMode, 'system'>;

const STORAGE_KEY = 'aperture-theme';
const VALID_MODES: ReadonlySet<string> = new Set([
	'dark', 'light', 'gruvbox-dark', 'gruvbox-light', 'system'
]);

function getSystemTheme(): 'dark' | 'light' {
	if (typeof window === 'undefined') return 'dark';
	return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function resolveTheme(mode: ThemeMode): ResolvedTheme {
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
		mode = saved && VALID_MODES.has(saved) ? (saved as ThemeMode) : 'dark';
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
		// Flip within the current palette family. Gruvbox stays in gruvbox;
		// otherwise toggle between the default dark / light pair.
		const current = resolveTheme(mode);
		if (current === 'gruvbox-dark') set('gruvbox-light');
		else if (current === 'gruvbox-light') set('gruvbox-dark');
		else set(current === 'dark' ? 'light' : 'dark');
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
export type { ThemeMode };
