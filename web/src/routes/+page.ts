import { redirect } from '@sveltejs/kit';

export const load = () => {
	// Send the bare root straight to the dashboard.
	// Using a load-time redirect (instead of an onMount goto in +page.svelte)
	// avoids a black flash while the empty root page mounts and then re-navigates.
	throw redirect(307, '/dashboard');
};
