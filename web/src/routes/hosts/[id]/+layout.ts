// Disable SSR for host detail pages: they poll live APIs and rely on Svelte 5
// $state proxies in shared context, which don't survive serialization.
export const ssr = false;
