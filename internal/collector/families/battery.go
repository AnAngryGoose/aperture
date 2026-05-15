package families

import "context"

// Battery is a stub for laptop / UPS battery monitoring. Future
// implementation reads /sys/class/power_supply/BAT*/ (charge_now,
// charge_full, status, cycle_count). Emits charge_pct, status
// (charging/discharging/full/unknown), cycle_count.
type Battery struct{}

func (b *Battery) Name() string                       { return "battery" }
func (b *Battery) Collect(ctx context.Context) Result { return Result{Family: "battery", Err: ErrNotImplemented} }
func (b *Battery) CacheTTL() int                      { return 30 }
