# Aperture Project Review and Plan

**Repository reviewed:** `aperture-master.zip`  
**Review type:** static project/codebase review  
**Date:** 2026-05-12

---

## Verification notes

This review is based on the uploaded repository contents and the Markdown context files in the repo.

I could not fully validate runtime behavior in this environment:

- `go test ./...` could not run because `go.mod` requires Go `1.25.0`, while the local toolchain is Go `1.23.2`. Evidence: `go.mod:3` says `go 1.25.0`.
- `npm run check` could not run because frontend dependencies were not installed. The script exists in `web/package.json:10-12`, but the command failed with `svelte-kit: not found`.
- I found no `*_test.go`, `*.spec.*`, or `*.test.*` files in the archive.

So treat this as a **design, architecture, and code-structure review**, not as a passing-build certification.

---

## Executive summary

Aperture has a real project identity. The README describes it as:

> “A self-hosted homelab command center. One interface for system metrics, container management, alerts, and remote host monitoring — designed to replace Beszel, Portainer/Dockge, and Dozzle with a single coherent tool.”  
> Evidence: `README.MD:3`

That is a strong premise. The project should not compete by becoming a feature-by-feature clone of every homelab management tool. Its better niche is:

> **A single operational surface for seeing what is wrong, understanding why, acting on it, and verifying the result.**

The biggest current risk is that the project is growing faster than its foundation. The repo already has:

| File | Current size |
|---|---:|
| `internal/api/api.go` | 1,659 lines |
| `internal/hub/agentws.go` | 989 lines |
| `internal/store/store.go` | 1,055 lines |
| `web/src/routes/alerts/+page.svelte` | 896 lines |
| `web/src/routes/hosts/[id]/compose/+page.svelte` | 865 lines |
| `web/src/routes/hosts/[id]/containers/+page.svelte` | 853 lines |
| `web/src/routes/hosts/[id]/+page.svelte` | 737 lines |

That is normal for an early fast-moving project, but it is also the point where continuing to add features will start compounding pain.

**Bottom line:** keep going, but do a stabilization and structure pass before the large UI overhaul becomes a second layer of complexity.

---

## Recommended order of work

Do **not** start with a pure visual UI rewrite.

Do this instead:

```text
1. Core stabilization pass
2. Frontend architecture/component extraction
3. Visual UI overhaul
4. App Store / deployment system
5. Efficiency and data-size rework
```

Your instinct to delay the efficiency rework is reasonable. The project has more urgent structural issues than metric storage efficiency right now.

---

# Project-level review

## What is working well

### 1. The product philosophy is good

The docs define the right UX principle:

> “present a **glanceable summary view** by default, with the ability to **drill into granular detail on demand**.”  
> Evidence: `overview.md:25`

The docs also say:

> “**Every** major feature in this codebase — current and future — should implement both layers.”  
> Evidence: `overview.md:32`

That is exactly the right design philosophy for a homelab command center. A good Aperture screen should answer two questions:

```text
1. Is something wrong?
2. What can I do about it?
```

Do not lose that. It is the main thing separating Aperture from a generic Docker dashboard.

### 2. Hub + agent architecture is the right base

The current architecture is directionally correct. The repo already separates host access through provider interfaces:

- `DockerProvider` in `internal/hub/hub.go:33-61`
- `ComposeProvider` in `internal/hub/hub.go:63-72`

That is the correct seam for supporting local and remote hosts without making the API care where the Docker engine lives.

### 3. Compose-first is the right feature direction

Compose management is more valuable for a homelabber than raw single-container creation. A homelab app usually lives as a stack, not as one manually created container.

The current Compose surface already has the right shape:

- discover existing stacks
- inspect services
- view/edit Compose YAML
- save + deploy
- view logs
- create new stacks
- operate on both local and remote hosts

That is worth building around.

### 4. The alerting direction is more mature than expected for this stage

The alerting code has a reasonable model: persistent alert rules/events plus transient sustained-breach state. The file header explains the split:

> “Persistent: alert_rules ... alert_events ... Transient: a per-(rule, host) ‘first observed breach’ timestamp...”  
> Evidence: `internal/alerts/alerts.go:10-20`

That is practical. It avoids a huge alerting framework while still supporting real state transitions.

---

## Main risk

### Feature growth is outrunning structure

The warning signs are:

- large monolithic route files in the frontend
- one very large API file
- one very large store file
- duplicated hub/agent protocol structs
- docs already drifting from implementation
- limited/no tests around risky seams
- destructive Docker/Compose actions without built-in auth

None of that means the project is bad. It means the project is at the normal alpha-stage fork:

```text
Option A: keep adding features until everything is harder to change
Option B: pause, harden the seams, then resume feature work faster
```

Choose Option B now.

---

# UI overhaul recommendation

## Do a UI architecture overhaul before a visual overhaul

The next UI pass should not be “make the current pages prettier.” It should be:

```text
Extract reusable layout, action, table, modal, status, and detail patterns.
Then redesign visually on top of those patterns.
```

Current Svelte pages are still too page-monolithic. That is fine for alpha velocity, but not a good base for an app-store/deployment system.

### Recommended frontend structure

```text
web/src/lib/components/
  AppShell.svelte
  PageHeader.svelte
  HostSubnav.svelte
  StatusPill.svelte
  MetricCard.svelte
  ResourceBar.svelte
  DataTable.svelte
  Modal.svelte
  ConfirmDialog.svelte
  Toast.svelte
  LogsPanel.svelte
  TerminalModal.svelte
  EntityInspectPanel.svelte
  DangerButton.svelte
```

Domain-specific components:

```text
web/src/lib/features/hosts/
web/src/lib/features/containers/
web/src/lib/features/compose/
web/src/lib/features/alerts/
web/src/lib/features/images/
web/src/lib/features/volumes/
web/src/lib/features/networks/
web/src/lib/features/deployments/
```

A route file should become orchestration, not the whole feature:

```svelte
<script lang="ts">
  // load data
  // call actions
  // hold page-level state only
</script>

<PageHeader />
<HostSubnav />
<ComposeStackList />
<ComposeEditorModal />
```

## Build the UI around operations, not Docker object taxonomy

The current structure appears to be mainly object-based:

```text
Hosts
Alerts
Settings
Host → Overview / Containers / Compose / Networks / Volumes / Images
```

That is logical, but the stronger command-center model is workflow-based:

```text
Overview
Needs Attention
Hosts
Stacks
Containers
Storage
Images
Networks
Alerts
Deployments
Settings
```

A user should not have to know whether a problem is a container issue, Compose issue, image issue, volume issue, or host issue before Aperture helps them find it.

## Make destructive actions first-class UI elements

The app manages real infrastructure. Destructive operations should not be treated like ordinary buttons.

Use a reusable confirmation component for:

```text
- remove container
- recreate container
- remove image
- remove volume
- compose down
- compose down --volumes
- stack deploy
- stack delete
- terminal open
- app-store deploy
```

A confirmation should show:

```text
- host name
- object name
- action to be performed
- consequence
- whether data may be deleted
- optional typed confirmation for high-risk actions
```

This matters more than visual polish.

---

# Specific findings and recommendations

## P0 — Add basic auth before expanding destructive features

The technical docs currently say:

> “**Auth absent.** Single-user homelab assumption.”  
> Evidence: `technical.md:382`

That may have been acceptable early, but Aperture now exposes high-impact operations:

- container create/start/stop/remove
- image pull/remove
- network create/remove/connect/disconnect
- Compose file write/deploy
- terminal access
- agent token management

The API docs list container create/remove and other write actions under `/api`. Evidence: `README.MD:590-597`. Agent token endpoints are also exposed. Evidence: `README.MD:621-625`.

### Recommendation

Add a minimal auth layer before the App Store/deployment system:

```text
- local admin password or setup token
- session cookie or bearer token
- auth middleware around every non-health endpoint
- explicit unauthenticated allowlist: /api/health only
- optional reverse-proxy trusted-header mode later
- audit log for destructive actions
```

Even if the app is normally behind Cloudflare Access or a VPN, Aperture itself should not assume the outer layer is perfect.

---

## P0 — Terminal access appears to be agent-only from the API path

The API terminal handler defaults to `/bin/sh` and then calls `s.agentHandler.StartTerminal(...)`:

```go
cmd := r.URL.Query().Get("cmd")
if cmd == "" {
    cmd = "/bin/sh"
}

reqID, outCh, err := s.agentHandler.StartTerminal(r.Context(), hostID, cid, cmd)
```

Evidence: `internal/api/api.go:592-597`

But `StartTerminal` only succeeds if the host is present in the connected agent sessions map:

```go
sess, ok := ah.sessions[hostID]
if !ok {
    return "", nil, fmt.Errorf("agent not connected")
}
```

Evidence: `internal/hub/agentws.go:879-885`

That means the terminal likely works only for remote agent hosts, not for the local hub host, even though the frontend appears to expose terminal access generically.

### Recommendation

Do not route terminal exclusively through `AgentHandler`.

Add a provider seam:

```go
type TerminalProvider interface {
    StartTerminal(ctx context.Context, containerID, cmd string) (reqID string, output <-chan []byte, err error)
    SendTerminalData(ctx context.Context, reqID string, data []byte) error
    ResizeTerminal(ctx context.Context, reqID string, cols, rows uint) error
    CloseTerminal(ctx context.Context, reqID string) error
}
```

Then implement:

```text
- local Docker terminal provider
- remote agent terminal provider
```

---

## P0/P1 — Terminal WebSocket frame encoding looks wrong

In `StartTerminal`, the code manually marshals a request to bytes and then passes the `[]byte` to `wsjson.Write`:

```go
b, _ := json.Marshal(req)
if err := wsjson.Write(ctx, sess.conn, b); err != nil {
```

Evidence: `internal/hub/agentws.go:904-905`

The same pattern appears in terminal data/resize/close paths. Evidence: `internal/hub/agentws.go:944-988`.

That is suspicious because `wsjson.Write` JSON-encodes the value passed to it. Passing `[]byte` can encode the bytes as a JSON string/base64-like payload instead of the intended JSON object. Elsewhere, the code correctly writes the struct directly. Evidence: `internal/hub/agentws.go:420`.

### Recommendation

Change terminal frame writes to pass the struct:

```go
if err := wsjson.Write(ctx, sess.conn, req); err != nil {
    ...
}
```

Then add tests for frame encoding/decoding.

---

## P1 — Hub/agent protocol types should move to a shared package

The agent file explicitly says:

> “wire frame types (must match hub/agentws.go)”  
> Evidence: `cmd/agent/main.go:36`

That is a maintenance trap. Every new protocol action now requires manually keeping two copies synchronized.

### Recommendation

Create:

```text
internal/agentproto/
  frames.go
  docker.go
  compose.go
  terminal.go
```

Both `cmd/agent` and `internal/hub` can import this because it is still internal to the module.

---

## P1 — `samplesIn` can leak goroutines on agent reconnects

`AgentHandler.ServeHTTP` creates a `samplesIn` channel for connected agents. Evidence: `internal/hub/agentws.go:306`.

`Hub.samplesIn` creates a goroutine that loops forever until the returned channel closes:

```go
out := make(chan types.MetricSample, 16)
go func() {
    for s := range out {
        ...
    }
}()
return out
```

Evidence: `internal/hub/hub.go:237-249`

For local collectors, this is acceptable. For remote agents that disconnect/reconnect, it may leave one goroutine per old connection unless the channel is closed, and the returned channel is send-only from the caller perspective.

### Recommendation

Make the helper context-aware:

```go
func (h *Hub) samplesIn(ctx context.Context, hostID string) chan<- types.MetricSample
```

Then exit the goroutine on `ctx.Done()`.

---

## P1 — API versioning is promised but not implemented

The overview says:

> “every WebUI action goes through a documented REST API, versioned `/v1` from day one.”  
> Evidence: `overview.md:240`

The actual router mounts routes under `/api`:

```go
r.Route("/api", func(r chi.Router) {
```

Evidence: `internal/api/api.go:69`

The README also says:

> “All endpoints are under `/api`.”  
> Evidence: `README.MD:565`

### Recommendation

Move to:

```text
/api/v1/...
```

Keep `/api/...` as a temporary alpha compatibility alias if desired.

---

## P1 — Compose path writes need boundaries

The API accepts arbitrary `working_dir` for Compose write/create flows. Evidence: `internal/api/api.go:1510` and `internal/api/api.go:1600`.

The Compose writer creates the directory and writes `compose.yml` if no compose file exists:

```go
if err := os.MkdirAll(workingDir, 0755); err != nil {
    ...
}
path := FindComposeFile(workingDir)
if path == "" {
    path = filepath.Join(workingDir, "compose.yml")
}
return os.WriteFile(path, []byte(content), 0644)
```

Evidence: `internal/compose/compose.go:135-144`

That is powerful, but it needs guardrails before an App Store/deployment system starts writing stacks.

### Recommendation

Add host-level allowed Compose roots:

```yaml
compose_roots:
  - /opt/docker
  - /srv/compose
```

Then enforce:

```text
- clean path
- reject empty path
- reject relative path unless explicitly supported
- reject paths outside allowed roots
- consider resolving symlinks before validation
- reject obvious system paths like /, /etc, /usr, /var/lib/docker unless explicitly unsafe-enabled
```

---

## P1 — Compose errors are hidden in key paths

`DiscoverStacks` treats any `docker compose ls` error as an empty stack list:

```go
if err != nil {
    // ls may exit non-zero when there are no stacks; treat as empty.
    return nil, nil
}
```

Evidence: `internal/compose/compose.go:41-45`

`GetStack` also ignores errors:

```go
stacks, _ := l.DiscoverStacks(ctx)
...
psOut, _ := l.run(ctx, base.WorkingDir, psArgs...)
svcs, _ := ParsePS(psOut)
```

Evidence: `internal/compose/compose.go:51-70`

This can make the UI show “empty” when Compose is actually broken.

### Recommendation

Differentiate these states:

```text
- no stacks found
- Docker Compose not installed
- Docker daemon unavailable
- permission denied
- invalid JSON from compose
- stack exists but ps failed
```

The UI should show a broken/error state, not a clean empty state.

---

## P1 — `createCompose` may call Compose with an empty project name

The create flow calls:

```go
cp.StackAction(r.Context(), "", body.WorkingDir, "up", "", "--remove-orphans")
```

Evidence: `internal/api/api.go:1615`

But `StackAction` always begins with:

```go
args := []string{"--project-name", project}
```

Evidence: `internal/compose/compose.go:85-87`

If `project` is empty, this may produce a bad Compose command.

### Recommendation

Only add `--project-name` when non-empty:

```go
args := []string{}
if project != "" {
    args = append(args, "--project-name", project)
}
```

---

## P1 — Add tests before the next feature wave

The repo currently has no test files in the archive. Add tests around stable core behavior before adding the App Store/deployment system.

Priority test targets:

```text
internal/compose:
  - ParseLS
  - ParsePS
  - FindComposeFile
  - StackAction argument construction
  - WriteFile path validation after allowlist support

internal/dockerctl:
  - stripLogHeaders
  - create config generation
  - resource update handling

internal/hub:
  - DeriveHostID
  - samplesIn lifecycle
  - provider dispatch local vs agent

internal/hub or internal/agentproto:
  - agent frame encode/decode
  - terminal request/data/resize/close frames

internal/store:
  - schema migration behavior
  - alert rule/event CRUD
  - metrics range/downsampling behavior

internal/alerts:
  - sustained breach timing
  - resolve behavior
  - all-host vs host-specific rules
```

---

## P2 — Docs are useful but drifting

Examples of drift:

- README says current version is `v0.3.0-alpha.3`. Evidence: `README.MD:5`.
- `overview.md` current-state heading says `v0.3.0-alpha.1`. Evidence: `overview.md:7`.
- `technical.md` still says “No SSE/WebSocket yet.” Evidence: `technical.md:381`.
- `export_state.md` says there is a completed Web Terminal pipeline using WebSocket multiplexing. Evidence: `export_state.md:9-13`.

### Recommendation

Keep the docs, but give them clear roles:

```text
README.MD       User-facing install/use/status doc
technical.md    Current implementation truth
architecture.md Stable architecture decisions
roadmap.md      Planned future work
changelog.md    Historical record
export_state.md Temporary handoff/session state only
```

Do not let temporary session handoff docs become the source of truth.

---

# Backend refactor plan

## Split `internal/api/api.go`

Suggested layout:

```text
internal/api/
  server.go
  router.go
  responses.go
  middleware.go
  system.go
  hosts.go
  metrics.go
  containers.go
  compose.go
  networks.go
  volumes.go
  images.go
  alerts.go
  agents.go
  terminal.go
```

Keep the public `Server` type. Move handlers by domain.

## Split `internal/store/store.go`

Suggested layout:

```text
internal/store/
  store.go
  migrations.go
  hosts.go
  metrics.go
  alert_rules.go
  alert_events.go
  alert_channels.go
  agent_tokens.go
  compose_versions.go
```

Also address the schema TODO:

> “TODO: Rework this database storage system in the near future to a more efficient and sensible system.”  
> Evidence: `internal/store/schema.sql:129`

Do not over-optimize yet, but do introduce migrations before stored data matters more.

## Split provider interfaces

`DockerProvider` currently includes container, network, volume, image, and update-check operations. Evidence: `internal/hub/hub.go:33-61`.

Split it conceptually:

```go
type ContainerProvider interface { ... }
type NetworkProvider interface { ... }
type VolumeProvider interface { ... }
type ImageProvider interface { ... }
type TerminalProvider interface { ... }
```

The implementation can still be one concrete Docker client, but the interfaces should not keep growing into one giant surface.

---

# Efficiency rework: what to delay and what to prepare

The efficiency/data-size rework is real but not the next bottleneck.

A later target is historical metric loading. `MetricsRange` loads rows and downsamples in Go. That is acceptable for alpha, but eventually you may want:

```text
- SQL-level bucketing
- pre-aggregated tables
- retention tiers
- lower-resolution long-term storage
- per-metric retention controls
```

The overview already mentions data retention tiers as an architecture consideration:

> “high-resolution (minutes) for 24h, medium (hourly) for 30d, low (daily) for 1y+. Auto-downsampling.”  
> Evidence: `overview.md:241`

Do not build that first. Prepare for it by cleaning store structure and adding migrations.

---

# App Store / deployment system guidance

The handoff doc says:

> “The next phase of the roadmap focuses on building an **App Store / App Deployments** system.”  
> Evidence: `export_state.md:21-23`

That should wait until the following are done:

```text
- auth exists
- compose path allowlist exists
- API versioning decision is made
- store migrations exist
- Compose errors are not hidden
- frontend modal/confirm/action patterns are reusable
```

An App Store system will multiply every existing risk. It writes files, deploys containers, creates env/config state, and exposes destructive actions. Build it after the core is safer.

## Suggested App Store model

Use a template format that is powerful but explicit:

```text
templates/
  app-name/
    template.yaml
    compose.yaml.tmpl
    README.md
```

Example `template.yaml` shape:

```yaml
id: homepage
name: Homepage
category: dashboard
summary: Homelab dashboard
compose_file: compose.yaml.tmpl

variables:
  - name: PUID
    label: User ID
    type: string
    default: "1000"
    required: true
  - name: PGID
    label: Group ID
    type: string
    default: "1000"
    required: true
  - name: CONFIG_DIR
    label: Config directory
    type: path
    default: /opt/docker/homepage
    required: true
    root_policy: compose_roots

ports:
  - container: 3000
    default_host: 3000

volumes:
  - variable: CONFIG_DIR
    target: /app/config
```

Keep the generated Compose YAML visible before deploy. Power users should always be able to inspect and edit the final YAML.

---

# Concrete implementation plan

## Phase A — Stabilize core before UI overhaul

```text
- [ ] Run gofmt on all Go files.
- [ ] Add minimal auth middleware.
- [ ] Allow unauthenticated access only to /api/health.
- [ ] Add audit log table for destructive actions.
- [ ] Fix terminal frame encoding by passing structs to wsjson.Write.
- [ ] Decide local terminal support path.
- [ ] Add TerminalProvider interface.
- [ ] Move agent protocol structs to internal/agentproto.
- [ ] Make samplesIn context-aware.
- [ ] Add /api/v1 route group or decide explicitly to defer versioning.
- [ ] Add Compose allowed-root validation.
- [ ] Stop hiding Compose command errors as empty states.
- [ ] Fix empty project-name behavior in StackAction.
- [ ] Add initial unit tests around compose parsing and agent frames.
```

## Phase B — Frontend structure pass

```text
- [ ] Extract AppShell.
- [ ] Extract PageHeader.
- [ ] Extract HostSubnav.
- [ ] Extract StatusPill.
- [ ] Extract shared Modal.
- [ ] Extract ConfirmDialog.
- [ ] Extract DataTable.
- [ ] Extract LogsPanel.
- [ ] Extract TerminalModal.
- [ ] Extract DangerButton.
- [ ] Move container-specific UI into web/src/lib/features/containers/.
- [ ] Move compose-specific UI into web/src/lib/features/compose/.
- [ ] Move alert-specific UI into web/src/lib/features/alerts/.
- [ ] Keep visual design mostly unchanged during extraction.
```

## Phase C — Visual UI overhaul

```text
- [ ] Redesign home dashboard around “Needs Attention.”
- [ ] Add host health summary cards.
- [ ] Add stack health summary cards.
- [ ] Add global alert/event feed.
- [ ] Create consistent detail-page layout.
- [ ] Standardize action buttons and danger states.
- [ ] Improve logs and terminal as first-class tools.
- [ ] Add responsive/mobile handling after desktop layout is stable.
```

## Phase D — App Store / deployments

```text
- [ ] Define template schema.
- [ ] Add template parser/validator.
- [ ] Add dry-run/render endpoint.
- [ ] Show generated Compose YAML before deploy.
- [ ] Save deployment metadata in DB.
- [ ] Deploy only into allowed Compose roots.
- [ ] Add rollback/redeploy story.
- [ ] Add tests for template rendering and path validation.
```

## Phase E — Efficiency/data rework

```text
- [ ] Split store implementation by domain.
- [ ] Add schema migration system.
- [ ] Define metric retention tiers.
- [ ] Add SQL-level or pre-aggregated downsampling.
- [ ] Add pruning jobs per metric type.
- [ ] Add DB size/status view in UI.
```

---

# Priority table

| Priority | Item | Why |
|---|---|---|
| P0 | Add basic auth | The app exposes infrastructure-changing actions. |
| P0 | Fix/verify terminal routing | Terminal likely only works for agent hosts from current API path. |
| P0/P1 | Fix terminal frame encoding | Current `json.Marshal` + `wsjson.Write([]byte)` pattern is likely wrong. |
| P1 | Shared agent protocol package | Avoid hub/agent drift. |
| P1 | Compose path allowlist | Required before App Store/deployments. |
| P1 | Stop hiding Compose errors | Empty state and broken state must differ. |
| P1 | Add tests | Refactors will be risky without them. |
| P1 | Split large API/store files | Keeps future feature work manageable. |
| P2 | Clean docs drift | Good docs exist, but sources of truth need separation. |
| P2 | Efficiency rework | Important later, not the next bottleneck. |

---

# Final recommendation

Aperture is worth continuing. The current state is not “too messy”; it is normal alpha-stage momentum. But this is exactly the point where you should stop treating the structure as disposable.

The best next move is:

```text
Stabilize the core → extract frontend architecture → then redesign the UI.
```

Do **not** build the App Store/deployment system on top of the current structure yet. It will touch the riskiest parts of the app: auth, file writes, Compose actions, database state, destructive operations, and UI confirmation flows.

Once the foundation is cleaned up, the UI overhaul will be much easier and the efficiency rework will be less painful.
