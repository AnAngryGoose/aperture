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

## [Unreleased]

Deciding the next major thrust now that 0.1.0 is out. Likely candidates per the roadmap: **section 1 (solidify the core)** — agent ↔ hub auth/heartbeats and WebUI stabilization, which unblocks every multi-host feature later — or **section 2 (compose-first)** — full compose stack management on the local host, which lets aperture replace dockge/portainer for the user's current homelab usage. Worth picking after a brief week or two of just running 0.1.0 and seeing what feels missing in practice.
