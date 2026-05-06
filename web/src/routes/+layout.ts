// Pure SPA: no SSR, no build-time prerender. The hub serves the static
// build (or a fallback index.html) and the client-side router handles
// everything else.
export const ssr = false;
export const prerender = false;
