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

## [Unreleased]

In progress: alert evaluator and rule-management UI. Then container *create* to round out v0.1's docker management story.
