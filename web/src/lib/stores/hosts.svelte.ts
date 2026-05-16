// Re-export the monitoring store under the legacy `hostStore` name so
// pre-v0.5 components (PageHeader, FilterBar, Sidebar, RichCard, etc.) keep
// working without import churn. Single source of truth lives in
// monitoring.svelte.ts now — this file is purely an alias.

export type { HostEntry, HostStatus, HostKind } from './monitoring.svelte';
export { monitoringStore as hostStore } from './monitoring.svelte';
