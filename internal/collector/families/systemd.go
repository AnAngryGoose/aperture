package families

import "context"

// Systemd is a stub for monitoring named systemd services. Future
// implementation runs `systemctl list-units --type=service --output=json`,
// filters by host_config.filters.service_patterns (regexes or glob patterns),
// and emits per-unit active_state, sub_state, load_state.
//
// Alert vocabulary: service.<unit>.active_state ("active" | "inactive" |
// "failed" | "activating").
type Systemd struct{}

func (s *Systemd) Name() string                       { return "systemd" }
func (s *Systemd) Collect(ctx context.Context) Result { return Result{Family: "systemd", Err: ErrNotImplemented} }
func (s *Systemd) CacheTTL() int                      { return 10 }
