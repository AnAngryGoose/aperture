# Aperture — Technical Reference

Per-package, per-function detail of how aperture is built. This is a living document — every code change should be reflected here in the same commit. See `overview.md` for the user-facing description and `changelog.md` for version history.

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

### `internal/store`

SQLite wrapper. Uses `modernc.org/sqlite` (pure-Go) so the binary cross-compiles freely. `schema.sql` is embedded with `//go:embed` and applied unconditionally on `Open` — the schema is idempotent (`CREATE TABLE IF NOT EXISTS`) so this doubles as a lightweight "migration on startup" until enough versions accumulate to need real migrations.

| Function | Use & reason |
| --- | --- |
| `Open(path string)` | Opens a SQLite file with WAL journal mode, foreign keys on, and a 5-second busy timeout. WAL is critical: it lets the metrics ingest loop write while readers (the API) read without blocking each other. The busy timeout absorbs short contention spikes (e.g. retention pruning) so callers don't have to retry. |
| `Close` | Flushes and closes the SQLite handle. Called from `cmd/hub` on graceful shutdown. |
| `UpsertHost` | Inserts a host row, or updates it when a known host re-registers (e.g. across hub restarts). The `ON CONFLICT(id) DO UPDATE` keeps `created_at` stable while refreshing identity and `last_seen` — important so historical metrics stay linked to a host even if its OS version changed. |
| `TouchHost` | Bumps `last_seen` only. Cheap update called on every metric ingest so the UI can show a recency indicator without inferring it from the metrics table. |
| `ListHosts` | Returns all hosts, sorted by name. Used by the host-list dashboard. |
| `GetHost` | Single-host lookup. Returns `(nil, nil)` for missing hosts so callers can disambiguate "not found" from "DB error". |
| `InsertMetric` | Append a sample to `metrics`. Primary key is `(host_id, ts)` so duplicate timestamps from a faulty source are rejected at the DB layer. |
| `LatestMetric` | Most recent sample for a host. Drives the dashboard's "current state" cards. |
| `MetricsRange` | Time-bounded sample fetch with optional uniform-stride downsampling (`maxPoints`). Stride downsampling is intentional: it's O(n) and doesn't smooth peaks the way averaging would, which matters for spotting spikes. The last sample is always included so the chart's right edge matches the latest sample even when the stride wouldn't otherwise land on it. |
| `PruneMetrics(cutoff)` | Bulk delete of old samples. Called hourly from `hub.retentionLoop` when a retention duration is configured. Returns rows-deleted so the caller can log non-zero prunings. |

### `internal/collector`

Local-host metric source. Implements `hub.MetricSource`. The package documentation explicitly notes that future remote agents produce samples in the same shape and feed the same ingest path — this comment is load-bearing for the multi-host invariant.

| Function | Use & reason |
| --- | --- |
| `NewLocal(interval)` | Constructor with default `DiskPath = "/"`. The disk path is a struct field rather than a flag so per-host overrides become trivial when remote agents land. |
| `(*Local).HostInfo` | Builds the `HostInfo` descriptor from `host.Info`, `cpu.Info`, and `mem.VirtualMemory`. Cached in `hostInfo` after the first call (cleared only by re-creating the collector) so repeated registrations don't re-syscall. |
| `(*Local).Run(ctx, out)` | The collection loop. Sends one sample immediately, then on every tick. Uses a select+default `send` (see below) to drop on backpressure rather than block — losing a sample is preferable to stalling collection if the consumer is slow. Cancels cleanly on `ctx.Done()`. |
| `send(out, s)` | Internal non-blocking channel send. Reason: collection cadence must be predictable; if the receiver wedges, dropping samples is the right behavior. |
| `(*Local).sample(ctx)` | One sampling pass. Each metric is independently fetched and silently zeroed on error so a single broken probe (e.g. unreadable swap on minimal containers) doesn't poison the whole sample. |

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
| `Start`, `Stop`, `Restart`, `Pause`, `Unpause`, `Kill`, `Remove` | Thin wrappers exposing the docker container lifecycle. Stop/Restart take a `*int` timeout pointer because the Docker SDK distinguishes "default" (`nil`) from "zero" (`*int = 0`, meaning "kill immediately"). |
| `Logs(ctx, id, tail)` | Fetches stdout+stderr with a `tail` limit, then strips docker's 8-byte multiplexed log header so the payload is plain text the UI can render. |
| `stripLogHeaders(b)` | Parses docker's TTY-disabled log framing: a 4-byte stream prefix followed by a big-endian length, repeated. Without this, raw output contains binary control bytes. |
| `FilterRunning(in)` | Helper for callers who want only the running subset. Currently unused by the API but kept because the alerting work (next) needs to scope alerts to running containers. |
| `FindByName(ctx, name)` | Resolves a container name to an ID via the docker filter API. Used by future container-create flows; still useful enough to keep around. |

### `internal/hub`

Orchestration layer. Owns the host registry, the central metric ingest channel, retention, and the docker-provider lookup table.

| Type / func | Use & reason |
| --- | --- |
| `MetricSource` interface | The seam for "where metrics come from". v0.1 has one impl (`collector.Local`); the remote-agent transport will be a sibling. The interface is intentionally tiny (`HostInfo` + `Run`) so a transport doesn't have to carry hub-specific concepts. |
| `DockerProvider` interface | The seam for "how the hub reaches a host's docker engine". Mirrors `dockerctl.Client`'s public surface; a remote agent will satisfy it over the wire. The compile-time assertion `var _ DockerProvider = (*dockerctl.Client)(nil)` catches drift between the two. |
| `Hub` struct | Holds the store, logger, retention duration, the central `samples` channel (buffered 256), and the per-host `dockers` and `hosts` maps protected by an RWMutex. |
| `New(cfg)` | Constructor. Slog default is used if no logger is supplied; this keeps tests and quick scripts simple. |
| `(*Hub).Run(ctx)` | Spins up the ingest loop and (when retention > 0) the retention loop, then blocks until `ctx` is cancelled. Returns only after both goroutines exit so `cmd/hub` can rely on the post-Run quiescence. |
| `(*Hub).ingestLoop(ctx)` | Reads samples off `h.samples`, persists each one, and bumps `last_seen`. Errors are logged and the loop continues — losing one sample is better than wedging ingest. |
| `(*Hub).retentionLoop(ctx)` | Hourly `PruneMetrics` for samples older than `h.retain`. Hourly is a deliberate compromise: frequent enough to keep the table small, infrequent enough not to interfere with reads. |
| `(*Hub).RegisterSource(ctx, src)` | Asks the source for `HostInfo`, derives a stable host_id, upserts the host row, then launches the source's `Run` against a per-source channel adapter that stamps the host_id onto every sample. Returns the host_id so the caller can pair it with a docker provider. |
| `(*Hub).samplesIn(hostID)` | Returns a per-source send channel that stamps `host_id` on samples and forwards to the central channel. Decouples sources from the host_id assignment — sources don't need to know what id they got. Drops on full central buffer with a warning, matching collector backpressure semantics. |
| `(*Hub).RegisterDocker(hostID, p)` / `Docker(hostID)` | Concurrent-safe registry of docker providers by host. The API uses `Docker` to dispatch container endpoints. |
| `(*Hub).Store()` | Exposes the store for the API package. The alternative — passing the store separately — would require keeping two pointers in lockstep; one accessor is simpler. |
| `DeriveHostID(info)` | First 16 hex chars of `sha1(source + "|" + name)`. Stable across restarts so historical metrics stay linked to the same host record. When remote agents land, they will provide their own UUID and this is fallback for the local source only. |

### `internal/api`

HTTP layer. chi-based, all routes under `/api`. The same handler can optionally serve a SvelteKit static build at `/` with SPA fallback so a single binary covers UI + API.

| Function | Use & reason |
| --- | --- |
| `NewServer(h)` | Constructor. Holds only a `*hub.Hub`; the store is reached via `hub.Store()`. |
| `(*Server).Router(webFS)` | Builds the chi router with standard middleware (RequestID, RealIP, Logger, Recoverer) and the dev-CORS shim. When `webFS != nil` the SPA handler is mounted at `/*`. |
| `spaHandler(webFS)` | Custom file handler that falls back to `index.html` for any path that doesn't resolve to a real file. Required for client-side routing — without this, refreshing on `/hosts/abc` would 404. |
| `health` | Returns `{ok, time}`. Used as a liveness probe. |
| `listHosts`, `getHost` | Thin wrappers over `store.ListHosts`/`GetHost`. |
| `latestMetric` | Returns the most recent sample, or JSON `null` (not 404) when none exist — the UI treats "no samples yet" as a normal early state, not an error. |
| `metricsRange` | Parses `range` (default `1h`) and `points` (default `300`) query params, computes `since/until`, and delegates to `MetricsRange`. Empty result returns `[]` rather than `null` so the frontend never has to null-check. |
| `listContainers` | Looks up the `DockerProvider` for the host or 404s. Returns `[]` on empty for the same reason as above. |
| `containerAction` | One handler dispatches start/stop/restart/pause/unpause/kill via a switch. Centralizing keeps URL surface small and lets the UI be uniform. |
| `containerRemove` | DELETE — separated from `containerAction` because its parameter shape differs (`force`, `volumes` query args) and conceptually it's not a state transition. |
| `containerLogs` | Returns `text/plain`. Uses `tail` query param (default 200). Renders directly in a modal on the frontend. |
| `writeJSON`, `writeErr` | Tiny helpers; `writeErr` returns `{error: string}` consistently so the frontend's `api.ts` can extract a message uniformly. |
| `parseDuration(s, def)` | `time.ParseDuration` with a default fallback. Why a wrapper: inline `if s == "" || ...` was repeating; this clarifies intent. |
| `corsForDev` | Allow-lists `localhost`/`127.0.0.1` origins so the SvelteKit dev server can hit the hub during development. In production, same-origin means this is a no-op. Keeping it permanently in the chain (rather than a build flag) avoids the "forgot to enable for dev" trap. |

### `cmd/hub`

The hub binary entry point. Responsible for: parsing flags/env, opening the store, constructing the hub, registering the local collector and docker client, starting the HTTP server, and shutting everything down on signal.

| Function | Use & reason |
| --- | --- |
| `main` | The whole startup sequence is intentionally linear (no helper indirection) so you can read it top-to-bottom. Order matters: store before hub, hub before sources, sources before docker, server last, then block on context. Shutdown reverses naturally because of `defer` and signal-cancellation. |
| `envOr(k, def)` | Env-var override with default. Used so flags can be configured via env without pulling in a config library. |
| `parseDurEnv(k, def)` | Same as `envOr` but for durations. |

### `cmd/agent`

Placeholder binary. Compiles, runs, and exits with a message explaining that v0.1 ships single-host. The directory exists from day 1 so the multi-host structure is visible in the repo and so build/CI pipelines that target `./...` cover both binaries already.

---

## Frontend (SvelteKit + Svelte 5 runes)

Project at `web/`. Output is a static SPA in `web/build/`, served by the hub at `/`. Dev mode runs Vite at :5173 and proxies API calls to the hub.

### `src/lib/types.ts`

Hand-mirrored TypeScript versions of the Go `types` package. Manual sync is intentional for v0.1 — codegen will pay off once the type list grows beyond a half-page or once a third client appears.

### `src/lib/api.ts`

Typed HTTP client. The base URL is resolved at build time:

- `VITE_API_BASE` env var, if set (lets you split UI and API in production).
- Else `http://localhost:8080` when `import.meta.env.DEV` is true (hub running separately during dev).
- Else empty string (same-origin, the default production setup).

Every method delegates to one of three private helpers (`get`, `post`, `del`) that throw on non-2xx with the response body included so UI error messages stay informative.

### `src/lib/format.ts`

`formatBytes`, `formatPct`, `formatDuration`, `relTime`. All defensive against `NaN` / `Infinity` because samples can be sparse (e.g. before the first reading) and we don't want `NaN%` in the UI.

### `src/lib/styles.css`

Global stylesheet. Defines a dark theme via CSS custom properties (`--bg`, `--text`, `--accent`, etc.), generic primitives (`.card`, `.bar`, `.pill`, `.grid`), and pulls in `uplot/dist/uPlot.min.css`. Custom-property approach keeps theming pluggable later.

### `src/lib/Bar.svelte`

Tiny progress bar. Takes `value` and optional `warn`/`bad` thresholds; switches color when crossing them. Used in host cards and container memory rows so the eye instantly catches saturated resources.

### `src/lib/Chart.svelte`

uPlot wrapper. Reasons for choosing uPlot: ~45 KB minified, draws thousands of points without a measurable hit, and exposes the underlying chart for future zoom/pan. The wrapper:

- Takes `x` (timestamps in seconds), one or more `series`, and optional `yMin/yMax/title/valueSuffix`.
- Builds options once on mount, then updates data via `plot.setData` on each prop change — no remount per refresh, so animations feel fluid.
- Listens via `ResizeObserver` and calls `plot.setSize` so charts re-flow when the window resizes.
- Cleans up the plot and observer on destroy.

### Routes

| Route | Use & reason |
| --- | --- |
| `+layout.ts` | `ssr = false; prerender = false`. Pure SPA: no server rendering, no build-time prerender. The data is live, the hub serves the static fallback `index.html`, the client takes over from there. |
| `+layout.svelte` | Top nav with the brand and a "Hosts" link. Sticky header so navigation stays in view while scrolling long pages. |
| `+page.svelte` (host list) | Every 5s: fetch `/api/hosts`, then in parallel fetch `latestMetric` and `containers` for each. Failures on the per-host calls are caught individually so one bad host doesn't blank the page. Card per host with CPU/Mem/Disk progress bars and a footer row of load average, uptime, container counts, and last-seen relative time. |
| `hosts/[id]/+page.svelte` | Host detail. Reactive: changing `id` or `range` triggers a reload via `$effect`. Renders four "stat" cards for the latest sample, then time-series charts for CPU, memory, disk, network throughput (rates derived from cumulative counters), and load average. Auto-refresh every 5s. |
| `hosts/[id]/containers/+page.svelte` | Container management. Table of containers with their state pill, CPU bar, memory bar, exposed ports, and per-state action buttons (Pause/Restart/Stop on running, Unpause/Stop on paused, Start/Remove on stopped). Logs button opens a modal with the last 500 lines. Remove confirms via `confirm()` — adequate for v0.1; a styled modal can replace it later. |

`uPlot.AlignedData` is the expected data shape (one x-array followed by N y-arrays); the wrapper builds it from props each render.

---

## Data flow (one full request)

1. `Local.Run` produces a `MetricSample` every `interval`.
2. The collector sends it (non-blocking) on the per-source channel returned by `Hub.samplesIn`.
3. That goroutine stamps `host_id` and forwards to the central buffered `samples` channel.
4. `Hub.ingestLoop` receives, calls `store.InsertMetric`, then `store.TouchHost`.
5. The frontend polls `/api/hosts/{id}/metrics?range=…` every 5s.
6. `api.metricsRange` calls `store.MetricsRange`, which downsamples in Go before serializing.
7. `Chart.svelte` calls `plot.setData` with the new arrays; uPlot redraws.

This whole loop avoids per-request locks beyond the SQLite WAL and the small RWMutex on the docker registry — the central `samples` channel is the only synchronization point on the hot path.

## Configuration knobs that matter

- `interval` — controls write rate to SQLite. At `1s` and 14d retention you get ~1.2M rows; SQLite handles this comfortably but charts default to `points=300` to keep response sizes bounded. Default is `5s` for a balance.
- `retain` — older samples are deleted hourly. Set to `0` to disable pruning entirely (use with care; the table grows unboundedly).
- `disk-path` — only one filesystem is reported in v0.1. Multi-mount support is a roadmap item.

## Known design choices worth flagging

- **No SSE/WebSocket yet.** Polling is simpler and the data is small. We'll switch to push when remote agents land — they'll already be pushing, so the hub will push to the UI naturally then.
- **Auth absent.** Single-user homelab assumption. Adding it later is non-disruptive because the API is namespaced under `/api` and middleware injection is trivial in chi.
- **No embedded frontend yet.** `-web-dir` is the seam. `embed.FS` lands when the project is more stable; embedding now would slow Go-only iteration cycles.
