# Aperture — Technical Reference

Per-package, per-function detail of how aperture is built. This is a living document — every code change should be reflected here in the same commit. See `overview.md` for the user-facing description and `changelog.md` for version history.

**Design constraint that shapes every choice below:** *clean surface, deep ability* — every feature must support a glanceable summary view AND a full-power detail view, with seamless transition between them (see `overview.md` for the full rationale). When evaluating a new package, type, or API: ask which layer it serves, and confirm the other layer has a path forward. A function that locks data into "summary only" or "raw only" is a design smell — raise it before merging.

---

## Backend (Go)

Module: `github.com/aperture/aperture`. Toolchain pinned to Go 1.25 (driven by the `modernc.org/sqlite` dependency).

### `internal/types`

Shared data types used across all backend packages and mirrored to TypeScript in `web/src/lib/types.ts`. Defining them in a leaf package avoids import cycles between `store`, `hub`, `api`, and `collector`.

| Type | Why |
| --- | --- |
| `Host` | Identity + static descriptors for one machine known to the hub. Multi-host from day 1: every host-scoped record everywhere else carries a `host_id` that points here. |
| `MetricSample` | One snapshot of host-level resource usage. Aggregate counters (network bytes, uptime) are stored cumulatively; the UI derives rates client-side so historic samples don't need re-computation if the rate definition changes. |
| `Container` | Docker container observed on a host, including point-in-time stats for running ones. State stored as the docker-native string (`running`, `exited`, `paused`, `dead`, etc.) so we don't have to translate back when issuing actions. |
| `PortMapping` | Subset of docker's port struct — the fields the UI actually needs. |
| `HostInfo` | Static descriptor that a `MetricSource` supplies once on registration. Separated from `Host` so the source doesn't have to know its assigned `host_id`; the hub fills that in. |
| `AlertRule` | One threshold check. `HostID *string` — `nil` means "applies to all hosts". `Severity` is `"info"|"warning"|"critical"` (default `"warning"`); used for per-channel filtering. `DurationS` is the sustained-breach window; `0` fires on the next sample. |
| `AlertEvent` | One firing of a rule. `ResolvedAt *time.Time` is `nil` while the alert is still firing; set when the breach ends. The pointer lets the open-only query stay simple (`WHERE resolved_at IS NULL`). |
| `AlertChannel` | A notification destination. `Type` is `"discord"|"slack"|"ntfy"|"gotify"|"webhook"`. `Config` is raw JSON (type-specific fields). `MinSeverity` filters which rule severities trigger this channel. `NotifyResolve` controls whether resolved events generate a message. |
| `CreateSpec` | Surface-layer container-create request: only image, name, restart policy, env, ports, volumes, and auto-start. Deep config (capabilities, ulimits, healthcheck, security opts, network aliases) is intentionally absent — it lands with the compose-first work in roadmap section 2 where YAML is the natural surface for the long tail of options. |
| `PortBinding` (create-side) | `host_port=0` means "let docker pick"; `container_port` is required; protocol defaults to tcp. Distinct from the read-side `PortMapping` (which mirrors docker's list shape) because create is asymmetric: we send a request, not echo a snapshot. |
| `VolumeBinding` (create-side) | Bind mount with optional `read_only`. v0.1 surface only handles bind mounts — named-volume support is part of the deep volume-management work in roadmap section 3. |
| `SystemInfo` | Operational snapshot returned by `/api/system/info`: hub version, started-at timestamp, SQLite path, and total on-disk size (sum of the main file plus its `-wal` and `-shm` companions, since between WAL checkpoints the WAL can be a non-trivial fraction of total bytes). Polled by the layout footer; intentionally cheap (one in-memory read + a few `os.Stat` calls). |
| `NetInterfaceSample` | Per-network-interface counters: cumulative rx/tx bytes plus derived rx/tx rates (bytes/s). Rates are computed in the collector from delta/elapsed across consecutive samples. `lo` (loopback) and `veth*` (Docker container virtual links) are filtered out — they're internal and would confuse the UI. |
| `DiskMountSample` | One mounted filesystem: device, mount point, fstype, used bytes, total bytes, and usage percent. Pseudo-filesystems (sysfs, proc, devtmpfs, overlay, squashfs, cgroup, etc.) and Docker overlay mounts are filtered in the collector. Only real user-owned mounts appear. |
| `DiskIOSample` | Per-block-device read/write: cumulative bytes plus derived read/write rates. Loop devices, RAM disks, and zram are filtered. Sorted by device name for stable UI ordering. |
| `TempSample` | One hardware temperature sensor: sensor key and celsius reading. Sourced from `sensors.TemperaturesWithContext` (gopsutil v4 moved this from `host` to a separate `sensors` package). May be empty on VMs or hosts without readable sensors — callers treat an empty slice as "unavailable", not "error". |
| `ProcessSample` | One snapshot of a running process: pid, name, CPU percent (measured since the previous tick, not since process creation), memory percent, and RSS bytes. Live-only — never stored in SQLite. `MetricSample.Processes` is `omitempty` so historical samples are unchanged. Processes are collected via a cached `map[int32]*gopsprocess.Process`; caching is required so `CPUPercent(0)` measures elapsed time on the same object across ticks. |
| `NetIfaceHistory` / `NetIfaceSeries` | Pivoted history response: one `timestamps []int64` array (Unix seconds) shared across all series, and a `map[string]*NetIfaceSeries` with per-interface `rx_bytes` / `tx_bytes` arrays. Client derives rates from delta/elapsed. Returned by `/api/hosts/{id}/metrics/net`. |
| `DiskMountHistory` / `DiskMountSeries` | Same pivot shape for per-mount `used` / `total` byte arrays. Returned by `/api/hosts/{id}/metrics/mounts`. |
| `DiskIOHistory` / `DiskIOSeries` | Same pivot shape for per-device `read_bytes` / `write_bytes` arrays. Returned by `/api/hosts/{id}/metrics/diskio`. |
| `ContainerInspect` | Full container detail for the deep-inspect panel: all fields from `Container` plus timestamps (`CreatedAt`, `StartedAt *time.Time`, `FinishedAt *time.Time`), restart policy, entrypoint, cmd, env, ports, mounts (`ContainerMount`), labels, live CPU/mem/net stats, and editable resource limits (`NanoCPUs`, `MemLimitBytes`). Sourced from `dockerctl.Inspect`, not the list endpoint. |
| `ContainerMount` | One bind/volume/tmpfs mount: type, source, destination, mode, and rw flag. Maps Docker SDK's `types.MountPoint`. |
| `ResourceUpdate` | Live resource-limit patch: `NanoCPUs *int64` and `MemoryBytes *int64`. Pointer fields so `0` means "unlimited" and `nil` means "don't change". Sent as the body to `PUT .../resources`. |

### `internal/store`

SQLite wrapper. Uses `modernc.org/sqlite` (pure-Go) so the binary cross-compiles freely. `schema.sql` is embedded with `//go:embed` and applied unconditionally on `Open` — the schema is idempotent (`CREATE TABLE IF NOT EXISTS`) so this doubles as a lightweight "migration on startup" until enough versions accumulate to need real migrations.

| Function | Use & reason |
| --- | --- |
| `Open(path string)` | Opens a SQLite file with WAL journal mode, foreign keys on, and a 5-second busy timeout. WAL is critical: it lets the metrics ingest loop write while readers (the API) read without blocking each other. The busy timeout absorbs short contention spikes (e.g. retention pruning) so callers don't have to retry. |
| `Close` | Flushes and closes the SQLite handle. Called from `cmd/hub` on graceful shutdown. |
| `Path()` | Returns the on-disk path passed to `Open`. Used by the API to `os.Stat` the database (and its WAL/SHM companions) for size reporting in `/api/system/info`. Keeping it on the store rather than threading the path separately means a future runtime DB swap (not currently supported) only changes one place. |
| `UpsertHost` | Inserts a host row, or updates it when a known host re-registers (e.g. across hub restarts). The `ON CONFLICT(id) DO UPDATE` keeps `created_at` stable while refreshing identity and `last_seen` — important so historical metrics stay linked to a host even if its OS version changed. |
| `TouchHost` | Bumps `last_seen` only. Cheap update called on every metric ingest so the UI can show a recency indicator without inferring it from the metrics table. |
| `ListHosts` | Returns all hosts, sorted by name. Used by the host-list dashboard. |
| `GetHost` | Single-host lookup. Returns `(nil, nil)` for missing hosts so callers can disambiguate "not found" from "DB error". |
| `InsertMetric` | Append a sample to `metrics`. Primary key is `(host_id, ts)` so duplicate timestamps from a faulty source are rejected at the DB layer. |
| `LatestMetric` | Most recent sample for a host. Drives the dashboard's "current state" cards. |
| `MetricsRange` | Time-bounded sample fetch with optional uniform-stride downsampling (`maxPoints`). Stride downsampling is intentional: it's O(n) and doesn't smooth peaks the way averaging would, which matters for spotting spikes. The last sample is always included so the chart's right edge matches the latest sample even when the stride wouldn't otherwise land on it. |
| `InsertNetIfaces(ctx, m)` | Bulk insert of `m.NetIfaces` into `net_iface_metrics` using a transaction of `INSERT OR IGNORE` statements. `ON CONFLICT DO NOTHING` means duplicate timestamps from a faulty source are silently rejected. Called best-effort from `hub.ingestLoop` after `InsertMetric`; failure is logged but doesn't abort ingest. |
| `InsertDiskMounts(ctx, m)` | Same pattern for `disk_mount_metrics`. |
| `InsertDiskIO(ctx, m)` | Same pattern for `disk_io_metrics`. |
| `NetIfaceRange(ctx, hostID, since, until, maxPoints)` | Fetches `net_iface_metrics` rows in range, groups by timestamp in Go, applies stride downsampling on unique timestamps (always keeping the last), and pivots into `*types.NetIfaceHistory` using a `tsIndex` map for O(1) slot lookup when filling pre-allocated series arrays. Returns a non-nil empty struct (not `nil`) when no data exists. |
| `DiskMountRange` / `DiskIORange` | Same downsampling + pivot pattern for their respective tables. |
| `PruneMetrics(cutoff)` | Bulk delete of old samples from all four tables (`metrics`, `net_iface_metrics`, `disk_mount_metrics`, `disk_io_metrics`). Called hourly from `hub.retentionLoop` when a retention duration is configured. Returns total rows-deleted across all tables so the caller can log non-zero prunings. |
| `scanAlertRule(rs)` | Internal helper that scans a row into `types.AlertRule`. `host_id` is a `sql.NullString` because `NULL` legitimately means "all hosts"; this conversion is the only place that abstraction leaks across. Centralizing it stops every list method from reimplementing the same scan shape. |
| `ListAlertRules(hostID *string)` | UI listing. When `hostID` is non-nil, returns only rules that apply to that host (its id or `NULL`). Used by the frontend's per-host filtering and by the global rules table. |
| `ListEnabledRulesFor(hostID)` | Evaluator hot path. Same filter as `ListAlertRules` but restricted to `enabled = 1`. Called once per metric ingest, so the index is implicitly: filter on a small column with a small predicate, scan a small table. At homelab scale a full scan per ingest is fine. |
| `GetAlertRule(id)` | Single-rule lookup. Returns `(nil, nil)` for missing rows so handlers can disambiguate not-found from DB error — same convention as `GetHost`. |
| `CreateAlertRule(rule)` | Insert. `host_id` is passed as `any` so the SQLite driver writes `NULL` when `rule.HostID == nil`. Returns the auto-increment id. The schema's `DEFAULT CURRENT_TIMESTAMP` populates `created_at`; the API layer reads it back so responses include the stamped value. |
| `UpdateAlertRule(rule)` | Full replace by id. Same `host_id` NULL handling as create. We don't `RETURNING *` (modernc supports it but keeping queries portable across the SQL driver line is cheap insurance). |
| `DeleteAlertRule(id)` | Remove a rule. The `alert_events.rule_id` foreign key uses `ON DELETE CASCADE`, so the event history goes with it. The API layer additionally calls `Evaluator.HandleRuleDelete` so transient in-memory state doesn't leak. |
| `InsertAlertEvent(event)` | Persist a fired event. Returns the event id so the evaluator can stash it in its `open` map for the eventual `ResolveAlertEvent` call. |
| `ResolveAlertEvent(id, t)` | Idempotent resolve: `WHERE id = ? AND resolved_at IS NULL`. The IS-NULL guard prevents a double-resolve from rewriting the original timestamp if a race ever happens (today the evaluator's mutex prevents this, but the guard is one tiny clause for a meaningful invariant). |
| `ListAlertEvents(filter)` | Event history with optional `HostID`, `OpenOnly`, and `Limit` (default 200). Sorted `fired_at DESC` so the UI gets newest-first. The default cap is intentional — without it a long-running homelab could cough up thousands of events at once on the alerts page. |
| `AlertEventFilter` (struct) | Bag of filter args for `ListAlertEvents`. A struct (rather than positional args) keeps the call sites self-documenting and lets future fields be added without breaking existing callers. |

### `internal/collector`

Local-host metric source. Implements `hub.MetricSource`. The package documentation explicitly notes that future remote agents produce samples in the same shape and feed the same ingest path — this comment is load-bearing for the multi-host invariant.

| Function | Use & reason |
| --- | --- |
| `NewLocal(interval)` | Constructor with default `DiskPath = "/"`. The disk path is a struct field rather than a flag so per-host overrides become trivial when remote agents land. |
| `(*Local).HostInfo` | Builds the `HostInfo` descriptor from `host.Info`, `cpu.Info`, and `mem.VirtualMemory`. Cached in `hostInfo` after the first call (cleared only by re-creating the collector) so repeated registrations don't re-syscall. |
| `(*Local).Run(ctx, out)` | The collection loop. Sends one sample immediately, then on every tick. Uses a select+default `send` (see below) to drop on backpressure rather than block — losing a sample is preferable to stalling collection if the consumer is slow. Cancels cleanly on `ctx.Done()`. |
| `send(out, s)` | Internal non-blocking channel send. Reason: collection cadence must be predictable; if the receiver wedges, dropping samples is the right behavior. |
| `(*Local).sample(ctx)` | One sampling pass. Each metric is independently fetched and silently zeroed on error so a single broken probe (e.g. unreadable swap on minimal containers) doesn't poison the whole sample. Now also calls `diskMounts`, `netIfaces`, `diskIO`, temperature, and per-core collectors to populate the rich live-only fields on `MetricSample`. |
| `(*Local).diskMounts(ctx)` | Reads `disk.PartitionsWithContext(ctx, true)` (all partitions), filters entries whose fstype is in the `pseudoFS` map (sysfs, proc, devtmpfs, overlay, squashfs, cgroup2, tmpfs, etc.) or whose path contains `/docker/`, `/containerd/`, or `/overlay`. Calls `disk.UsageWithContext` per surviving mount. **Why:** docker overlay mounts flood the list otherwise; filtering by both fstype and path handles overlayfs mounts that escape the fstype filter. |
| `(*Local).netIfaces(ctx, dt)` | Reads per-interface counters via `net.IOCountersWithContext(ctx, true)`. Skips `lo` and any interface starting with `veth`. Computes rates using `prevNetIO` delta / elapsed seconds. Updates `prevNetIO` after each call. |
| `(*Local).diskIO(ctx, dt)` | Reads `disk.IOCountersWithContext(ctx)` (no filter arg — the full map). Skips devices starting with `loop`, `ram`, or `zram`. Computes read/write rates from `prevDiskIO` delta / elapsed. Sorts results by device name for stable ordering. |
| `(*Local).processes(ctx)` | Collects a live process list and returns the union of top 20 by CPU + top 20 by RSS (up to 40 total). Maintains a `procCache map[int32]*gopsprocess.Process` across ticks (protected by `procMu`): dead PIDs are evicted, new PIDs are added via `NewProcessWithContext`. Calling `CPUPercentWithContext(ctx)` on the cached object measures elapsed time since the *previous* call on that same object — this is the correct way to get an accurate CPU reading, matching `top`. First tick for a newly-started process reports CPU=0 (acceptable). Processes that fail any stat call (e.g. permission denied, process exited mid-collection) are silently skipped. |

`cpu.Percent(0, false)` is primed once at the start of `Run` because gopsutil's CPU percentage requires a baseline reading; without priming the very first sample reports 0%.

### `internal/dockerctl`

Docker engine wrapper. Bound to a specific `host_id` at construction so multi-host doesn't leak into the call sites — each host's docker access is one `*Client`.

| Function | Use & reason |
| --- | --- |
| `New(hostID)` | Connects via env (DOCKER_HOST and friends), negotiates API version. The negotiation matters because the daemon may be older than the SDK we're built against. |
| `Close` | Releases the underlying transport. |
| `Ping` | Verifies daemon reachability. Called once on startup so the hub can log a clear warning instead of getting opaque errors per request. |
| `List(ctx, all)` | Lists containers and inlines per-container stats for running ones. Stats are fetched via `ContainerStatsOneShot` to avoid keeping a streaming connection per container. The trade-off is one extra round-trip per running container per UI refresh; acceptable for homelab scale. |
| `stats(ctx, id)` | Calls one-shot stats endpoint and unmarshals into `container.StatsResponse`. |
| `computeStats(v)` | Translates docker's raw counters into UX-friendly numbers: CPU percent normalized to total cores (so 200% on a 4-core host is meaningful, not capped at 100%); memory excluding `cache` (matching `docker stats`); network sums across all interfaces. |
| `Create(ctx, spec)` | Create a new container from a `types.CreateSpec` and (optionally) start it. If the image is not local, pulls it once and retries the create. Returns the new container id. Pull-on-not-found rather than always-pull keeps the common case (image already cached) fast while still working transparently for fresh images. The pull progress stream is drained with `io.Copy(io.Discard, …)` so the pull actually completes. If `AutoStart` is true and the start fails, the id is returned alongside the error so the caller can decide whether to leave the half-built container or remove it. |
| `buildCreateConfig(spec)` | Internal helper that translates a `CreateSpec` into the docker SDK's `container.Config` + `container.HostConfig`. Pulled out for testability and to keep `Create` readable: env map → `[]"K=V"`, port bindings → `nat.PortSet` + `nat.PortMap` (with empty host port meaning "docker chooses"), volume bindings → `[]string` in `host:container[:ro]` form, restart policy → `container.RestartPolicy{Name: …}` validated against the docker-supported set. |
| `Start`, `Stop`, `Restart`, `Pause`, `Unpause`, `Kill`, `Remove` | Thin wrappers exposing the docker container lifecycle. Stop/Restart take a `*int` timeout pointer because the Docker SDK distinguishes "default" (`nil`) from "zero" (`*int = 0`, meaning "kill immediately"). |
| `Logs(ctx, id, tail)` | Fetches stdout+stderr with a `tail` limit, then strips docker's 8-byte multiplexed log header so the payload is plain text the UI can render. |
| `stripLogHeaders(b)` | Parses docker's TTY-disabled log framing: a 4-byte stream prefix followed by a big-endian length, repeated. Without this, raw output contains binary control bytes. |
| `Inspect(ctx, id)` | Returns a full `*types.ContainerInspect` via `ContainerInspect` + one-shot stats. `buildInspect` maps Docker SDK's `types.ContainerJSON` to `ContainerInspect`: timestamps (`started_at`, `finished_at` as `*time.Time` so zero-value maps to nil), ports from `NetworkSettings.Ports`, mounts from `Mounts`, config from `Config`, and live stats from `computeStats`. **Why:** the list endpoint returns only the fields needed for the table row; the inspect endpoint pays the extra round-trip to get everything the deep-inspect panel needs. |
| `UpdateResources(ctx, id, update)` | Calls Docker SDK's `ContainerUpdate` with `container.Resources{NanoCPUs, Memory}`. Only sets the fields present in `update` (nil pointer means "don't change"). **Why:** live cgroup updates don't require a stop/restart; surfacing this as a separate action from recreate gives the operator a low-cost option for temporary tuning. |
| `inspectToSpec(inspect)` | (in `internal/api`) Converts a `ContainerInspect` into a `CreateSpec` for the recreate flow. Strips the leading `/` from docker's name format, converts `env []string` back to a `map[string]string`, maps `PortMapping` to `CreatePortBinding`, and maps `ContainerMount` to `VolumeBinding`. **Why:** keeping the translation in the API layer (not dockerctl) means dockerctl stays decoupled from the create-side types. |
| `FilterRunning(in)` | Helper for callers who want only the running subset. Currently unused by the API but kept because the alerting work (next) needs to scope alerts to running containers. |
| `FindByName(ctx, name)` | Resolves a container name to an ID via the docker filter API. Used by future container-create flows; still useful enough to keep around. |

### `internal/alerts`

Threshold-based rule evaluator. Hooked into `hub.ingestLoop` so every persisted sample is checked against every applicable rule. Designed to be cheap: rules are queried per-host on the hot path, so cost scales with rules-per-host rather than total rules.

**State model.** Two tables of state, one persistent and one transient:

- *Persistent* (SQLite): `alert_rules` is the configuration; `alert_events` is the history (including currently-open events with `resolved_at IS NULL`).
- *Transient* (memory): per-`(rule_id, host_id)`, when did we first observe the breach? Needed to enforce `duration_s` semantics. Not persisted because doing so would mean a SQLite write *per rule per sample* — a hot-path cost we don't need. The cost of losing it across a hub restart is bounded: a sustained breach that hadn't yet fired starts its timer fresh, delaying the fire by at most one duration window. Open events themselves *are* persistent, so a firing alert isn't lost to a restart — only the wait-to-fire timer.

Rehydration: on `New(...)`, the evaluator loads all open events and seeds its `open` map. That way, when the same rule still breaches after a restart, the evaluator sees `open[key]` is set and won't insert a duplicate event.

| Function / type | Use & reason |
| --- | --- |
| `Evaluator` (struct) | Holds the store handle, logger, and the two in-memory maps (`pending`, `open`) protected by a mutex. The mutex is taken per `evalOne`, not per `Evaluate`, so concurrent ingests for *different* hosts can interleave their rule evaluations. |
| `ruleHostKey` (struct) | Map key combining `rule_id` and `host_id`. A struct (rather than a string `"rule_id:host_id"` join) avoids string allocation on the hot path. |
| `New(ctx, store, log)` | Constructs the evaluator and rehydrates `open` from `store.ListAlertEvents(OpenOnly: true, Limit: 10000)`. The 10k cap is a sanity bound — homelab scale is dozens, not thousands; if it's ever exceeded we want a config knob, not a silent slow startup. |
| `Evaluate(ctx, sample)` | Hub calls this after every successful insert. Fetches the host's enabled rules (`ListEnabledRulesFor`) and dispatches each to `evalOne`. Errors during the DB fetch are logged and ignored — losing a tick of evaluation is preferable to crashing the ingest goroutine. |
| `evalOne(ctx, rule, sample)` | The per-rule decision tree. If breaching: skip when already firing, otherwise start a `pending` timer (firing immediately when `duration_s == 0`) and fire once the sustained window has elapsed. If not breaching: clear any `pending` entry and resolve any `open` event. Holds the mutex for the duration so the maps stay consistent. |
| `fire(ctx, rule, sample, val, key)` | Internal helper. Inserts the `alert_events` row, records the new id in `open`, drops the `pending` entry, and emits a `WARN` log line so an operator tailing logs sees alerts without needing the UI. Caller holds the mutex. |
| `HandleRuleDelete(ruleID)` | Drops every `pending` and `open` entry for the deleted rule. Called by the API's DELETE handler so we don't leak transient state for a rule that no longer exists (such state could otherwise sit forever — an event resolved on the next breach end, but a `pending` entry would never resolve at all). |
| `MetricValue(sample, name)` | Translates a metric name (`cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load_1`, `load_5`, `load_15`) to its numeric value on a sample. Returns `(0, false)` for unknown names so the evaluator can warn-log rather than fire spuriously. `swap_pct` divides `swap_used / swap_total` defensively (returns 0 when the host has no swap). |
| `SupportedMetrics` (var) | Canonical metric list. The API exposes it via `/api/alerts/metadata` so the UI dropdown stays in sync without a second source of truth. |
| `compare(v, op, threshold)` | The four-way operator dispatch. Tiny on purpose — keeping it switch-based (rather than a map of funcs) keeps it inlinable and obvious. |
| `fire(ctx, rule, sample, val, key)` + notifier hook | After persisting the event and updating the `open` map, `fire()` calls `go e.notifier.Dispatch(ctx, ev, r, false)` if a notifier is wired in. The resolve path similarly calls `go e.notifier.Dispatch(ctx, ev, r, true)`. Both are goroutines so a slow/hanging HTTP sender can't stall the evaluator's mutex-hold window. |
| `SetNotifier(n)` | Sets the `Notifier` on the evaluator. Called from `cmd/hub/main.go` after constructing both. Nil-safe — tests and scripts that don't need notifications can skip it. |

**Notification delivery (`notify.go` + `ch_*.go`):**

| Name | Purpose |
| --- | --- |
| `Notifier` | Loads enabled channels from the store and dispatches per-channel. `Dispatch(ctx, event, rule, resolved)` loads the host row (for name), filters channels by `SeverityLevel(ch.MinSeverity) <= SeverityLevel(rule.Severity)` and `ch.NotifyResolve`, then fires a goroutine per channel. One DB query for host + one for channels per dispatch; acceptable for low-frequency alert events. |
| `SeverityLevel(s)` | `"info"→0`, `"warning"→1`, `"critical"→2`. Exported so the API can use it for validation if needed. |
| `BuildSender(ch)` | Exported wrapper around `buildSender(ch)` so the `testAlertChannel` API handler can validate a channel's config without a full Dispatch. |
| `DiscordSender` | POSTs a rich embed to a Discord incoming webhook. Embed color is severity-coded (`#e74c3c` critical, `#f39c12` warning, `#3498db` info, `#2ecc71` resolved). |
| `SlackSender` | POSTs a Slack attachment to an incoming webhook. Color maps to `danger/warning/good`. |
| `NtfySender` | Posts to `{url}/{topic}`. Priority auto-mapped (critical→urgent, warning→high, info→default, resolved→low). Tags use ntfy's built-in emoji tags (`rotating_light` / `white_check_mark`). Optional bearer-token auth. |
| `GotifySender` | Posts to `{url}/message?token={token}`. Priority auto-mapped (critical→10, warning→5, info→1, resolved→2). |
| `WebhookSender` | POSTs (or configured method) a structured JSON payload to any URL. Optional `headers` map applied to the request. Payload includes `type` (`alert_fired`/`alert_resolved`), `host`, `rule`, `event`, and `resolved_at`. |
| `SupportedOps` (var) | Canonical op list, exposed alongside `SupportedMetrics` in the metadata endpoint. |
| `ValidateRule(rule)` | Centralized validation: non-empty metric, metric in `SupportedMetrics`, op in `SupportedOps`, `duration_s >= 0`. Both create and update call this before touching the DB so invalid rules can never be persisted. |
| `contains(xs, x)` | Tiny linear-search helper. The metric/op slices are small fixed sets; a map would be over-engineered. |

### `internal/hub`

Orchestration layer. Owns the host registry, the central metric ingest channel, retention, and the docker-provider lookup table.

| Type / func | Use & reason |
| --- | --- |
| `MetricSource` interface | The seam for "where metrics come from". v0.1 has one impl (`collector.Local`); the remote-agent transport will be a sibling. The interface is intentionally tiny (`HostInfo` + `Run`) so a transport doesn't have to carry hub-specific concepts. |
| `DockerProvider` interface | The seam for "how the hub reaches a host's docker engine". Mirrors `dockerctl.Client`'s public surface (now including `Create`) so a remote agent will satisfy it over the wire. The compile-time assertion `var _ DockerProvider = (*dockerctl.Client)(nil)` catches drift between the two. |
| `Hub` struct | Holds the store, logger, retention duration, the central `samples` channel (buffered 256), the per-host `dockers` and `hosts` maps, and `latestRich map[string]types.MetricSample` — all protected by an RWMutex. |
| `latestRich` | In-memory map from `host_id` to the most recent full `MetricSample`. Written by `ingestLoop` before the SQLite insert. Rich live-only fields (per-core, per-interface, disk mounts, disk I/O, temps) live here only — the SQLite row has none of them. |
| `LatestSample(hostID)` | RLock-protected read of `latestRich`. Returns `(sample, true)` when present, `({}, false)` otherwise. Used by `latestMetric` to prefer the in-memory rich snapshot over the DB row. |
| `New(cfg)` | Constructor. Slog default is used if no logger is supplied; this keeps tests and quick scripts simple. |
| `(*Hub).Run(ctx)` | Spins up the ingest loop and (when retention > 0) the retention loop, then blocks until `ctx` is cancelled. Returns only after both goroutines exit so `cmd/hub` can rely on the post-Run quiescence. |
| `(*Hub).ingestLoop(ctx)` | Reads samples off `h.samples`, persists each one, bumps `last_seen`, and (best-effort) inserts rich historical data into the three subsidiary tables. After `InsertMetric` succeeds, `InsertNetIfaces`, `InsertDiskMounts`, and `InsertDiskIO` are called when their respective slices are non-empty; errors are logged but don't abort the loop. This means a single disk write failure doesn't poison historical network or mount data. |
| `(*Hub).retentionLoop(ctx)` | Hourly `PruneMetrics` for samples older than `h.retain`. Hourly is a deliberate compromise: frequent enough to keep the table small, infrequent enough not to interfere with reads. |
| `(*Hub).RegisterSource(ctx, src)` | Asks the source for `HostInfo`, derives a stable host_id, upserts the host row, then launches the source's `Run` against a per-source channel adapter that stamps the host_id onto every sample. Returns the host_id so the caller can pair it with a docker provider. |
| `(*Hub).samplesIn(hostID)` | Returns a per-source send channel that stamps `host_id` on samples and forwards to the central channel. Decouples sources from the host_id assignment — sources don't need to know what id they got. Drops on full central buffer with a warning, matching collector backpressure semantics. |
| `(*Hub).RegisterDocker(hostID, p)` / `Docker(hostID)` | Concurrent-safe registry of docker providers by host. The API uses `Docker` to dispatch container endpoints. |
| `ComposeProvider` interface | Six-method seam for compose stack operations: `DiscoverStacks`, `GetStack`, `StackAction`, `Logs`, `ReadFile`, `WriteFile`. Local hosts use `compose.Local`; remote agents satisfy it via `agentComposeProvider` in `agentws.go`. |
| `(*Hub).RegisterCompose(hostID, p)` / `Compose(hostID)` | Parallel to the docker registry. Registered when the local docker socket is available (hub) or when `hello.HasCompose` is true (agent). The API returns 503 if no provider is registered for a host. |
| `(*Hub).Store()` | Exposes the store for the API package. The alternative — passing the store separately — would require keeping two pointers in lockstep; one accessor is simpler. |
| `DeriveHostID(info)` | First 16 hex chars of `sha1(source + "|" + name)`. Stable across restarts so historical metrics stay linked to the same host record. When remote agents land, they will provide their own UUID and this is fallback for the local source only. |
| `Evaluator` interface | One-method seam (`Evaluate(ctx, sample)`) the hub uses to dispatch persisted samples to the alert evaluator. Defined on the hub side (rather than imported from `internal/alerts`) to avoid an import cycle: `alerts` imports `store` for its types and rules, and the hub imports neither. `*alerts.Evaluator` satisfies it; tests can substitute a stub. |
| `Hub.evaluator` field + `SetEvaluator(e)` | The evaluator is settable post-construction (rather than a constructor arg) so call sites that don't have alerts wired — tests, dev scripts — don't have to construct one. `cmd/hub` always sets it before calling `Run`. |
| `ingestLoop` (updated) | After `InsertMetric` + `TouchHost` succeed, dispatches the same sample to `h.evaluator.Evaluate` when set. Errors inside the evaluator are the evaluator's problem (it logs them); the hub does not retry or back off. Keeping evaluation inline (rather than on a separate goroutine) means a fired alert's `fired_at` timestamp lines up tightly with the sample that caused it. |

### `internal/api`

HTTP layer. chi-based, all routes under `/api`. The same handler can optionally serve a SvelteKit static build at `/` with SPA fallback so a single binary covers UI + API.

| Function | Use & reason |
| --- | --- |
| `NewServer(h)` | Constructor. Holds only a `*hub.Hub`; the store is reached via `hub.Store()`. |
| `(*Server).Router(webFS)` | Builds the chi router with standard middleware (RequestID, RealIP, Logger, Recoverer) and the dev-CORS shim. When `webFS != nil` the SPA handler is mounted at `/*`. |
| `spaHandler(webFS)` | Custom file handler that falls back to `index.html` for any path that doesn't resolve to a real file. Required for client-side routing — without this, refreshing on `/hosts/abc` would 404. |
| `health` | Returns `{ok, time}`. Used as a liveness probe. |
| `systemInfo` | Returns `types.SystemInfo` — version (compile-time `cmd/hub.Version`), started-at (recorded in `cmd/hub.main`), DB path from `store.Path()`, and the summed on-disk size of the DB file + `-wal` + `-shm`. Missing companions return 0 size (normal between checkpoints), not an error. The handler is cheap so the layout footer can poll it every 30s without ceremony. |
| `sizeOnDisk(path)` | Internal helper around `os.Stat` that returns 0 (rather than an error) when the file is missing — the WAL / SHM files come and go around checkpoints; absence is normal. |
| `listHosts`, `getHost` | Thin wrappers over `store.ListHosts`/`GetHost`. |
| `latestMetric` | Returns the most recent sample, preferring `hub.LatestSample` (the in-memory rich snapshot) when available. Falls back to `store.LatestMetric` for hosts that haven't sent a sample since the current hub start. Returns JSON `null` (not 404) when none exist. **Why:** the SQLite row never carries rich live-only fields; the in-memory snapshot is the only source for per-core CPU, interfaces, disk mounts, I/O, and temps. |
| `metricsRange` | Parses `range` (default `1h`) and `points` (default `300`) query params, computes `since/until`, and delegates to `MetricsRange`. Empty result returns `[]` rather than `null` so the frontend never has to null-check. |
| `netIfaceHistory` | Same param parsing as `metricsRange`; delegates to `store.NetIfaceRange`. Returns an empty-but-valid `{timestamps:[], ifaces:{}}` (not `null`) when no data exists so the frontend can render a "no data yet" state without null checks. Registered before `/metrics` so chi's static-segment routing takes precedence. |
| `diskMountHistory` / `diskIOHistory` | Same pattern as `netIfaceHistory` for their respective tables. |
| `listContainers` | Looks up the `DockerProvider` for the host or 404s. Returns `[]` on empty for the same reason as above. |
| `containerCreate` | POST handler for `/api/hosts/{id}/containers`. Decodes a `types.CreateSpec`, dispatches to the host's `DockerProvider.Create`. On full success returns `201 Created` with `{id}`. On partial success (created but failed to start) returns `202 Accepted` with `{id, warning}` so the UI can decide whether to keep or remove the half-built container — better UX than 500ing on a self-contained warning. On total failure returns `502 Bad Gateway` with the error message. |
| `containerInspect` | GET `.../inspect`. Returns `*types.ContainerInspect` from `DockerProvider.Inspect`. Distinct from the list endpoint — pays the extra round-trip to supply full config, mounts, env, labels, and live stats needed by the deep-inspect panel. |
| `containerUpdateResources` | PUT `.../resources`. Decodes `types.ResourceUpdate`, dispatches to `DockerProvider.UpdateResources`. Returns `{ok: true}` on success. No restart required — the change is applied via cgroups live. |
| `containerRecreate` | POST `.../recreate`. Calls `Inspect` → `inspectToSpec` → `Stop` → `Remove` → `Create`. Returns `{id}` or `{id, warning}` (202). Rationale: one atomic endpoint avoids client-side stop/remove/create orchestration and partial-recreate state if the connection drops mid-way. |
| `inspectToSpec(inspect)` | Converts a `ContainerInspect` into a `CreateSpec` for recreate. Strips docker's leading `/` from the container name, maps env `[]"K=V"` to `map[string]string`, maps `PortMapping` to `CreatePortBinding`, maps `ContainerMount` to `VolumeBinding`. Lives in `internal/api` (not `dockerctl`) so `dockerctl` stays decoupled from the create-side types. |
| `containerAction` | One handler dispatches start/stop/restart/pause/unpause/kill via a switch. Centralizing keeps URL surface small and lets the UI be uniform. |
| `containerRemove` | DELETE — separated from `containerAction` because its parameter shape differs (`force`, `volumes` query args) and conceptually it's not a state transition. |
| `containerLogs` | Returns `text/plain`. Uses `tail` query param (default 200). Renders directly in a modal on the frontend. |
| `writeJSON`, `writeErr` | Tiny helpers; `writeErr` returns `{error: string}` consistently so the frontend's `api.ts` can extract a message uniformly. |
| `parseDuration(s, def)` | `time.ParseDuration` with a default fallback. Why a wrapper: inline `if s == "" || ...` was repeating; this clarifies intent. |
| `corsForDev` | Allow-lists `localhost`/`127.0.0.1` origins so the SvelteKit dev server can hit the hub during development. In production, same-origin means this is a no-op. Keeping it permanently in the chain (rather than a build flag) avoids the "forgot to enable for dev" trap. |
| `alertsMetadata` | Returns `{metrics, ops}` from the alerts package's canonical lists. Used by the UI to populate dropdowns so the metric-name vocabulary has a single source of truth. |
| `listAlertRules` | Reads the optional `host_id` query and forwards to `store.ListAlertRules`. Returns `[]` (not `null`) on empty so the frontend never null-checks. |
| `alertRulePayload` (struct) | Wire DTO for create + update. Differences from `types.AlertRule`: `host_id` is a plain string (empty = "all hosts" — the empty/NULL mapping happens in `toRule`); `enabled` is a `*bool` so omitting it on create defaults to `true` rather than silently disabling the rule. |
| `(alertRulePayload).toRule(id)` | Converts the wire DTO to a `types.AlertRule`. Promotes empty-string `host_id` to `nil`. |
| `createAlertRule` | Decode → `ValidateRule` → `CreateAlertRule` → `GetAlertRule` to read back, so the response includes `created_at` (DB default). On read-back failure we still return the in-memory rule rather than 500ing — the rule was successfully persisted, the response is just slightly degraded. |
| `getAlertRule` | Single-rule fetch. 404 on missing. |
| `updateAlertRule` | Same shape as create: decode → validate → update → read back. |
| `deleteAlertRule` | Calls `store.DeleteAlertRule` (which cascades event history) then `evaluator.HandleRuleDelete` so transient pending/open entries don't leak. |
| `listAlertEvents` | Builds a `store.AlertEventFilter` from `host_id`, `open`, and `limit` query params. Default `limit` 200; the frontend asks for 100 on the alerts page and 200 for the layout's open-count badge. |

### `cmd/hub`

The hub binary entry point. Responsible for: parsing flags/env, opening the store, constructing the hub, registering the local collector and docker client, starting the HTTP server, and shutting everything down on signal.

| Function | Use & reason |
| --- | --- |
| `main` | The whole startup sequence is intentionally linear (no helper indirection) so you can read it top-to-bottom. Order matters: store before hub, hub before sources, sources before docker, server last, then block on context. Shutdown reverses naturally because of `defer` and signal-cancellation. |
| `envOr(k, def)` | Env-var override with default. Used so flags can be configured via env without pulling in a config library. |
| `parseDurEnv(k, def)` | Same as `envOr` but for durations. |

### `cmd/agent`

Production binary for remote host monitoring. Probes docker and docker compose at startup; sends both capability flags in the hello frame. Read loop handles `docker_req` and `compose_req` frames concurrently (each dispatched to a goroutine so slow compose operations — image pulls, large `up` runs — don't block metric delivery). The agent binary imports `internal/compose` directly and calls `compose.Local` methods to service compose requests, returning structured JSON in `compose_resp` frames.

### `internal/compose`

New package (`compose.go`). `Local` wraps the `docker compose` CLI (or `docker-compose` v1 fallback, detected at `NewLocal()` time). All operations are pure exec-based — no Docker SDK dependency in this package. Core:

| Method | Implementation |
| --- | --- |
| `DiscoverStacks(ctx)` | `docker compose ls --all --format json` → `ParseLS` |
| `GetStack(ctx, project)` | `DiscoverStacks` for working_dir + `docker compose ps --all --format json` → `ParsePS` |
| `StackAction(ctx, project, workingDir, action, service, extraArgs...)` | `docker compose --project-name P [--project-directory D] <action> [flags] [service]`. Flags injected per action: `up` gets `-d --remove-orphans`; `pull` gets `--quiet`. Returns combined stdout+stderr. |
| `Logs(ctx, project, workingDir, service, tail)` | `docker compose logs --tail=N --no-color [service]`. Non-zero exit is tolerated when output is non-empty (stopped stacks). |
| `ReadFile / WriteFile` | `os.ReadFile` / `os.WriteFile` against the first compose filename found by `FindComposeFile`. `WriteFile` creates the directory and defaults to `compose.yml` when no file exists. |
| `ParseLS(stdout)` | Handles `[]lsEntry` JSON array. Infers `WorkingDir` from `ConfigFiles` (dirname of first path). |
| `ParsePS(stdout)` | Handles both JSON array (Compose v2.23+) and NDJSON (older). Sorts services by name. |

---

## Frontend (SvelteKit + Svelte 5 runes)

Project at `web/`. Output is a static SPA in `web/build/`, served by the hub at `/`. Dev mode runs Vite at :5173 and proxies API calls to the hub.

### `src/lib/types.ts`

Hand-mirrored TypeScript versions of the Go `types` package. Manual sync is intentional for v0.1 — codegen will pay off once the type list grows beyond a half-page or once a third client appears. Includes `AlertRule`, `AlertEvent`, `AlertMetadata`, `CreateSpec`, `CreatePortBinding`, `CreateVolumeBinding`, and the rich live-metric types: `NetInterfaceSample`, `DiskMountSample`, `DiskIOSample`, `TempSample`, `ProcessSample`. Also `ContainerMount`, `ContainerInspect`, `ResourceUpdate` for the deep-inspect and resource-edit flows. History response types: `NetIfaceSeries`/`NetIfaceHistory`, `DiskMountSeries`/`DiskMountHistory`, `DiskIOSeries`/`DiskIOHistory`. `MetricSample` carries the optional rich live fields matching the Go type (`cpu_per_core?`, `net_interfaces?`, `disk_mounts?`, `disk_io?`, `temps?`, `mem_avail?`, `mem_cached?`, `processes?`). Note: `CreatePortBinding` and `CreateVolumeBinding` are deliberately separate from the read-side `PortMapping` because create is asymmetric (we *send* a binding spec, not echo a docker snapshot).

### `src/lib/api.ts`

Typed HTTP client. The base URL is resolved at build time:

- `VITE_API_BASE` env var, if set (lets you split UI and API in production).
- Else `http://localhost:8080` when `import.meta.env.DEV` is true (hub running separately during dev).
- Else empty string (same-origin, the default production setup).

Every method delegates to one of four private helpers (`get`, `post`, `del`, `send`) that throw on non-2xx with the response body included so UI error messages stay informative. `send` is a JSON-bodied helper used by `POST /alerts/rules` and `PUT /alerts/rules/{id}` — it sets `Content-Type` and stringifies the body so route call sites stay short.

Alert-related methods: `alertMetadata`, `alertRules(hostID?)`, `createAlertRule(rule)`, `updateAlertRule(id, rule)`, `deleteAlertRule(id)`, `alertEvents({hostID?, openOnly?, limit?})`. The events query helper builds a `URLSearchParams` so callers don't worry about encoding. Container methods: `createContainer(hostID, spec)` returns `{id, warning?}`; `containerInspect(hostID, cid)` returns `ContainerInspect`; `containerUpdateResources(hostID, cid, update)` uses PUT and returns `{ok: boolean}`; `containerRecreate(hostID, cid)` returns `{id, warning?}`. History methods: `netHistory(id, range, points)` → `NetIfaceHistory`, `diskMountHistory(id, range, points)` → `DiskMountHistory`, `diskIOHistory(id, range, points)` → `DiskIOHistory`.

### `src/lib/format.ts`

`formatBytes`, `formatPct`, `formatDuration`, `relTime`, `absTime`, `formatBytesRate`. All defensive against `NaN` / `Infinity` because samples can be sparse (e.g. before the first reading) and we don't want `NaN%` in the UI. `formatDuration` includes seconds for durations under one minute. `absTime` returns a locale-formatted absolute timestamp string used in `title` attributes so every `relTime(...)` span shows the full date/time on hover. `formatBytesRate` auto-scales bytes/s → KiB/s → MiB/s → GiB/s.

### `src/lib/types.ts` — `AgentToken`

`AgentToken { id, name, created_at, last_used?, revoked, token? }`. The `token` field is only populated on creation (the server never stores or re-returns the plaintext). The UI renders it once in the wizard's copy-command step.

### Agent transport

The hub exposes `GET /api/agents/ws`. Agents dial this endpoint with `Authorization: Bearer <token>` on the HTTP upgrade request. The token is verified against the `agent_tokens` table (SHA-256 hash comparison); on mismatch the upgrade is rejected with 401.

**WS frame protocol** (JSON, framed by coder/websocket):

| Direction | Frame type | Purpose |
|---|---|---|
| Agent → Hub | `hello` | HostInfo + version + has_docker; must be first frame (10s timeout) |
| Hub → Agent | `ack` | Confirms host_id assigned |
| Agent → Hub | `metric` | One MetricSample per interval |
| Agent → Hub | `heartbeat` | Every 5s; hub calls TouchHost |
| Hub → Agent | `docker_req` | Docker operation (action, cid, params JSON) |
| Agent → Hub | `docker_resp` | Result of docker operation (ok, data, error) |

**`AgentHandler`** (`internal/hub/agentws.go`) manages the session map (`host_id → *agentSession`). On disconnect: docker provider is deregistered, all in-flight `docker_req` pending channels are drained with an "agent disconnected" error (callers unblock immediately).

**`agentDockerProvider`** implements `hub.DockerProvider` by forwarding all 11 methods over the WS. Uses an atomic counter for req_id and a `map[string]chan dockerRespFrame]` pending map. 30s per-command timeout.

**Agent binary** (`cmd/agent`) uses the same `collector.Local` and `dockerctl` packages as the hub's embedded collector. Reconnects with 2s→60s exponential backoff. Sends heartbeats every 5s independently of the metric interval.

### Agent onboarding flow

1. User navigates to Settings (nav link in header).
2. Clicks "+ Add agent" → two-step wizard:
   - Step 1: name (e.g. `nas-box`) → Generate Token → API `POST /api/agents/tokens`
   - Step 2: ready-to-paste command in a code block. Tab toggle: binary vs Docker variant. `window.location.origin` is used as the hub URL. Copy button. One-time token warning banner.
3. Token is shown once and never retrievable again. Revoke button in the token table cuts off the agent.

### `src/lib/toast.ts` + `src/lib/Toast.svelte`

Global toast notification system. `toast.ts` is a Svelte writable store of `{ id, message, kind }` records. Three helpers — `toast.info`, `toast.success`, `toast.error` — push a toast and schedule auto-dismiss (4s for info/success, 6s for errors). `Toast.svelte` is a fixed-position stack (bottom-right, `z-index: 1000`) with a slide-in animation and a per-toast dismiss button. It is mounted once in `+layout.svelte` so any page can call `toast.*` without re-mounting the component. Replaces bare `alert()` / `confirm()` calls in container management actions.

### `src/lib/styles.css`

Global stylesheet. Defines a dark theme via CSS custom properties (`--bg`, `--text`, `--accent`, etc.), generic primitives (`.card`, `.bar`, `.pill`, `.grid`), and pulls in `uplot/dist/uPlot.min.css`. Custom-property approach keeps theming pluggable later.

### `src/lib/Bar.svelte`

Tiny progress bar. Takes `value` and optional `warn`/`bad` thresholds; switches color when crossing them. Used in host cards and container memory rows so the eye instantly catches saturated resources.

### `src/lib/Chart.svelte`

uPlot wrapper. Reasons for choosing uPlot: ~45 KB minified, draws thousands of points without a measurable hit, and exposes the underlying chart for future zoom/pan. The wrapper:

- Takes `x` (timestamps in seconds), one or more `series`, and optional `yMin/yMax/title/valueSuffix/valueFormatter`. When `valueFormatter` is provided it replaces the default `Math.round(v) + valueSuffix` on Y-axis ticks — used by memory/disk charts to render "4.2 GiB" and network charts to render "3.1 MiB/s" without baking unit awareness into the generic component.
- Builds options once on mount, then updates data via `plot.setData` on each prop change — no remount per refresh, so animations feel fluid.
- Listens via `ResizeObserver` and calls `plot.setSize` so charts re-flow when the window resizes.
- Cleans up the plot and observer on destroy.
- **Disables uPlot's built-in legend** (`legend: { show: false }`). Earlier versions left it on, which caused two layout problems: single-series charts spent a row on the redundant `time -- [hover]` line, and multi-series charts (network rx/tx, load 1/5/15) stacked the rows vertically and bled into the next chart card's header. Instead, the wrapper renders a compact chip-row legend (color dot + label) above the canvas only when there are 2+ series. Hover values still work via uPlot's cursor.
- **uPlot's stylesheet must be imported via JavaScript** (we do this in `src/routes/+layout.svelte`: `import 'uplot/dist/uPlot.min.css';`), *not* via a CSS `@import` rule placed after other declarations. Per the CSS spec, `@import` is only valid at the top of a stylesheet (before any rule), and bundlers silently drop misplaced imports. We hit exactly that regression once: the chart canvas ended up stacked *below* the in-flow `u-under` element instead of overlaid on `u-wrap`, offsetting it by the plot-area height (~133 px for a 200 px chart) and overflowing into the next chart card. The chart still half-rendered (axes, data) — just shifted — which is exactly the kind of failure that's hard to spot. JS-importing the stylesheet bypasses the source-order rule entirely.
- The host page's chart-title `<div>` above each chart is intentional: it's the persistent label users read at a glance. The chip-row sits between that title and the canvas only when needed.

### Routes

| Route | Use & reason |
| --- | --- |
| `+layout.ts` | `ssr = false; prerender = false`. Pure SPA: no server rendering, no build-time prerender. The data is live, the hub serves the static fallback `index.html`, the client takes over from there. |
| `+layout.svelte` | Top nav with the brand and links to "Hosts" and "Alerts". Sticky header so navigation stays in view while scrolling long pages. Polls `/api/alerts/events?open=true` every 5s and renders a red `.badge` next to the Alerts link with the firing count when > 0. The badge is in the layout (rather than the alerts page) so it's visible from anywhere in the app — the main reason to have an alert system in the first place. **Footer** polls `/api/system/info` every 30s and shows `vX.Y.Z · DB <size> · uptime <duration>`. A 1s in-memory clock is used to tick the uptime visibly without re-hitting the API. The DB-path tooltip on hover gives the absolute file path for users who want to know where state is stored. Why a footer rather than a settings page: surface-layer info should be reachable from anywhere, and a single info row beats hiding it behind a click. |
| `alerts/+page.svelte` | The alerts management page. Three tabs: Rules, Events, Channels. **Rules tab**: new rule form (host selector, metric/op dropdowns from `/api/alerts/metadata`, threshold, duration, severity selector), rules table (severity badge, toggle, delete; row tinted red when firing). **Events tab**: event history with state pill, host/rule/value, relative timestamps. **Channels tab**: card list of notification channels (Discord/Slack/ntfy/Gotify/webhook) with type badge, min-severity filter, resolve-notify state, and Test/Edit/Enable/Delete actions; add/edit channel modal (type selector, name, type-specific config fields, min severity, notify-resolve toggle). ESC closes the channel modal via `<svelte:window onkeydown>`. Page title set via `<svelte:head>`. Auto-refresh every 5s. |
| `+page.svelte` (host list) | Every 5s: fetch `/api/hosts`, then in parallel fetch `latestMetric`, `containers`, and open `alertEvents`. **Host status pills** (online < 15s / stale < 90s / offline ≥ 90s) computed from `last_seen`. **Alert badges** show firing count per host. **Network rate** (↓/↑) in footer when either direction > 500 B/s — derived from consecutive `latest`/`prevLatest` sample deltas. Card border tints: stale = amber, offline = red, alert = red. `absTime` tooltip on the "seen X ago" span. Dynamic page title. |
| `hosts/[id]/+page.svelte` | Host detail. Fetches host, metrics history, latest, net/mount/diskIO history, and open alerts for this host in parallel. **Alert banner** (red) lists firing count + metric names with a link to `/alerts`. **Stale/offline banners** (amber/red) shown when `last_seen` age >= 15s/90s. **Status pill** in h1. Dynamic title (`Aperture — {host.name}`). `absTime` tooltip on relative-time spans. All other monitoring sections unchanged. |
| `hosts/[id]/containers/+page.svelte` | Container management. Filter controls (all / running / exited / paused), **text search box** (filters by container name and image, case-insensitive, integrated into `filtered()` derived state). Sort controls (Name / State / CPU / Mem). ESC closes logs modal → create modal → inspect panel (priority order) via `<svelte:window onkeydown>`. Dynamic page title includes host name (fetched once on mount). `absTime` tooltip on container age. Inspect panel, logs modal, and create modal otherwise unchanged. |
| `hosts/[id]/compose/+page.svelte` | **Compose stack management.** Auto-discovers all stacks via `GET /api/hosts/{id}/compose` on mount with 8s auto-refresh. Each stack is a collapsible card: colored status dot, project name + working dir path, `N/N running` service count badge, status pill (running/partial/stopped), four quick-action buttons (▶ Up, ⏹ Down, ↺ Restart, ⬇ Pull), expand chevron. Action stdout appears inline below the card. **Expanded** shows a 3-tab detail panel: **Services** (table: service name, short container ID, state pill, health badge, human-readable status, ports, per-service restart/stop-start/logs actions), **Compose File** (monospace YAML textarea loaded on demand; toolbar: reload, Save, Save + Deploy; dirty indicator when modified), **Logs** (service selector, tail-count selector, Refresh; scrollable pre block). **Down…** button opens a confirm modal with optional --volumes checkbox. **New Stack modal**: directory path, YAML editor pre-filled with a working template, "Start immediately" toggle. 503 from the API (compose not available) shows a styled banner. ESC closes modals; `<svelte:head>` title. |
| `hosts/[id]/networks/+page.svelte` | Placeholder. Shows "Docker network inspection, creation, and management coming in a future release." Satisfies the sub-nav link so it doesn't 404; implementation is roadmap section 3. |
| `hosts/[id]/volumes/+page.svelte` | Placeholder. Same pattern as networks. |
| `hosts/[id]/images/+page.svelte` | Placeholder. Same pattern. |
| `hosts/[id]/logs/+page.svelte` | Placeholder. Notes that container logs are already accessible on the Containers tab. |

`uPlot.AlignedData` is the expected data shape (one x-array followed by N y-arrays); the wrapper builds it from props each render.

---

## Data flow (one full request)

1. `Local.Run` produces a `MetricSample` every `interval`.
2. The collector sends it (non-blocking) on the per-source channel returned by `Hub.samplesIn`.
3. That goroutine stamps `host_id` and forwards to the central buffered `samples` channel.
4. `Hub.ingestLoop` receives, calls `store.InsertMetric`, then (best-effort) `InsertNetIfaces` / `InsertDiskMounts` / `InsertDiskIO` for the rich history tables, then `store.TouchHost`, then `evaluator.Evaluate(ctx, sample)` (when set). The evaluator does its own per-rule DB reads and event writes inside that call.
5. The frontend polls `/api/hosts/{id}/metrics?range=…` every 5s and `/api/alerts/events?open=true` every 5s (from the layout, for the firing-count badge).
6. `api.metricsRange` calls `store.MetricsRange`, which downsamples in Go before serializing.
7. `Chart.svelte` calls `plot.setData` with the new arrays; uPlot redraws.

This whole loop avoids per-request locks beyond the SQLite WAL and the small RWMutex on the docker registry — the central `samples` channel is the only synchronization point on the hot path.

## Configuration knobs that matter

- `interval` — controls write rate to SQLite. At `1s` and 14d retention you get ~1.2M rows; SQLite handles this comfortably but charts default to `points=300` to keep response sizes bounded. Default is `5s` for a balance.
- `retain` — older samples are deleted hourly. Set to `0` to disable pruning entirely (use with care; the table grows unboundedly).
- `disk-path` — used for the historical disk-usage metric (stored in SQLite). Live disk mounts (all real mounts) are now collected separately in `diskMounts` independent of this flag and surfaced in `/metrics/latest`.

## Known design choices worth flagging

- **No SSE/WebSocket yet.** Polling is simpler and the data is small. We'll switch to push when remote agents land — they'll already be pushing, so the hub will push to the UI naturally then.
- **Auth absent.** Single-user homelab assumption. Adding it later is non-disruptive because the API is namespaced under `/api` and middleware injection is trivial in chi.
- **No embedded frontend yet.** `-web-dir` is the seam. `embed.FS` lands when the project is more stable; embedding now would slow Go-only iteration cycles.
