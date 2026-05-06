// Command agent is the placeholder for the remote-host agent binary.
//
// In v0.1 the hub embeds a local collector, so this binary is intentionally
// minimal — it exists so the multi-host architecture is visible in the repo
// layout from day 1. When implemented, it will:
//
//   1. Sample the host via the same internal/collector code path.
//   2. Push types.MetricSample over a network transport (HTTPS or WS) to a
//      configured hub URL.
//   3. Expose the same docker surface (internal/dockerctl) over that
//      transport so the hub's DockerProvider abstraction is satisfied
//      remotely.
//
// Until then it just prints a version line and exits.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.0-placeholder"

func main() {
	fmt.Fprintf(os.Stderr, "aperture-agent %s\n", version)
	fmt.Fprintln(os.Stderr, "remote-agent transport is not implemented in v0.1; the hub embeds a local collector. Run cmd/hub instead.")
	os.Exit(0)
}
