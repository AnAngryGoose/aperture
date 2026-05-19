# Aperture

A self-hosted homelab command center. One UI for system metrics, container management, and alerts across local + remote hosts. Designed to replace Beszel, Portainer/Dockge, and Dozzle with a single tool.

**Current version: v0.0.4-alpha.3**

---

## Contents

- [What it does](#what-it-does)
- [Quick start](#quick-start)
- [Building](#building)
- [Running the hub](#running-the-hub)
- [Adding remote hosts (agents)](#adding-remote-hosts-agents)
- [Per-host monitoring config](#per-host-monitoring-config)
- [Alerts](#alerts)
- [API reference](#api-reference)
- [Development setup](#development-setup)
- [Troubleshooting](#troubleshooting)
- [Roadmap](#roadmap)

---

## What it does

**Monitoring**
- CPU (aggregate + per-core), memory (used/cached/free + ZFS-ARC-aware via `mem_calc`), swap, disk per mount, disk I/O per device, network per interface, load, uptime, temperatures, top processes, container stats — all with historical charts.
- Per-host **monitoring config** persisted in SQLite: pick which families collect, NIC/sensor/mount allow/deny filters, per-host warn/crit thresholds, retention overrides.
- **Multi-host** via lightweight agents (outbound WebSocket — no port forward needed at the agent).
- Single **`/api/monitoring/{overview,bundle,catalog}`** endpoints replace per-host polling: one fetch + typed SSE for live updates.

**Containers**
- Full lifecycle (create/start/stop/restart/recreate/remove), live stats, deep inspect, live CPU/mem limit edits, logs.
- Per-host container summary cached in the hub and broadcast over SSE — no fan-out Docker polling from the browser.

**Alerts**
- **Threshold rules** on flat scalars (`cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load_1/5/15`, `temp.max`) and **dotted targets** (`iface.eth0.rx_rate`, `mount./.pct`, `temp.cpu_thermal.value`, `proc.nginx.cpu_pct`, `host.status`).
- Sustained-breach duration, severity (info/warning/critical), auto-resolve.
- **Status alerts** — `host.status == offline` fires via an offline watchdog when a host stops sending samples.
- **Templates** — apply "Beszel defaults" / "Aggressive" / "Quiet" preset rule sets per-host or globally.
- **Notifications** through 17+ services via Shoutrrr (Discord, Slack, Telegram, Matrix, Pushover, Teams, ntfy, Gotify, Mattermost, Rocketchat, etc.) plus a generic webhook.

**UI**
- Sidebar shell, Geist font, dark/light/system theme, six accent colors.
- Dashboard with rich/tile/list card variants + a per-card widget picker (pick 2–4 metrics per host).
- Host detail with 10 tabs: Overview / CPU / Memory / Disk / Network / Sensors / Processes / Docker / Events / Settings.
- Drag-to-zoom charts, live SSE sparklines, drill-in slide-over modal.

---

## Quick start

```sh
git clone <repo> aperture && cd aperture
make build
./bin/aperture-hub -web-dir web/build
# Open http://localhost:8080 — you'll be redirected to /setup to create an admin password.
```

The hub auto-registers itself as a local host and starts sampling immediately. No config needed for the basic case.

---

## Building

Requires Go ≥ 1.25 (pure-Go SQLite, no CGO) and Node.js ≥ 20.

```sh
make build    # everything: web/build + bin/aperture-hub + bin/aperture-agent
make hub      # hub binary only
make agent    # agent binary only
make web      # frontend only
make dev      # hub with -tags dev (CORS for the Vite dev server)
make tidy     # go mod tidy
make clean
```

---

## Running the hub

```sh
./bin/aperture-hub -web-dir web/build
```

The hub serves both the API and the SPA from a single binary. First start creates `aperture.db`, registers the local machine, and begins sampling.

### Hub flags

| Flag | Env var | Default | Notes |
|---|---|---|---|
| `-listen` | `APERTURE_LISTEN` | `:8080` | Use `0.0.0.0:8080` to bind all interfaces. |
| `-db` | `APERTURE_DB` | `aperture.db` | Use an absolute path for systemd. |
| `-interval` | `APERTURE_INTERVAL` | `5s` | Local-collector sample cadence. Agents have their own. Can be overridden per-host via host_config. |
| `-retain` | `APERTURE_RETAIN` | `336h` (14 days) | Global fallback. Per-host retention takes precedence. `0` = forever. |
| `-disk-path` | `APERTURE_DISK_PATH` | `/` | Filesystem root for the aggregate disk metric. |
| `-web-dir` | `APERTURE_WEB_DIR` | *(unset)* | Built SvelteKit assets. Unset = API only. |

```sh
# Bind all interfaces, 30-day retention, data on /mnt/storage
./bin/aperture-hub \
  -listen 0.0.0.0:8080 \
  -db /var/lib/aperture/aperture.db \
  -retain 720h \
  -disk-path /mnt/storage \
  -web-dir web/build
```

### Hub as a systemd service

`/etc/systemd/system/aperture-hub.service`:

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
  -web-dir /opt/aperture/web/build
Restart=on-failure
RestartSec=5s
SupplementaryGroups=docker

[Install]
WantedBy=multi-user.target
```

```sh
sudo useradd -r -s /sbin/nologin aperture
sudo mkdir -p /var/lib/aperture && sudo chown aperture:aperture /var/lib/aperture
sudo systemctl daemon-reload && sudo systemctl enable --now aperture-hub
```

---

## Adding remote hosts (agents)

Agents are a single binary that connects outbound to the hub over WebSocket. No port forwarding, no polling.

### 1. Generate a token in the UI

**Settings → + Add agent → Generate token**. Copy the command shown — the token is displayed **once**.

### 2. Run the agent on the remote machine

```sh
aperture-agent --hub http://your-hub:8080 --token <token>
```

Get the binary onto the remote host either by building locally (`go build -o aperture-agent ./cmd/agent`) or scp-ing it from the hub machine. The host appears in the dashboard within seconds.

### Agent flags

| Flag | Default | Notes |
|---|---|---|
| `--hub` | *(required)* | Full URL of the hub. `https://` if behind TLS termination. |
| `--token` | *(required)* | From the Settings UI. |
| `--name` | hostname | Override the name reported to the hub. |
| `--interval` | `5s` | Sample cadence. Hub-supplied `host_config.sample_interval_s` overrides this at runtime. |
| `--disk` | `/` | Mount point to report disk usage for. |
| `--no-docker` | false | Disable Docker collection on this host. |

The agent **receives** its monitoring config from the hub at connect time and on every UI edit — you don't need flags for which families to collect, threshold values, or filters. Those live in `host_config`.

### Agent as a systemd service

`/etc/systemd/system/aperture-agent.service`:

```ini
[Unit]
Description=Aperture Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=aperture-agent
ExecStart=/usr/local/bin/aperture-agent \
  --hub http://192.168.1.10:8080 \
  --token YOUR_TOKEN_HERE \
  --name nas-box
Restart=always
RestartSec=5s
SupplementaryGroups=docker

[Install]
WantedBy=multi-user.target
```

```sh
sudo useradd -r -s /sbin/nologin aperture-agent
sudo usermod -aG docker aperture-agent  # if you want container management
sudo systemctl daemon-reload && sudo systemctl enable --now aperture-agent
```

### Reconnection

Exponential backoff (2s → 60s cap). The hub marks hosts **offline** after ~30s with no sample (configurable via `host_config.sample_interval_s` — threshold is `max(2× interval, 30s)`). Historical data is preserved across disconnects.

### Token management

**Settings → Agent tokens** lists name, created-at, last-used. Revoking immediately disconnects any agent using that token. Tokens are 32 bytes of random data; only the SHA-256 hash is stored.

---

## Per-host monitoring config

Each host has a `host_config` row driving what it monitors and when status flips. Edit via **Hosts → {host} → Settings tab** in the UI, or via the API.

| Field | Default | Notes |
|---|---|---|
| `sample_interval_s` | 5 | Pushed to the agent / applied to the local collector live. |
| `enabled_families` | all on | `cpu`, `cpu_per_core`, `mem`, `disk`, `mounts`, `disk_io`, `net`, `load`, `uptime`, `temps`, `processes`, `containers`. Disable a family to stop collecting it entirely. |
| `mem_calc` | `used` | `avail` mode uses `MemAvailable` (matches htop). Useful on hosts with heavy page cache. |
| `filters.nic_allow` / `nic_deny` | `[]` | Allow/deny lists for network interfaces. Deny wins. |
| `filters.sensor_allow` / `sensor_deny` | `[]` | Same for temperature sensors. |
| `filters.mount_allow` / `mount_deny` | `[]` | Same for filesystem mounts. |
| `retention_days` | 30 | Default retention. |
| `retention_overrides` | `{}` | Per-table retention in days, e.g. `{"process_metrics": 3}` to keep process history short. |
| `warn_cpu` / `crit_cpu` etc. | 70 / 90 (CPU, mem, disk); 70 / 85 (temp) | Drive the host status flip (`ok` → `warn` → `crit`). |

Hosts without a row use **global defaults** edited in **Settings → Monitoring defaults**. The hardcoded fallbacks are the values above.

### Optional family scaffolds

`smart` (disk health), `gpu`, `battery`, `systemd` are registered in the catalog as `experimental:true` and return "not implemented" today. The seam in `internal/collector/families/` is in place so each is a single new file when implemented.

---

## Alerts

**Alerts → Rules → New rule**.

The metric picker is two-step:
1. **Category**: `simple` (host-wide scalar), `iface` (per network interface), `mount` (per filesystem), `temp` (per sensor), `proc` (per process name), `host` (host status).
2. **Target + leaf** (for dotted categories): e.g. category=`iface`, target=`eth0`, leaf=`rx_rate` builds `iface.eth0.rx_rate`.

Supported scalar metrics: `cpu_pct`, `mem_pct`, `disk_pct`, `swap_pct`, `load_1/5/15`, `temp.max` (hottest sensor), `host.status` (encoded `ok=0`, `warn=1`, `crit=2`, `offline=3`).

Operators: `>`, `>=`, `<`, `<=`, `==`, `!=`.

```
Example rules:
  cpu_pct                > 90 for 60s     → warning
  mount./.pct            > 85 for 5m      → warning
  iface.eth0.rx_rate     > 1048576 for 30s → warning   (1 MiB/s sustained)
  proc.nginx.cpu_pct     > 50 for 30s     → warning
  temp.max               > 80 for 60s     → critical
  host.status            == 3 for 120s    → critical   (host offline)
```

Rules with `host_id` set are scoped to one host; rules with `host_id: null` apply globally.

### Templates

**Alerts → Apply template** lets you seed a rule set in one click:

| Template | Rules |
|---|---|
| **Beszel defaults** | CPU>90/60s warn, Mem>90/60s warn, Disk>85/5m warn, temp.max>80/60s critical, host.status==offline/2m critical |
| **Aggressive** | Same metrics, lower thresholds, shorter durations |
| **Quiet** | Same metrics, higher thresholds, longer durations |

Apply is idempotent — duplicate `(metric, op, threshold)` rules are skipped.

### Notification channels

Configure at **Alerts → Channels**. Each channel has a minimum-severity filter and a notify-on-resolve toggle.

| Type | What you paste |
|---|---|
| **discord** | Discord webhook URL (Channel Settings → Integrations → Webhooks) |
| **slack** | Slack incoming-webhook URL |
| **ntfy** | Server URL, topic, optional token + priority |
| **gotify** | Server URL + app token |
| **shoutrrr** | Raw [Shoutrrr URL](https://containrrr.dev/shoutrrr/services/overview/) — unlocks Telegram, Matrix, Pushover, Teams, Mattermost, Rocketchat, OpsGenie, IFTTT, Pushbullet, Bark, Join, Lark, WeCom, Zulip, and more |
| **webhook** | Generic POST URL with custom headers — gets the alert payload as JSON |

The discord/slack/ntfy/gotify legacy types route through Shoutrrr internally. Their stored config rows are translated to Shoutrrr URLs at send time — no migration needed if you already had channels.

---

## API reference

All endpoints are under `/api`. Auth required via `aperture_session` cookie except `/api/health`, `/api/auth/*`, and `/api/agents/ws`.

### Auth

| Method | Path | Notes |
|---|---|---|
| GET | `/api/auth/status` | `{configured, authenticated}` |
| POST | `/api/auth/setup` | First-run only. `{password}` → session cookie. |
| POST | `/api/auth/login` | `{password}` → session cookie (24h). |
| POST | `/api/auth/logout` | Clears the session. |
| POST | `/api/auth/change-password` | `{current, new}` |

### Monitoring spine

| Method | Path | Notes |
|---|---|---|
| GET | `/api/monitoring/overview` | Hosts + latest sample + container counts + open-alert counts + status. One call replaces the dashboard's N+1 fan-out. |
| GET | `/api/monitoring/catalog` | Families, scalar metrics, alert categories+leaves, rule templates. Single source of truth for UI pickers. |
| GET | `/api/hosts/{id}/monitoring/bundle?range=1h&points=300[&include=metrics,net,mounts,diskio,temps,cpu]` | Host + latest + config + all history series + open alerts in one response. Drives the host-detail tabs. |
| GET / PUT | `/api/hosts/{id}/config` | Per-host monitoring policy (see [Per-host monitoring config](#per-host-monitoring-config)). PUT pushes to the running collector/agent. |
| GET / PUT | `/api/settings/monitoring-defaults` | Global defaults applied to hosts without their own row. |
| GET | `/api/stream/metrics` | SSE stream. Typed envelope `{type, hostId, ts, ...}` — `type` ∈ `metric` / `host_status` / `container_summary` / `alert`. |

### Per-metric history (single-chart drill-ins)

| Method | Path |
|---|---|
| GET | `/api/hosts/{id}/metrics?range=&points=` (aggregate) |
| GET | `/api/hosts/{id}/metrics/latest` (live snapshot with rich fields) |
| GET | `/api/hosts/{id}/metrics/net?range=&points=` (per-interface) |
| GET | `/api/hosts/{id}/metrics/mounts?range=&points=` (per-mount) |
| GET | `/api/hosts/{id}/metrics/diskio?range=&points=` (per-device) |
| GET | `/api/hosts/{id}/metrics/temps?range=&points=` (per-sensor) |
| GET | `/api/hosts/{id}/metrics/cpu?range=&points=` (per-core + aggregate) |
| GET | `/api/hosts/{id}/metrics/procs?name=X&range=&points=` (one process by name) |
| GET | `/api/hosts/{id}/containers/{cid}/metrics?range=&points=` (per-container CPU/mem/net) |

### Containers

| Method | Path |
|---|---|
| GET | `/api/hosts/{id}/containers?all=true` |
| POST | `/api/hosts/{id}/containers` |
| GET | `/api/hosts/{id}/containers/{cid}/inspect` |
| PUT | `/api/hosts/{id}/containers/{cid}/resources` |
| POST | `/api/hosts/{id}/containers/{cid}/{start,stop,restart,pause,unpause,kill,recreate}` |
| DELETE | `/api/hosts/{id}/containers/{cid}?force=&volumes=` |
| GET | `/api/hosts/{id}/containers/{cid}/logs?tail=` |

### Alerts

| Method | Path |
|---|---|
| GET | `/api/alerts/metadata` — supported metrics, categories+leaves, ops, severities, channel types |
| GET / POST | `/api/alerts/rules?host_id=` |
| GET / PUT / DELETE | `/api/alerts/rules/{id}` |
| POST | `/api/alerts/templates/apply` — `{template:"Beszel defaults", host_id:null|"id"}` |
| GET | `/api/alerts/events?host_id=&open=true&limit=` |
| GET / POST | `/api/alerts/channels` |
| GET / PUT / DELETE | `/api/alerts/channels/{id}` |
| POST | `/api/alerts/channels/{id}/test` |

### Agent tokens

| Method | Path |
|---|---|
| GET | `/api/agents/tokens` |
| POST | `/api/agents/tokens` — `{name}` returns plaintext token **once** |
| DELETE | `/api/agents/tokens/{id}` |
| GET | `/api/agents/connected` |
| GET | `/api/agents/ws` (agent WebSocket endpoint) |

---

## Development setup

```sh
# Terminal 1 — hub with -tags dev (enables CORS for the Vite dev server)
make dev

# Terminal 2 — frontend with hot reload
cd web && npm install && npm run dev
```

Open `http://localhost:5173`. First run redirects to `/setup` for password creation.

The `-tags dev` build tag activates the CORS middleware that allows cross-origin requests from `localhost`/`127.0.0.1` origins with credentials. Without it, the session cookie is rejected.

### Project layout (high-level)

```
cmd/{hub,agent}      Binaries
internal/
├── agentproto       Hub ↔ agent WebSocket frame types (TypeConfig pushes host policy)
├── alerts           Evaluator + StatusProvider + Shoutrrr dispatcher + templates
├── api              chi router + monitoring spine + auth + middleware
├── collector        gopsutil collector + family scaffolds (internal/collector/families/)
├── dockerctl        Docker SDK wrapper
├── hub              Orchestration: ingest loop, SSE broadcast, retention, offline watchdog
├── store            SQLite store + schema (host_config + metric history tables)
└── types            Shared Go types
web/src/
├── lib/             Components, monitoring store, metric catalog, api client
└── routes/          dashboard, /hosts/[id] (10 tabs), /alerts, /settings, /containers, /stacks, ...
```

`docs/changelog.md` has the full per-release change log. `docs/technical.md` has per-function implementation detail. `docs/overview.md` has the feature roadmap.

---

## Troubleshooting

**`docker unavailable` on startup.** The hub user can't reach the Docker socket. Fix: `sudo usermod -aG docker $USER && newgrp docker`. Metrics still work without it — only container endpoints fail.

**Charts show no data.** Charts need ≥ 2 samples in the range. Wait one or two sample intervals, or pick a longer range.

**Database growing large.** Cap retention globally via `-retain` or per-host via `host_config.retention_days` + `retention_overrides`. To reclaim space immediately: stop the hub and `sqlite3 aperture.db "VACUUM;"`.

**CORS errors in dev.** Run the hub via `make dev` (passes `-tags dev`). Without it the CORS middleware is compiled out.

**Agent says `unauthorized`.** Token was revoked or wrong. Generate a new one in Settings.

**Agent stays offline after connect.** Check `curl http://hub-ip:8080/api/health` from the agent machine. If using a hostname, verify DNS resolution.

**Host detail page making thousands of requests.** This was an early regression that's been fixed — if you somehow still see it, you're running an unfixed build of `hosts/[id]/+page.svelte`. Rebuild from current source.

**High CPU on agent machine.** Either lower its sample interval (UI → host → Settings → Sample interval) or disable expensive families (processes, cpu_per_core).

**Want different metrics on a card.** Click the `⋯` menu on a host card → **Configure widget…** → pick up to 4 from the metric catalog. Persisted per-host in localStorage + dashboard layout.

---

## Roadmap

Phase 4 (monitoring depth + Beszel parity + remote-host expansion) is **done**. Planned next:

| Phase | Focus |
|---|---|
| 5 | **Alerting depth** — cooldown / dedup, ack / silence, escalation chains, agent-side rule offload for high-frequency metrics |
| 6 | **Container & stack management refresh** — Portainer/Dockge parity polish; compose-stack metrics on the host detail page |
| 7 | **Network** — subnet scanning, device inventory, OPNsense integration, WireGuard/OpenVPN tunnel status, topology map |
| 8 | **Ansible** — playbook discovery, in-app editor, run against agent inventory, streaming output, execution history |
| 9 | **Optional families implementation** — SMART (smartctl), GPU (nvidia-smi/AMD sysfs/Intel), battery, systemd unit health |
| 10 | **Auth & multi-user** — reverse-proxy header trust, RBAC, OIDC/LDAP |

Architecture invariants we intend to keep:
- API-first: every UI action is an API call.
- Agent receives config from the hub — no per-agent flags for what to monitor.
- Pluggable collector families, alert channels (via Shoutrrr), and notification formats.
- mTLS as a future upgrade path for agent auth.
