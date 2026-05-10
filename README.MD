# Aperture

A self-hosted homelab command center. One interface for system metrics, container management, alerts, and remote host monitoring — designed to replace Beszel, Portainer/Dockge, and Dozzle with a single coherent tool.

**Current version: v0.2.0-alpha.5**

---

## Contents

- [What it does](#what-it-does)
- [Screenshots / UI overview](#ui-overview)
- [Architecture](#architecture)
- [Quick start](#quick-start)
- [Building](#building)
- [Running the hub](#running-the-hub)
  - [Hub configuration](#hub-configuration)
  - [Systemd service](#hub-as-a-systemd-service)
  - [Docker](#hub-in-docker)
- [Adding remote hosts (agents)](#adding-remote-hosts-agents)
  - [How it works](#how-it-works)
  - [Step 1 — generate a token](#step-1--generate-a-token-in-the-ui)
  - [Step 2 — run the agent](#step-2--run-the-agent)
  - [Agent configuration](#agent-configuration)
  - [Agent systemd service](#agent-as-a-systemd-service)
  - [Agent in Docker](#agent-in-docker)
  - [Token management](#token-management)
- [Alerts](#alerts)
  - [Notification channels](#notification-channels)
- [API reference](#api-reference)
- [Development setup](#development-setup)
- [Troubleshooting](#troubleshooting)
- [Roadmap](#roadmap)

---

## What it does

### Monitoring
- **System metrics** collected every 5 seconds: CPU (aggregate + per-core), memory (used / cached / free), swap, disk usage per mount, disk I/O per device, network throughput per interface, load averages, uptime, temperatures.
- **Process list** — top 40 processes by CPU or memory, live.
- **Historical charts** — CPU, memory, disk, network, load average, per-interface network, per-mount disk, per-device I/O — across 15m / 1h / 6h / 24h windows. Drag to zoom, double-click to reset.
- **Multi-host** — hub embeds a local collector for the machine it runs on; remote hosts connect via lightweight agents over WebSocket.

### Container management
- Full lifecycle: create, start, stop, pause, unpause, restart, kill, remove, recreate.
- Live CPU / memory stats per container, port mappings, mount list.
- Deep inspect panel: config, env vars, labels, resource limits — all inline in the table row.
- Live resource editing (CPU cores / memory) with no container restart.
- Log viewer with search/filter.
- Create new containers with a form (image, name, restart policy, env vars, port bindings, volume mounts, auto-start toggle).

### Alerts
- Threshold rules: any metric (`cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load1`) with any comparison operator, optional sustained-breach duration before firing.
- Per-rule severity: `info`, `warning`, `critical`.
- Auto-resolves when the breach ends. Full event history.
- Red badge on the Alerts nav link shows firing count from anywhere in the app.

### Notification channels
Five channel types, all configurable from the UI with a Test button:
- **Discord** — rich embed with severity-coded color and field layout.
- **Slack** — incoming webhook with attachment.
- **ntfy** — push to any ntfy instance (self-hosted or ntfy.sh) with priority and emoji tags.
- **Gotify** — POST to a Gotify server with auto-mapped priority.
- **Generic webhook** — POST a structured JSON payload to any URL, with custom headers and method.

Per-channel: minimum severity filter (suppress info-level noise on critical channels), resolve-notify toggle.

---

## UI overview

```
Hosts (dashboard)
│
├── Host card — status pill (online/stale/offline), CPU/mem/disk bars,
│              alert badge, net throughput, container count, last-seen
│
└── Host detail (/hosts/{id})
    ├── Overview tab
    │   ├── Stat cards: CPU, Memory (segmented bar), Disk, Uptime
    │   ├── Live tables: per-core CPU, network interfaces, disk mounts,
    │   │              disk I/O, temperatures, processes
    │   └── Charts: aggregate + per-interface + per-mount + per-device
    └── Containers tab
        ├── Filter bar: state + text search (name/image)
        ├── Table with inline inspect + logs
        └── Create modal

Alerts (/alerts)
├── Rules tab — create/edit/delete rules, severity badge
├── Events tab — firing/resolved history
└── Channels tab — add/edit/test notification channels

Settings (/settings)
└── Agent tokens — generate, list, revoke
```

---

## Architecture

```
                       ┌─────────────────────────────────────────┐
                       │ aperture-hub                            │
                       │                                         │
  Browser ──HTTP──────►│ SvelteKit SPA  (static, served by hub) │
                       │        │                                │
                       │        ▼                                │
                       │   /api/*  (chi router)                  │
                       │        │                                │
                       │   ┌────┴────────────────────┐          │
                       │   │ internal/hub             │          │
                       │   │  ├─ host registry        │          │
                       │   │  ├─ ingest loop          │          │
                       │   │  ├─ retention pruner     │          │
                       │   │  └─ alert evaluator      │          │
                       │   └────┬──────────┬──────────┘          │
                       │        │          │                      │
                       │  local collector  │  AgentHandler        │
                       │  (gopsutil)       │  (WebSocket)         │
                       │                  │                      │
                       └──────────────────┼──────────────────────┘
                                          │ ws:// (token auth)
                              ┌───────────┴────────────┐
                              │                        │
                      aperture-agent            aperture-agent
                      (nas-box)                 (media-server)
                      collector + dockerctl     collector + dockerctl
```

The hub auto-registers itself as a local host on startup. Remote hosts run `aperture-agent`, which connects outbound to the hub over WebSocket — no port forwarding needed on the agent side, no polling from the hub.

---

## Quick start

```sh
# 1. Clone and build
git clone <repo> aperture && cd aperture
make build

# 2. Run the hub
./bin/aperture-hub -web-dir web/build

# 3. Open the dashboard
open http://localhost:8080
```

The hub immediately starts monitoring the local machine. No configuration is required to get the dashboard up.

---

## Building

### Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Go | ≥ 1.25 | Required by `modernc.org/sqlite` (pure-Go, no CGO) |
| Node.js | ≥ 20 | For the SvelteKit frontend |
| npm | ≥ 10 | Bundled with Node.js ≥ 16 |
| Docker daemon | any | Optional — container features only; metrics work without it |

On Debian/Ubuntu:
```sh
sudo apt-get install -y golang-go nodejs npm
```

### Build commands

```sh
make build    # build everything: web/build + bin/aperture-hub + bin/aperture-agent
make hub      # hub binary only → bin/aperture-hub
make agent    # agent binary only → bin/aperture-agent
make web      # frontend only → web/build/
make tidy     # go mod tidy
make clean    # remove bin/, web/build/, web/.svelte-kit/
```

---

## Running the hub

The hub is a single binary that serves both the API and the SvelteKit SPA.

```sh
./bin/aperture-hub -web-dir web/build
# → listening on :8080
```

On first start it creates `aperture.db` in the working directory, registers the local machine as a host, and begins sampling metrics every 5 seconds.

### Hub configuration

All flags accept an environment variable equivalent. Flags take precedence over env vars.

| Flag | Env var | Default | Description |
|---|---|---|---|
| `-listen` | `APERTURE_LISTEN` | `:8080` | HTTP listen address. Use `0.0.0.0:8080` to bind all interfaces. |
| `-db` | `APERTURE_DB` | `aperture.db` | SQLite database file path. Use an absolute path for a systemd service. |
| `-interval` | `APERTURE_INTERVAL` | `5s` | How often the local collector samples the machine. Agents set their own interval independently. |
| `-retain` | `APERTURE_RETAIN` | `336h` (14 days) | How long metric samples are kept. Accepts any Go duration string. `0` keeps forever. |
| `-disk-path` | `APERTURE_DISK_PATH` | `/` | Filesystem mount point to report disk usage for. Change if your data lives on a different partition (e.g. `/mnt/data`). |
| `-web-dir` | `APERTURE_WEB_DIR` | *(unset)* | Directory of the built SvelteKit SPA. If unset, the hub is API-only. |

**Examples:**

```sh
# Listen on all interfaces, store DB in /var/lib/aperture/
./bin/aperture-hub \
  -listen 0.0.0.0:8080 \
  -db /var/lib/aperture/aperture.db \
  -web-dir web/build

# Keep 30 days of data, monitor /mnt/data for disk usage
./bin/aperture-hub \
  -retain 720h \
  -disk-path /mnt/data \
  -web-dir web/build

# Environment variables (e.g. for Docker or systemd)
APERTURE_LISTEN=0.0.0.0:8080 \
APERTURE_DB=/var/lib/aperture/aperture.db \
APERTURE_RETAIN=720h \
./bin/aperture-hub -web-dir web/build
```

### Hub as a systemd service

Create `/etc/systemd/system/aperture-hub.service`:

```ini
[Unit]
Description=Aperture Hub
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
User=aperture
Group=aperture
WorkingDirectory=/opt/aperture
ExecStart=/opt/aperture/bin/aperture-hub \
  -listen 0.0.0.0:8080 \
  -db /var/lib/aperture/aperture.db \
  -web-dir /opt/aperture/web/build \
  -retain 336h
Restart=on-failure
RestartSec=5s

# The hub needs docker socket access for container management.
SupplementaryGroups=docker

[Install]
WantedBy=multi-user.target
```

```sh
# Create the user and data directory
sudo useradd -r -s /sbin/nologin aperture
sudo mkdir -p /var/lib/aperture
sudo chown aperture:aperture /var/lib/aperture

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable --now aperture-hub
sudo systemctl status aperture-hub
```

### Hub in Docker

```sh
docker run -d \
  --name aperture-hub \
  --restart unless-stopped \
  -p 8080:8080 \
  -v aperture-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd)/web/build:/web \
  -e APERTURE_DB=/data/aperture.db \
  -e APERTURE_WEB_DIR=/web \
  ghcr.io/aperture/hub:latest
```

> **Note:** The Docker image is not published yet (no registry release process exists in v0.2). Build locally with `docker build -t aperture-hub .` once a Dockerfile is added.

---

## Adding remote hosts (agents)

### How it works

Each remote machine runs `aperture-agent`, a lightweight process that:
1. Connects **outbound** to the hub over WebSocket (`ws://hub-address:8080/api/agents/ws`).
2. Authenticates with a pre-shared token you generate in the UI.
3. Streams metric samples to the hub every 5 seconds (configurable).
4. Sends heartbeats every 5 seconds to keep `last_seen` fresh.
5. Receives and executes docker commands from the hub (if docker is available on the agent host).

The agent only needs **outbound** network access to the hub — no firewall rules, port forwards, or VPN tunnels needed beyond what lets the agent reach the hub's address. The hub manages all agent state; if the agent disconnects and reconnects, it re-registers automatically.

### Step 1 — generate a token in the UI

1. Open the dashboard and click **Settings** in the top navigation.
2. Click **+ Add agent**.
3. Enter a name identifying the remote machine (e.g. `nas-box`, `media-server`, `pi4`). This name is for your reference; the agent's actual hostname is sent on first connect.
4. Click **Generate token**.
5. Copy the command shown — you'll need it on the remote machine.

> The token is shown **once** at generation time. It is not stored in plaintext anywhere and cannot be retrieved after you close this dialog. If you lose it, revoke it and generate a new one.

### Step 2 — run the agent

Copy the command from the UI and run it on your remote machine. It takes this form:

```sh
aperture-agent \
  --hub http://your-hub-ip:8080 \
  --token <generated-token>
```

The agent binary needs to be on the remote host. Options:

**A. Build on the remote host** (requires Go):
```sh
git clone <repo> aperture && cd aperture
go build -o aperture-agent ./cmd/agent
./aperture-agent --hub http://hub-ip:8080 --token <token>
```

**B. Copy from the hub machine**:
```sh
scp hub-machine:/opt/aperture/bin/aperture-agent remote-host:/usr/local/bin/
ssh remote-host aperture-agent --hub http://hub-ip:8080 --token <token>
```

The remote host appears in the Hosts dashboard within a few seconds of connecting.

### Agent configuration

| Flag | Default | Description |
|---|---|---|
| `--hub` | *(required)* | Full URL of the hub, e.g. `http://192.168.1.10:8080`. Use `https://` if the hub is behind a TLS-terminating proxy. |
| `--token` | *(required)* | Token generated via Settings → + Add agent. |
| `--name` | machine hostname | Override the hostname reported to the hub. Useful if your hostname is not human-friendly (e.g. `raspberrypi`). |
| `--interval` | `5s` | Metric sample interval. Increase to `30s` on low-power devices. |
| `--disk` | `/` | Filesystem path to report disk usage for. Set to your main data volume if it isn't `/`. |
| `--no-docker` | false | Disable docker collection even if the socket is available. Container management for this host will be unavailable. |

**Example — NAS with data on /mnt/storage:**
```sh
aperture-agent \
  --hub http://192.168.1.10:8080 \
  --token eyJhbGci... \
  --name nas-box \
  --disk /mnt/storage
```

**Example — Raspberry Pi, no docker, 15s interval:**
```sh
aperture-agent \
  --hub http://192.168.1.10:8080 \
  --token eyJhbGci... \
  --name pi4 \
  --interval 15s \
  --no-docker
```

#### Reconnection behavior

The agent reconnects automatically if the connection drops, using exponential backoff:
- First retry: 2 seconds
- Each subsequent retry doubles (2s → 4s → 8s → … → 60s cap)
- Once reconnected, the host's data resumes immediately

The hub shows the host as **stale** (amber) after 15 seconds without a heartbeat, and **offline** (red) after 90 seconds. This is display-only — the data is preserved and resumes when the agent reconnects.

### Agent as a systemd service

Create `/etc/systemd/system/aperture-agent.service` on the remote host:

```ini
[Unit]
Description=Aperture Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=aperture-agent
Group=aperture-agent
ExecStart=/usr/local/bin/aperture-agent \
  --hub http://192.168.1.10:8080 \
  --token YOUR_TOKEN_HERE \
  --name nas-box \
  --disk /mnt/storage
Restart=always
RestartSec=5s

# Add to docker group if you want container management on this host:
SupplementaryGroups=docker

[Install]
WantedBy=multi-user.target
```

```sh
# Create a minimal user for the agent
sudo useradd -r -s /sbin/nologin aperture-agent

# Add to docker group (if needed)
sudo usermod -aG docker aperture-agent

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable --now aperture-agent
sudo journalctl -u aperture-agent -f
```

### Agent in Docker

The agent can run in a container on the remote host. It needs access to the Docker socket to manage containers:

```sh
docker run -d \
  --name aperture-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/aperture/agent:latest \
  --hub http://192.168.1.10:8080 \
  --token YOUR_TOKEN_HERE \
  --name nas-box
```

> **Note:** The Docker image is not published yet. Build locally: `docker build -f Dockerfile.agent -t aperture-agent .`

If you want the agent to report disk usage for a path outside the container (e.g. your NAS storage), mount it read-only and set `--disk`:

```sh
docker run -d \
  --name aperture-agent \
  --restart unless-stopped \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /mnt/storage:/mnt/storage:ro \
  ghcr.io/aperture/agent:latest \
  --hub http://192.168.1.10:8080 \
  --token YOUR_TOKEN_HERE \
  --disk /mnt/storage
```

### Token management

Tokens are managed at **Settings → Agent tokens**.

| Column | Notes |
|---|---|
| Name | The label you gave the token when you generated it. |
| Created | When the token was generated. |
| Last used | When the agent last authenticated (updates on WS connect). |
| Revoke | Immediately invalidates the token. Any agent using it will disconnect and be unable to reconnect until a new token is issued. |

**Rotating a token:**
1. Generate a new token for the same machine.
2. Update the agent's `--token` flag (update systemd unit or docker run command).
3. Restart the agent.
4. Revoke the old token once the agent reconnects with the new one.

**Security notes:**
- Tokens are 32 bytes of cryptographically random data (256 bits of entropy).
- Only the SHA-256 hash is stored in the database — the hub cannot recover a plaintext token.
- Tokens are sent over the WebSocket upgrade HTTP request as a `Bearer` header. Use HTTPS/TLS termination at the reverse proxy layer if the hub is accessible over an untrusted network.

---

## Alerts

Alerts are configured at **Alerts → Rules**.

### Creating a rule

| Field | Description |
|---|---|
| Host | Specific host, or "all hosts" to apply everywhere. |
| Metric | `cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load1` |
| Operator | `>`, `>=`, `<`, `<=` |
| Threshold | Numeric value (percentages as 0–100, load as raw). |
| Duration | Breach must be sustained for this many seconds before firing. `0` = fire immediately. |
| Severity | `info`, `warning`, `critical` — used for per-channel filtering. |

Rules evaluate on every metric ingest. An event fires when the threshold is crossed for the full duration. It auto-resolves when the metric returns below the threshold and a new sample arrives.

**Example rules:**
- CPU above 80% for 60 seconds → warning
- Disk above 90% for 0 seconds → critical (fires immediately)
- Load average above 4.0 for 120 seconds → warning

### Notification channels

Channels are configured at **Alerts → Channels**.

Each channel has:
- **Type** — Discord, Slack, ntfy, Gotify, or Webhook.
- **Minimum severity** — only notify for rules at or above this level. Set a critical-only channel for your phone and an info+ channel for a low-noise logging destination.
- **Notify on resolve** — send a follow-up notification when the alert auto-resolves.
- **Test button** — sends a synthetic test notification immediately to verify the config.

#### Discord

Paste your Discord webhook URL (Channel Settings → Integrations → Webhooks).

Notifications are sent as rich embeds with severity-coded colors:
- Critical → red
- Warning → orange
- Info → blue
- Resolved → green

#### Slack

Paste the Incoming Webhook URL from your Slack app configuration.

#### ntfy

| Field | Description |
|---|---|
| Server URL | `https://ntfy.sh` for the public server, or your self-hosted instance URL. |
| Topic | The topic name (e.g. `homelab-alerts`). Keep this private — anyone who knows the topic can subscribe. |
| Token | Bearer token if your ntfy server requires auth. Leave blank for public topics. |
| Priority | Override the default priority mapping. Leave blank to use auto (critical=urgent, warning=high, info=default, resolved=low). |

#### Gotify

| Field | Description |
|---|---|
| Server URL | Your Gotify server, e.g. `https://gotify.example.com`. |
| App Token | The token from your Gotify app. |
| Priority | Custom priority. Leave `0` to use auto (critical=10, warning=5, info=1, resolved=2). |

#### Generic webhook

Posts a JSON payload to any URL. Useful for Home Assistant, n8n, Zapier, custom scripts, etc.

| Field | Description |
|---|---|
| URL | Target URL. |
| Method | `POST` (default) or `GET`. |
| Headers | Custom HTTP headers as key:value pairs. |

Payload shape:
```json
{
  "type": "alert_fired",
  "host": { "id": "...", "name": "nas-box", ... },
  "rule": { "metric": "disk_pct", "op": ">", "threshold": 90, "severity": "critical" },
  "event": { "id": 42, "fired_at": "2026-05-09T14:30:00Z", "value": 93.5 },
  "resolved_at": null
}
```
When resolved, `"type"` is `"alert_resolved"` and `"resolved_at"` is an ISO timestamp.

---

## API reference

All endpoints are under `/api`. Responses are JSON.

### System

| Method | Path | Description |
|---|---|---|
| GET | `/api/health` | Liveness probe. Returns `{"ok":true}`. |
| GET | `/api/system/info` | Version, started-at, DB path, and DB size on disk (includes WAL). |

### Hosts

| Method | Path | Description |
|---|---|---|
| GET | `/api/hosts` | All registered hosts (local + agents). |
| GET | `/api/hosts/{id}` | One host. |
| GET | `/api/hosts/{id}/metrics/latest` | Most recent sample — includes live-only rich fields (per-core CPU, net interfaces, disk mounts, disk I/O, temps, processes). |
| GET | `/api/hosts/{id}/metrics?range=1h&points=300` | Downsampled historical aggregate samples. `range`: any Go duration. `points`: max rows returned (uniform stride). |
| GET | `/api/hosts/{id}/metrics/net?range=1h&points=300` | Historical per-interface byte counters. |
| GET | `/api/hosts/{id}/metrics/mounts?range=1h&points=300` | Historical per-mount used/total bytes. |
| GET | `/api/hosts/{id}/metrics/diskio?range=1h&points=300` | Historical per-device read/write byte counters. |

### Containers

| Method | Path | Description |
|---|---|---|
| GET | `/api/hosts/{id}/containers?all=true` | List containers. `all=false` returns running only. |
| POST | `/api/hosts/{id}/containers` | Create container. Body: `{image, name?, restart_policy?, env?, ports?, volumes?, auto_start}`. |
| GET | `/api/hosts/{id}/containers/{cid}/inspect` | Full container detail: config, env, ports, mounts, labels, live stats, resource limits. |
| PUT | `/api/hosts/{id}/containers/{cid}/resources` | Update CPU/memory limits live. Body: `{nano_cpus?, memory_bytes?}`. `0` = unlimited. |
| POST | `/api/hosts/{id}/containers/{cid}/recreate` | Stop → remove → recreate from same spec. |
| POST | `/api/hosts/{id}/containers/{cid}/start` | Start. Also: `stop`, `restart`, `pause`, `unpause`, `kill?signal=SIGTERM`. |
| DELETE | `/api/hosts/{id}/containers/{cid}?force=false&volumes=false` | Remove. |
| GET | `/api/hosts/{id}/containers/{cid}/logs?tail=200` | Container logs (plaintext). |

### Alerts

| Method | Path | Description |
|---|---|---|
| GET | `/api/alerts/metadata` | Available metrics, operators, severities, channel types. |
| GET | `/api/alerts/rules?host_id=` | List rules. Omit `host_id` for all rules. |
| POST | `/api/alerts/rules` | Create rule. Body: `{host_id?, metric, op, threshold, duration_s, enabled, severity}`. |
| GET | `/api/alerts/rules/{id}` | Get one rule. |
| PUT | `/api/alerts/rules/{id}` | Update rule (full replace). |
| DELETE | `/api/alerts/rules/{id}` | Delete rule. Cascades to events and clears evaluator state. |
| GET | `/api/alerts/events?host_id=&open=true&limit=200` | Event history. `open=true` = currently firing only. |
| GET | `/api/alerts/channels` | List notification channels. |
| POST | `/api/alerts/channels` | Create channel. |
| GET | `/api/alerts/channels/{id}` | Get one channel. |
| PUT | `/api/alerts/channels/{id}` | Update channel. |
| DELETE | `/api/alerts/channels/{id}` | Delete channel. |
| POST | `/api/alerts/channels/{id}/test` | Send a test notification immediately. Returns `{"ok":true}` or `{"error":"..."}`. |

### Agent tokens

| Method | Path | Description |
|---|---|---|
| GET | `/api/agents/tokens` | List all non-revoked tokens (names, created, last-used — no plaintext). |
| POST | `/api/agents/tokens` | Create token. Body: `{"name":"my-machine"}`. Returns the struct with `token` field set — **only time the plaintext is returned**. |
| DELETE | `/api/agents/tokens/{id}` | Revoke token. Connected agents using it disconnect immediately. |
| GET | `/api/agents/connected` | List host IDs of currently connected agents. |
| GET | `/api/agents/ws` | WebSocket endpoint for agent connections (not for direct use; see agent binary). |

---

## Development setup

### Backend

```sh
# Run the hub with a short sample interval for faster feedback
go run ./cmd/hub -interval 2s -web-dir web/build

# Or use the Makefile target
make dev
```

### Frontend

```sh
cd web
npm install
npm run dev      # Vite dev server on :5173, proxies API to :8080
```

The Vite dev server auto-sets `VITE_API_BASE=http://localhost:8080`. The hub's CORS middleware allows `localhost` origins in any mode, so cross-origin requests work without any manual config.

### Both together

Terminal 1:
```sh
make dev   # hub on :8080
```

Terminal 2:
```sh
cd web && npm run dev   # SPA on :5173
```

Open `http://localhost:5173` for live-reload development.

### Project layout

```
aperture/
├── cmd/
│   ├── hub/            Hub binary (main.go)
│   └── agent/          Agent binary (main.go)
├── internal/
│   ├── alerts/         Threshold evaluator + notification senders
│   │   ├── alerts.go   Evaluator: sustained-breach detection, auto-resolve
│   │   ├── notify.go   Notifier: dispatch to channels, severity filtering
│   │   ├── ch_discord.go
│   │   ├── ch_slack.go
│   │   ├── ch_ntfy.go
│   │   ├── ch_gotify.go
│   │   └── ch_webhook.go
│   ├── api/            HTTP handlers + chi router
│   ├── collector/      gopsutil-based local metric sampler
│   ├── dockerctl/      Docker SDK wrapper
│   ├── hub/            Orchestration + AgentHandler (WebSocket)
│   ├── store/          SQLite store + schema
│   └── types/          Shared Go types
├── web/                SvelteKit 5 frontend
│   └── src/
│       ├── lib/        Shared components, API client, types, format utils
│       └── routes/     Pages: /, /hosts/[id], /hosts/[id]/containers,
│                               /alerts, /settings
├── Makefile
├── README.MD           This file
├── overview.md         Internal feature + architecture overview
├── technical.md        Per-function implementation detail
└── changelog.md        Versioned change log
```

---

## Troubleshooting

### Hub

**`docker unavailable` on startup**
The user running the hub cannot reach the Docker socket. Fix:
```sh
sudo usermod -aG docker $USER
# then log out and back in, or: newgrp docker
```
Metrics continue to work; only container endpoints are affected.

**Charts show no data**
Charts require at least 2 samples in the requested time range. Wait one or two `-interval` cycles after starting. The "15m" range means the last 15 minutes of stored data — if the hub just started, switch to a longer range or wait.

**Port already in use**
```sh
./bin/aperture-hub -listen :8081 -web-dir web/build
```

**Database growing large**
Set a shorter retention period:
```sh
./bin/aperture-hub -retain 72h -web-dir web/build
```
The pruner runs every hour. To reclaim space immediately, stop the hub and run:
```sh
sqlite3 aperture.db "VACUUM;"
```

**CORS errors in dev**
Make sure you opened `http://localhost:5173` (the Vite dev server), not `:8080` directly, and that the hub is running on `:8080`.

### Agent

**Agent won't connect — `unauthorized`**
The token was revoked, or the wrong token was used. Generate a new one in Settings.

**Agent connects but shows as offline immediately**
The hub address in `--hub` is reachable from the agent (try `curl http://hub-ip:8080/api/health` from the agent machine). If using a hostname, verify DNS resolution.

**No docker tab / container management unavailable for an agent host**
Either the agent was started with `--no-docker`, or docker is not installed/running on the agent machine, or the agent user doesn't have socket access. Check:
```sh
# On the agent machine:
docker ps   # if this fails, fix docker permissions
# Then restart the agent without --no-docker
```

**Agent reporting wrong disk usage**
Use `--disk /path/to/your/storage` to point at the mount you care about. The default `/` reports the root filesystem.

**High CPU on agent machine**
Increase `--interval`:
```sh
aperture-agent --hub ... --token ... --interval 30s
```

**Can't see historical data after agent reconnects**
Historical data is preserved in the hub's SQLite DB across disconnections. Only the in-memory live snapshot is cleared on disconnect. Once the agent reconnects and sends a sample, the live data (per-core CPU, process list, etc.) reappears.

---

## Roadmap

Phase 1 (Solidify the Core) is done. Planned phases, in rough order:

| Phase | Focus |
|---|---|
| 2 | **Compose-first workflow** — discover, import, edit, and deploy `docker-compose.yml` stacks from the UI; git-style version diffs and rollback |
| 3 | **Deep docker** — network topology, volume management, image update detection, log streaming (Dozzle replacement) |
| 4 | **Metrics depth** — ZFS pools, SMART per-drive, RAID status, NFS/SMB mounts, container-level metrics, retention tiers |
| 5 | **Alerting depth** — container state alerts, storage alerts, agent-offline detection, cooldown/dedup, ack/silence, escalation |
| 6 | **Network** — subnet scanning, device inventory, OPNsense integration, WireGuard/OpenVPN tunnel status, topology map |
| 7 | **Ansible** — playbook discovery, in-app editor, run against agent inventory, streaming output, execution history |
| 8 | **Auth & multi-user** — Traefik/Caddy reverse proxy integration, RBAC, sessions + 2FA, optional OIDC/LDAP |

**Architecture goals locked in from day 1:**
- API-first: every UI action is an API call; the API is the product.
- Agent capabilities reporting: ZFS available? Ansible installed? Hub surfaces and disables irrelevant tabs automatically.
- Pluggable providers for notification channels, proxy integrations, metric collectors.
- mTLS as a future upgrade path for agent authentication (token auth is the Phase 1 baseline).
