package families

import "context"

// SMART is a stub for disk-health monitoring via smartctl. Future
// implementation will parse `smartctl --all --json` output for the device
// list supplied via host_config.filters.smart_devices, falling back to a
// best-effort discovery of /dev/sd* and /dev/nvme* when the list is empty.
// Supported device types: SATA/ATA, NVMe, eMMC (Linux only), mdraid.
// Each device emits temp_c, power_on_hours, drive_failing bool,
// reallocated_sectors, and the raw smart attributes for the deep panel.
// Alert vocabulary: smart.<device>.failing (boolean).
type SMART struct{}

func (s *SMART) Name() string                       { return "smart" }
func (s *SMART) Collect(ctx context.Context) Result { return Result{Family: "smart", Err: ErrNotImplemented} }
func (s *SMART) CacheTTL() int                      { return 60 } // smartctl is slow; once a minute is fine
