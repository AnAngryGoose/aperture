# Aperture — Overview

A single pane of glass for homelab command-and-control. Aperture is a self-hosted web application that lets you view and manage homelab resources from one interface.

## What it does (current state)

**v0.1 (in progress)** — monitoring + docker container management:

- Auto-discovers the local host, samples its system metrics every few seconds, and stores them in SQLite.
- Lists all docker containers on the host with live CPU/memory stats.
- Web dashboard:
  - Host overview cards (CPU, memory, disk, container count) with auto-refresh.
  - Per-host detail view with charts for CPU, memory, disk, network throughput, and load average across configurable time ranges (15m / 1h / 6h / 24h).
  - Container management: start, stop, pause, unpause, restart, kill, remove, view logs.
- Threshold-based alerts: configurable rules per host (or all hosts) on cpu/mem/disk/swap/load with optional sustained-breach duration; live event history with auto-resolve when the breach ends; nav badge with currently-firing count.

## Why it exists

Off-the-shelf homelab tools are excellent in their niches (beszel for monitoring, dockge for compose, portainer for containers, etc.) but operating a homelab means stitching their UIs together. Aperture's premise is to consolidate the read and write paths into one application designed for extensibility, security, and scalability as the homelab grows.

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
│   ├── hub/        Hub binary entry point
│   └── agent/      Future remote-agent binary (placeholder in v0.1)
├── internal/
│   ├── api/        HTTP handlers + chi router + SPA fallback
│   ├── collector/  Local system-metrics sampler (gopsutil)
│   ├── dockerctl/  Docker engine wrapper (list, lifecycle, logs)
│   ├── hub/        Orchestration: host registry, ingest loop, retention
│   ├── store/      SQLite + schema.sql
│   └── types/      Shared types across packages
├── web/            SvelteKit project (UI)
├── bin/            Build outputs (gitignored)
├── Makefile        Build / run / dev / clean targets
├── overview.md     This file
├── technical.md    Per-function detail
└── changelog.md    Versioned change history
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
# (equivalent to `go run ./cmd/hub -interval 2s` — API only, no UI)
```

Vite dev server on :5173:

```sh
cd web && npm run dev
```

The dev server proxies API calls to the hub via `VITE_API_BASE` (defaulted to `http://localhost:8080` when `import.meta.env.DEV` is true). The hub's CORS middleware allows `localhost`/`127.0.0.1` origins in dev.

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

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Liveness probe. |
| GET | `/api/hosts` | List all known hosts. |
| GET | `/api/hosts/{id}` | Get a single host. |
| GET | `/api/hosts/{id}/metrics/latest` | Most recent metric sample. |
| GET | `/api/hosts/{id}/metrics?range=1h&points=300` | Down-sampled samples for the range. |
| GET | `/api/hosts/{id}/containers?all=true` | Containers on the host. |
| POST | `/api/hosts/{id}/containers/{cid}/start` | (also `stop`, `restart`, `pause`, `unpause`, `kill?signal=…`) |
| DELETE | `/api/hosts/{id}/containers/{cid}?force=&volumes=` | Remove container. |
| GET | `/api/hosts/{id}/containers/{cid}/logs?tail=200` | Plaintext logs. |

`range` accepts any Go-style duration (`15m`, `6h`, `24h`). `points` caps the returned series via uniform stride downsampling.

## Troubleshooting

- **`docker unavailable` on startup** — the user running the hub can't reach the docker socket. Add the user to the `docker` group or run with appropriate privileges. Metrics still work; container endpoints will 404.
- **Empty charts** — charts need at least 2 samples in the requested range. Wait one or two `-interval` cycles.
- **CORS errors in dev** — make sure you're hitting the dev server origin (`http://localhost:5173`); the hub allows that origin explicitly.
- **`port already in use`** — change `-listen`, e.g. `-listen :8081`.
- **High memory growth** — set `-retain` lower, or run `VACUUM` on the SQLite file periodically. Pruning runs hourly when retention > 0.

## Roadmap (post-v0.1)

- Threshold-based alert evaluator + UI.
- Container *create* (compose-style or one-off).
- Remote-agent binary + transport (the hub-side abstractions are ready).
- Authentication.
- Embedded frontend via `embed.FS` so a single binary needs no `-web-dir`.
