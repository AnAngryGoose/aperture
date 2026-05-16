# Aperture — Technical Reference

Per-package, per-function detail of how aperture is built. This is a living document — every code change should be reflected here in the same commit. See `overview.md` for the user-facing description and `changelog.md` for version history.

**Design constraint that shapes every choice below:** *clean surface, deep ability* — every feature must support a glanceable summary view AND a full-power detail view, with seamless transition between them (see `overview.md` for the full rationale). When evaluating a new package, type, or API: ask which layer it serves, and confirm the other layer has a path forward. A function that locks data into "summary only" or "raw only" is a design smell — raise it before merging.

---

## v0.4.1 — Monitoring rewrite at a glance

A six-compartment effort landed in v0.4.1-alpha.1; the per-package sections below have the function-level detail. High-level map of what moved:

- **`host_config` is now the source of truth** for per-host monitoring policy (sample interval, enabled collector families, NIC/sensor/mount allow-deny filters, `mem_calc` mode, retention + per-table overrides, warn/crit thresholds). Persisted in SQLite; pushed to the running collector or remote agent on every `PUT /api/hosts/{id}/config`.
- **`internal/agentproto.TypeConfig`** is the new wire frame that carries that policy from hub to agent at connect time and on every edit. Agent's read loop calls `collector.Local.ApplyConfig` on receipt — takes effect on the next sample tick.
- **`internal/hub.ConfigPusher` + `LocalApplier` seams** route `host_config` changes to the right transport (agent handler for remotes, in-process collector for the hub's own host). `cmd/hub` wires both at startup.
- **Five new SQLite tables** (`temp_metrics`, `cpu_core_metrics`, `process_metrics`, `container_metrics`, `host_config`) plus a hot-path index `idx_alert_rules_eval`. Store gets matching `Insert{Temps,CPUCores,ProcessSnapshot,ContainerMetrics}` and `{Temp,CPUCore,Process,ContainerMetric}Range` helpers, plus `PruneHostMetrics` for per-host scoped retention and `Get/SetMonitoringDefaults` for user-customizable admin defaults overlaid via `user_settings['monitoring.defaults']`.
- **`internal/collector/families/`** (new subpackage) holds the `MetricFamily` interface plus stubs for `smart`, `gpu`, `battery`, `systemd` — registered in `/api/monitoring/catalog` as `experimental:true` so the UI can list them but they currently return `ErrNotImplemented`.
- **`internal/collector.Local`** gains `ApplyConfig`, family enablement gates (one `if l.familyEnabled("...")` per block in `sample()`), filter application (`allowName(name, allow, deny)` shared between NIC/sensor/mount/container), `mem_calc=avail` mode for ZFS-ARC-aware hosts, a heap-based top-N processes (`procCPUHeap`, `procRSSHeap` — O(N log K) instead of two full sorts), and a 30s mount-list cache.
- **`internal/alerts`** gains the dotted-target evaluator (`iface.<name>.{rx_rate,tx_rate,rx_bytes,tx_bytes}`, `mount.<path>.{pct,used,total}`, `temp.<sensor>.value`, `proc.<name>.{cpu_pct,mem_pct,mem_rss}`, plus `temp.max` and `host.status`), `==` / `!=` ops, in-memory rules cache invalidated on mutation, `StatusProvider` seam for `host.status` resolution, `EvaluateStatus(ctx, hostID, ts)` for transition-driven evaluation, three predefined templates (`Templates()`), and a single Shoutrrr dispatcher (`ch_shoutrrr.go` + URL translators) that replaced the four hand-rolled `ch_{discord,slack,ntfy,gotify}.go` files. New `shoutrrr` channel type accepts a raw URL for any of Shoutrrr's 16+ supported services.
- **`internal/hub`** gains the typed SSE v2 envelope (`metric` / `host_status` / `container_summary` / `alert`, backwards-compat flat metric fields), per-host retention loop (iterates hosts and prunes scoped to each), container-summary loop (polls registered docker providers every 15s, broadcasts on change), offline watchdog (every 30s, flips stale hosts to "offline" and asks the evaluator to run host.status rules), and server-side status derivation in `computeStatus` reading per-host warn/crit thresholds.
- **`internal/api/monitoring.go`** (new file) holds the aggregated spine: `/api/monitoring/{overview,catalog}`, `/api/hosts/{id}/monitoring/bundle`, `/api/hosts/{id}/config`, `/api/settings/monitoring-defaults`, `/api/alerts/templates/apply`, and per-metric history endpoints (`metrics/{temps,cpu,procs}`, `containers/{cid}/metrics`). The bundle drives the host detail page in one fetch.
- **Frontend** gets a `monitoringStore.svelte.ts` (single overview fetch + typed SSE consumer + 30s reconciliation poll), a `metricCatalog.ts` (single source of truth for widget picker + rule editor scalars + dynamic RichCard rendering), a rewritten 10-tab host detail page driven by the bundle, a per-card widget picker (`CardConfigModal.svelte`), interactive sparkline tooltips, a two-step alert rule editor (category → target → leaf with host-status special case), and an Apply-template panel on `/alerts`.

The reactive infinite loop fix on the host detail page is documented in the changelog under v0.4.1 "Fixed".

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
| `DockerNetwork` | One docker network: ID, Name, Driver, Scope, Internal, IPAM configs, and Labels. |
| `NetworkContainer` | Sub-type representing a container connected to a network. |
| `NetworkCreateSpec` | Surface-layer network-create request: Name, Driver, Internal, Attachable, IPAM config, and Labels. |

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
| `PruneMetrics(cutoff)` | Legacy bulk delete from the original four metric tables. Kept for backwards-compat callers; v0.4.1 retention runs via `PruneHostMetrics` instead so per-host policies aren't constrained by the largest retention. |
| `PruneMetricsPerTable(cutoffs)` | Global per-table prune. Takes a `map[table]time.Time` and runs `DELETE … WHERE ts < ?` for each known metric table. `isMetricTable` allow-lists tables to prevent injection of arbitrary names. |
| `PruneHostMetrics(hostID, cutoffs)` | Per-host scoped delete used by the v0.4.1 retention loop in `hub.pruneAllHosts`. Lets host A's 7-day temp retention prune without waiting for host B's 30-day retention to expire. |
| `InsertTemps / InsertCPUCores / InsertProcessSnapshot / InsertContainerMetrics` | Bulk-insert helpers for the rich-history tables added in v0.4.1. Each opens a tx, prepares one statement, iterates the slice. `INSERT OR IGNORE` on the PK collision so duplicate-timestamp ingests are silently rejected at the PK layer. |
| `TempRange / CPUCoreRange / ProcessRange / ContainerMetricRange` | History readers mirroring `NetIfaceRange` / `DiskMountRange` / `DiskIORange`. All share the new `stridePicks(all, maxPoints)` helper for uniform-stride downsampling that always keeps the last timestamp (so charts align to the right edge of the range). |
| `GetHostConfig(hostID) / UpsertHostConfig(cfg)` | Per-host monitoring policy CRUD. Resolution order in GetHostConfig: `host_config` row → `user_settings['monitoring.defaults']` → built-in `DefaultHostConfig`. List- and map-typed fields (`enabled_families`, `family_intervals`, `filters`, `retention_overrides`) are JSON-serialized so the field set can evolve without schema migrations. |
| `GetMonitoringDefaults(ctx) / SetMonitoringDefaults(cfg)` | User-customizable global defaults edited from `/settings`. Stored as a single JSON blob under `user_settings['monitoring.defaults']`. |
| `DefaultHostConfig(hostID)` | Built-in fallbacks: sample_interval 5s, all families enabled, mem_calc "used", retention 30d, warn/crit 70/90 (CPU/mem/disk) and 70/85 (temp). |
| `IsPasswordSet(ctx)` | Returns whether the `auth_config` table contains a password hash row. Used by `requireAuth` to decide whether to enforce auth or pass through (first-run mode). |
| `GetPasswordHash(ctx)` | Returns the stored bcrypt hash, or `("", nil)` when no password is set. |
| `SetPasswordHash(ctx, hash)` | Upserts the single `auth_config` row (id=1). Safe to call on first setup and on password change — the constraint ensures there is always exactly one row. |
| `CreateSession(ctx, token)` | Inserts a new row into `sessions` with `expires_at = now + 24h`. The token is stored as-is (the caller — `authLogin` / `authSetup` — generates a random token; no additional hashing at this layer). |
| `ValidateSession(ctx, token)` | Looks up the token and checks `expires_at > now`. Returns `true` when valid. Does not bump expiry — sessions are fixed-duration for simplicity. |
| `DeleteSession(ctx, token)` | Removes the session row. Called by `authLogout`. |
| `PruneExpiredSessions(ctx)` | Bulk `DELETE FROM sessions WHERE expires_at < now`. Returns the number of deleted rows. Called hourly from `api.PruneSessions`. |
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
| `(*Local).processes(ctx)` | Collects a live process list and returns the union of top-K by CPU + top-K by RSS (K=20) via `topKByCPU` / `topKByRSS` — both use min-heaps (`procCPUHeap`, `procRSSHeap`) so the cost is O(N log K) rather than two full O(N log N) sorts. Maintains a `procCache map[int32]*gopsprocess.Process` across ticks (protected by `procMu`): dead PIDs are evicted, new PIDs are added via `NewProcessWithContext`. Calling `CPUPercentWithContext(ctx)` on the cached object measures elapsed time since the *previous* call on that same object. First tick for a newly-started process reports CPU=0. |
| `(*Local).cachedPartitions(ctx)` | Short-TTL cache (default 30s) over `disk.PartitionsWithContext` — the syscall is expensive and the mount list rarely changes between ticks. `disk.UsageWithContext` is still called fresh per cached mount each tick. |
| `(*Local).ApplyConfig(cfg)` | Swaps in a per-host `types.HostConfig`. Sets `enabledSet` (nil = all on), `filters`, `memCalc`, and overrides `Interval` if `SampleIntervalS > 0`. Safe to call at any time; the new config takes effect on the next tick. Reads under `cfgMu.RLock` from `familyEnabled` / `getFilters` / `getMemCalc` so sampling threads can't starve a config push. |
| `familyEnabled(name)` / `getFilters()` / `getMemCalc()` | Read-side helpers. Each block in `sample()` is gated by `familyEnabled` (e.g. `if l.familyEnabled("temps") { s.Temps = l.tempSensors(ctx) }`). Filter helpers apply allow/deny semantics via the shared `allowName(name, allow, deny)` function (deny wins; empty allow = "all allowed"). |
| `AllFamilies` (var) | Canonical list of inline family keys (cpu, mem, disk, net, load, uptime, temps, processes, cpu_per_core, disk_io, mounts) used as the default when no host_config row exists. |

`cpu.Percent(0, false)` is primed once at the start of `Run` because gopsutil's CPU percentage requires a baseline reading; without priming the very first sample reports 0%.

**`internal/collector/families/`** is a separate subpackage holding the `MetricFamily` interface and stubs for opt-in collectors that shell out to external tools (`smart`, `gpu`, `battery`, `systemd`). Each stub satisfies the interface but returns `Result{Err: ErrNotImplemented}`. The seam exists so adding GPU monitoring is a single new file rather than a refactor — see `families/families.go` for the contract.

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
| `ListNetworks(ctx)`, `InspectNetwork(ctx, id)`, `CreateNetwork(ctx, spec)`, `RemoveNetwork(ctx, id)`, `ConnectContainer(ctx, netID, containerID)`, `DisconnectContainer(ctx, netID, containerID)` | Full suite of network management wrappers translating between SDK types and Aperture's surface types. |

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
| `Evaluate(ctx, sample)` | Hub calls this after every successful insert. Reads the host's enabled rules from the in-memory cache (`rulesFor` — populated lazily, invalidated on rule mutations via `Invalidate()`) and dispatches each to `evalOne`. Errors during the DB fetch are logged and ignored — losing a tick of evaluation is preferable to crashing the ingest goroutine. |
| `EvaluateStatus(ctx, hostID, ts)` | Runs `host.status` rules independent of sample arrival. The hub calls this on every status transition and from the offline watchdog (every 30s on hosts with stale `last_seen`) so an "offline" alert fires even though the host stopped sending samples. |
| `evalOne(ctx, rule, sample)` | The per-rule decision tree. Uses `resolveMetric` (a thin wrapper around `MetricValue` that special-cases `host.status` via the registered `StatusProvider`). If breaching: skip when already firing, otherwise start a `pending` timer (firing immediately when `duration_s == 0`) and fire once the sustained window has elapsed. If not breaching: clear any `pending` entry and resolve any `open` event. Holds the mutex for the duration so the maps stay consistent. |
| `fire(ctx, rule, sample, val, key)` | Internal helper. Inserts the `alert_events` row, records the new id in `open`, drops the `pending` entry, emits a `WARN` log line, and (if a notifier is wired) goroutine-dispatches it. Caller holds the mutex. |
| `HandleRuleDelete(ruleID)` | Drops every `pending` and `open` entry for the deleted rule and calls `Invalidate()` so the rules cache reloads. Called by the API's DELETE handler. |
| `Invalidate()` | Clears the in-memory rules cache. The API calls it after every rule create / update / delete so the next `Evaluate` reloads. |
| `rulesFor(ctx, hostID)` | Cache-backed lookup. First call loads all enabled rules via `ListAlertRules(nil)`; subsequent calls filter in-memory. RLock-fast-path / Lock-on-populate pattern. |
| `MetricValue(sample, name)` | Translates a metric name to its numeric value. Supports flat scalars (`cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load_1/5/15`, `temp.max`) and **dotted targets**: `iface.<name>.{rx_rate,tx_rate,rx_bytes,tx_bytes}`, `mount.<path>.{pct,used,total}`, `temp.<sensor>.value`, `proc.<name>.{cpu_pct,mem_pct,mem_rss}` (max across matching processes). Uses `SplitN(metric, ".", 2)` + `LastIndex(".")` to handle mount paths with no nested dots. Returns `(0, false)` on unknown name or missing target so the evaluator can warn-log rather than fire spuriously. |
| `StatusProvider` (type) | Function type `func(hostID string) float64` used to resolve `host.status` outside the sample payload. `cmd/hub` wires `Hub.LatestStatus` into it. |
| `StatusToFloat(s)` | Encodes a host status string to a numeric value (`ok=0`, `warn=1`, `crit=2`, `offline=3`) so the existing `threshold REAL` column works unchanged. |
| `SupportedMetrics` / `MetricCategories` (vars) | Canonical metric list + dotted-target category map. The API exposes both via `/api/alerts/metadata` and `/api/monitoring/catalog` so the UI dropdown stays in sync without a second source of truth. |
| `SupportedOps` (var) | `>`, `>=`, `<`, `<=`, `==`, `!=`. The string ops (`==`/`!=`) are needed for `host.status` rules using the encoded numeric mapping above. |
| `compare(v, op, threshold)` | The six-way operator dispatch. Tiny on purpose — switch-based for inlinability. |
| `ValidateRule(rule)` | Centralized validation. Accepts flat names in `SupportedMetrics` *or* dotted names whose category is in `MetricCategories` and whose leaf suffix is one of that category's known leaves. Both create and update call this before touching the DB. |
| `SetNotifier(n)` / `SetStatusProvider(p)` | Wires the notifier and status provider into the evaluator. Called from `cmd/hub/main.go` after constructing the hub. Nil-safe. |

**Templates (`templates.go`):**

| Name | Purpose |
| --- | --- |
| `Template` / `TemplateRule` | Wire shape for predefined rule sets. |
| `Templates()` | Returns the built-in set: "Beszel defaults" (5 rules — cpu/mem/disk/temp.max thresholds + host.status==offline/2m), "Aggressive" (lower thresholds, shorter durations), "Quiet" (higher thresholds). Defined as a function rather than a `var` so future user-defined templates can layer in via `user_settings`. |
| `TemplateByName(name)` | Lookup helper used by the API endpoint. |
| `ApplyTemplate(ctx, store, template, hostID *string)` | Clones the template's rules into `alert_rules`. Snapshots existing rules first and skips duplicates by `(metric, op, threshold)` triple — apply is additive, not destructive. Calls `ValidateRule` defensively so a bad template fails loudly. |

**Notification delivery (`notify.go` + `ch_shoutrrr.go` + `ch_webhook.go`):**

| Name | Purpose |
| --- | --- |
| `Notifier` | Loads enabled channels from the store and dispatches per-channel. `Dispatch(ctx, event, rule, resolved)` loads the host row (for name), filters channels by `SeverityLevel(ch.MinSeverity) <= SeverityLevel(rule.Severity)` and `ch.NotifyResolve`, then fires a goroutine per channel. Each goroutine creates its own `context.WithTimeout(context.Background(), 15*time.Second)` so a hung webhook cannot hold a goroutine open indefinitely. |
| `SeverityLevel(s)` | `"info"→0`, `"warning"→1`, `"critical"→2`. |
| `BuildSender(ch)` | Exported wrapper so the `testAlertChannel` API handler can validate a channel's config without a full Dispatch. |
| `buildSender(ch)` | Switches on `ch.Type`. For `"discord"`, `"slack"`, `"ntfy"`, `"gotify"`, `"shoutrrr"`: calls `ToShoutrrrURL` and constructs a `ShoutrrrSender`. For `"webhook"`: keeps the dedicated `WebhookSender` because its JSON-POST body shape doesn't map cleanly onto Shoutrrr's `generic://` service. |
| `ShoutrrrSender.Send(ctx, n)` | Builds a `shoutrrr/pkg/router.ServiceRouter` with a 12s timeout (the outer dispatch context adds another 15s cap), calls `Send` with a formatted message and a `*types.Params` carrier (title, color hex, priority) that target services consume opportunistically. |
| `ToShoutrrrURL(ch)` | Translates a legacy channel row to a Shoutrrr service URL. Native `"shoutrrr"` channels return their config URL unchanged. `"discord"` → `discord://<token>@<id>` parsed from the webhook URL. `"slack"` → `slack://hook:<T>-<B>-<X>@webhook`. `"ntfy"` → `ntfy://[token@]<host>/<topic>?scheme=<http|https>&priority=...`. `"gotify"` → `gotify://<host>/<token>[?disableTLS=true]`. |
| `WebhookSender` | POSTs (or configured method) a structured JSON payload to any URL. Optional `headers` map applied to the request. Payload includes `type` (`alert_fired`/`alert_resolved`), `host`, `rule`, `event`, and `resolved_at`. |
| Legacy config structs (`DiscordConfig`, `SlackConfig`, `NtfyConfig`, `GotifyConfig`) | Now in `legacy_configs.go` — retained for unmarshalling stored rows. The Send implementations they used to accompany have been removed in favor of `ch_shoutrrr.go`. |
| `contains(xs, x)` | Tiny linear-search helper for the small fixed sets. |

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
| `(*Hub).Run(ctx)` | Spins up four background goroutines: `ingestLoop`, `retentionLoop`, `containerSummaryLoop`, and `offlineWatchdog`. Blocks until `ctx` is cancelled, then waits for all four to exit. |
| `(*Hub).ingestLoop(ctx)` | Reads samples off `h.samples`, persists each one, bumps `last_seen`, and (best-effort) inserts the rich historical tables (`NetIfaces`, `DiskMounts`, `DiskIO`, `Temps`, `CPUCores`, `Processes`). Each insert is independent — a temp write failure doesn't poison disk-io history. After persisting, calls `evaluator.Evaluate(ctx, sample)` then computes the host's new status via `computeStatus` (reading per-host warn/crit thresholds from `host_config`). Broadcasts a `metric` SSE event (backwards-compat flat fields) and, on status transition, a `host_status` event + `evaluator.EvaluateStatus` to fire host.status rules. |
| `(*Hub).computeStatus(ctx, sample, maxTemp)` | Returns `"ok"` / `"warn"` / `"crit"` based on the host's `warn_*` / `crit_*` thresholds. Falls back to `DefaultHostConfig` if the config read fails — never panics on a transient DB error during ingest. The watchdog (not this function) emits `"offline"`. |
| `(*Hub).retentionLoop(ctx)` / `pruneAllHosts(ctx)` | Hourly tick. For each host: read its config, build per-table cutoffs from `retention_days` + `retention_overrides`, call `store.PruneHostMetrics(host.ID, cutoffs)`. Errors per host are logged so a single failing host doesn't stop the others. |
| `(*Hub).containerSummaryLoop(ctx)` / `refreshAllContainerCounts(ctx)` | 15s ticker. Iterates registered docker providers, counts running/stopped/unhealthy from `Provider.List(ctx, true)`, calls `SetContainerCounts(hostID, counts)`. Replaces the dashboard's per-host Docker polling. |
| `(*Hub).SetContainerCounts(hostID, counts)` / `ContainerCounts(hostID)` | Cached read/write. `Set` broadcasts a `container_summary` SSE event only when the counts changed (saves bandwidth and client churn). Public so a remote-agent transport can push its own counts on its own cadence. |
| `(*Hub).offlineWatchdog(ctx)` / `tickOfflineWatchdog(ctx)` | 30s ticker. For each known host, computes a stale threshold of `max(2 × cfg.SampleIntervalS, 30s)` and flips hosts whose `last_seen` exceeds it to `"offline"`. On transition broadcasts a `host_status` SSE event and calls `evaluator.EvaluateStatus(ctx, host.ID, now)` so host.status rules can fire even though the host stopped sending samples. |
| `LatestStatus(hostID)` | RLock-protected lookup of the cached per-host status. Used by the monitoring overview endpoint and (via `cmd/hub`) by the evaluator's `StatusProvider`. |
| `ConfigPusher` / `LocalApplier` interfaces | Seams for routing `host_config` changes. `cmd/hub` wires the agent handler as the pusher and the in-process collector as the applier (registered with the local host's id). `PushConfigToAgent(ctx, hostID)` reads the config from store and dispatches via the right transport. |
| `SSEEvent` (struct) | Typed envelope with `Type` ∈ `"metric"` / `"host_status"` / `"container_summary"` / `"alert"`. Flat CPU/Mem/NetIn/NetOut/Temp/DiskPct fields are non-`omitempty` so a host at exactly 0% still emits the value (otherwise the frontend would see undefined and skip the update). Status / Containers / Alert fields are `omitempty` and only set for their respective types. |
| `ContainerCounts` / `AlertEnvelope` | Nested payload structs for the `container_summary` and `alert` event types. |
| `(*Hub).RegisterSource(ctx, src)` | Asks the source for `HostInfo`, derives a stable host_id, upserts the host row, then launches the source's `Run` against a per-source channel adapter that stamps the host_id onto every sample. Returns the host_id so the caller can pair it with a docker provider. |
| `(*Hub).samplesIn(hostID)` | Returns a per-source send channel that stamps `host_id` on samples and forwards to the central channel. Decouples sources from the host_id assignment — sources don't need to know what id they got. Drops on full central buffer with a warning, matching collector backpressure semantics. |
| `(*Hub).RegisterDocker(hostID, p)` / `Docker(hostID)` | Concurrent-safe registry of docker providers by host. The API uses `Docker` to dispatch container endpoints. |
| `ComposeProvider` interface | Six-method seam for compose stack operations: `DiscoverStacks`, `GetStack`, `StackAction`, `Logs`, `ReadFile`, `WriteFile`. Local hosts use `compose.Local`; remote agents satisfy it via `agentComposeProvider` in `agentws.go`. |
| `(*Hub).RegisterCompose(hostID, p)` / `Compose(hostID)` | Parallel to the docker registry. Registered when the local docker socket is available (hub) or when `hello.HasCompose` is true (agent). The API returns 503 if no provider is registered for a host. |
| `TerminalProvider` interface | Four-method seam for interactive terminal sessions: `StartTerminal(ctx, cid, cmd) (reqID string, output <-chan []byte, err error)`, `SendTerminalData(ctx, reqID, data []byte)`, `ResizeTerminal(ctx, reqID, cols, rows uint)`, `CloseTerminal(ctx, reqID)`. Allows the API to route terminal sessions to either `localTerminalProvider` (local Docker socket) or `agentTerminalProvider` (remote agent WebSocket) without the handler knowing which transport is in use. |
| `(*Hub).RegisterTerminal(hostID, p)` / `Terminal(hostID)` / `UnregisterTerminal(hostID)` | Concurrent-safe registry of terminal providers by host, parallel to the docker and compose registries. The local provider is registered at hub startup; agent providers are registered on agent connect and removed on disconnect. |
| `(*Hub).UnregisterDocker(hostID)` / `UnregisterCompose(hostID)` | Public helpers so `agentws.go` can clean up all three registries on agent disconnect without accessing hub struct fields directly. |
| `localTerminalProvider` (`internal/hub/terminal.go`) | Implements `TerminalProvider` for the local hub host. Wraps `dockerctl.Client`'s exec API. Owns a mutex-protected `sessions map[string]*localTermSession`; each session holds the stdin `io.WriteCloser`, a resize callback, and a close function. The `reqID` counter is an atomic int64 so session IDs are unique and allocation-free. A compile-time `var _ TerminalProvider = (*localTerminalProvider)(nil)` assertion catches drift. |
| `agentTerminalProvider` (`internal/hub/agentws.go`) | Implements `TerminalProvider` for a remote agent host. Each method delegates to `AgentHandler` with a fixed `hostID`, forwarding the call as a WebSocket frame and waiting on a pending channel. Same pattern as `agentDockerProvider` but for the `TypeTerminalReq` / `TypeTerminalResp` frame types. |
| `(*Hub).Store()` | Exposes the store for the API package. The alternative — passing the store separately — would require keeping two pointers in lockstep; one accessor is simpler. |
| `DeriveHostID(info)` | First 16 hex chars of `sha1(source + "|" + name)`. Stable across restarts so historical metrics stay linked to the same host record. When remote agents land, they will provide their own UUID and this is fallback for the local source only. |
| `Evaluator` interface | One-method seam (`Evaluate(ctx, sample)`) the hub uses to dispatch persisted samples to the alert evaluator. Defined on the hub side (rather than imported from `internal/alerts`) to avoid an import cycle: `alerts` imports `store` for its types and rules, and the hub imports neither. `*alerts.Evaluator` satisfies it; tests can substitute a stub. |
| `Hub.evaluator` field + `SetEvaluator(e)` | The evaluator is settable post-construction (rather than a constructor arg) so call sites that don't have alerts wired — tests, dev scripts — don't have to construct one. `cmd/hub` always sets it before calling `Run`. |
| `ingestLoop` (updated) | After `InsertMetric` + `TouchHost` succeed, dispatches the same sample to `h.evaluator.Evaluate` when set. Errors inside the evaluator are the evaluator's problem (it logs them); the hub does not retry or back off. Keeping evaluation inline (rather than on a separate goroutine) means a fired alert's `fired_at` timestamp lines up tightly with the sample that caused it. |

### `internal/api`

HTTP layer. chi-based, all routes under `/api`. The same handler can optionally serve a SvelteKit static build at `/` with SPA fallback so a single binary covers UI + API.

The **monitoring spine** added in v0.4.1 lives in `monitoring.go` and aggregates what the dashboard and host-detail page need into single calls, replacing the prior N+1 fan-out. Frontend code uses these as the primary data source; the older per-host endpoints stay for single-slice fetches.

| Endpoint | Handler | Purpose |
| --- | --- | --- |
| `GET /api/monitoring/overview` | `monitoringOverview` | Hosts + latest sample + container counts + open-alert counts + status. One query for the open-events count grouped in Go; everything else is in-memory from the hub. The dashboard's only blocking fetch on first paint. |
| `GET /api/monitoring/catalog` | `monitoringCatalog` | Families list (including experimental scaffolds), scalar metric list, alert categories+leaves, alert ops, predefined templates. Sourced from `alerts.SupportedMetrics`, `alerts.MetricCategories`, `alerts.SupportedOps`, and `buildCatalogTemplates()` (which maps `alerts.Templates()` to the wire shape). |
| `GET /api/hosts/{id}/monitoring/bundle?range=&points=&include=` | `monitoringBundle` | Host record + latest sample + host_config + history series + open alerts in one response. The optional `include=metrics,net,mounts,diskio,temps,cpu` query param lets the caller skip series they don't need (the host-detail page passes the full set; the dashboard's drill-in passes nothing and gets only the host + latest). |
| `GET /api/hosts/{id}/config` / `PUT` | `getHostConfig` / `putHostConfig` | Per-host monitoring policy. PUT validates, persists via `store.UpsertHostConfig`, then calls `hub.PushConfigToAgent(ctx, hostID)` to dispatch to the right transport. Returns `{ok:true, warning?:"..."}` so an agent-disconnected warning surfaces without failing the request. |
| `GET /api/settings/monitoring-defaults` / `PUT` | `getMonitoringDefaults` / `putMonitoringDefaults` | Global defaults applied to hosts without their own row. Stored as JSON in `user_settings['monitoring.defaults']`. |
| `POST /api/alerts/templates/apply` | `applyAlertTemplate` | Body: `{template:"Beszel defaults", host_id:null|"id"}`. Calls `alerts.ApplyTemplate` (which skips duplicates) and `evaluator.Invalidate()` if any rules were created. Returns `{template, created:[ids], created_n, skipped_n}`. |
| `GET /api/hosts/{id}/metrics/{temps,cpu,procs}` and `GET /api/hosts/{id}/containers/{cid}/metrics` | `tempHistory`, `cpuCoreHistory`, `procHistory`, `containerHistory` | Per-metric history for single-chart drill-ins. `procHistory` requires `?name=`. All share `parseRangeQuery(r)` for `range` + `points` parsing (defaults: 1h, 300 points). |
| `includeSet(raw)` (helper) | — | Parses `?include=a,b,c`. Empty/missing param returns a function that says "yes" for everything. |
| SSE envelope changes | `streamMetrics` | The handler is unchanged structurally but now JSON-marshals the typed `SSEEvent` envelope. Pre-v0.5 clients still read `event.cpu` / `event.mem` etc. directly under the implicit `type:"metric"` default. New clients route on `event.type` to handle `host_status` / `container_summary` / `alert`. |
| `s.evaluator.Invalidate()` callers | `createAlertRule`, `updateAlertRule`, `deleteAlertRule`, `applyAlertTemplate` | Every rule mutation invalidates the evaluator's rules cache so the next sample evaluates against the fresh set. |

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
| `listNetworks`, `inspectNetwork`, `createNetwork`, `removeNetwork`, `connectNetwork`, `disconnectNetwork` | New routes under `/api/hosts/{id}/networks/` that delegate to `DockerProvider` network methods, supporting list, deep-inspect (with connected containers), creation, removal, and container connection lifecycle. |
| `parseDuration(s, def)` | `time.ParseDuration` with a default fallback. Why a wrapper: inline `if s == "" || ...` was repeating; this clarifies intent. |
| `corsForDev` | Split into two build-tagged files. `cors_dev.go` (`//go:build dev`): allows `localhost`/`127.0.0.1` origins with `Access-Control-Allow-Credentials: true` so the Vite dev server can send the session cookie to the hub. `cors_prod.go` (`//go:build !dev`): no-op passthrough — in production the SPA is same-origin so no CORS headers are needed. Build the hub with `-tags dev` (or `make dev`) to get the permissive middleware. |
| `alertsMetadata` | Returns `{metrics, ops}` from the alerts package's canonical lists. Used by the UI to populate dropdowns so the metric-name vocabulary has a single source of truth. |
| `listAlertRules` | Reads the optional `host_id` query and forwards to `store.ListAlertRules`. Returns `[]` (not `null`) on empty so the frontend never null-checks. |
| `alertRulePayload` (struct) | Wire DTO for create + update. Differences from `types.AlertRule`: `host_id` is a plain string (empty = "all hosts" — the empty/NULL mapping happens in `toRule`); `enabled` is a `*bool` so omitting it on create defaults to `true` rather than silently disabling the rule. |
| `(alertRulePayload).toRule(id)` | Converts the wire DTO to a `types.AlertRule`. Promotes empty-string `host_id` to `nil`. |
| `createAlertRule` | Decode → `ValidateRule` → `CreateAlertRule` → `GetAlertRule` to read back, so the response includes `created_at` (DB default). On read-back failure we still return the in-memory rule rather than 500ing — the rule was successfully persisted, the response is just slightly degraded. |
| `getAlertRule` | Single-rule fetch. 404 on missing. |
| `updateAlertRule` | Same shape as create: decode → validate → update → read back. |
| `deleteAlertRule` | Calls `store.DeleteAlertRule` (which cascades event history) then `evaluator.HandleRuleDelete` so transient pending/open entries don't leak. |
| `listAlertEvents` | Builds a `store.AlertEventFilter` from `host_id`, `open`, and `limit` query params. Default `limit` 200; the frontend asks for 100 on the alerts page and 200 for the layout's open-count badge. |
| `requireAuth` (middleware) | Reads the `aperture_session` cookie and calls `store.ValidateSession`. Passes through unauthenticated requests when `IsPasswordSet` is false (first-run mode, so the setup page is reachable). Returns `401 {"error":"..."}` otherwise. Applied via a chi inner group that covers all data and management endpoints; health, `auth/*`, and `agents/ws` are outside that group. |
| `authStatus` | `GET /api/auth/status`. Returns `{configured: bool, authenticated: bool}`. The layout calls this on mount to decide whether to redirect to `/setup`, `/login`, or proceed normally. |
| `authSetup` | `POST /api/auth/setup`. First-run only — returns 409 if a password is already configured. Hashes the submitted password with bcrypt (cost 12), calls `SetPasswordHash`, creates a session, and sets the cookie. |
| `authLogin` | `POST /api/auth/login`. Calls `bcrypt.CompareHashAndPassword`; on match creates a 32-byte random hex token, inserts it with a 24-hour expiry into `sessions`, and sets an HttpOnly + SameSite=Lax `aperture_session` cookie. |
| `authLogout` | `POST /api/auth/logout`. Deletes the current session from the DB and overwrites the cookie with an expired value. |
| `authChangePassword` | `POST /api/auth/change-password`. Verifies the current password via bcrypt, then re-hashes the new password and calls `SetPasswordHash`. Does not invalidate the current session (single-admin assumption). |
| `PruneSessions(ctx, st)` | Exported hourly pruner. `cmd/hub` starts it as `go api.PruneSessions(ctx, st)`. Loops on a 1-hour ticker, calling `store.PruneExpiredSessions` and logging the count of deleted rows. |
| `newSessionToken()` | Generates 32 bytes from `crypto/rand` and hex-encodes them. The resulting 64-character string gives 256 bits of entropy. Only ever stored hashed in the `sessions` table. |
| `setSessionCookie(w, token, duration)` | Sets the `aperture_session` cookie with HttpOnly, SameSite=Lax, Path=/. Passing `token=""` and a negative duration clears the cookie (used by logout). |

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

### `internal/agentproto`

Shared wire-frame type definitions for the agent ↔ hub WebSocket protocol. Both `internal/hub/agentws.go` (hub side) and `cmd/agent/main.go` (agent side) import this package so the frame shapes cannot silently drift between the two binary endpoints.

| Export | Content |
| --- | --- |
| Frame-type constants | `TypeHello`, `TypeAck`, `TypeMetric`, `TypeHeartbeat`, `TypeDockerReq`, `TypeDockerResp`, `TypeComposeReq`, `TypeComposeResp` — string constants used as the `"type"` discriminator field in every JSON frame. |
| `HelloFrame` | Sent by the agent immediately after the WebSocket upgrade. Carries `HostInfo`, `Version`, `HasDocker`, `HasCompose`. |
| `AckFrame` | Hub → agent reply to hello; carries the assigned `HostID`. |
| `MetricFrame` | Agent → hub; wraps `types.MetricSample`. |
| `DockerReqFrame` / `DockerRespFrame` | Hub → agent / agent → hub; carry `ReqID`, `Action`, `CID`, and a JSON `Params` blob for requests; `OK`, `Data`, and `Error` for responses. |
| `ComposeReqFrame` / `ComposeRespFrame` | Same pattern for compose operations. |

---

## Frontend (SvelteKit + Svelte 5 runes)

Project at `web/`. Output is a static SPA in `web/build/`, served by the hub at `/`. Dev mode runs Vite at :5173 and proxies API calls to the hub.

### `src/lib/types.ts`

Hand-mirrored TypeScript versions of the Go `types` package. Manual sync is intentional for v0.1 — codegen will pay off once the type list grows beyond a half-page or once a third client appears. Includes `AlertRule`, `AlertEvent`, `AlertMetadata`, `CreateSpec`, `CreatePortBinding`, `CreateVolumeBinding`, and the rich live-metric types: `NetInterfaceSample`, `DiskMountSample`, `DiskIOSample`, `TempSample`, `ProcessSample`. Also `ContainerMount`, `ContainerInspect`, `ResourceUpdate` for the deep-inspect and resource-edit flows. History response types: `NetIfaceSeries`/`NetIfaceHistory`, `DiskMountSeries`/`DiskMountHistory`, `DiskIOSeries`/`DiskIOHistory`. `MetricSample` carries the optional rich live fields matching the Go type (`cpu_per_core?`, `net_interfaces?`, `disk_mounts?`, `disk_io?`, `temps?`, `mem_avail?`, `mem_cached?`, `processes?`). Note: `CreatePortBinding` and `CreateVolumeBinding` are deliberately separate from the read-side `PortMapping` because create is asymmetric (we *send* a binding spec, not echo a docker snapshot). Also includes Docker network models (`DockerNetwork`, `NetworkContainer`, `NetworkCreateSpec`).

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

**`agentDockerProvider`** implements `hub.DockerProvider` by forwarding all methods (including the 6 new network methods) over the WS. Uses an atomic counter for req_id and a `map[string]chan dockerRespFrame]` pending map. 30s per-command timeout.

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

### Design system (`web/src/lib/styles/`)

Two files replace the old `styles.css`:

- **`tokens.css`** — all CSS custom properties. Dark and light themes toggle via `[data-theme="dark|light"]` on `<html>`. Six user-selectable accent colors (teal default, indigo, amber, violet, lime, rose), each with hex/soft/line variants applied to `:root` by the `accent` store. Status colors (`--ok`, `--warn`, `--crit`, `--info`, `--offline`) are theme-invariant — never used for selection. Geist Sans + Geist Mono imported via `@fontsource/*`. Motion tokens (`--ease-card`, `--dur-slide`, `--dur-modal`, etc.) are all inside `@media (prefers-reduced-motion: no-preference)`.
- **`global.css`** — imports `tokens.css`, then adds base resets, typography scale, utility classes (`.mono`, `.label-mono`, `.text-dim`, etc.), table/input/button global styles, `.card`, `.pill`, `.segmented`, `.glass-topbar`, `.glass-drillin`, `.skeleton` shimmer, `.pulse-crit`. Also exports **legacy aliases** (`--border → --line`, `--bad → --crit`, `--bg-elev-1 → --bg-elev`, `--mono → --font-mono`, `.muted`) so existing pages work without a rewrite.

**Key design rules:**
- All numbers, addresses, timestamps, sizes, rates: `font-family: var(--font-mono)`.
- Status colors (`--ok/--warn/--crit`) are health-only. `--accent` is brand/selection/focus only.
- Card hover: `translateY(-1px)` 180ms `--ease-card`. Drill-in slide: 260ms same easing.
- Sparkline never re-animates on data update — ring buffer is append+shift, no CSS on the SVG path.

### Shell (`web/src/lib/components/shell/`)

| Component | Role |
| --- | --- |
| `AppShell.svelte` | CSS grid: `220px 1fr`. Mounts `Sidebar` left and `Topbar + <main.content>` right. **`main.content` owns global page padding (`22px 28px 60px`), `max-width: 1600px`, and `margin: 0 auto`** so every routed page shares the same gutter and content cap. Initializes theme and accent stores on mount. |
| `Sidebar.svelte` | 220px fixed sidebar. WORKSPACE section: Dashboard, Hosts, Containers, Stacks, Storage, Network. OBSERVE section: Logs, Shell, Automation, Alerts. Active item: 2px left accent rail + accent text. Alert badge on Alerts item: polls `/api/alerts/events?open=true`. Collapsed-label toggle planned but not yet wired. |
| `Topbar.svelte` | Search input (⌘K hint), sync indicator dot, theme toggle, avatar initials chip. |

### Stores (`web/src/lib/stores/`)

| Store | What it holds |
| --- | --- |
| `theme.svelte.ts` | `ThemeMode` (`dark|light|system`). Reads/writes `localStorage`. Applies `document.documentElement.dataset.theme`. Listens to `prefers-color-scheme` media query when mode is `system`. |
| `accent.svelte.ts` | `AccentKey` (one of 6). Applies `--accent`, `--accent-soft`, `--accent-line` to `:root`. |
| `monitoring.svelte.ts` (v0.4.1, **canonical**) | Replaces the legacy host store. `Record<string, HostEntry>` with `host`, `latest`, ring buffers (`cpuSeries`, `memSeries`, `netInSeries`, `netOutSeries`, `tsSeries`), derived `netInRate` / `netOutRate`, server-supplied `status`, `containers: ContainerCounts \| null`, `openAlerts: number`. Exposes `hydrate(overview)` for the single overview fetch, `connectSSE(baseUrl)` for the typed-envelope consumer, and `disconnectSSE()`. SSE handler switches on `env.type` and routes `metric` / `host_status` / `container_summary` / `alert` to the right state mutation. Ring buffers are preserved across `hydrate()` so reconciliation reloads don't flatten the sparkline. |
| `hosts.svelte.ts` (alias) | A thin passthrough that re-exports `monitoringStore as hostStore` + the relevant types. Lets every pre-v0.4.1 import (`PageHeader`, `FilterBar`, `Sidebar`, `RichCard`, etc.) keep working without an import-rename pass. |
| `dashboardLayout.svelte.ts` | `cardLayout` (rich/tile/list), `pinnedHostIds`, `cardOrder`, `activeFilter`, plus v0.4.1's **`cardWidgets: Record<hostID, string[]>`** carrying the per-host metric selection from `CardConfigModal`. Persists to `localStorage` + `/api/settings/dashboard-layout` on change. |

### Dashboard components (`web/src/lib/components/dashboard/`)

| Component | Role |
| --- | --- |
| `PageHeader.svelte` | H1 + counts strip: Healthy / Warning / Critical / Offline / Containers / Alerts. |
| `FilterBar.svelte` | Tag filter pill tabs (derived from all host tags) + Rich/Tile/List segmented control + gradient "Add host" button. |
| `HostGrid.svelte` | Outer grid container. Switches grid-template-columns by layout. Renders loading skeletons, `EmptyBlock`, or `ErrorBlock` when appropriate. |
| `HostCard.svelte` | Variant switcher — delegates to `RichCard`, `TileCard`, or `CompactRow`. |
| `RichCard.svelte` | `minmax(560px, 1fr)`. Left 3px status rail (ok/warn/crit/offline). **Dynamic metric rows** driven by `dashboardLayout.getCardWidgets(host.id)` (falls back to `DEFAULT_WIDGETS` from `monitoring/metricCatalog.ts`). Each row resolves via `getMetric(key)` to label / value / color / sparkline series — CPU/Mem/Net use the entry's ring buffers, other metrics render label + value with a placeholder spacer. Side info panel (OS/arch, uptime, container counts). Alert footer when `openAlerts > 0`. `⋯` menu (`stopPropagation`) → `CardMenu` with "Configure widget…" item opening `CardConfigModal`. |
| `CardConfigModal.svelte` | Per-card widget picker. Lists metrics from `metricCatalog.metricsByCategory()` grouped by CPU/Memory/Disk/Network/Load/Temperature/Other. 2–4 selections enforced; at the cap, picking a new metric replaces the oldest. Persists via `dashboardLayout.setCardWidgets(hostID, keys)`. |
| `CardMenu.svelte` | Popover anchored to the `⋯` button. Items: Configure widget… / Pin to dashboard / Open full monitoring → / Open Shell / Restart / Remove. Closes on click-outside; "Configure widget" calls the provided `onconfigure` prop instead. |
| `TileCard.svelte` | `minmax(320px, 1fr)`. 2×2 metric grid: CPU/Mem/Net↓/Temp tiles each with status-colored sparkline. |
| `CompactRow.svelte` | Single row, 7 columns: status dot, name+kind, OS/arch, CPU%, Mem%, Net↓, tags. |
| `AddWidgetTile.svelte` | Dashed "+" tile matching the current layout's card size. Turns accent background on hover. |
| `CardMenu.svelte` | Popover anchored to the `⋯` button. Actions: Pin/Unpin, Open Shell, Restart, Remove. Danger style on Remove. Closes on click-outside. |

### Host components (`web/src/lib/components/host/`)

This directory holds two surfaces: the DrillIn slide-over modal (kept as the dashboard's fast preview), and the v0.4.1 host detail page tab components.

| Component | Role |
| --- | --- |
| `DrillIn.svelte` | Full-height right panel sliding in from the right (260ms `--ease-card`). Backdrop with blur. Sticky header: close btn, `HostKindIcon`, host name, `StatusIndicator`, tag chips, action buttons (Restart/SSH/Update/Stop). Tab nav: Overview / Containers / Stacks / Logs / Shell. Overview is now: 4-column `BigMetric` grid (CPU/Mem/Net/Temperature with live max-temp value), `StoragePanel` + (kind-conditional) `ContainersPanel` or top-5 process peek + `EventsPanel`, a sensors mini-panel under the lower panels, and an "Open full host monitoring →" CTA into `/hosts/{id}`. Esc and backdrop click close. |
| `BigMetric.svelte` | Elevated card with 26px mono value, sub label, and `Sparkline`. |
| `StoragePanel.svelte` | Per-mount rows (name + size) each with a `Meter` bar. Falls back to root disk if no `disk_mounts`. |
| `ContainersPanel.svelte` | Running/Stopped/Unhealthy stat grid + "Top by CPU" list (up to 4 containers). |
| `EventsPanel.svelte` | Last 8 alert events. `fired_at` relative time + "Firing/Resolved — rule #N (val)". Warn color / ok color on resolved. |
| `HostHeader.svelte` | The full host detail page's identity strip — back link, host icon, name + status indicator + tag chips, mono meta line (platform / arch / cpu count / RAM / uptime / last-seen), and action buttons. |
| `RangePicker.svelte` | Segmented control with `15m` / `1h` / `6h` / `24h` / `7d` / `30d`. Persists the choice per-host in `localStorage` under `aperture.range.{hostID}`. |
| `OverviewTab.svelte` | 4 BigMetric cards (CPU/Mem/Net/Temp, color-coded against `host_config` thresholds), alert banner, top-3 process peek. |
| `CPUTab.svelte` | Aggregate CPU chart, live per-core grid with bars, per-core history (multi-series), load-average chart. |
| `MemoryTab.svelte` | Used/cached/free segmented bar, used-vs-total history, swap chart (when swap exists). |
| `DiskTab.svelte` | Mount table with usage meters, live disk-I/O table, disk-I/O history chart (read+write per device), per-mount history chart per mount. |
| `NetworkTab.svelte` | Live interface table (rates + totals), aggregate rate chart, per-interface history charts. |
| `SensorsTab.svelte` | Live sensor grid (colored by threshold), multi-series temperature history. |
| `ProcessesTab.svelte` | Sortable table (CPU or Memory). Click a row to expand inline history (calls `/api/hosts/{id}/metrics/procs?name=...`). |
| `DockerTab.svelte` | Running/Stopped/Unhealthy summary, container table, link out to `/hosts/{id}/containers` for full management. |
| `EventsTab.svelte` | Per-host alert event history (resolved + firing). |
| `MonitoringSettingsTab.svelte` | Edits `host_config` — sample interval, retention, mem_calc, family checkboxes, warn/crit thresholds, NIC/sensor/mount filters. PUTs to `/api/hosts/{id}/config` and reloads the bundle on save. |

The detail page (`web/src/routes/hosts/[id]/+page.svelte`) loads one `bundle` on mount and on `(id, range)` change via a `lastFetchKey`-guarded `$effect` (the effect must NOT read `bundle` itself — that was the v0.4.1 infinite-loop regression and is documented in the changelog as the post-mortem).

### Add-host components (`web/src/lib/components/addhost/`)

| Component | Role |
| --- | --- |
| `AddHostModal.svelte` | 2-step glass modal (scale-in 220ms). Step 1: `MethodRadio` + method-specific form fields. Step 2: async `VerifyRow` list + install command block for agent method. Calls `api.createAgentToken` on the agent path. |
| `MethodRadio.svelte` | Three radio cards: Install Agent / Docker API / SSH Probe. Each shows an icon, label, and description. Accent-tinted border + background when selected. |
| `VerifyRow.svelte` | Displays pending / checking (CSS spinner) / ok / error states with optional detail text. |

### SSE stream (`/api/stream/metrics`)

The hub broadcasts a `SSEEvent` after every successful metric ingest. The browser's `hosts.ts` store subscribes with `new EventSource(...)` and updates its ring buffers on each event. Events are per-host: `{ hostId, cpu, mem, netIn, netOut, ts }`. If SSE is unavailable (hub not running, proxy strips keep-alive) the store falls back to the initial HTTP-loaded snapshot; sparklines just stop updating rather than throwing an error.

| Route | Use & reason |
| --- | --- |
| `+layout.ts` | `ssr = false; prerender = false`. Pure SPA: no server rendering, no build-time prerender. The data is live, the hub serves the static fallback `index.html`, the client takes over from there. |
| `+layout.svelte` | Replaced top-nav with `<AppShell>`. Auth pages render in a centered `.auth-page` wrapper. All other pages render inside the sidebar shell. Alert badge on Alerts nav item via Sidebar. Footer (version/DB/uptime) retained in existing pages. |
| `alerts/+page.svelte` | The alerts management page. Three tabs: Rules, Events, Channels. **Rules tab**: new rule form (host selector, metric/op dropdowns from `/api/alerts/metadata`, threshold, duration, severity selector), rules table (severity badge, toggle, delete; row tinted red when firing). **Events tab**: event history with state pill, host/rule/value, relative timestamps. **Channels tab**: card list of notification channels (Discord/Slack/ntfy/Gotify/webhook) with type badge, min-severity filter, resolve-notify state, and Test/Edit/Enable/Delete actions; add/edit channel modal (type selector, name, type-specific config fields, min severity, notify-resolve toggle). ESC closes the channel modal via `<svelte:window onkeydown>`. Page title set via `<svelte:head>`. Auto-refresh every 5s. |
| `+page.ts` (root) | `load()` throws `redirect(307, '/dashboard')` so `/` never paints. |
| `dashboard/+page.svelte` | v0.4 card-based host overview. Every 5s: fetch `/api/hosts`, per-host `latestMetric` in parallel, then per-host `containers` (docker-kind only) to populate `hostStore.containers`. SSE subscription to `/api/stream/metrics` updates ring buffers live. Drill-in opens on card click. |
| `hosts/+page.svelte` | v0.4 hosts table. Polls `hostStore` (shared with dashboard). Columns: Host (status + name + tags), Kind, OS, Arch, CPU, Memory, Agent, Uptime, Last sync, Alerts. Rows route to `/hosts/[id]`. Intended as the future system-management surface. |
| `hosts/[id]/+page.svelte` | Host detail. Fetches host, metrics history, latest, net/mount/diskIO history, and open alerts for this host in parallel. **Alert banner** (red) lists firing count + metric names with a link to `/alerts`. **Stale/offline banners** (amber/red) shown when `last_seen` age >= 15s/90s. **Status pill** in h1. Dynamic title (`Aperture — {host.name}`). `absTime` tooltip on relative-time spans. All other monitoring sections unchanged. |
| `hosts/[id]/containers/+page.svelte` | Container management. Filter controls (all / running / exited / paused), **text search box** (filters by container name and image, case-insensitive, integrated into `filtered()` derived state). Sort controls (Name / State / CPU / Mem). ESC closes logs modal → create modal → inspect panel (priority order) via `<svelte:window onkeydown>`. Dynamic page title includes host name (fetched once on mount). `absTime` tooltip on container age. Inspect panel, logs modal, and create modal otherwise unchanged. |
| `hosts/[id]/compose/+page.svelte` | **Compose stack management.** Auto-discovers all stacks via `GET /api/hosts/{id}/compose` on mount with 8s auto-refresh. Each stack is a collapsible card: colored status dot, project name + working dir path, `N/N running` service count badge, status pill (running/partial/stopped), four quick-action buttons (▶ Up, ⏹ Down, ↺ Restart, ⬇ Pull), expand chevron. Action stdout appears inline below the card. **Expanded** shows a 3-tab detail panel: **Services** (table: service name, short container ID, state pill, health badge, human-readable status, ports, per-service restart/stop-start/logs actions), **Compose File** (monospace YAML textarea loaded on demand; toolbar: reload, Save, Save + Deploy; dirty indicator when modified), **Logs** (service selector, tail-count selector, Refresh; scrollable pre block). **Down…** button opens a confirm modal with optional --volumes checkbox. **New Stack modal**: directory path, YAML editor pre-filled with a working template, "Start immediately" toggle. 503 from the API (compose not available) shows a styled banner. ESC closes modals; `<svelte:head>` title. |
| `hosts/[id]/networks/+page.svelte` | Docker network management. Renders a table of networks, a deep-inspect expansion panel (showing config and connected containers), and a "+ New network" modal. Networks can be removed, and containers can be connected or disconnected directly from the inspect view. |
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

- **WebSocket terminal only; no SSE push to the UI.** The browser UI still polls most endpoints every 5s. The agent WebSocket already pushes metrics to the hub; push-to-browser is a future step once the UI stabilizes.
- **Single-admin auth only.** The `auth_config` table uses `CHECK (id = 1)` to enforce one row. Multi-user RBAC is roadmap section 8 — the middleware injection point is already in chi.
- **No embedded frontend yet.** `-web-dir` is the seam. `embed.FS` lands when the project is more stable; embedding now would slow Go-only iteration cycles.
- **`InsecureSkipVerify: true` on agent WS accepts.** The agent WebSocket upgrade uses a TLS config that skips certificate verification. Token-based auth is enforced at the application layer. mTLS is the planned upgrade path for a future version.
