// Package families defines the contract for optional metric collectors
// (SMART, GPU, battery, systemd, etc.) that extend the base local collector.
//
// The base collector (internal/collector.Local.sample) already inlines the
// always-on families: cpu, mem, disk, net, load, temps, processes,
// cpu_per_core, disk_io, mounts, containers. Those don't need this seam.
//
// The families in this package are *opt-in* — off by default in host_config,
// vendor-specific, and each one shells out to an external tool. Wiring one in
// is intentionally cheap: implement MetricFamily, register it in
// collector.Local.optionalFamilies, and add the key to the user-facing
// catalog. No core code changes required.
//
// Stubs in this package return (nil, ErrNotImplemented). They exist so the
// host_config.enabled_families list and the /api/monitoring/catalog endpoint
// can reference them as known-but-disabled families today.
package families

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by stub families that aren't built out yet.
var ErrNotImplemented = errors.New("collector family not yet implemented")

// Result is whatever a family collected this tick. Each family defines its
// own concrete payload type embedded behind the Data interface{}; the base
// collector unpacks it by type-assertion. Empty/zero-value Result means
// "nothing to report this tick" (legitimate — a battery family on a desktop).
type Result struct {
	Family string      // family key, e.g. "smart", "gpu"
	Data   interface{} // family-specific payload
	Err    error       // non-fatal collection errors
}

// MetricFamily is the contract for an opt-in collector. Collect runs each
// tick (subject to CacheTTL). Families MUST be safe for concurrent Collect
// calls returning before the next tick.
type MetricFamily interface {
	// Name returns the stable identifier used in host_config.enabled_families
	// and on the wire. e.g. "smart", "gpu", "battery", "systemd".
	Name() string

	// Collect runs one sampling pass. ctx carries any deadline.
	Collect(ctx context.Context) Result
}

// CacheTTLer is an optional interface a family can implement to declare its
// own cadence. Expensive probes (smartctl, nvidia-smi) typically only need
// re-running every minute or two, not every 5s sample.
type CacheTTLer interface {
	// CacheTTL is how long a Result remains fresh before another Collect is
	// scheduled. Zero means "every tick".
	CacheTTL() int // seconds
}
