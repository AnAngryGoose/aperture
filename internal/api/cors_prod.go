//go:build !dev

package api

import "net/http"

// corsForDev is a no-op in production builds. The SPA is served from the
// same origin as the API so no extra CORS headers are needed.
func corsForDev(next http.Handler) http.Handler { return next }
