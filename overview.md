# Aperture — Overview

A single pane of glass for homelab command-and-control. Aperture is a self-hosted web application that lets you view and manage homelab resources from one interface.

## What it does (current state)

**v0.3.0-alpha.4** — Wave 2: single-admin session auth, bcrypt password, HttpOnly session cookies, first-run setup page, login page, change-password UI, per-channel 15s notification timeout, build-tag CORS split. **v0.3.0-alpha.3** — Wave 1 correctness: fixed silent alert evaluation failure (missing SQL column), fixed terminal JSON double-encoding, fixed resolve timestamp drift, added `TerminalProvider` interface and `internal/agentproto` shared frame types. **v0.3.0-alpha.2** — Phase 3 kickoff: deep Docker network management. **v0.3.0-alpha.1** — Phase 2 complete: docker-compose stack management directly from the UI. Discovers all existing compose stacks automatically (no ownership, no migration), shows live service state per stack, and supports the full lifecycle (up, down, restart, pull, stop, start) at both stack and individual-service granularity. Per-stack compose file viewer/editor with Save + Deploy in one click. Aggregated and per-service log tailing. Create new stacks from scratch with a YAML editor and optional immediate start. Works identically for the local hub host and remote agent hosts. **v0.2.0-alpha.5** — Phase 1 complete: remote agent transport, token-based auth, exponential-backoff reconnect, and the agent onboarding wizard. **v0.2.0-alpha.4** — UI/UX quality-of-life pass: host status pills, per-host alert badges, stale/offline banners, quick container search, ESC-to-close, dynamic page titles, absolute-timestamp tooltips, global toast system. **v0.2.0-alpha.3** added chart UX improvements and extensible alert notification channels: Discord, Slack, ntfy, Gotify, and generic webhook, each with per-rule severity levels and configurable resolve notifications. **v0.2.0-alpha.2** added full Beszel monitoring parity. **v0.2.0-alpha.1** added rich live monitoring depth and complete container lifecycle. **v0.1.0** established the base foundation.

- Auto-discovers the local host, samples its system metrics every few seconds, and stores them in SQLite. Live snapshot is cached in-memory to carry rich fields (per-core CPU, per-interface network, all disk mounts, disk I/O, temps, process list) that are not stored historically. Three new tables (`net_iface_metrics`, `disk_mount_metrics`, `disk_io_metrics`) store per-entity historical data for charting.
- Lists all docker containers on the host with live CPU/memory stats.
- Web dashboard:
  - Host overview cards (CPU, memory, disk, container count) with auto-refresh.
  - Per-host detail view: stat cards (CPU %, memory with 3-segment used/cached/free bar, disk, uptime), per-core CPU grid, network interfaces live table (rates + totals), disk mounts live table (usage bars), disk I/O live table, temperature grid, live process table (top 40 by CPU/memory with sort toggle), and time-series charts for CPU, memory, disk, network, and load average across 15m / 1h / 6h / 24h. Historical charts also include per-interface network rates, per-mount disk usage, and disk I/O rates per device.
  - Container management: create (surface form: image, name, restart policy, env, ports, volumes, auto-start), deep-inspect (inline expand with full config + live stats + resource limits + actions), resource limit editing (CPU/memory live update), recreate, start, stop, pause, unpause, restart, kill, remove, view logs. Sorting (name/state/cpu/mem) and state filtering.
- Threshold-based alerts: configurable rules per host (or all hosts) on cpu/mem/disk/swap/load with optional sustained-breach duration; per-rule severity (info/warning/critical); live event history with auto-resolve when the breach ends; nav badge with currently-firing count.
- Alert notification channels: Discord, Slack, ntfy, Gotify, and generic webhook. Each channel has a minimum-severity filter and a configurable resolve-notify toggle. Channels are configured from the Alerts → Channels tab with a Test button to validate config before it's used in anger.
- Docker Compose stack management: auto-discovers all existing stacks on a host (no ownership, no migration). Stack cards show live service counts and status. Collapsible detail view with three tabs: Services (per-service state/health/ports/actions), Compose File (YAML editor with Save + Deploy), and Logs (tailed, per-service or aggregate). Full lifecycle: up, down, restart, pull, stop, start — at stack or individual service level. Create new stacks from the UI with a directory picker and YAML editor. Works for both the local hub host and remote agent hosts.

## Why it exists

Off-the-shelf homelab tools are excellent in their niches (beszel for monitoring, dockge for compose, portainer for containers, etc.) but operating a homelab means stitching their UIs together. Aperture's premise is to consolidate the read and write paths into one application designed for extensibility, security, and scalability as the homelab grows.

## Design philosophy: clean surface, deep ability

The core UX principle for every feature in aperture: present a **glanceable summary view** by default, with the ability to **drill into granular detail on demand**. Two layers, one feature, never separate "advanced" sections buried in menus.

| Layer | Answers | What it shows |
| --- | --- | --- |
| **Surface** | "Is everything okay?" | Health indicators, status pills, sparklines, quick-action buttons, badge counts. Optimized for one-glance scanning and minimal-click common operations. |
| **Deep** | "Why? Show me everything." | Full data, raw values, historical trends, editable configuration, raw JSON/YAML when needed. Optimized for power and explainability. |

The transition between the two must feel seamless: clicking a host card *navigates* to the host detail page, clicking a container row *expands* to actions and logs, clicking a metric chart *opens* a detail panel. **Every** major feature in this codebase — current and future — should implement both layers. If a proposed feature has only a surface or only a deep view, that's a design smell worth raising before it ships.

This is not progressive disclosure for its own sake. It's about making the common case fast without sacrificing power for the complex case.

## How it's structured

Aperture is built as a **hub + agents** architecture from day 1, even though v0.1 ships single-host. The hub is the central web server, API, database, and dashboard. Agents (not yet implemented) are lightweight per-host processes that push metrics and proxy docker actions to the hub. In v0.1 the hub embeds a "local source" that fills the agent's role for the machine it runs on. This means future remote-host support arrives as a new transport, **not** a refactor of the data model or APIs — every host-scoped record is already keyed by `host_id`.

```
┌────────────────────────────────────────────────────────┐
│ aperture-hub (one process, single binary)              │
│                                                        │
│  cmd/hub  ──►  internal/hub  ◄─── internal/collector   │
│      │             │                  (local source)   │
│      │             ▼                                   │
│      │        SQLite store (hosts, metrics, alerts)    │
│      │             │   ▲                               │
│      │             ▼   │                               │
│      │        internal/alerts (evaluator)              │
│      │             │                                   │
│      ▼             ▼                                   │
│  internal/api  ◄── internal/dockerctl (local socket)   │
│      │                                                 │
│      ▼                                                 │
│  /api/* + static SvelteKit SPA                         │
└────────────────────────────────────────────────────────┘
```

When remote agents land, `cmd/agent` (currently a placeholder) will run on each remote host, sample via `internal/collector`, and push samples to the hub over HTTPS or WebSocket — registering through the same `hub.MetricSource` and `hub.DockerProvider` interfaces.

## Tech stack

| Layer | Choice | Why |
| --- | --- | --- |
| Backend | Go 1.25 | Single static binary, low memory footprint, mature stdlib HTTP. |
| HTTP router | go-chi/chi v5 | Idiomatic, no surprises, lightweight middleware. |
| Metrics collection | shirou/gopsutil v4 | De-facto standard cross-platform system metrics for Go. |
| Docker | docker/docker SDK | Official client; full container API surface. |
| Database | SQLite via `modernc.org/sqlite` | No CGO, easy cross-compile, perfect for homelab single-file storage. |
| Frontend | SvelteKit (static SPA, Svelte 5 runes) | Small bundles, ergonomic, served as static files by the hub. |
| Charts | uPlot | ~45 KB, fast for dense time-series. |

## Directory layout

```
aperture/
├── cmd/
│   ├── hub/          Hub binary entry point
│   └── agent/        Remote-agent binary
├── internal/
│   ├── agentproto/   Shared agent ↔ hub WebSocket frame types
│   ├── alerts/       Threshold rule evaluator (sustained-breach, auto-resolve)
│   ├── api/          HTTP handlers + chi router + auth middleware + SPA fallback
│   ├── collector/    Local system-metrics sampler (gopsutil)
│   ├── compose/      docker compose CLI wrapper
│   ├── dockerctl/    Docker engine wrapper (list, lifecycle, logs, networks)
│   ├── hub/          Orchestration: host registry, ingest loop, retention, TerminalProvider
│   ├── store/        SQLite + schema.sql
│   └── types/        Shared types across packages
├── web/              SvelteKit project (UI)
├── bin/              Build outputs (gitignored)
├── Makefile          Build / run / dev / clean targets
├── overview.md       This file
├── technical.md      Per-function detail
└── changelog.md      Versioned change history
```

## Prerequisites

- Go ≥ 1.25 (the modernc sqlite driver requires it)
- Node.js ≥ 20 + npm
- Docker daemon accessible to the user running the hub (the hub needs read access to `/var/run/docker.sock`; users running the hub must be in the `docker` group, or run with appropriate privileges).

On Debian 13 (trixie) these are all available via apt:

```sh
sudo apt-get install -y golang-go nodejs npm
```

## Building

```sh
cd /opt/aperture
make build       # builds web/build, bin/aperture-hub, bin/aperture-agent
```

Individual targets:

| Target | What it does |
| --- | --- |
| `make hub` | Build only the Go hub binary into `bin/aperture-hub`. |
| `make agent` | Build the agent placeholder into `bin/aperture-agent`. |
| `make web` | `npm install` and build the SvelteKit static SPA into `web/build`. |
| `make tidy` | `go mod tidy`. |
| `make clean` | Remove `bin/`, `web/build/`, `web/.svelte-kit/`. |

## Running

### Production-style (single binary serves UI + API)

```sh
make run
# or directly:
./bin/aperture-hub -web-dir web/build
```

Then open <http://localhost:8080>.

### Development (live-reload UI, separate API)

Two terminals. Hub on :8080:

```sh
make dev
# equivalent to: go run -tags dev ./cmd/hub -interval 2s
# -tags dev compiles in the CORS middleware for localhost origins
```

Vite dev server on :5173:

```sh
cd web && npm run dev
```

The dev server proxies API calls to the hub via `VITE_API_BASE` (defaulted to `http://localhost:8080` when `import.meta.env.DEV` is true). The `-tags dev` build tag is required: it compiles the CORS middleware that allows cross-origin requests with credentials from `localhost`/`127.0.0.1`. Without it, the session cookie cannot be sent from the Vite dev server and all API calls will 401.

### Configuration

All flags can also be set via environment variables.

| Flag | Env | Default | Purpose |
| --- | --- | --- | --- |
| `-listen` | `APERTURE_LISTEN` | `:8080` | HTTP listen address. |
| `-db` | `APERTURE_DB` | `aperture.db` | SQLite database file path. |
| `-interval` | `APERTURE_INTERVAL` | `5s` | Local metric sample interval. |
| `-retain` | `APERTURE_RETAIN` | `336h` (14d) | How long to keep samples; `0` = forever. |
| `-disk-path` | `APERTURE_DISK_PATH` | `/` | Root path for disk usage reporting. |
| `-web-dir` | `APERTURE_WEB_DIR` | _(unset)_ | If set, serve a SvelteKit `build/` directory at `/`. Empty = API-only. |

The frontend respects `VITE_API_BASE` at build time if you ever split the SPA from the API in production.

## Stopping

The hub installs handlers for `SIGINT` and `SIGTERM`. From an interactive shell, `Ctrl-C` runs the graceful shutdown path: HTTP server stops accepting new connections, in-flight requests get up to 5 seconds to finish, then the SQLite handle is closed.

If running in the background:

```sh
pkill -SIGTERM -f aperture-hub
```

Avoid `kill -9` unless the hub has actually hung — the WAL journal is robust but graceful shutdown is preferable.

## REST API surface

All endpoints live under `/api`. Responses are JSON unless noted.

All endpoints except `/api/health`, `/api/auth/*`, and `/api/agents/ws` require a valid session cookie (`aperture_session`). The 401 response is always `{"error":"..."}`.

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/auth/status` | Returns `{configured, authenticated}`. Layout calls this on mount to decide whether to redirect to `/setup` or `/login`. |
| POST | `/api/auth/setup` | First-run only. Body: `{"password":"..."}`. Hashes the password (bcrypt cost 12) and creates a session. |
| POST | `/api/auth/login` | Body: `{"password":"..."}`. On match, creates a 24-hour HttpOnly session cookie. |
| POST | `/api/auth/logout` | Deletes the session and clears the cookie. |
| POST | `/api/auth/change-password` | Body: `{"current":"...","new":"..."}`. Verifies current password, re-hashes the new one. |
| GET | `/api/health` | Liveness probe. |
| GET | `/api/system/info` | Hub version, started-at timestamp, SQLite DB path, and total on-disk size (`aperture.db` + `-wal` + `-shm`). Used by the layout footer. |
| GET | `/api/hosts` | List all known hosts. |
| GET | `/api/hosts/{id}` | Get a single host. |
| GET | `/api/hosts/{id}/metrics/latest` | Most recent metric sample (includes rich live-only fields: per-core CPU, per-interface network, disk mounts, disk I/O, temps, process list). |
| GET | `/api/hosts/{id}/metrics?range=1h&points=300` | Down-sampled aggregate samples for the range. |
| GET | `/api/hosts/{id}/metrics/net?range=1h&points=300` | Historical per-interface rx/tx bytes, pivoted into `{timestamps, ifaces}`. Rates derived client-side. |
| GET | `/api/hosts/{id}/metrics/mounts?range=1h&points=300` | Historical per-mount used/total bytes, pivoted into `{timestamps, mounts}`. |
| GET | `/api/hosts/{id}/metrics/diskio?range=1h&points=300` | Historical per-device read/write bytes, pivoted into `{timestamps, devices}`. Rates derived client-side. |
| GET | `/api/hosts/{id}/containers?all=true` | Containers on the host. |
| POST | `/api/hosts/{id}/containers` | Create container from a surface-layer spec (image, name, restart policy, env, ports, volumes, auto-start). Pulls image if missing. Returns `{id}` on success, `{id, warning}` (HTTP 202) if the container was created but failed to start. |
| GET | `/api/hosts/{id}/containers/{cid}/inspect` | Full container detail: config, timestamps, env, ports, mounts, labels, live stats, and resource limits. |
| PUT | `/api/hosts/{id}/containers/{cid}/resources` | Live update CPU (`nano_cpus`) and/or memory (`memory_bytes`) limits via cgroups. `0` means "unlimited". No restart required. |
| POST | `/api/hosts/{id}/containers/{cid}/recreate` | Stop → remove → recreate with the same spec (image, name, restart policy, env, ports, mounts). Returns `{id}` on success, `{id, warning}` (202) on partial success. |
| POST | `/api/hosts/{id}/containers/{cid}/start` | (also `stop`, `restart`, `pause`, `unpause`, `kill?signal=…`) |
| DELETE | `/api/hosts/{id}/containers/{cid}?force=&volumes=` | Remove container. |
| GET | `/api/hosts/{id}/containers/{cid}/logs?tail=200` | Plaintext logs. |
| GET | `/api/alerts/metadata` | Supported metrics + ops for UI dropdowns. |
| GET | `/api/alerts/rules?host_id=` | List rules; if `host_id` is set, only rules applicable to that host (matching id or `NULL` = all hosts). |
| POST | `/api/alerts/rules` | Create a rule. Body: `{host_id?, metric, op, threshold, duration_s, enabled?}`. Empty `host_id` means "all hosts". |
| GET | `/api/alerts/rules/{id}` | Fetch one rule. |
| PUT | `/api/alerts/rules/{id}` | Update a rule (full replace). |
| DELETE | `/api/alerts/rules/{id}` | Delete a rule. Cascades to its event history and clears in-memory evaluator state. |
| GET | `/api/alerts/events?host_id=&open=&limit=` | Event history. `open=true` returns currently-firing events only; default `limit=200`. |

`range` accepts any Go-style duration (`15m`, `6h`, `24h`). `points` caps the returned series via uniform stride downsampling.

## Troubleshooting

- **`docker unavailable` on startup** — the user running the hub can't reach the docker socket. Add the user to the `docker` group or run with appropriate privileges. Metrics still work; container endpoints will 404.
- **Empty charts** — charts need at least 2 samples in the requested range. Wait one or two `-interval` cycles.
- **CORS errors in dev** — make sure the hub is running with `make dev` (passes `-tags dev`). Without that build tag the CORS middleware is compiled out and cross-origin requests from the Vite dev server are rejected. Also confirm you're hitting `http://localhost:5173`, not `:8080` directly.
- **`port already in use`** — change `-listen`, e.g. `-listen :8081`.
- **High memory growth** — set `-retain` lower, or run `VACUUM` on the SQLite file periodically. Pruning runs hourly when retention > 0.

## Roadmap (post-v0.1)

The full roadmap lives in `/opt/aperture-roadmap.docx` (canonical source). Aperture is intended to consolidate Dozzle (logs), Beszel (metrics), Portainer/Dockge (containers/stacks), and other single-purpose dashboards into one tool. The eight numbered sections below are in rough dependency order — not a strict execution order; work lands in whichever order is most logical given current state.

1. **Solidify the core** — agent ↔ hub auth (mTLS or token), auto-reconnect, heartbeats, agent registration/discovery, stale-data UI; WebUI stabilization (component library, responsive desktop-first, error/feedback patterns); container lifecycle complete (recreate, restart policy, resource limits, env vars, port editing).
2. **Compose-first workflow** — discover/import existing `docker-compose.yml` from configurable paths per host; YAML + visual editor with autocomplete and validation; deploy/teardown/rebuild/per-service actions; git-style version diffs and rollback; service dependency graph.
3. **Deep docker management** — networks (list, attach, topology graph), volumes (list, orphan detection, growth), images (list, registry update detection, layer inspection, dangling cleanup), log streaming (Dozzle replacement: live tail, multi-container interleave, regex search, time windowing, export).
4. **Metrics, storage & host overview** — multi-host system metrics (per-core CPU, mem incl. per-process top, per-iface net, per-device disk I/O, temps, retention with downsampling); container-level metrics (limit-vs-actual, healthcheck history, restart counts); storage depth (ZFS pools/scrub/snapshots, RAID arrays, NFS/SMB mounts, SMART per-drive, growth projections).
5. **Alerting & notifications** — extend the existing rules: container state alerts, storage alerts (ZFS degraded, SMART), agent-offline, network alerts, severities (info/warn/crit); channels (Discord/Slack/ntfy/Gotify/generic webhook, SMTP, SMS); per-channel severity filters; cooldown/dedup; ack/silence; escalation paths.
6. **Network scanning & overview** — subnet discovery (hostname, MAC/OUI, OS fingerprint, port scan), scheduled scans with change detection, device inventory with custom labels, unknown-device alerts; OPNsense integration (firewall rules, interface health, DHCP leases, state table, VPN tunnels, Unbound DNS log); visual network topology map; latency monitoring.
7. **Ansible integration** — playbook discovery from filesystem paths; in-app YAML editor; library with tags/search; variable mgmt incl. vault; run against hosts/groups picked from agent inventory; streaming stdout/stderr; execution history; dry-run; cron-style scheduling; auto-generated dynamic inventory from agent registry.
8. **Reverse proxy & multi-user** — Traefik first (label injection on stack deploy), then Caddy and nginx-proxy-manager behind a provider interface; routing table with SSL cert status + expiry warnings; local auth with RBAC (admin / operator / viewer); audit log; sessions + 2FA; optional OIDC/LDAP.

### Architecture considerations to lock in early

These are non-negotiable groundwork for items late in the list — defer the *feature*, not the architectural seam:

- **API-first** — every WebUI action goes through a documented REST API, versioned `/v1` from day one. Enables CLI, mobile, third-party integrations without rework.
- **Data retention tiers** — high-resolution (minutes) for 24h, medium (hourly) for 30d, low (daily) for 1y+. Auto-downsampling. Configurable per metric type.
- **Agent capabilities** — agents report capabilities on registration ("I have ZFS", "I have Ansible"). Hub gracefully handles mixed-capability agents. Agent auto-update mechanism.
- **Pluggable providers** — notification channels, reverse-proxy integrations, metric collectors, network scanners as extension points so community contributions land without modifying core.
- **Security from day 1** — encrypted agent ↔ hub (mTLS preferred), no plaintext secrets in DB, API auth even in single-user mode, audit logging infrastructure before multi-user lands.

### Future-scope ideas (no fixed order)

Scheduled tasks/cron management; DNS management (Pi-hole / AdGuard / Unbound integration); backup orchestration (Restic/Borg, schedule + retention + verification); service health dashboards (custom groupings, uptime SLA, dependency mapping); update management (image-update detection, one-click compose bump, host OS updates); secrets management (central store, rotation reminders, Ansible Vault integration); CLI companion (`aperture` CLI sharing the WebUI's API); plugin/extension system; Proxmox VM/LXC management; mobile-optimized UI with PWA push notifications.
