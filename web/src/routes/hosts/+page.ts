import { redirect } from '@sveltejs/kit';

export const load = () => {
	// The Hosts sidebar item points at /hosts, but the host listing lives on
	// /dashboard. Redirect at load time so the sidebar click never hits a 404.
	throw redirect(307, '/dashboard');
};
