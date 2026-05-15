package families

import "context"

// GPU is a stub for GPU monitoring. Future implementation strategy:
//   - Nvidia: nvidia-smi (default) or NVML Go bindings (lower overhead,
//     requires CGO and the nvml library on the host).
//   - AMD: amd_sysfs (read /sys/class/drm/card*/device/) — preferred on Linux.
//     rocm-smi available as a fallback for cards/drivers that don't expose
//     the sysfs interface.
//   - Intel: intel_gpu_top (requires CAP_PERFMON and possibly a kernel param
//     tweak: perf_event_paranoid=2).
//   - Multi-vendor fallback: nvtop, which exposes a uniform interface across
//     all three GPU vendors.
//
// Each GPU emits: usage_pct, mem_used, mem_total, temp_c, power_w, name.
type GPU struct{}

func (g *GPU) Name() string                       { return "gpu" }
func (g *GPU) Collect(ctx context.Context) Result { return Result{Family: "gpu", Err: ErrNotImplemented} }
func (g *GPU) CacheTTL() int                      { return 5 }
