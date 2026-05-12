# Aperture Development State Export

This document provides a comprehensive state of the Aperture development environment, designed to be ingested by the next Antigravity session to seamlessly resume work.

## Current Project Status
We are actively working through **Phase 3** of the Aperture roadmap (achieving feature parity with Dockhand and pushing past it). 

### What has been completed recently:
1. **Web Terminal Pipeline (Phase 3)**:
   - Built a robust, bi-directional Web Terminal allowing interactive shell access (`/bin/sh`) directly into running containers.
   - Utilizes `xterm.js` and `xterm-addon-fit` on the frontend.
   - Proxies the terminal I/O over the existing WebSocket multiplexing layer (`agentws`), passing securely through the Hub and straight to the Docker engine via `dockerctl` (`ExecCreate`, `ExecAttach`, `ExecResize`).
   - The UI integration is fully complete, rendering a seamless floating terminal modal with a "Terminal" button on the containers list and inspect panels.

2. **Recent Bug Fixes**:
   - **Terminal SSR Crash**: Fixed a silent failure where the Terminal button did nothing due to SvelteKit Server-Side Rendering (SSR) evaluating `xterm.js` statically. This was resolved by dynamically importing `xterm` inside the `onMount` hook.
   - **Images Timestamp Glitch ("Created 20564d ago")**: Fixed `format.ts` `relTime` and `absTime` to correctly multiply raw Unix seconds (sent by the Agent) by 1000 to cast them into Javascript's expected millisecond timestamps.
   - **Image Inspect/Remove 502s**: Repositories containing slashes/colons (e.g., `linuxserver/homeassistant:latest`) were causing 502 proxy errors. Added `url.PathUnescape` directly to the `chi.URLParam` captures within the Hub API to decode `%2F` and `%3A`.
   - **Host Disk Mounts showing 0.00 GiB**: Fixed a double-division bug where the raw bytes from SQLite were divided by `1073741824` before being passed to the `<Chart>` component, which then passed it to `fmtGiB` and divided it again.

## Pending Tasks (Next Steps for Phase 3)
The next phase of the roadmap focuses on building an **App Store / App Deployments** system:
- **Compose App Store**: Implementation of a template-driven deploy system (analogous to Dockhand's templates).
- Need to establish the exact structure for App Store templates. The user requested deep power-user abilities while remaining intuitive.
- The UI should surface available templates, allow users to configure environment variables/volumes visually, and execute the deployment via the existing Compose engine on the respective Agent.
- **Database Refactor Note**: The database storage backend (SQLite) currently requires a "somewhat near future" rework to a more efficient and sensible system, per the user's explicit instructions. Keep this in mind when designing how App Store configurations are saved.

## Environment Details
- **Architecture**: Go backend (`cmd/hub` & `cmd/agent`) with an embedded SvelteKit frontend (`web/`). 
- **Start Command**: Start the backend using `go build -o bin/hub ./cmd/hub && go build -o bin/agent ./cmd/agent`.
- **Frontend Commands**: Normal `npm run dev` in the `/web` directory; the API gets automatically proxy routed.
- **WebSocket Protocol**: The hub and agent communicate exclusively through the `internal/hub/agentws.go` layer using JSON frames. Any new agent-to-hub communication must be multiplexed here.

*Proceed to plan and implement the Phase 3 App Store / Deployments system based on this context.*
