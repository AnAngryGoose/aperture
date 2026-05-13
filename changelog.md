# Aperture — Changelog

All notable changes are recorded here. The project follows semantic versioning. v0.1 development uses pre-release tags (`0.1.0-alpha.N`); we cut `0.1.0` when monitoring + container management are implemented, tested, and functional per the v0.1 scope in `overview.md`.

Each entry lists **what changed** and **why** the change was made.

---

## [0.1.0-alpha.1] — 2026-05-05

Initial scaffold. Sets up the multi-host architecture, monitoring data flow, container management surface, and dashboard skeleton.

### Added — Project structure & toolchain

- Created the repo layout under `/opt/aperture` with `cmd/hub`, `cmd/agent`, `internal/{api,collector,dockerctl,hub,store,types}`, `web/`. **Why:** the directory layout encodes the v0.1 → multi-host transition (hub + agent split visible from day 1) so future work doesn't reshape the tree.
- Initialized the Go module (`github.com/aperture/aperture`) and pinned to Go 1.25 (driven by `modernc.org/sqlite` requiring it). **Why:** pure-Go SQLite removes CGO from the cross-compile story, which matters for multi-arch homelab deploys later.
- Initialized SvelteKit with the `static` adapter and Svelte 5 runes. **Why:** static SPA can be served by the Go binary with one `-web-dir` flag (and embedded later via `embed.FS`); runes mode is the modern Svelte API and avoids the legacy reactive-statement footguns.
- Added `Makefile` with `build`, `hub`, `agent`, `web`, `tidy`, `dev`, `run`, `clean`. **Why:** one entry point for both subprojects so contributors don't need to memorize the sub-build commands.
- Added `.gitignore` covering `bin/`, `web/build/`, `web/.svelte-kit/`, `web/node_modules/`, `*.db*`, editor noise. **Why:** keep build output and local state out of version control.

### Added — Backend

- **`internal/types`** — shared types (`Host`, `MetricSample`, `Container`, `PortMapping`, `HostInfo`). **Why:** leaf package avoids import cycles between `store`, `hub`, `api`, `collector`, `dockerctl`. Every host-scoped record carries a `host_id` so multi-host support is a transport question, not a schema change.
- **`internal/store`** — SQLite store with embedded `schema.sql`. WAL mode, foreign keys on, 5s busy timeout. Tables: `hosts`, `metrics` (PK `(host_id, ts)`), `alert_rules`, `alert_events`. Methods: `Open/Close`, `UpsertHost`, `TouchHost`, `ListHosts`, `GetHost`, `InsertMetric`, `LatestMetric`, `MetricsRange` (with uniform-stride downsampling), `PruneMetrics`. **Why:** WAL is required so the hot ingest path doesn't block API reads; PK on `(host_id, ts)` rejects accidental duplicates; the schema includes alerting tables now so adding the evaluator later doesn't need a migration.
- **`internal/collector`** — local-host metric source (gopsutil). `Local.Run` ticks at `interval`, drops on backpressure rather than blocks, primes `cpu.Percent` once at start so the first sample is non-zero. **Why:** matches the multi-host invariant — it's just one impl of `hub.MetricSource`. Backpressure drop avoids stalling collection cadence if the consumer is slow.
- **`internal/dockerctl`** — docker engine wrapper bound to a `host_id`. `List` includes per-container CPU/mem/net stats; CPU normalized to total cores so >100% on multi-core hosts is meaningful. Lifecycle wrappers: `Start/Stop/Restart/Pause/Unpause/Kill/Remove`. `Logs` strips docker's 8-byte multiplexed header so payloads are clean text. **Why:** binding to host_id at construction means callers don't worry about cross-host plumbing; future remote agents will register a `DockerProvider` for their host and the API layer keeps working.
- **`internal/hub`** — orchestration. Defines `MetricSource` and `DockerProvider` interfaces. `Hub.RegisterSource` derives a stable `host_id` from `(source + name)` (sha1 truncated), upserts the host row, and runs the source against a per-source channel adapter that stamps `host_id` onto every sample. `ingestLoop` persists samples and bumps `last_seen`. `retentionLoop` prunes hourly when retention is configured. **Why:** the interfaces are the seam between v0.1 and multi-host. The id derivation is stable so the local host keeps the same row across hub restarts; remote agents will provide their own UUIDs.
- **`internal/api`** — chi-based router under `/api`, plus optional SPA file server with `index.html` fallback. Endpoints: `/api/health`, `/api/hosts`, `/api/hosts/{id}`, `/api/hosts/{id}/metrics/latest`, `/api/hosts/{id}/metrics`, `/api/hosts/{id}/containers`, `/api/hosts/{id}/containers/{cid}/{action}`, `/api/hosts/{id}/containers/{cid}` (DELETE), `/api/hosts/{id}/containers/{cid}/logs`. Dev-only CORS allowlist for `localhost`. **Why:** namespacing under `/api` lets the SPA live at `/`. SPA fallback is needed for client-side routing (otherwise refreshing on `/hosts/abc` 404s). CORS shim is permanent in the chain so dev never has to remember to enable it.
- **`cmd/hub`** — binary entry. Flags: `-listen`, `-db`, `-interval`, `-retain`, `-disk-path`, `-web-dir`. Env-var overrides for each. Signal-driven graceful shutdown with a 5s shutdown deadline. **Why:** single-binary deploy with sensible defaults (`:8080`, 5s interval, 14d retention). `-web-dir` is the SPA seam; embedding can follow when the frontend stabilizes.
- **`cmd/agent`** — placeholder binary that prints a "not implemented in v0.1" message. **Why:** the package exists from day 1 so the multi-host structure is visible and so build/CI pipelines targeting `./...` already cover it.

### Added — Frontend

- **`src/lib/types.ts`** — TS mirrors of Go types. Hand-synced for v0.1.
- **`src/lib/api.ts`** — typed HTTP client with `VITE_API_BASE` resolution (dev: `http://localhost:8080`, prod: same-origin). Throws on non-2xx with body context. **Why:** dev/prod base resolution removes manual config; informative errors are critical for fast iteration.
- **`src/lib/format.ts`** — `formatBytes`, `formatPct`, `formatDuration`, `relTime`. Defensive against `NaN/Infinity`.
- **`src/lib/styles.css`** — global dark theme via CSS custom properties; primitives (`.card`, `.bar`, `.pill`, `.grid`); imports `uPlot.min.css`. **Why:** custom properties make theming pluggable; primitives keep page CSS terse.
- **`src/lib/Bar.svelte`** — value-based color progress bar (warn ≥ 75%, bad ≥ 90%).
- **`src/lib/Chart.svelte`** — uPlot wrapper that updates data via `plot.setData` and resizes via `ResizeObserver`. **Why:** uPlot is ~45 KB, fast for dense series, and we get the underlying instance for future zoom/pan.
- **`src/routes/+layout.ts`** — `ssr = false; prerender = false`. Pure SPA. **Why:** data is live, hub serves a fallback `index.html`, no need for SSR.
- **`src/routes/+layout.svelte`** — sticky top nav.
- **`src/routes/+page.svelte`** — host list dashboard, 5s auto-refresh, parallel per-host fetch, individual error tolerance. **Why:** one bad host shouldn't blank the page.
- **`src/routes/hosts/[id]/+page.svelte`** — per-host detail with stat cards (CPU/Mem/Disk/Uptime+Load) and time-series charts (CPU, Memory, Disk, Network throughput rates, Load average). Range picker (15m / 1h / 6h / 24h). Network rates are derived client-side from cumulative counters so historical samples aren't recomputed if the rate definition changes.
- **`src/routes/hosts/[id]/containers/+page.svelte`** — container management table with state-aware action buttons, port labels, CPU/mem bars, and a logs modal. **Why:** state-aware buttons keep the UI uncluttered (no "Start" on a running container).

### Verified

- `go build ./...` succeeds.
- `go vet ./...` clean.
- `npm run build` produces a static SPA in `web/build/` (~57 KB largest chunk gzipped).
- End-to-end smoke test: hub started against a real homelab box, registered the local host, collected 25 samples in 25s, and listed 18 running containers (homeassistant, vaultwarden, dockge, etc.) with live CPU/mem stats. SPA fallback verified by hitting `/hosts/abc123` and getting `index.html` (1301 bytes), same as `/`.

### Deferred from this version

- Threshold-based alert evaluator + UI (schema is in place; no logic yet).
- Container *create*.
- Authentication.
- Embedded frontend via `embed.FS`.
- Remote-agent transport.

---

## [0.1.0-alpha.2] — 2026-05-05

Adds the threshold-based alert evaluator end-to-end: evaluator package, store CRUD, hub wiring, REST endpoints, and a SvelteKit management page with a layout-wide firing-count badge.

### Added — Backend

- **`internal/types`** — `AlertRule` and `AlertEvent`. `AlertRule.HostID` is `*string` so `nil` (applies to all hosts) is distinguishable from an empty-string id; `AlertEvent.ResolvedAt` is `*time.Time` so the open-only DB query stays a clean `WHERE resolved_at IS NULL`. **Why:** the schema already had these tables; the types complete the matching shape so handlers and the evaluator can share one vocabulary.
- **`internal/store`** — alert CRUD: `ListAlertRules`, `ListEnabledRulesFor` (evaluator hot path), `GetAlertRule`, `CreateAlertRule`, `UpdateAlertRule`, `DeleteAlertRule`, `InsertAlertEvent`, `ResolveAlertEvent` (idempotent — guards against double-resolve overwriting timestamps), `ListAlertEvents` + `AlertEventFilter`. Helper `scanAlertRule` centralizes the `sql.NullString` host_id mapping. **Why:** the evaluator needs a fast per-host enabled-rules query separate from the broader UI listing; bundling them shared the scan code without coupling the call patterns.
- **`internal/alerts`** — new package. `Evaluator` with `pending` (transient first-breach timestamps) and `open` (currently-firing event ids) maps. `New(ctx, store, log)` rehydrates the `open` map from open events at startup so a hub restart doesn't double-fire. `Evaluate` is host-scoped — only rules applicable to the sample's host get evaluated. `evalOne` enforces sustained-breach semantics: with `duration_s == 0`, fire on first breach; otherwise wait for the breach to be sustained for the window before firing. `HandleRuleDelete` clears transient state after a rule deletion so we don't leak entries that can never resolve. `MetricValue`, `compare`, `SupportedMetrics`, `SupportedOps`, and `ValidateRule` give the API a single source of truth for the metric vocabulary. **Why:** the schema put `alert_rules` and `alert_events` in place from day 1 explicitly so this work would land without a migration. Memory-only `pending` state is a deliberate tradeoff — persisting it would mean a SQLite write per rule per sample for state that's seconds-to-minutes old; the cost of restart loss is bounded to one duration window.
- **`internal/hub`** — added `Evaluator` interface (defined here to avoid an `alerts → hub` import cycle), `Hub.evaluator` field, and `SetEvaluator(e)`. `ingestLoop` now dispatches every persisted sample to `evaluator.Evaluate` after `InsertMetric` + `TouchHost` succeed. **Why:** keeping evaluation inline (rather than on a separate goroutine) keeps `fired_at` tightly correlated with the sample that caused it. The settable interface means tests/dev scripts don't have to construct an evaluator.
- **`internal/api`** — handlers: `alertsMetadata`, `listAlertRules`, `createAlertRule`, `getAlertRule`, `updateAlertRule`, `deleteAlertRule`, `listAlertEvents`. Wire DTO `alertRulePayload` with `host_id` as a regular string (empty = "all hosts") and `enabled` as a `*bool` so omitting it on create defaults to enabled rather than silently disabling the rule. Delete cascades event history via the schema and additionally calls `evaluator.HandleRuleDelete` to clear transient state. **Why:** centralising validation in `alerts.ValidateRule` (called in both create and update) makes the failure modes consistent across the surface.
- **`cmd/hub`** — constructs `alerts.New` with the store, then `h.SetEvaluator(ev)` before `Run`. Passes `ev` into `api.NewServer` so DELETE handlers can reach `HandleRuleDelete`.

### Added — Frontend

- **`src/lib/types.ts`** — `AlertRule`, `AlertEvent`, `AlertMetadata` mirrors of the Go types.
- **`src/lib/api.ts`** — new `send<T>(path, method, body)` JSON-bodied helper for create/update; alert methods `alertMetadata`, `alertRules`, `createAlertRule`, `updateAlertRule`, `deleteAlertRule`, `alertEvents`.
- **`src/routes/+layout.svelte`** — second nav link ("Alerts") with a red `.badge` showing the firing-count, polled every 5s from `/api/alerts/events?open=true`. **Why:** alerts are only useful if their visibility doesn't depend on remembering to visit a page.
- **`src/routes/alerts/+page.svelte`** — new route. Create-rule form (host selector, metric/op dropdowns from metadata, threshold + duration inputs, enabled checkbox), rules table (inline toggle + delete; row tinted when a matching open event exists), and events table (firing/resolved pill, rule summary, host, value, fired/resolved relative time). 5s auto-refresh.

### Fixed

- API responses for alert rule create + update now include `created_at`. **Why:** the schema's `DEFAULT CURRENT_TIMESTAMP` populates the row but the in-memory struct returned to the client previously had a zero time. Both handlers now read the rule back via `GetAlertRule` after writing. Caught during the smoke test (`"created_at":"0001-01-01T00:00:00Z"` in the create response).

### Verified

- `go build ./...` and `go vet ./...` — clean.
- `npm run build` — clean; new `entries/pages/alerts/_page.svelte.js` chunk is 6.73 kB (1.85 kB gzipped).
- End-to-end smoke test against the local host:
  - `POST /api/alerts/rules` with `mem_pct > 1, duration_s = 0` → fired on the next sample (~1s) with `value ≈ 39.89`.
  - `PUT` updating threshold to `99` (no longer breached) → event auto-resolved on the next sample.
  - `POST` with `mem_pct > 1, duration_s = 8` → no fire at +2s, fired by +10s as the sustained window elapsed.
  - Restart with the open event still present → no duplicate fire; the open map rehydrated from the DB.
  - `DELETE` of a rule → cascaded the event history and dropped transient state.

### Deferred

- Container *create*.
- Authentication.
- Embedded frontend via `embed.FS`.
- Remote-agent transport.
- Notification channels (email / webhook / Slack) wired off `alert_events`.

---

## [0.1.0-alpha.3] — 2026-05-06

Closes the last v0.1 docker-management gap: surface-layer container *create*. After this version, v0.1 should be ready to cut as `0.1.0` once any minor follow-ups are addressed.

### Added — Backend

- **`internal/types`** — `CreateSpec`, `PortBinding` (create-side), `VolumeBinding` (create-side). The create-side bindings are intentionally separate types from the read-side `PortMapping` because create is asymmetric: we send a binding request, not echo a docker snapshot. **Why:** keeping the two shapes apart prevents drift and makes the surface form's validation rules explicit (e.g. `host_port=0` legitimately means "let docker pick", which has no equivalent on the read side).
- **`internal/dockerctl`** — `Create(ctx, spec)` and the helper `buildCreateConfig(spec)`. Image is pull-on-not-found rather than always-pull (fast common case, transparent for fresh images). The pull progress stream is drained so the pull actually completes. If `AutoStart` is true and the start fails, the container id is returned alongside the error so the API layer can surface it as a partial-success rather than swallow the half-built container. **Why:** the surface form should "just work" for new images without forcing a separate manual pull step; partial-success is honest UX vs throwing away a created-but-not-started container.
- **`internal/hub`** — `DockerProvider` interface gains `Create`. The compile-time `var _ DockerProvider = (*dockerctl.Client)(nil)` already in place catches drift the next time it's edited.
- **`internal/api`** — new `containerCreate` handler at `POST /api/hosts/{id}/containers`. Returns `201 Created` with `{id}` on full success, `202 Accepted` with `{id, warning}` on partial success, `502 Bad Gateway` with the error message on total failure. **Why:** the three-status taxonomy is what lets the UI render an actionable "created but didn't start" state instead of leaking a blanket 500.

### Added — Frontend

- **`src/lib/types.ts`** — `CreateSpec`, `CreatePortBinding`, `CreateVolumeBinding` mirrors of the new Go types.
- **`src/lib/api.ts`** — `createContainer(hostID, spec)` returning `{id, warning?}`.
- **`src/routes/hosts/[id]/containers/+page.svelte`** — added a **"+ New container"** button in the page header that opens a modal create form. Form fields: image (required, autofocused), name (optional), restart policy dropdown, auto-start checkbox, plus three sections with dynamic add/remove rows for env vars (KEY/VALUE), port mappings (host → container with tcp/udp dropdown), and volume binds (host → container with `ro` checkbox). Surface-only — capabilities, healthchecks, ulimits, etc. are intentionally absent and will live in the compose-first YAML editor that ships with roadmap section 2.

### Tooling

- **`.gitignore`** — cleaned up the deduplicated/typo-ridden file (the previous version had `bin/aperture-hub` listed three times) and replaced it with a structured one that explicitly covers `aperture.db` and its `-shm`/`-wal` companions, the `bin/` build outputs, the SvelteKit build artifacts, common editor/OS noise, and `.env*`. Also untracked `aperture.db`, `bin/aperture-hub`, and `bin/aperture-agent` via `git rm --cached` — these had been accidentally committed in an earlier session. **Why:** keeping the binary and the live SQLite database out of version control prevents history bloat, prevents the database lock from being noisy in `git status`, and avoids leaking host-local state to anyone who clones the repo.

### Verified

- `go build ./...` and `go vet ./...` clean.
- `npm run build` clean.
- End-to-end smoke test against the local docker daemon:
  - `POST /api/hosts/{id}/containers` with `{image: "nginx:alpine", name: "aperture-smoketest", restart_policy: "unless-stopped", ports: [{host_port: 18080, container_port: 80, protocol: "tcp"}], env: {HELLO: "world"}, auto_start: true}` → `201` with the new id.
  - Container appeared in the `containers` list with `state: running`, `Up 12 seconds`, port mapping visible, low CPU/mem.
  - `curl http://localhost:18080/` against the published port → `HTTP 200 (896 bytes)` from nginx.
  - Pull-on-not-found path exercised by creating a `hello-world:latest` container with `auto_start: false` → `201`.
  - Cleanup via existing stop/remove endpoints worked.

### Deferred (intentional, lands later)

- Deep container spec editor (capabilities, healthchecks, ulimits, security opts, network aliases, named volumes, multi-network attach) → roadmap section 2 (compose-first), where YAML is the natural surface.
- Container *recreate* with the same spec (i.e. update an existing container's config) → roadmap section 1 ("Container Lifecycle Complete").
- Image management UI (list, pull progress visibility, dangling cleanup) → roadmap section 3.

---

## [0.1.0-alpha.4] — 2026-05-06

v0.1 polish pass — chart layout fix and basic operational visibility (version + DB size + uptime in the layout footer). Caught from running v0.1 in the browser.

### Fixed

- **Chart legend overlap** — `web/src/lib/Chart.svelte` was leaving uPlot's built-in legend enabled. On single-series charts (CPU%, Mem%, Disk%) it wasted a row on a redundant `time -- [hover]` line; on multi-series charts (network rx/tx, load 1/5/15) the rows stacked vertically and bled into the next chart card's title, with `time` and series labels overlapping the Y-axis ticks (visible in screenshots from the v0.1 walkthrough). **Why:** the previous code relied on uPlot to size its own legend within our fixed-height container, but the legend's vertical growth wasn't accounted for, so it overflowed every chart card. **Fix:** disable uPlot's legend (`legend: { show: false }`) entirely; render a compact chip-row legend (color dot + series label) above the canvas in the wrapper *only* when there are 2+ series. Single-series charts now have zero legend chrome — the parent's chart-title `<div>` is sufficient. Hover values still work through uPlot's cursor.

### Added — Backend

- **`internal/store`** — `Path()` accessor returning the on-disk path passed to `Open`. **Why:** the API needs to `os.Stat` the database (and its WAL/SHM companions) for size reporting; threading the path through every layer would couple `cmd/hub` to `internal/api`, while a single accessor on the Store keeps the dependency one-way.
- **`internal/types`** — `SystemInfo` shape: `version`, `started_at`, `db_path`, `db_size_bytes`. The DB size is the *sum* of `aperture.db` + `aperture.db-wal` + `aperture.db-shm` so the user sees the real on-disk footprint between WAL checkpoints, not just the main file. **Why:** "DB is 4 KiB" when there's actually 150+ KiB of WAL would be misleading for retention-budget decisions.
- **`internal/api`** — new `systemInfo` handler at `GET /api/system/info`. The `Server` struct now carries `version` and `startedAt`; the helper `sizeOnDisk(path)` returns 0 (rather than erroring) for absent files because WAL/SHM files come and go around checkpoints — absence is normal. `NewServer` signature gained two parameters: `version string, startedAt time.Time`.
- **`cmd/hub`** — added `const Version = "0.1.0-alpha.4"`, captured `startedAt := time.Now().UTC()` at the top of `main`, and now logs `aperture hub starting version=… db=… listen=…` at boot. Threads version + startedAt into `api.NewServer`. **Why:** versioning the binary is the API-first principle from the roadmap's architecture-considerations list — clients (current WebUI footer, future CLI/mobile) should be able to discover what they're talking to.

### Added — Frontend

- **`src/lib/types.ts`** — `SystemInfo` mirror of the Go type.
- **`src/lib/api.ts`** — `systemInfo()` returning `SystemInfo`.
- **`src/routes/+layout.svelte`** — added a centered footer rendering `vX.Y.Z · DB <size> · uptime <duration>`. The footer polls `/api/system/info` every 30s (DB size doesn't change quickly) and uses a separate 1-second in-memory clock to tick uptime visibly without re-hitting the API. The DB path is shown as a tooltip on hover for users who want to know where state lives. **Why:** the user explicitly asked for an obvious always-visible indicator of database size; surface-layer info belongs on every page, not behind a settings menu (per the design philosophy).

### Verified

- `go build ./...` and `go vet ./...` clean.
- `npm run build` clean; the chart fix added a few hundred bytes for the chip-row markup.
- End-to-end smoke test against a local DB: `/api/system/info` returned `db_size_bytes: 189336` which is exactly `4096 + 32768 + 152472` — the sum of `aperture-final.db` (4 KiB) + `-shm` (32 KiB) + `-wal` (152 KiB). WAL is ~80% of the on-disk total in this point-in-time, confirming why summing matters.
- Chart fix verification deferred to in-browser inspection — visual regression is what motivated the fix.

---

## [0.1.0] — 2026-05-08

**First usable release.** Aperture's v0.1 scope is met: the local host's system metrics and docker containers are visible, manageable, and alertable from a single web UI. The hub-and-agent architecture is scaffolded but only the local source is wired in v0.1; remote agents come in a later section.

### What's in 0.1.0

- **Multi-host data model from day 1.** Hosts table, metrics tagged by `host_id`, `MetricSource` and `DockerProvider` interfaces in `internal/hub` so a remote-agent transport drops in without restructuring core types.
- **Local host monitoring.** Auto-registered local source samples CPU, memory, swap, disk, network, load average, and uptime via gopsutil at a configurable interval (default 5s). Stored in SQLite (modernc.org pure-Go driver, WAL journaling) with hourly retention pruning.
- **Web dashboard.** Host list with auto-refreshing status cards; per-host detail with five charts (CPU, memory, disk, network throughput, load average) across 15m / 1h / 6h / 24h ranges; uPlot-based rendering with a chip-row legend for multi-series charts.
- **Docker container management.** List with live CPU/memory/network stats, lifecycle actions (start, stop, restart, pause, unpause, kill, remove), log streaming modal, and surface-layer create form (image, name, restart policy, env, ports, volumes, auto-start) with image pull-on-not-found.
- **Threshold-based alerts.** Rule CRUD UI on `/alerts`; sustained-breach `duration_s` semantics with in-memory pending state and persisted open events; auto-resolve on threshold-clear; restart rehydration of open events from the DB; nav badge with currently-firing count.
- **Operational visibility.** `/api/system/info` exposes hub version, started-at, DB path, and total on-disk size (db + WAL + SHM); rendered in a layout footer that ticks uptime locally.

### Cumulative summary of work that landed across the alphas

This `[0.1.0]` rolls up every change from `[0.1.0-alpha.1]` through `[0.1.0-alpha.4]` plus the polish entries below. See the alpha entries below for per-package detail and rationale on each piece.

### Fixed (post-alpha.4 polish)

- **Chart canvas misaligned by 133 px (uPlot stylesheet was silently dropped).** The actual root cause of the chart layout problems users saw in alpha.4 wasn't only the legend — uPlot's stylesheet (`uplot/dist/uPlot.min.css`) was never being applied at all because it was imported with `@import` at the *bottom* of `web/src/lib/styles.css`. Per the CSS spec, `@import` rules must precede every other rule in a stylesheet (only `@charset`/`@layer` may come before); when an `@import` follows declarations it's silently invalid and the bundler drops it. Without uPlot's CSS, `.u-wrap` lost its `position: relative`, and `.u-under`/`.u-over`/`.u-axis` lost their `position: absolute`, so the canvas was no longer overlaid on the chart wrap — it stacked beneath the in-flow `u-under` element, ending up offset by exactly the plot-area height (133 px in our 200 px chart) and overflowing into the next chart card. **Confirmed via CDP inspection** that `canvas.top - u-wrap.top = 133` and that the built CSS bundle contained zero uPlot rules. **Fix:** move the import to JavaScript — `import 'uplot/dist/uPlot.min.css';` in `+layout.svelte` — so it's bundled regardless of CSS source-order. The `.uplot { font-family: inherit }` override stays in `styles.css` (now correctly cascading after the JS-imported uplot rules thanks to module evaluation order). **Verification:** headless chromium screenshot at 1280×2200 shows all five charts rendering cleanly within their card boundaries with proper Y-axis spans and no inter-card overlap.

### Changed

- **`cmd/hub.Version`** bumped to `"0.1.0"` (was `"0.1.0-alpha.4"`). Surfaced via `/api/system/info` and rendered in the layout footer.
- **`overview.md`** — `**v0.1 (in progress)**` → `**v0.1.0** … The first usable release.`

### Docs

- **`technical.md`** — added a uPlot-CSS-import warning under the Chart entry: `@import` for uPlot.min.css must be at the very top of a stylesheet (or imported via JS). Easy to regress, hard to spot since charts still partly render — they just sit at the wrong vertical offset.
- **`overview.md`** (carried over from `[Unreleased]` polish before the version bump) — added a "Design philosophy: clean surface, deep ability" section documenting the two-layer UX rule from `/opt/aperture-roadmap.docx`; replaced the short post-v0.1 roadmap stub with the full eight-section outline plus architecture-considerations and future-scope lists, pointing at the docx as canonical source.
- **`technical.md`** (carried over) — added a top-of-document "Design constraint" callout pointing to the design philosophy and how to apply it during code review.

### Known limitations / explicitly deferred

- No remote agents — the hub embeds a local source. Multi-host transport is roadmap section 1.
- No authentication — single-user homelab assumption. Roadmap section 8.
- No embedded frontend in the binary — `-web-dir` is the seam. Will switch to `embed.FS` once the frontend stabilizes.
- Container *create* form is the surface layer only (image, name, restart, env, ports, volumes). Deep config (capabilities, healthchecks, ulimits, etc.) lives with compose-first work in roadmap section 2.
- Container *recreate* (update a container's config) — roadmap section 1.
- Image management UI (pull progress, dangling cleanup, layer inspection) — roadmap section 3.
- No notification channels for alerts (Discord, Slack, ntfy, email, SMS) — roadmap section 5.

---

## [0.2.0-alpha.1] — 2026-05-08

Roadmap section 1 monitoring and docker management depth. Adds rich live metrics (per-core CPU, per-interface network, disk mounts, disk I/O, temperatures), full container inspect/resource-edit/recreate lifecycle, container sorting and filtering, and per-host sub-navigation with placeholder pages for upcoming docker-depth features.

### Added — Backend

- **`internal/types`** — Six new types for rich live metrics: `NetInterfaceSample` (per-interface rx/tx bytes + rates), `DiskMountSample` (mount, device, fstype, used/total/pct), `DiskIOSample` (per-device read/write bytes + rates), `TempSample` (sensor name + celsius). `MetricSample` gains optional live-only fields for these (`cpu_per_core`, `net_interfaces`, `disk_mounts`, `disk_io`, `temps`) plus `mem_avail` and `mem_cached` — all `omitempty` so the stored/historical shape is unchanged. Also `ContainerInspect` (full docker container detail: timestamps, config, env, ports, mounts, labels, live stats, resource limits), `ResourceUpdate` (nano_cpus and memory_bytes as pointers so 0 is "unlimited", not "unset"). **Why:** separating live-only and stored fields keeps the SQLite schema and historical queries stable while the live view gets arbitrarily richer.
- **`internal/collector`** — Major expansion. `Local` now tracks `prevNetIO` and `prevDiskIO` maps across samples to compute bytes/sec rates. New methods: `diskMounts` (reads `disk.PartitionsWithContext`, filters pseudoFS types and Docker overlay/overlay2 paths, calls `disk.UsageWithContext` per mount), `netIfaces` (reads per-interface counters, skips `lo` and `veth*` for the Docker container virtual links, computes rate via delta/elapsed), `diskIO` (reads `disk.IOCountersWithContext`, skips loop/ram/zram devices, computes rates). Temperature collection via `sensors.TemperaturesWithContext` from the separate gopsutil v4 `sensors` package (moved there from `host` in v4). Per-core CPU via `cpu.PercentWithContext(..., true)`. **Why:** `prevNetIO`/`prevDiskIO` maps are the only correct way to derive rates from cumulative OS counters; storing them in the struct (rather than computing across sequential channel sends) keeps the derivation self-contained and correct even if a sample is dropped.
- **`internal/hub`** — Added `latestRich map[string]types.MetricSample` field. `ingestLoop` stores the full sample in this map (under the mutex) before inserting into SQLite. New `LatestSample(hostID) (MetricSample, bool)` accessor. **Why:** the rich live-only fields (per-core, per-interface, etc.) are not stored in SQLite (no schema migration, no column bloat). The in-memory cache is the only way to expose them. `/metrics/latest` now prefers the cache so callers always get the richest possible snapshot.
- **`internal/dockerctl`** — `Inspect(ctx, id)` returns a full `*types.ContainerInspect` by calling `ContainerInspect` then one-shot stats. `buildInspect` maps Docker SDK's `ContainerJSON` to `ContainerInspect` — handles timestamps (`*time.Time` for started/finished), ports from `NetworkSettings.Ports`, mounts. `UpdateResources(ctx, id, update)` calls Docker SDK's `ContainerUpdate` with `container.Resources{NanoCPUs, Memory}` for live CPU/memory limit changes without a recreate. **Why:** live limit changes (cgroups) don't require a stop/start; surfacing both paths (update-in-place and recreate) gives the operator flexibility.
- **`internal/hub`** — `DockerProvider` interface extended with `Inspect` and `UpdateResources` so the compile-time assertion catches new methods.
- **`internal/api`** — Four new container endpoints, registered before the generic `{action}` route so chi's static-segment matching takes precedence: `GET .../inspect` → `containerInspect`, `PUT .../resources` → `containerUpdateResources`, `POST .../recreate` → `containerRecreate`, `GET .../logs` moved up to same block. `containerRecreate` uses `inspectToSpec` to rebuild a `CreateSpec` from `ContainerInspect` (image, name sans slash, restart policy, env, ports, mounts), then stop → remove → create. **Why:** recreate is stop/remove/create with the same config — surfacing it as one atomic endpoint keeps the UI simple and avoids partial-recreate state if the network call drops mid-way.
- CORS `Access-Control-Allow-Methods` updated to include `PUT`. **Why:** `containerUpdateResources` uses PUT; without this header the browser's preflight blocks dev-mode API calls.
- **`latestMetric` handler** — now checks `hub.LatestSample` first and returns the in-memory rich snapshot when available, falling back to `store.LatestMetric` for hosts that haven't sent a sample since the current hub start. **Why:** the SQLite row never has rich live-only fields; only the in-memory snapshot does.

### Added — Frontend

- **`src/lib/types.ts`** — New interfaces: `NetInterfaceSample`, `DiskMountSample`, `DiskIOSample`, `TempSample`, `ContainerMount`, `ContainerInspect`, `ResourceUpdate`. `MetricSample` updated with optional rich fields to match the Go type.
- **`src/lib/api.ts`** — Three new container methods: `containerInspect(hostID, cid)`, `containerUpdateResources(hostID, cid, update)` (PUT), `containerRecreate(hostID, cid)` (POST).
- **`src/lib/Chart.svelte`** — Added `valueFormatter?: ((v: number) => string) | null` prop. When provided, replaces the default `Math.round(v) + valueSuffix` formatting on Y-axis ticks. **Why:** memory and disk charts need "4.2 GiB" rather than "4512 MB"; a formatter prop avoids baking unit awareness into the generic Chart component.
- **`src/routes/hosts/[id]/+page.svelte`** — Comprehensive rewrite. Now fetches `/metrics/latest` separately for the rich live snapshot alongside the historical range data. Sub-navigation added (Overview active, Containers, Networks/Volumes/Images/Logs as placeholders). New sections: per-core CPU grid (small bars labeled C0…Cn, color-coded at 75%/90% thresholds); network interfaces table (rx/tx rate + cumulative bytes, loopback and veth* filtered); disk mounts table (mount, device, fstype, used/total GB, usage bar); disk I/O table (device, read/write rate + total); temperature grid. Historical charts updated with `fmtGiB` valueFormatter for memory/disk and `fmtBytesRate` for network.
- **`src/routes/hosts/[id]/containers/+page.svelte`** — Major rewrite. Same sub-nav. Filter controls (all / running / exited / paused). Sort controls (Name / State / CPU / Mem with ascending/descending toggle). Row click expands an inline inspect panel (two-column: config left, live stats + resource limits + actions right). Config column: image, state, restart policy, cmd, ports, mounts, env (expandable), labels. Actions: restart/stop/pause/unpause/recreate (with confirm), force-remove, view logs. Resource limits: shows current nano_cpus and mem_limit_bytes with an edit form (CPU in cores × 1e9, memory in GiB × 1073741824). Logs modal upgraded to 1000-line tail with filter input.
- **`src/routes/hosts/[id]/networks/+page.svelte`**, **`…/volumes/+page.svelte`**, **`…/images/+page.svelte`**, **`…/logs/+page.svelte`** — New placeholder pages so the sub-nav links don't 404. Each shows a centered card with a brief description of what's coming. **Why:** placeholder pages are better UX than 404s — they communicate intent and breadth without requiring the features to be built yet.

### Verified

- `go build ./...` — clean.
- `npm run build` — clean; four new placeholder page chunks; no a11y errors (two a11y warnings on `role="dialog"` divs were fixed by adding `tabindex="-1"` and `onkeydown` handler).

### Deferred

- Remote agents, auth, embedded frontend — unchanged from v0.1.0 deferred list.
- Networks / Volumes / Images / Logs docker management — placeholder pages are in place; implementation is roadmap section 3.
- Notification channels for alerts — roadmap section 5.
- Container *create* spec depth (capabilities, healthchecks, ulimits, network aliases) — roadmap section 2 (compose-first).

---

## [0.2.0-alpha.2] — 2026-05-08

Full Beszel monitoring parity. Adds historical per-interface network, per-mount disk, and disk I/O charts backed by three new SQLite tables, plus a live process list (top 40 by CPU ∪ memory) and a 3-segment memory breakdown bar.

### Added — Backend

- **`internal/store/schema.sql`** — Three new tables: `net_iface_metrics (host_id, ts, iface, rx_bytes, tx_bytes)`, `disk_mount_metrics (host_id, ts, mount, device, fstype, used, total)`, `disk_io_metrics (host_id, ts, device, read_bytes, write_bytes)`. Each uses `PRIMARY KEY (host_id, ts, <entity>)` and a matching index on `(host_id, ts DESC)`. All are `CREATE TABLE IF NOT EXISTS` (idempotent — no migration step; tables appear on next hub start). **Why:** separate tables (not JSON blobs in the `metrics` row) keep queries typed, indexed, and prunable without schema gymnastics. The schema's idempotency means rolling out to an existing install requires no ALTER TABLE.
- **`internal/types`** — `ProcessSample` (`pid`, `name`, `cpu_pct`, `mem_pct`, `mem_rss`). `MetricSample` gains `Processes []ProcessSample` (`omitempty`, live-only, never stored). Six new history response types: `NetIfaceSeries`/`NetIfaceHistory`, `DiskMountSeries`/`DiskMountHistory`, `DiskIOSeries`/`DiskIOHistory` — pivoted format with a shared `timestamps` array and a per-entity series map. **Why:** the pivoted format lets the frontend render one chart per interface/mount/device with a single array slice — no per-point map lookup.
- **`internal/store`** — Three new insert methods (`InsertNetIfaces`, `InsertDiskMounts`, `InsertDiskIO`) — each opens a transaction and does `INSERT OR IGNORE` for every entity in the sample. Three new range query methods (`NetIfaceRange`, `DiskMountRange`, `DiskIORange`) — same uniform-stride downsampling pattern as `MetricsRange`: query all rows in time range, group by timestamp in Go, apply stride on unique timestamp list (always keeping the last), then pivot into the history response type using a `tsIndex` map for O(1) slot lookup when filling pre-allocated series arrays. `PruneMetrics` updated to delete from all four tables so rich history respects the same retention window as aggregate metrics. **Why:** the tsIndex approach avoids re-scanning the timestamp list for every row; pre-allocated arrays are cheaper than repeated `append` for the common case of uniform sampling.
- **`internal/collector`** — `Local` gains `procMu sync.Mutex` and `procCache map[int32]*gopsprocess.Process`. The process cache is initialized in `NewLocal` and updated every tick: dead PIDs are evicted, new PIDs are added via `gopsprocess.NewProcessWithContext`. `CPUPercentWithContext(ctx)` is called on the cached object so it measures elapsed time since the *previous* tick's call (not since process creation) — the correct behaviour matching `top`. First tick for a new process reports CPU=0; acceptable. `sample()` now calls `l.processes(ctx)` at the end and sets `s.Processes`. The method returns the union of top 20 by CPU + top 20 by RSS (up to 40 total). **Why:** without caching the process object, `CPUPercent(0)` would always return 0 for every process because gopsutil computes CPU as `delta / elapsed` since the *same object's* last call.
- **`internal/hub`** — `ingestLoop` calls `InsertNetIfaces`, `InsertDiskMounts`, `InsertDiskIO` after `InsertMetric` (best-effort: failures are logged but don't abort the loop or affect `TouchHost` / `evaluator.Evaluate`). **Why:** rich data loss is acceptable; the live view still works from the in-memory cache.
- **`internal/api`** — Three new GET routes registered *before* the existing `/metrics/latest` and `/metrics` routes so chi's static-segment matching takes precedence: `GET /api/hosts/{id}/metrics/net`, `.../metrics/mounts`, `.../metrics/diskio`. Each handler follows the same `range` + `points` query param pattern as `metricsRange` and returns an empty-but-valid struct (non-null) when the time range has no data. **Why:** empty-but-valid allows the frontend to render "no data yet" without a null check.

### Added — Frontend

- **`src/lib/types.ts`** — `ProcessSample` interface; `processes?: ProcessSample[]` added to `MetricSample`; `NetIfaceSeries`, `NetIfaceHistory`, `DiskMountSeries`, `DiskMountHistory`, `DiskIOSeries`, `DiskIOHistory` mirroring the Go history types.
- **`src/lib/api.ts`** — Three new methods: `netHistory(id, range, points)`, `diskMountHistory(id, range, points)`, `diskIOHistory(id, range, points)`.
- **`src/routes/hosts/[id]/+page.svelte`** — `load()` now fetches six things in parallel (host, metrics, latest, netHistory, diskMountHistory, diskIOHistory). `deriveRates(timestamps, bytes)` helper computes bytes/s from cumulative delta/elapsed (same pattern as the existing aggregate network chart). New derived states: `ifaceCharts` (per-interface rx/tx rates), `mountCharts` (per-mount used/total GiB), `diskIOSeries` (all devices combined read/write rates). `sortedProcs` sorts the process list by `cpu_pct` or `mem_rss` based on a `procSort` toggle. `memBreakdown` computes a 3-segment memory bar (used / cached / free) from `mem_used`, `mem_cached ?? 0`, and `mem_total`. New sections in the page: **"Processes — live"** table (Name / PID / CPU% / Mem% / RSS with CPU/Memory sort toggle); **"Network — per interface"** (one chart per interface showing rx/tx rates); **"Disk — per mount"** (one chart per mount showing used/total GiB); **"Disk I/O — per device"** (one chart for all devices showing read/write rates). Memory stat card updated to show the 3-segment bar with `.seg-used` / `.seg-cached` / `.seg-free` and a breakdown legend row.

### Verified

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `npm run build` — clean; host detail page chunk grew from ~47 kB to ~71 kB (gzip ~28 kB) for the new sections.
- Smoke test (15s, 3 samples at 5s interval):
  - `GET /api/hosts/{id}/metrics/net` → `timestamps[3]` + 10+ interfaces in `ifaces`.
  - `GET /api/hosts/{id}/metrics/mounts` → `/`, `/boot/efi`, `/mnt/appdata`, and others in `mounts`.
  - `GET /api/hosts/{id}/metrics/diskio` → `nvme0n1` and partition devices in `devices`.
  - `GET /api/hosts/{id}/metrics/latest` → `processes[40]`, top entries showing non-zero `cpu_pct` and `mem_rss`.

### Deferred

- Process list historical storage (live-only, like Beszel) — permanently.
- GPU monitoring, ZFS / RAID / SMART — roadmap section 4.
- Remote agents, auth, embedded frontend — unchanged from v0.1.0 deferred list.

---

## [0.2.0-alpha.3] — 2026-05-09

Chart UX improvements and extensible alert notification channels (roadmap section 5 start).

### Added — Alert notification channels

- **`internal/store/schema.sql`** — New `alert_channels` table (`id`, `name`, `type`, `config` JSON, `enabled`, `min_severity`, `notify_resolve`, `created_at`). `alert_rules` gains a `severity` column (`'info'|'warning'|'critical'`, default `'warning'`).
- **`internal/types`** — `AlertChannel` struct (mirrors the table). `AlertRule` gains `Severity string`.
- **`internal/store`** — `Open` now runs an idempotent `ALTER TABLE alert_rules ADD COLUMN severity` migration (silently ignores "duplicate column name" so existing DBs upgrade transparently). New channel CRUD: `ListAlertChannels`, `ListEnabledChannels`, `GetAlertChannel`, `CreateAlertChannel`, `UpdateAlertChannel`, `DeleteAlertChannel`. Updated `scanAlertRule` to include severity; updated `CreateAlertRule`/`UpdateAlertRule` to persist it.
- **`internal/alerts/notify.go`** — New `Notifier` type with `NewNotifier(st, log)` and `Dispatch(ctx, event, rule, resolved)`. Dispatch loads enabled channels, filters by `SeverityLevel(ch.MinSeverity) <= SeverityLevel(rule.Severity)` and `ch.NotifyResolve`, then calls `buildSender(ch).Send(ctx, n)` in a goroutine per channel. `SeverityLevel("info")=0`, `"warning"=1`, `"critical"=2`. `BuildSender` (exported) lets the API test handler call it directly without a full Dispatch.
- **`internal/alerts/ch_discord.go`** — Discord webhook sender: rich embed with title, description, fields (host/metric/value/threshold/severity), color-coded by severity (critical=red/`#e74c3c`, warning=orange/`#f39c12`, info=blue/`#3498db`, resolved=green/`#2ecc71`).
- **`internal/alerts/ch_slack.go`** — Slack incoming-webhook sender: attachment with `danger/warning/good` color, field rows, title text.
- **`internal/alerts/ch_ntfy.go`** — ntfy sender: POST to `{url}/{topic}` with `Title`, `Priority` (auto-mapped from severity: critical=urgent, warning=high, info=default; resolved=low), `Tags` (🚨 or ✅ emoji tag). Optional bearer-token auth.
- **`internal/alerts/ch_gotify.go`** — Gotify sender: POST to `{url}/message?token={token}` with priority auto-mapped (critical=10, warning=5, info=1, resolved=2).
- **`internal/alerts/ch_webhook.go`** — Generic webhook sender: POST (or configured method) with a JSON payload (`type`, `host`, `rule`, `event`, `resolved_at`). Optional custom headers map.
- **`internal/alerts/alerts.go`** — `Evaluator` gains `notifier *Notifier` field and `SetNotifier(n)` setter. `fire()` now calls `go e.notifier.Dispatch(ctx, ev, r, false)` after persisting the event. The resolve path in `evalOne` calls `go e.notifier.Dispatch(ctx, ev, r, true)` after `ResolveAlertEvent` succeeds.
- **`internal/api`** — `Server` gains `notifier *alerts.Notifier`. `NewServer` accepts it. Six new routes under `/api/alerts/channels/`: `GET` list, `POST` create, `GET /{id}`, `PUT /{id}`, `DELETE /{id}`, `POST /{id}/test`. Test handler builds a synthetic `Notification` (cpu_pct > 75, value 75.5) and calls `BuildSender(ch).Send(ctx, notif)` — validates config before it's first used in anger. `alertsMetadata` response extended with `severities` and `channel_types` fields.
- **`cmd/hub/main.go`** — Constructs `alerts.NewNotifier(st, log)` and calls `ev.SetNotifier(notif)`. Passes `notif` to `api.NewServer`.

### Added — Chart UX

- **`web/src/lib/Chart.svelte`** — Complete rewrite:
  - **Rich hover tooltip**: `hooks.setCursor` driven, shows timestamp + per-series value + colored dot for each series in an absolutely-positioned overlay div; smart left/right flip when near right edge.
  - **Dynamic Y-axis sizing**: `axis.size` function measures the longest tick label string (at 7.5px/char + 20px padding) — fixes labels like "29.55 GiB" being clipped at the edge.
  - **Hex-alpha gradient fills**: `stroke + '29'` (16% opacity) replaces the previous gradient-function approach which crashed the draw cycle when `u.bbox.height == 0` on first render.
  - **Drag-to-zoom**: `cursor.drag: { x: true }` enabled.
  - **Double-click to reset**: `hooks.ready` registers a `dblclick` listener on `u.over` that calls `u.setScale('x', {min, max})` back to the full data range.
  - **`fill: false` series prop**: pass `fill: false` to suppress area fill on reference/total lines.
  - **Zoom hint**: shows "drag to zoom · double-click to reset" below multi-series charts.
- **`web/src/routes/hosts/[id]/+page.svelte`** — Charts reorganized into a 2-column responsive grid (`.chart-grid`, `grid-template-columns: repeat(auto-fill, minmax(420px, 1fr))`). Load average chart spans full width (`.span-full`). "Total" series in memory/disk/mount charts use `fill: false` to suppress area fill on the reference line.

### Added — Frontend (notifications)

- **`src/lib/types.ts`** — `AlertChannel` interface; `AlertRule.severity` added; `AlertMetadata` extended with `severities` and `channel_types`.
- **`src/lib/api.ts`** — Five new alert-channel methods: `alertChannels`, `createAlertChannel`, `updateAlertChannel`, `deleteAlertChannel`, `testAlertChannel`.
- **`src/routes/alerts/+page.svelte`** — Major rewrite:
  - **Tab bar** — Rules / Events / Channels tabs.
  - **Rules tab** — New rule form gains a Severity selector (info/warning/critical). Rules table gains a colored severity badge column.
  - **Channels tab** — Card list of configured channels; each shows type badge, min-severity pill, resolve toggle state, plus Test / Edit / Enable-toggle / Delete actions. "+ Add Channel" button opens the modal.
  - **Add/Edit Channel modal** — Type selector (icon buttons: Discord / Slack / ntfy / Gotify / Webhook). Name field. Type-specific config fields (Webhook URL for Discord/Slack; URL + Topic + Token + Priority for ntfy; URL + Token + Priority for Gotify; URL + Method + Headers key-value rows for generic webhook). Min severity radio group. Notify-on-resolve checkbox. Enabled checkbox. Test button (saved channels only), Save and Cancel.

### Verified

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `npm run build` — clean.

---

## [0.2.0-alpha.5] — 2026-05-09

Phase 1 complete: remote agent transport, token auth, and agent onboarding UI.

### Added — Remote agent transport

- **`github.com/coder/websocket`** — added as the WebSocket library (pure-Go, no CGO).
- **`internal/store/schema.sql`** — new `agent_tokens` table (`id`, `name`, `token_hash UNIQUE`, `created_at`, `last_used`, `revoked`). `hosts` gains an `agent_version` column.
- **`internal/store/store.go`** — token CRUD: `CreateAgentToken` (generates 32-byte random token, stores SHA-256 hash, returns plaintext once), `ListAgentTokens`, `RevokeAgentToken`, `VerifyAgentToken` (hash comparison + last_used update). `UpsertHost`, `ListHosts`, `GetHost` updated to include `agent_version`.
- **`internal/types/types.go`** — `AgentToken` struct; `Host.AgentVersion` field.
- **`internal/hub/agentws.go`** (NEW) — `AgentHandler`: manages all active agent WebSocket sessions. `ServeHTTP`: token auth (Bearer header) → WebSocket upgrade → hello handshake (10s timeout) → host upsert → ack frame → metric/heartbeat/docker_resp receive loop → deregister on disconnect. `agentDockerProvider`: implements `hub.DockerProvider` by forwarding all 11 methods (list, inspect, start, stop, restart, pause, unpause, kill, remove, logs, update_resources, create) over the WS via a pending-channel request/response pattern with 30s timeout. Disconnected sessions drain all pending requests with a clear "agent disconnected" error so API callers don't hang.
- **`cmd/agent/main.go`** — full agent implementation. Flags: `--hub` (required, e.g. `http://hub-ip:8080`), `--token` (required), `--name` (override hostname), `--interval` (default 5s), `--disk` (disk path to monitor, default `/`), `--no-docker`. Uses the same `collector.Local` and `dockerctl` packages as the hub's local collector. Reconnects with exponential backoff (2s → 60s). Heartbeat every 5s. Dispatches hub docker_req frames to local docker and sends docker_resp frames back.
- **`internal/api/api.go`** — `Server` gains `agentHandler *hub.AgentHandler`. `NewServer` accepts it. New routes: `GET /api/agents/ws` (WS upgrade, handled by AgentHandler), `GET /api/agents/tokens`, `POST /api/agents/tokens`, `DELETE /api/agents/tokens/{id}`, `GET /api/agents/connected`.
- **`cmd/hub/main.go`** — constructs `hub.NewAgentHandler(h, st, log)` and passes it to `api.NewServer`. Version bumped to `0.2.0-alpha.4`.

### Added — Agent onboarding UI

- **`web/src/routes/settings/+page.svelte`** (NEW) — Settings page. Token management table (name, created, last used, revoke). Empty state with contextual help. "+ Add agent" button opens a two-step wizard:
  - Step 1: enter a name (e.g. `nas-box`) → Generate Token
  - Step 2: copy-ready command shown in a code block with tab toggle between Binary and Docker variants. Hub URL auto-detected from `window.location.origin`. One-time token warning banner. Clipboard copy button with confirmation feedback. ESC closes.
- **`web/src/routes/+layout.svelte`** — Settings nav link added.
- **`web/src/routes/+page.svelte`** — "+ Add agent" button in dashboard header links to Settings. Agent host cards get a subtle cyan `agent` source badge in the name row (with hover tooltip showing agent version). Badge CSS added.
- **`web/src/routes/hosts/[id]/+page.svelte`** — Agent version shown inline in the host platform/arch row when `source === 'agent'`.
- **`web/src/lib/types.ts`** — `AgentToken` interface; `Host.agent_version` optional field.
- **`web/src/lib/api.ts`** — `agentTokens`, `createAgentToken`, `revokeAgentToken`, `connectedAgents` methods.

### Verified

- `go build ./... && go vet ./...` — clean.
- `npm run build` — clean.

---

## [0.2.0-alpha.4] — 2026-05-09

UI/UX quality-of-life pass across all pages.

### Added — Format utilities

- **`web/src/lib/format.ts`** — `formatBytesRate(bps)` — auto-scales bytes/s through B/s → KiB/s → MiB/s → GiB/s. `formatDuration` now includes seconds for durations < 1 min (e.g. `42s`, `1m 30s`). `absTime(iso)` returns a locale-formatted absolute timestamp string for use in `title` attributes.

### Added — Toast notification system

- **`web/src/lib/toast.ts`** — Svelte writable store driving an auto-dismiss toast queue. `toast.info`, `toast.success`, `toast.error(msg, durationMs)` add toasts; `toast.remove(id)` dismisses immediately. Auto-dismiss is 4s for info/success, 6s for errors.
- **`web/src/lib/Toast.svelte`** — Fixed-position stack (bottom-right). Slide-in animation. Color-coded left border (accent=info, green=success, red=error). Per-toast dismiss button. `aria-live="polite"` for accessibility.
- **`web/src/routes/+layout.svelte`** — `<Toast />` added to layout so toasts appear globally; individual pages no longer need to mount it.

### Added — Dashboard (`/`)

- **Host status pill** — each card now shows an `online` / `stale` / `offline` pill computed from `last_seen` age (< 15s = online, < 90s = stale, ≥ 90s = offline). Card border tints: stale = amber, offline = red.
- **Per-host alert badge** — cards with firing alerts show `⚠ N` badge and a red border tint.
- **Network rate footer item** — `↓ rx ↑ tx` shown in each card footer when either direction > 500 B/s. Derived from consecutive sample deltas (the page now tracks `prevLatest` alongside `latest`).
- **Absolute timestamp on hover** — "seen X ago" span has `title={absTime(h.last_seen)}` for full timestamp on hover.
- **`<svelte:head>`** — page title set to `Aperture — Hosts`.

### Added — Host detail (`/hosts/[id]`)

- **`<svelte:head>`** — title set to `Aperture — {host.name}` (dynamic once host loads).
- **Status pill in h1** — stale/offline pill shown inline in the heading when host is not online.
- **Alert banner** — red banner appears below the subnav listing the count and metric names of any firing alerts for this host, with a "View alerts →" link.
- **Stale/offline banners** — amber warning or red error banner when the host is stale or offline, showing last-seen time with absolute timestamp on hover.
- **Absolute timestamp on hover** — "seen X ago" in the Uptime stat card has `title={absTime(host.last_seen)}`.
- **`openAlerts` state** — `api.alertEvents({ hostID: id, openOnly: true })` is fetched in parallel with the rest of the page data on each refresh cycle.

### Added — Containers page (`/hosts/[id]/containers`)

- **Quick search/filter** — text input in the toolbar filters the table by container name and image substring, case-insensitive. Empty state message includes the search term when filtering.
- **ESC closes modals** — `<svelte:window onkeydown>` handles Escape to dismiss logs modal, create modal, and the inspect expand panel (in priority order).
- **`<svelte:head>`** — title set to `Aperture — {hostName} — Containers` (host name fetched once on mount).
- **Absolute timestamp on hover** — container age `relTime` has `title={absTime(c.created_at)}`.

### Added — Alerts page (`/alerts`)

- **`<svelte:head>`** — page title set to `Aperture — Alerts`.
- **ESC closes channel modal** — `<svelte:window onkeydown>` dismisses the add/edit channel modal on Escape.

---

## [0.3.0-alpha.1] — 2026-05-09

Phase 2: Compose-First Workflow — full docker-compose stack management directly from the UI, for both local and remote (agent) hosts.

### Added — Compose backend

- **`internal/compose/compose.go`** — New `compose` package. `Local` struct wraps `docker compose` (v2 plugin) with auto-fallback to `docker-compose` (v1 standalone), detected at startup via version probe. `NewLocal()` returns error if neither binary is available, which the hub and agent handle gracefully. Core methods: `DiscoverStacks` (runs `docker compose ls --all --format json`), `GetStack` (discover + `docker compose ps --all --format json` per project), `StackAction` (runs `up -d --remove-orphans`, `down`, `restart`, `pull`, `stop`, `start` — returns combined stdout/stderr), `Logs` (tailed, per-service or aggregate), `ReadFile` / `WriteFile` (reads/writes compose.yml from the working dir, creates dir and file if absent). Exported `ParseLS` / `ParsePS` helpers parse the JSON output formats including NDJSON (older Compose) and JSON-array (newer Compose). `FindComposeFile` checks all four standard filenames in priority order.
- **`internal/types`** — `ComposeStack` (project, working_dir, config_files, services, status, running_count, total_count) and `ComposeService` (name, container_id, image, state, status, health, exit_code, ports).
- **`internal/hub/hub.go`** — `ComposeProvider` interface (same pattern as `DockerProvider`). `Hub` gains `composes map[string]ComposeProvider`, `RegisterCompose(hostID, p)`, and `Compose(hostID) (ComposeProvider, bool)`.
- **`internal/hub/agentws.go`** — `helloFrame` gains `HasCompose bool`. New `composeReqFrame` / `composeRespFrame` wire types. `agentSession` gains `composePending map[string]chan composeRespFrame`. Cleanup defer drains compose pending channels (parallel to docker pending drain). New `sendComposeCmd` (5-minute timeout for slow pulls). `agentComposeProvider` implements `hub.ComposeProvider` by forwarding all six methods over the WebSocket. On agent connect, hub registers a compose provider if `hello.HasCompose`.
- **`cmd/hub/main.go`** — After Docker socket registers successfully, probes `compose.NewLocal()`; registers it with `h.RegisterCompose(hostID, lc)`. Logs warn if compose is unavailable without aborting. Version bumped to `0.3.0-alpha.1`.
- **`cmd/agent/main.go`** — Probes compose availability at startup (`compose.NewLocal()`). Sends `HasCompose: lc != nil` in hello frame. New `composeReqFrame` / `composeRespFrame` frame types (matching hub). Read loop handles `"compose_req"` frames → `go handleComposeReq(...)`. `dispatchCompose` routes actions (`discover`, `get_stack`, `exec`, `logs`, `read_file`, `write_file`) to the local `compose.Local`.

### Added — Compose API

- **`internal/api/api.go`** — Ten new routes under `/api/hosts/{id}/compose/`:
  - `GET /compose` → list all stacks via `DiscoverStacks`
  - `POST /compose` → create a new stack (write file + optional `up -d`)
  - `GET /compose/{project}` → full stack detail with service list
  - `DELETE /compose/{project}` → `down` (optional `?volumes=true`)
  - `POST /compose/{project}/{action}` → lifecycle: `up`, `down`, `restart`, `pull`, `stop`, `start`; body can pass `service` (single service), `volumes` (for `down`), `extra_args`
  - `GET /compose/{project}/logs?service=&tail=200` → tailed logs
  - `GET /compose/{project}/file?working_dir=` → read compose YAML from disk
  - `PUT /compose/{project}/file` → write YAML; optional `deploy: true` re-runs `up -d`
  - All routes return 503 with a human error if no compose provider is registered (host offline, compose not installed).

### Added — Compose UI

- **`web/src/routes/hosts/[id]/compose/+page.svelte`** — New page at `/hosts/{id}/compose`.
  - **Stack list**: each stack is a collapsible card showing a colored status dot (green=running, orange=partial, gray=stopped), project name, working dir path, service count badge (`N/N running`), status pill, and four quick-action buttons (▶ Up, ⏹ Down, ↺ Restart, ⬇ Pull). Action output (docker compose stdout) appears inline below the card row.
  - **Expanded stack**: tab bar with three tabs:
    - **Services** — table of containers: name + short container ID, state pill (color-coded), health badge (healthy/unhealthy/starting), human-readable status, port mappings, and per-service actions (restart/stop or start, view logs shortcut). Per-service actions pre-fill the logs tab.
    - **Compose File** — monospace YAML textarea loaded on demand. Toolbar shows working-dir path, Reload, Save (write only), and Save + Deploy (write + `up -d`). Dirty indicator when content is modified.
    - **Logs** — service selector (All or individual service), line count selector (50/200/500/1000), Refresh button, scrollable pre block.
  - **Stack actions** (Down…): confirmation modal with optional "remove volumes" checkbox.
  - **New Stack modal**: directory path input (created if missing), YAML textarea (pre-filled with a working nginx template), "Start immediately" checkbox, Create button.
  - All operations show toasts on success/failure. 8-second auto-refresh keeps service states current without disrupting open panels.
  - ESC closes any open modal. `<svelte:head>` title: `Aperture — Compose · {hostname}`.
- **`web/src/routes/hosts/[id]/+page.svelte`** and **`containers/+page.svelte`** — Sub-nav gains **Compose** link between Containers and Networks.
- **`web/src/lib/types.ts`** — `ComposeStack` and `ComposeService` interfaces.
- **`web/src/lib/api.ts`** — `composeStacks`, `composeStack`, `composeAction`, `composeLogs`, `composeFile`, `composeWriteFile`, `createComposeStack`, `deleteComposeStack`.

## [0.3.0-alpha.2] — 2026-05-10

Phase 3 kickoff: deep Docker network management.

### Added — Backend

- **`internal/types`** — Added `DockerNetwork`, `NetworkContainer`, and `NetworkCreateSpec` representations for Docker networks.
- **`internal/dockerctl`** — Added `ListNetworks`, `InspectNetwork`, `CreateNetwork`, `RemoveNetwork`, `ConnectContainer`, and `DisconnectContainer` wrappers over the Docker SDK.
- **`internal/hub`** — `DockerProvider` expanded with the 6 new network methods.
- **`internal/hub/agentws.go`** — `agentDockerProvider` now correctly forwards network method requests to connected remote agents via WebSocket.
- **`cmd/agent/main.go`** — Remote agents successfully dispatch `list_networks`, `inspect_network`, `create_network`, `remove_network`, `connect_network`, and `disconnect_network` action types.
- **`internal/api/api.go`** — New REST routes added under `/api/hosts/{id}/networks/` mirroring the network operations.

### Added — Frontend

- **`web/src/lib/types.ts`** — Typescript interfaces for the new Go network types.
- **`web/src/lib/api.ts`** — New wrapper functions for the backend network management endpoints.
- **`web/src/routes/hosts/[id]/networks/+page.svelte`** — Replaced the placeholder with a functional management page. Includes a list view with inline deep-inspection of networks, visualization of connected containers, removal of networks, and a modal for creating new ones. Connect and disconnect functions integrated directly into the inspect view.

## [0.3.0-alpha.4] — 2026-05-12

Wave 2 — Security baseline. Adds a single-admin session authentication system to the hub. All management endpoints now require a valid session. Adds notification send timeouts and split build-tag CORS.

### Added — Backend

- **`internal/store/schema.sql`** — Two new tables: `auth_config` (single-row constrained by `CHECK (id = 1)`, stores the bcrypt hash of the admin password) and `sessions` (token PRIMARY KEY, created_at, expires_at). Both use `CREATE TABLE IF NOT EXISTS` so existing DBs gain them on next hub start without a migration step.
- **`internal/store/store.go`** — Auth and session methods: `IsPasswordSet`, `GetPasswordHash`, `SetPasswordHash` (upserts the single `auth_config` row), `CreateSession` (inserts a session with 24h expiry), `ValidateSession` (checks token exists and has not expired), `DeleteSession` (logout), `PruneExpiredSessions` (bulk delete of expired rows; called hourly).
- **`internal/api/auth.go`** (new) — Full auth handler file:
  - `requireAuth` middleware: reads the `aperture_session` cookie and calls `store.ValidateSession`. Passes through unauthenticated requests when `IsPasswordSet` is false (first-run mode, so the setup page is reachable without auth). Returns 401 JSON otherwise.
  - `authStatus` (`GET /api/auth/status`): returns `{configured, authenticated}`. Layout calls this on mount to decide whether to redirect to `/setup` or `/login`.
  - `authSetup` (`POST /api/auth/setup`): first-run only; rejects if a password is already set. Hashes with bcrypt cost 12, calls `SetPasswordHash`, creates a session, sets the cookie.
  - `authLogin` (`POST /api/auth/login`): `bcrypt.CompareHashAndPassword` against stored hash; on match creates a 32-byte random hex session token, inserts it into `sessions`, and sets a 24-hour HttpOnly + SameSite=Lax cookie.
  - `authLogout` (`POST /api/auth/logout`): calls `DeleteSession` and overwrites the cookie with an expired value.
  - `authChangePassword` (`POST /api/auth/change-password`): verifies current password with bcrypt, re-hashes the new password. Does not invalidate existing sessions (single-admin assumption).
  - `PruneSessions(ctx, st)` — exported hourly pruner goroutine; called from `cmd/hub`.
  - `newSessionToken()` — 32 bytes from `crypto/rand`, hex-encoded (64-char string, 256 bits of entropy).
  - `setSessionCookie(w, token, duration)` — sets `aperture_session`: HttpOnly, SameSite=Lax, Path=/. Pass `token=""` to clear it.
- **`internal/api/api.go`** — Router restructured into a public group (health, `auth/*`, `agents/ws`) and a `requireAuth`-protected inner group for all data and management endpoints. The agent WS endpoint carries its own token-based auth and is excluded from session auth.
- **`internal/api/cors_dev.go`** (new, `//go:build dev`) — CORS middleware that allows `localhost`/`127.0.0.1` origins with `Access-Control-Allow-Credentials: true`. Only compiled with `-tags dev`. Replaces the always-on CORS previously hardcoded in the router.
- **`internal/api/cors_prod.go`** (new, `//go:build !dev`) — No-op passthrough; in production the SPA is same-origin so CORS headers are unnecessary.
- **`internal/alerts/notify.go`** — Each per-channel goroutine now creates its own 15-second deadline via `context.WithTimeout`. Previously goroutines inherited the dispatch context with no deadline; a hung webhook could hold a goroutine open indefinitely.
- **`cmd/hub/main.go`** — Calls `go api.PruneSessions(ctx, st)` after server start.

### Added — Frontend

- **`web/src/lib/api.ts`** — `handleUnauthorized()` redirects to `/login` on any 401 response (except when already on an auth page); called from all fetch helpers. New `api.auth` object: `status()`, `setup(password)`, `login(password)`, `logout()`, `changePassword(current, next)`.
- **`web/src/routes/+layout.svelte`** — Calls `api.auth.status()` on mount; renders a blank page until auth is confirmed, then conditionally shows header/footer only on non-auth pages. Auth pages render in a centered `.auth-page` wrapper.
- **`web/src/routes/login/+page.svelte`** (new) — Login form with password input, error display, and redirect to `/` on success.
- **`web/src/routes/setup/+page.svelte`** (new) — First-run setup form: password + confirm, minimum 8-character validation, submit blocked until valid. Redirects to `/` on success.
- **`web/src/routes/settings/+page.svelte`** — Security section added: change-password form (current / new / confirm) and a Sign out button calling `api.auth.logout()` then navigating to `/login`.

### Build

- **`Makefile`** — `make dev` now passes `-tags dev` to `go run`: `$(GO) run -tags dev ./cmd/hub -interval 2s`. Without this tag the dev-CORS middleware is excluded and cross-origin API calls from the Vite dev server fail.

---

## [0.3.0-alpha.3] — 2026-05-12

Wave 1 — Correctness pass. Fixes silent alert failures, terminal double-encoding, resolve timestamp drift, and several shell-argument bugs. Introduces the `TerminalProvider` interface and the shared `internal/agentproto` package.

### Fixed — Backend

- **`internal/store/store.go`** — `ListEnabledRulesFor` was missing `severity` from its SELECT (column 8 of 9 expected by `scanAlertRule`), causing the scan to fail with a column-count mismatch. The evaluator's hot path silently discarded these scan errors, meaning **no alert rules were ever being evaluated**. **Fix:** added `severity` to the column list. This was a total silent failure of the alert system.
- **`internal/hub/agentws.go`** — Four terminal control methods (`StartTerminal`, `SendTerminalData`, `ResizeTerminal`, `CloseTerminal`) were double-encoding JSON: they called `json.Marshal(req)` then passed the resulting `[]byte` to `wsjson.Write`. `wsjson.Write` JSON-encodes its argument, so a `[]byte` is base64-encoded per the JSON spec and arrives on the agent as a JSON string, not an object. **Fix:** pass the struct directly to `wsjson.Write`.
- **`internal/alerts/alerts.go`** — The `open` map stored `int64` event IDs. On resolve, `fire()` used `sample.Timestamp` as the event's `FiredAt` instead of the actual timestamp from when the alert originally fired. Resolve notifications therefore reported the wrong fire time. **Fix:** changed `open map[ruleHostKey]int64` → `open map[ruleHostKey]types.AlertEvent` (storing the full event). The original event is retrieved from the map on resolve so `FiredAt` carries the correct value.
- **`internal/compose/compose.go`** — `StackAction` and `Logs` unconditionally prepended `--project-name <name>` even when the project name was empty, causing `docker compose` to reject the invocation. **Fix:** `--project-name` is only appended when non-empty.
- **`internal/api/api.go`** — `createCompose` success fallback used `stacks[len(stacks)-1]` (the last stack in the discovered list, unrelated to the new stack) as the response body. **Fix:** return `{"ok": true, "working_dir": body.WorkingDir}`.

### Added — Backend

- **`internal/agentproto/frames.go`** (new package) — Shared wire-frame type definitions for the agent ↔ hub WebSocket protocol: frame-type constants (`TypeHello`, `TypeAck`, `TypeMetric`, `TypeHeartbeat`, `TypeDockerReq`, `TypeDockerResp`, `TypeComposeReq`, `TypeComposeResp`) and exported frame structs. **Why:** previously the frame types were inline structs defined independently in `agentws.go` and `cmd/agent/main.go`; a shared package removes the risk of silent type drift between the two ends.
- **`internal/hub/hub.go`** — `TerminalProvider` interface (four methods: `StartTerminal`, `SendTerminalData`, `ResizeTerminal`, `CloseTerminal`) and a `terminals map[string]TerminalProvider` registry with `RegisterTerminal`, `Terminal`, `UnregisterTerminal` accessors. Also added `UnregisterDocker` and `UnregisterCompose` as public methods so `agentws.go` can clean up on disconnect without accessing hub internals directly. **Why:** terminal sessions were always routed through the agent WebSocket path even for the local host. The interface routes local terminals to `localTerminalProvider` and agent terminals to `agentTerminalProvider`.
- **`internal/hub/terminal.go`** (new) — `localTerminalProvider` wrapping `dockerctl.Client` for terminal sessions on the local host. Owns a mutex-protected session map (`reqID → localTermSession`); each session holds the stdin writer, resize function, and close callback. Satisfies `TerminalProvider` with a compile-time assertion.
- **`internal/hub/agentws.go`** — `agentTerminalProvider` added (delegates `StartTerminal`, `SendTerminalData`, `ResizeTerminal`, `CloseTerminal` to `AgentHandler` with a fixed `hostID`). Agent connect now calls `hub.RegisterTerminal`; agent disconnect uses the new public `Unregister*` hub methods.
- **`cmd/hub/main.go`** — Constructs and registers `hub.NewLocalTerminalProvider(dc)` alongside the docker client registration.

### Cleanup

- Removed `check_db.go` — root-level `package main` that imported `github.com/mattn/go-sqlite3` (not in go.mod), blocking `go build ./...`.
- Removed `scratch/dist_inspect.go`, `scratch/volcheck.go`, `export_state.md` — debug/scratch files.
- Removed `web/.vscode/extensions.json` from git tracking (`git rm --cached`).

---

## [0.4.0-alpha.1] — 2026-05-12

Full UI redesign. Replaces the single-column top-nav SPA with a sidebar shell, new design token system, three-variant host dashboard cards, live-streaming sparklines, host drill-in slide-over panel, and Settings → Appearance.

### Added — Design system

- **`web/src/lib/styles/tokens.css`** (new) — Complete CSS variable system. Dark and light themes toggled via `[data-theme="dark|light"]` on `<html>`. Accent variants: teal (default), indigo, amber, violet, lime, rose — each with a hex, soft background, and line variant. Status colors (`--ok`, `--warn`, `--crit`, `--info`, `--offline`) are theme-invariant. Geist Sans + Geist Mono imported via `@fontsource/geist` / `@fontsource/geist-mono`. Motion tokens: `--ease-card: cubic-bezier(.2,.7,.3,1)`, `--dur-slide: 260ms`, all inside `@media (prefers-reduced-motion: no-preference)`.
- **`web/src/lib/styles/global.css`** (new) — Base resets, typography scale, mono utility, table/input/button defaults, `.card`, `.pill` (ok/warn/crit/offline variants), `.segmented` control, `.glass-topbar` / `.glass-drillin` surfaces, shimmer skeleton, `.pulse-crit` animation, legacy compat aliases (`--border → --line`, `--bad → --crit`, `--bg-elev-1 → --bg-elev`, `--mono → --font-mono`).
- **`web/src/app.html`** — `data-theme="dark"` added to `<html>` for SSR-safe initial paint.
- **`web/node_modules`** — `@fontsource/geist`, `@fontsource/geist-mono`, `lucide-svelte` added.

### Added — Stores

- **`web/src/lib/stores/theme.ts`** — `ThemeMode` store (`dark|light|system`). Reads/writes `localStorage`. Applies `document.documentElement.dataset.theme`. Watches `prefers-color-scheme` media query when mode is `system`.
- **`web/src/lib/stores/accent.ts`** — `AccentKey` store with six options. Applies `--accent`, `--accent-soft`, `--accent-line` to `:root` on change and on init.
- **`web/src/lib/stores/hosts.ts`** — `HostEntry` map with 60-sample ring buffer per host (`cpuSeries`, `memSeries`, `netInSeries`, `netOutSeries`, `tsSeries`). `HostStatus` derived from `last_seen` age. SSE subscription to `/api/stream/metrics` updates the ring buffer live.
- **`web/src/lib/stores/dashboardLayout.ts`** — Card layout (`rich|tile|list`), pinned host set, active tag filter, and card order. Persists to `localStorage` and syncs to `/api/settings/dashboard-layout`.

### Added — Primitives

- `Sparkline.svelte` — lightweight SVG area sparkline, ring-buffer aware, no library dependency.
- `Meter.svelte` — 4px linear bar with `warn ≥ 75` / `crit ≥ 90` color thresholds.
- `StatusIndicator.svelte` — color dot with `.pulse-crit` on critical.
- `Tag.svelte` — host tag chip (11px mono, bg-elev-2).
- `Kbd.svelte` — keyboard shortcut chip.
- `Icon.svelte` — `lucide-svelte` wrapper with name-to-component map (40+ icons).
- `Button.svelte` — primary / ghost / mini / icon / danger variants.
- `Field.svelte` — label + input slot + error/hint.
- `Modal.svelte` — glass backdrop, scale-in animation, Esc and backdrop-click to close.
- `HostKindIcon.svelte` — 28×28 chip with docker/linux/edge icon.
- `SkeletonCard.svelte`, `EmptyBlock.svelte`, `ErrorBlock.svelte` — loading, empty, and error states.

### Added — Shell

- **`web/src/lib/components/shell/Sidebar.svelte`** — 220px sidebar. WORKSPACE (Dashboard, Hosts, Containers, Stacks, Storage, Network) and OBSERVE (Logs, Shell, Automation, Alerts) sections. Active nav item has 2px left accent rail. Alert badge on Alerts nav item.
- **`web/src/lib/components/shell/Topbar.svelte`** — Search input with ⌘K hint, sync indicator dot, theme toggle button, avatar initials chip.
- **`web/src/lib/components/shell/AppShell.svelte`** — CSS grid layout (220px sidebar + 1fr content). Wires theme and accent stores on mount.
- **`web/src/routes/+layout.svelte`** — Replaced top-nav with `<AppShell>`. Auth pages get centered wrapper; all other pages live inside the shell.
- **`web/src/routes/+page.svelte`** — Replaced with redirect to `/dashboard`.

### Added — Dashboard

- `PageHeader.svelte` — H1 + status counts strip (Healthy / Warning / Critical / Offline / Containers / Alerts).
- `FilterBar.svelte` — Tag filter tabs + Rich/Tile/List segmented control + gradient "Add host" button.
- `RichCard.svelte` — Full-width host card with left status rail, sparklines, side info panel (OS/arch, uptime, container counts), alert footer.
- `TileCard.svelte` — Compact 2×2 metric grid (CPU / Mem / Net / Temp) with status-colored sparkline.
- `CompactRow.svelte` — 7-column list row (status, name/kind, OS, CPU%, Mem%, Net↓, tags).
- `HostCard.svelte` — Variant switcher.
- `HostGrid.svelte` — Grid container with loading skeleton, empty block, and error block states.
- `AddWidgetTile.svelte` — Dashed "+" tile, turns accent on hover.
- `CardMenu.svelte` — Glass popover with pin / shell / restart / remove actions.
- **`web/src/routes/dashboard/+page.svelte`** — New dashboard page. Parallel host and metrics load. SSE connection. DrillIn + AddHostModal integration.

### Added — Drill-in

- `DrillIn.svelte` — Slide-over panel (260ms ease-card). Sticky header with HostKindIcon, name, StatusIndicator, tags, action buttons (Restart / SSH / Update / Stop). Tab nav: Overview / Containers / Stacks / Logs / Shell. Overview tab: 4-column BigMetric grid + 3-column lower panel (Storage / Containers / Events).
- `BigMetric.svelte` — 26px mono value + sparkline in an elevated card.
- `StoragePanel.svelte` — Disk mounts with Meter bars, falls back to root disk.
- `ContainersPanel.svelte` — Running/Stopped/Unhealthy counts + top-by-CPU list.
- `EventsPanel.svelte` — Recent alert events feed (last 8).

### Added — Add Host Modal

- `AddHostModal.svelte` — 2-step glass modal (scale-in 220ms). Step 1: method radio + form fields. Step 2: async verify rows with spinner.
- `MethodRadio.svelte` — Three radio cards: Install Agent / Docker API / SSH Probe. Accent-tinted on selection.
- `VerifyRow.svelte` — Check row with pending / checking (spinner) / ok / error states.

### Added — Settings → Appearance

- **`web/src/routes/settings/+page.svelte`** — New Appearance section at the top: Theme pill buttons (Dark / Light / System) and Accent color swatches (6 colors). Changes apply immediately via stores.

### Added — Backend (SSE, tags, kind, layout)

- **`internal/types/types.go`** — `Host` gains `Kind string`, `Tags []string`, `OpenAlerts int`.
- **`internal/store/schema.sql`** — `user_settings` table for key/value preference storage.
- **`internal/store/store.go`** — `UpdateHostTags`, `UpdateHostKind`, `UserSetting`, `SetUserSetting`. Idempotent migrations for `tags` and `kind` columns in `hosts`. `scanHost` helper centralises all column scanning.
- **`internal/hub/hub.go`** — `SSEEvent` struct. `sseSubscribers` map. `SubscribeSSE`, `broadcastSSE`. `ingestLoop` broadcasts after each sample.
- **`internal/hub/agentws.go`** — On agent connect, detects host kind (docker vs edge) and calls `store.UpdateHostKind`.
- **`internal/api/api.go`** — Routes: `PUT /api/hosts/{id}/tags`, `GET/PUT /api/settings/dashboard-layout`, `GET /api/stream/metrics` (SSE). `listHosts` annotates `open_alerts` count. `streamMetrics` fans out SSE to connected clients.
- **`web/src/lib/types.ts`** — `Host` gains `kind`, `tags`, `open_alerts`.
- **`web/src/lib/api.ts`** — `api.hosts.{list,get,updateTags}`, `api.settings.{getDashboardLayout,saveDashboardLayout}`.
- **`web/src/lib/format.ts`** — Short aliases `fmtBytes`, `fmtRate`, `fmtDuration`, `fmtRelative`, `fmtAbsolute` exported alongside the original names.

### Added — Stub routes

- `/containers`, `/stacks`, `/storage`, `/network`, `/logs`, `/shell`, `/automation` — stub pages with "Coming soon" message so sidebar nav links don't 404.

### Verified

- `npm run build` — clean, no TypeScript errors.
- `go build ./...` — clean.

---

## [Unreleased]

Phase 3 continues: Deep Docker surface — Volumes and Images next.
