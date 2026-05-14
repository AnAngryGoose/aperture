# Handoff: Aperture — Dashboard & Shell

## Overview

This package contains the design for **Aperture**, a unified web UI for homelab command & control (Docker + bare-metal + edge agents). It covers the application shell (sidebar, topbar) and the **Dashboard** tab in full, plus supporting screens: host drill-in, Add Host flow, Alerts, Settings, and empty/loading/error states.

Design principle: **simple surface, deep capability.** A Docker newbie should feel at home; a power user should reach every feature without leaving the web UI.

**Tech context.** Frontend is SvelteKit; backend is Go. The prototype is React+HTML purely for design clarity.

## About the design files

The files in this bundle (`Aperture Dashboard.html`, `aperture-*.jsx`, `tweaks-panel.jsx`) are **design references**. They show intended look, layout, and behavior — they are **not production code to copy directly**.

Recreate these designs in the Aperture SvelteKit codebase using its existing patterns (Svelte 5 runes, file-based routing, server load fns, stores). Component boundaries, naming, and data flow should follow the repo's conventions — the JSX is just the most precise way to describe the intended end result.

The **Tweaks panel** in the prototype is a design-exploration tool. **It must not ship.** The chosen defaults are:

- Theme: dark (with light variant + system follow)
- Accent: teal `#14b8a6` (user-selectable in Settings → Appearance)
- Typography: Geist Sans + Geist Mono
- Card layout: Rich
- Navigation: Labeled sidebar
- Sparkline: Area
- Status indicator: Dot

User-facing appearance preferences live in **Settings → Appearance** (not in a floating panel).

## Fidelity

**High-fidelity.** Colors, typography, spacing, radii, motion, and component anatomy are final. Match the design tokens exactly and recreate components pixel-for-pixel. Where the codebase already has a primitive (button, input, modal), prefer it but restyle to Aperture tokens.

---

## Design tokens

Define in `src/lib/styles/tokens.css`. Toggle theme via `<html data-theme="dark|light">`.

### Color — dark (default)

| Token | Hex |
|---|---|
| `--bg` | `#0d0f12` |
| `--bg-elev` | `#14171c` |
| `--bg-elev-2` | `#191d23` |
| `--bg-hover` | `#1c2128` |
| `--line` | `#242932` |
| `--line-strong` | `#2e343d` |
| `--text` | `#e6e8ec` |
| `--text-dim` | `#9aa0a9` |
| `--text-faint` | `#6a7079` |

### Color — light

| Token | Hex |
|---|---|
| `--bg` | `#f6f5f1` |
| `--bg-elev` | `#ffffff` |
| `--bg-elev-2` | `#fbfaf6` |
| `--bg-hover` | `#f0eee8` |
| `--line` | `#e6e3db` |
| `--line-strong` | `#d4d0c4` |
| `--text` | `#1a1c20` |
| `--text-dim` | `#5b6068` |
| `--text-faint` | `#8a8f97` |

### Accent — teal (default)

| Token | Value |
|---|---|
| `--accent` | `#14b8a6` |
| `--accent-soft` | `rgba(20,184,166,.14)` |
| `--accent-line` | `rgba(20,184,166,.4)` |

Other accent options: indigo `#6366f1`, amber `#f59e0b`, violet `#a855f7`, lime `#84cc16`, rose `#f43f5e`. Each defines its own `hex / soft (14%) / line (40%)` triple — see `aperture-app.jsx` → `ACCENTS`.

### Status (theme-invariant — never reused for accent)

| Token | Hex | Soft |
|---|---|---|
| `--ok` | `#34d399` | `rgba(52,211,153,.14)` |
| `--warn` | `#f59e0b` | `rgba(245,158,11,.14)` |
| `--crit` | `#ef4444` | `rgba(239,68,68,.14)` |
| `--info` | `#60a5fa` | — |
| `--offline` | `#6b7280` | — |

### Typography

```
--font-sans: 'Geist', ui-sans-serif, system-ui, sans-serif
--font-mono: 'Geist Mono', ui-monospace, monospace
```

Use Geist for everything; **mono for all numerics, identifiers, addresses, paths, timestamps, sizes.**

| Use | Size | Weight | Tracking |
|---|---|---|---|
| Page title (H1) | 22px | 600 | -0.02em |
| Section head | 16px | 600 | -0.01em |
| Card title | 14px | 600 | — |
| Body | 13.5px | 400 | — |
| Small | 12px | 400 | — |
| Caption | 11px | 400 | — |
| MONO label (uppercase) | 10–11px | 400 | 0.08em |

Body line-height `1.45`. Mono labels are `text-transform: uppercase`.

### Spacing (4px base)

`4, 6, 8, 10, 12, 14, 18, 22, 28`. Cards `14–18px`; sidebar items `7px 10px`; modals `18px 20px`.

### Radius

| Token | px | Use |
|---|---|---|
| sm | 3 | Badges, kbd, segmented buttons |
| md | 4 | Default button/input |
| lg | 6 | Cards, modals, drill-in |
| pill | 999 | Status dots, avatars |

### Elevation

- Card hover (dark): `0 6px 24px -12px rgba(0,0,0,.35)`
- Card hover (light): `0 8px 24px -16px rgba(20,20,30,.18)`
- Modal: `0 24px 60px -20px rgba(0,0,0,.5)`
- Menu popover: `0 18px 40px -16px rgba(0,0,0,.4)`

### Motion

| Element | Duration | Easing |
|---|---|---|
| Card hover lift `translateY(-1px)` | 180ms | `cubic-bezier(.2,.7,.3,1)` |
| Button press `scale(.96)` | 120ms | default |
| Status pulse (critical only) | 1.8s | ease-out, infinite |
| Card entrance (staggered) | 420ms | `cubic-bezier(.2,.7,.3,1)`, 40ms delay per card |
| Drill-in slide from right | 260ms | `cubic-bezier(.2,.7,.3,1)` |
| Modal scale-in `.97 → 1` | 220ms | `cubic-bezier(.2,.7,.3,1)` |
| Menu fade-down | 140ms | ease-out |
| Skeleton shimmer | 1.4s | linear, infinite |
| Status rail width on card hover | 200ms | `cubic-bezier(.2,.7,.3,1)` (2px → 3px) |
| Card accent halo opacity | 250ms | ease |

**Wrap all motion in `@media (prefers-reduced-motion: no-preference)`.** Sparklines never animate (data integrity).

### Glass surfaces (`backdrop-filter`)

| Surface | Backdrop |
|---|---|
| Topbar | `blur(14px) saturate(1.4)` over `color-mix(in srgb, var(--bg) 78%, transparent)` |
| Modal backdrop | `blur(6px) saturate(1.2)` over `rgba(0,0,0,.55)` |
| Drill-in panel | `blur(20px) saturate(1.3)` over `color-mix(in srgb, var(--bg) 92%, transparent)` |
| Menu popover | `blur(14px) saturate(1.2)` |

**Card accent halo (hover):** `radial-gradient(120% 60% at 0% 0%, var(--accent-soft), transparent 60%)`, fades from 0 → 0.5 opacity over 250ms.

---

## Screens

### 01 · Application shell

CSS grid `auto 1fr` — sidebar then main column. `min-height: 100vh`.

**Sidebar** — `220px` (labeled), sticky, `--bg-elev` background, 1px right border (`--line`), padding `16px 12px`, flex column.

- **Brand row** (top, 16px padding-bottom): 22px square mark — `--text` bg, accent SVG glyph — then "Aperture" (14px/600/-0.01em) stacked over version chip ("v0.9.0-dev", 10px mono, `--text-faint`).
- **Workspace section** — small uppercase mono label "WORKSPACE" (10px / `--text-faint` / 0.12em tracking), then nav items: **Dashboard, Hosts, Containers, Stacks, Storage, Network**.
- **Observe section** — label "OBSERVE", then **Logs, Shell, Automation, Alerts** (Alerts has badge = open alert count).
- **Bottom** (margin-top: auto) — **Settings**.

Nav item: `7px 10px`, gap `10px`, 16px icon + label, `--text-dim`. Hover: `--bg-hover` background, `--text` color. Active: `--accent-soft` background, `--accent` color, and a `2px` accent rail on the left edge (`left: -12px; top: 8px; bottom: 8px`).

**Topbar** — sticky, `10px 28px`, 1px bottom border (`--line` at 70%), **glass** as above.

- Left: search input — `360px / max-width 50%`, `var(--bg-elev)` background, `1px var(--line)` border, padding `6px 10px`, gap `8px`. Icon `var(--text-faint)`, placeholder "Search hosts, containers, stacks…". Trailing kbd chip `⌘K` (11px mono, `var(--bg-elev-2)` background, 1px border, radius 3). Focus-within: border `var(--accent-line)`.
- Right: sync indicator ("● synced 4s ago", 11px mono, `--text-faint`, 6px ok dot), refresh button, theme toggle, user avatar (28px accent-soft circle).

Buttons: `28×28`, `1px var(--line)`, radius 4, hover `--bg-hover`.

### 02 · Dashboard tab

Padding `22px 28px 60px`, max-width `1600px`, centered. Three blocks vertically: **page-header → filter-bar → host grid**.

**Page-header (counts strip)** — flex row, baseline aligned.

- Left: H1 "Dashboard" (22px/600/-0.02em) + sub "4 hosts · last sync 14:22:08" (12px mono, `--text-faint`).
- Right: stat strip — `--bg-elev` background, `1px var(--line)`, radius 4, padding `8px 18px`, gap `24px`. Six stats stacked label-over-value: **Healthy** (count, `--ok`), **Warning** (`--warn`), **Critical** (`--crit`), **Containers** (`running/total`, `--text`), **Unhealthy** (`--crit` if >0 else `--text-dim`), **Open alerts** (`--warn` if >0). Label = 10px mono uppercase / `--text-faint` / 0.08em tracking. Value = 18px/500/-0.02em mono.

**Filter-bar** — flex row, space-between.

- Left: pill-set of tabs in a `--bg-elev` container with 1px border, radius 4, padding 3px. Tabs: `all · prod · docker · linux · edge · mergerfs · alerts` (alerts tab leads with warn icon). Tab padding `5px 12px`, 12px text, radius 3. Active tab: `--bg-hover` background, `--text` color.
- Right: segmented control (Rich / Tile / List) + primary button "+ Add host".

Primary button has gradient + glow:
```
background-image: linear-gradient(180deg, color-mix(in srgb, var(--accent) 100%, white 8%), var(--accent));
box-shadow: 0 1px 0 rgba(255,255,255,.18) inset, 0 4px 16px -8px var(--accent);
```

**Host grid** — CSS Grid:

| Layout | Template |
|---|---|
| Rich | `repeat(auto-fit, minmax(560px, 1fr))`, gap 14px |
| Tile | `repeat(auto-fit, minmax(320px, 1fr))`, gap 14px |
| List | `1fr`, gap 6px |

Trailing **Add Widget tile** (dashed `1.5px var(--line-strong)`, radius 6, `+` + "Add host" + "or pin a widget" mono sub). Hover: `--accent` border + `--accent-soft` background + `--accent` color.

### 03 · Host card — Rich variant

This is the heart of the dashboard. See `RichCard` in `aperture-host-card.jsx`.

**Container.** `--bg-elev`, `1px var(--line)`, radius 6, overflow hidden. Hover: border `--line-strong`, `translateY(-1px)` lift, accent halo (top-left radial gradient).

**Status rail** (absolute left, 2px wide, full height). Color = status (`--ok` / `--warn` / `--crit`), gradient masked top/bottom for subtle fade. Grows to 3px on hover.

**Header row** (padding `14px 16px 6px 18px`, flex space-between).

- Left identity: 32px kind chip (`--bg-elev-2` bg, 1px border, radius 4, 14px icon — docker/linux/edge) → name (16px/600/-0.01em) + status indicator → mono sub (`atlas.lan · 10.0.4.12`, 11px / `--text-faint`).
- Right: tag chips (`prod`, `docker` — 11px mono, `--bg-elev-2` bg, 1px border, radius 3, height 18, padding `0 6px`) + 24×24 "more" icon button (opens action menu).

**Meta row** (padding `0 16px 12px 18px`). Single line, 11px mono, `--text-faint`: OS · CPU model · cores · RAM · docker version · uptime. Separated by `·` dots (opacity 0.5).

**Body** — CSS Grid `1fr 1fr` with 1px top border. Left = **metric stack**, right = **side panels** (1px left border).

**Metric row** (`MetricRow` component) — grid `18px 38px 1fr auto` / 2 rows, padding `8px 16px 8px 14px`. Icon spans both rows. 10px mono label spans both rows (`CPU`, `MEM`, `NET`, `DISK`). Middle column = `Sparkline` (140×26) or `Meter` (4px height). Right column = value (13px/500 mono) over mono sub (10px / `--text-faint`). Bottom border between rows.

Sparkline colors:
- CPU: `--accent` (default), `--warn` at ≥70%, `--crit` at ≥85%
- MEM: `--accent` (default), `--warn` at ≥80%, `--crit` at ≥90%
- NET: `--info`
- DISK: `Meter` (linear bar), color thresholds same as CPU/MEM

Net value format: `↓18.4 MB/s ↑4.2 MB/s` (arrows in `--text-dim`, values in `--text`). Sub-MB shown as `KB/s`.

**Side panel — Containers** (Docker hosts).
Padding `12px 16px`. Header: docker icon + "Containers" (11px mono uppercase / 0.08em) + right-aligned total ("25 total", `--text-dim`). Body: 3-col grid — `Running` (`--ok`) / `Stopped` (`--text-faint`) / `Unhealthy` (`--crit` if >0). Each: 22px/500/-0.02em mono number over 11px label.

**Side panel — Services** (bare-metal hosts).
Linux icon + "Services" + count. Body: vertical list, 6px dot (ok/offline color) + service name (mono 12px) + state (mono 11px / `--text-faint`), right-aligned.

**Side panel — Top by CPU** (Docker hosts, second slot).
CPU icon + "Top by CPU". 4-row grid (`1fr auto auto`): name (mono 12px) + `cpu%` + `mem GB`, right-aligned, mono.

**Side panel — Sensors** (bare-metal, no top procs).
Temp icon + "Sensors". Two stats: CPU °C, DISK °C — 17px/500 mono over 10px mono label.

**Alert footer.** When `host.alerts.length > 0`: full-width strip per alert with top border, `8px 16px 8px 18px`, warn icon + text. Background = `--warn-soft` or `--crit-soft`, text color matches.

### 04 · Host card — Tile variant

Compact 2-column card. Header (kind chip + name + role sub + status), 2×2 grid of metrics (CPU, MEM, NET ↓, TEMP), footer mono ("23/25 ctrs · up 42d 7h"). Each grid cell: label (10px mono / `--text-faint`) + value (16px/500 mono) + sparkline (140×22).

### 05 · Host card — List variant

Single-row layout. Grid columns: `220px 1fr 200px 200px 180px 160px 100px 32px`. Status rail (left 2px), identity (kind + name + status), address (mono), CPU+spark+%, MEM+spark+%, NET+spark+rate, container summary, uptime, more button.

### 06 · Host drill-in (right slide-over)

Triggered by clicking any card. Slides from right over a blurred backdrop (`rgba(0,0,0,.55)` + `blur(8px)`).

- Width `min(1080px, 95vw)`, full height, `--bg` background (glass — see tokens).
- **Sticky header**: close (X) + kind chip + name (20px/600) + status + tags · mono sub (address · IP · OS). Right side: action buttons — `Restart`, `SSH`, `Update`, `Stop` (Stop is `--warn-soft` background + `--warn` text). Buttons: `--bg-elev` bg, 1px border, radius 4, padding `6px 12px`.
- **Tabs row**: Overview / Containers / Stacks / Logs / Shell / Schedules / Settings. Active = bottom 2px accent line.
- **Big metric grid** (4 columns): CPU / Memory / Network ↓ / Temperature — large numeric (26px/500 mono / -0.02em), label-value-sub, 220×36 sparkline at thick stroke (2px).
- **Cols grid (1fr 1fr)** of panels:
  - **Storage** — list of volumes with name + `used / total unit · °C` + Meter.
  - **Containers** — 3-stat row (running / stopped / unhealthy) + divider + top-by-CPU list.
  - **Recent events** — `48px 1fr` rows: timestamp (mono / `--text-faint`) + event text (color by level).

Press **Esc** to close.

### 07 · Card action menu (popover)

Per-card "⋯" opens a glass popover (absolute, `min-width 200px`, radius 6). Items: `Pin to dashboard`, `Edit widgets…`, `Restart host`, `Open shell`, `Remove host` (danger — `--crit` text, `--crit-soft` hover).

### 08 · Add Host modal

Centered modal, `width 620px`, glass backdrop. Two-step:

**Step 1 — choose connection method.** Three radio cards stacked:
1. **Install agent** (recommended) — Edge icon. Shows install command in code block: `curl -fsSL https://aperture.lan/install.sh | sudo APERTURE_TOKEN=apr_h0st_… sh` with copy button.
2. **Docker API** — Docker icon. Form: alias, endpoint `tcp://host:2376`, CA cert path.
3. **SSH** — Shell icon. Form: alias, `user@host:22`, SSH key path.

Active method: `--accent-line` border + `--accent-soft` background + accent radio dot.

**Step 2 — verify.** Stacked verify rows with status circles:
- ✓ Endpoint reachable
- ✓ Authentication
- ✓ Engine version (`docker 27.5.1`)
- ◌ Initial metrics sync (`0.4s remaining`) — spinning border ring

Footer: `Cancel` (ghost) + `Continue →` / `Test connection` / `Done` (primary).

### 09 · Alerts tab

Page-header (counts) → list of alert cards:
- Left status icon chip (warn or crit) — 28px square, status-soft bg.
- Center: alert text (13.5px/500) + meta row (host kind icon + host name · "open 14m", mono).
- Right: `Silence`, `Acknowledge` mini buttons.
- Left border: 3px solid `--warn` or `--crit`.

Click row → opens that host's drill-in.

### 10 · Settings tab

Page-header → 2-column auto-fit grid of **SettingsGroup** cards. Each card:
- Head: 11px uppercase label / 0.08em / `--text-dim` / 1px bottom border.
- Rows: `1fr auto auto` — label (`--text`) + value (mono 12px / `--text-dim`) + optional `Edit` button.

Groups:
- **General** — Workspace name, Timezone, Refresh interval, Theme.
- **Security** — Auth provider, Session length, Audit log.
- **Notifications** — Email, ntfy URL, Webhook.
- **Backups** — Config export schedule, Last export.

Add **Appearance** group too: Theme (dark/light/system), Accent (6 swatches), Density.

### 11 · Empty / Loading / Error states

- **Empty** (`EmptyBlock`) — dashed border card, 48px icon chip, "No hosts yet" title + description + primary action button ("+ Add your first host").
- **Loading** — 4 `SkeletonCard` placeholders with shimmer (`linear-gradient` background-position animation).
- **Error** (`ErrorBlock`) — `--crit` border + `--crit-soft` background, warn icon, "Couldn't reach the orchestrator", retry button.

The Tweaks panel has a **Preview state** selector (live / loading / empty / error) — used during design only; ship live as the default and trigger the others from real load/error states.

---

## Interactions & behavior

| Interaction | Behavior |
|---|---|
| Click host card body | Open drill-in slide-over |
| Click card "⋯" | Open action menu (stopPropagation — doesn't open drill-in) |
| Click action menu item | Run action, close menu (current prototype logs to console) |
| Esc | Close drill-in OR modal OR menu (whichever is topmost) |
| Click outside modal / drill-in | Close it |
| Theme toggle (topbar) | Flip `<html data-theme>`; persist in localStorage |
| Filter tab | Filter hosts; `alerts` shows only hosts with alerts |
| Layout segmented | Switch grid template; persist |
| `+ Add host` | Open AddHostModal |
| `+ Add widget tile` | Open AddHostModal (in v1; later: widget picker too) |
| Refresh button | Force-poll metrics |
| Search (⌘K) | Command palette (out of scope for v1; reserve the slot) |

**Customization for v1 (per user request).** Stub out: pin/unpin host, reorder cards (drag handle on card hover), edit which widgets show per card. These can be no-ops at first, but the affordances should ship — the menu items, the dashed Add tile, and a draggable cursor on cards' headers. Persist order in `user_dashboard_layout` on the backend.

**Reduced motion.** All decorative motion is opt-out per OS preference. Only state-meaning motion (status pulse on critical) remains essential.

---

## SvelteKit component map

Suggested file layout under `src/lib/components/`:

```
shell/
  Sidebar.svelte          ← labeled nav, brand, sections
  Topbar.svelte           ← search + actions, glass
  AppShell.svelte         ← grid layout + theme toggle wiring

dashboard/
  PageHeader.svelte       ← title + counts strip
  FilterBar.svelte        ← tabs + segmented + add-host button
  HostGrid.svelte         ← grid container, picks variant
  HostCard.svelte         ← variant switch (rich/tile/list)
  RichCard.svelte
  TileCard.svelte
  CompactRow.svelte
  AddWidgetTile.svelte
  CardMenu.svelte         ← popover w/ pin/edit/restart/ssh/remove

host/
  DrillIn.svelte          ← slide-over panel
  BigMetric.svelte
  StoragePanel.svelte
  ContainersPanel.svelte
  EventsPanel.svelte

addhost/
  AddHostModal.svelte
  MethodRadio.svelte
  VerifyRow.svelte

alerts/
  AlertsView.svelte
  AlertCard.svelte

settings/
  SettingsView.svelte
  SettingsGroup.svelte
  AppearanceGroup.svelte

primitives/
  Modal.svelte
  Button.svelte           ← primary / ghost / mini / icon variants
  Field.svelte
  Tag.svelte
  Kbd.svelte
  StatusIndicator.svelte  ← dot / bar / ring / pill (just dot for ship)
  Sparkline.svelte        ← area / line / bar (just area for ship)
  Meter.svelte
  HostKindIcon.svelte
  SkeletonCard.svelte
  EmptyBlock.svelte
  ErrorBlock.svelte
  Icon.svelte             ← single component reading icon name prop
```

`icons.ts` — port the `I.*` SVG paths from `aperture-ui.jsx`. Or use `lucide-svelte` (already close in stroke style at 1.5) and remap names where they line up. Keep stroke at 1.5, line caps/joins round.

## Routes

```
/                       redirect → /dashboard
/dashboard              Dashboard tab
/hosts                  Hosts list (extends dashboard data)
/hosts/[id]             Host detail (full page; drill-in is the modal version)
/containers             Containers across all hosts
/stacks                 Compose stacks
/storage                Pools, mergerfs branches, parity
/network                Networks + tunnels
/logs                   Aggregated logs / tail
/shell                  Web shell
/automation             Schedules / playbooks
/alerts                 Alerts list
/settings               General / Security / Notifications / Backups / Appearance
```

## State & data flow

**Stores** (`src/lib/stores/`):
- `theme.ts` — `'dark' | 'light' | 'system'`, persisted, applies `<html data-theme>`.
- `accent.ts` — accent key, applies CSS vars on document root.
- `hosts.ts` — derived from SSE feed; map of host id → host.
- `dashboardLayout.ts` — pinned widgets, card order, per-card widget visibility. Persisted via API.
- `commandPalette.ts` — open state, query (for ⌘K reserve).

**Backend contract (Go).** Recommendation, not prescriptive:

```
GET  /api/hosts                       → Host[]              (list + last-known metrics)
GET  /api/hosts/:id                   → HostDetail
POST /api/hosts                       → { method, config } enrollment
DEL  /api/hosts/:id                   → remove
POST /api/hosts/:id/actions/restart   → 202
POST /api/hosts/:id/actions/ssh       → returns shell session id
GET  /api/alerts?state=open           → Alert[]
POST /api/alerts/:id/silence
POST /api/alerts/:id/ack

SSE  /api/stream/metrics              → { hostId, cpu, mem, net, temp, ts }
SSE  /api/stream/events               → container/system/alert events
```

Metrics SSE pushes per-host samples; client maintains a ring buffer (last 60 samples) for sparklines. **Sparklines never re-animate on update** — just append + shift.

**Type sketch** (mirror in `src/lib/types.ts`):

```ts
type HostKind = 'docker' | 'linux' | 'edge';
type Status = 'ok' | 'warn' | 'crit' | 'offline';
interface Host {
  id: string; name: string; role: string;
  address: string; ip: string; tags: string[]; kind: HostKind;
  os: string; cpuModel: string; cores: number;
  memTotalGB: number; docker?: string; uptime: string;
  status: Status;
  cpu: number; cpuSeries: number[]; tempCpu: number;
  mem: number; memUsedGB: number; memSeries: number[];
  netIn: number; netOut: number; netInSeries: number[]; netOutSeries: number[];
  tempDisk: number | null;
  disks: Disk[];
  containers?: { running: number; stopped: number; unhealthy: number; total: number };
  services?: { name: string; state: 'active' | 'inactive' | 'failed' }[];
  topCpu?: Proc[]; topMem?: Proc[];
  alerts: { level: 'warn' | 'crit' | 'info'; text: string }[];
  events: { t: string; kind: string; level: string; text: string }[];
}
```

## Assets

- **Fonts:** Geist + Geist Mono. Self-host via `@fontsource/geist-sans` and `@fontsource/geist-mono`.
- **Icons:** hand-rolled SVG line set in `aperture-ui.jsx` (`I.*`). 1.5px stroke, 24px viewbox, round caps/joins. Port to a Svelte `<Icon name>` component or replace with `lucide-svelte` equivalents (Server → `hosts`, Container → `containers`, etc.) restyled to 1.5 stroke.
- **No raster assets.** Brand mark is inline SVG (small triangle inside a circle on a `--text`-colored square).

## Files in this bundle

| File | Purpose |
|---|---|
| `Aperture Dashboard.html` | Entry; loads React+Babel, defines tokens, animation keyframes, glass surfaces. |
| `aperture-data.jsx` | Seeded mock data: 4 hosts (atlas, vega, lyra, orion-edge) covering every state. Use as **fixture data** when wiring backend. |
| `aperture-ui.jsx` | UI primitives: icon set (`I.*`), `Sparkline`, `StatusIndicator`, `Meter`, `Tag`, `Kbd`, `HostKindIcon`. |
| `aperture-host-card.jsx` | `HostCard` + `RichCard` / `TileCard` / `CompactRow`, `MetricRow`, `ContainerSummary`, `ServiceSummary`, `TopProcesses`, `TileMetric`. |
| `aperture-screens.jsx` | `AddHostModal`, `AlertsView`, `SettingsView`, `StubView`, `EmptyBlock`, `SkeletonCard`, `ErrorBlock`, `CardMenu`, `AddWidgetTile`, `Modal`, `Field`, `VerifyRow`. |
| `aperture-app.jsx` | `App` (root), `Sidebar`, `TopBar`, `PageHeader`, `FilterBar`, `DrillIn`, `BigMetric`, tweak wiring. |
| `tweaks-panel.jsx` | Design-time only — **do not port**. |

## Implementation order (suggested)

1. **Tokens** — drop `tokens.css`, wire theme toggle (works end-to-end before anything else).
2. **Primitives** — `Sparkline`, `StatusIndicator`, `Meter`, `Tag`, `Kbd`, `Icon`. These unblock everything.
3. **Shell** — `Sidebar` + `Topbar` + route slot. Hardcode routes; stub pages.
4. **Dashboard with mock data** — wire `aperture-data.jsx`-shaped fixtures into `hosts.ts` store. Implement `RichCard` first; Tile and List can follow.
5. **DrillIn** — slide-over modal.
6. **AddHostModal** — wire to backend enroll endpoints.
7. **Backend SSE** — replace fixture with live metrics; ring-buffer sparklines.
8. **Alerts + Settings tabs.**
9. **Customization** — pin/reorder/edit widget persistence.

## Notes for Claude Code

- Open `Aperture Dashboard.html` in a browser to see the live prototype with all three layouts, both themes, all states, and the drill-in. The Tweaks panel (bottom-right) lets you toggle every variant.
- When in doubt about a value, **read it from the JSX** — every padding, gap, and color shown in the design is in the source.
- Match motion timings precisely — they're tuned. Skip motion only behind `prefers-reduced-motion`.
- The prototype uses `--bg-elev` for cards and `--bg` for the page. Don't invert.
- Mono font for **all** numbers, including `42d 7h`, `64 GB`, `10.0.4.12`, `18.4 MB/s`, etc.
- Status colors **never** mean "selected" — only health. Use `--accent` for selection / focus / brand.
- Don't ship the Tweaks panel.
