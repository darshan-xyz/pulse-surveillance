# Project: Dashboard Unification & WebRTC Streaming Architecture

## Architecture
- **Frontend (`web_dashboard`)**: Single unified Svelte 5 application running on Vite 8 and Tailwind CSS 4. Provides unified Home Dashboard and Remote Viewer modes with dynamic routing/mode switching, multi-camera responsive grid (1, 2, 3, 4 columns), Theater Mode layout with thumbnail switcher, real-time WebRTC stream player (`recvonly` transceivers + trickle ICE buffering), live Telemetry HUD (`pc.getStats()` polling for FPS, resolution, bitrate, RTT latency, ICE transport), Google Sign-in / JWT authentication, and dynamic protocol/hostname/port resolution (`config.ts`).
- **Backend (`hss-phase1a`)**: High-performance Go microservice architecture. Handles edge camera streaming, signaling hubs (`/client`, `/signaling`, `/ws`), dynamic RFC 5766 STUN/TURN credential generation (`/api/turn/credentials`), Valkey session routing, multi-tenant RBAC device access, and token lifecycle management.
- **Redundancy Elimination**: Safe removal of historical `viewer_dashboard` directory to establish `web_dashboard` as the single source of truth.

## Feature Inventory
| # | Feature | Description | Milestone | Status |
|---|---------|-------------|-----------|--------|
| 1 | Redundant Folder Removal | Remove obsolete `viewer_dashboard` directory | M1 | DONE |
| 2 | Unified Dashboard & Viewer Mode | Seamless mode toggling and navigation between Home Console and Viewer Mode | M1 | DONE |
| 3 | Multi-Camera Grid & Theater Mode | Responsive multi-camera surveillance grid (1-4 cols) and dedicated Theater Mode (dominant 16:9 + thumbnail switcher) | M1 | DONE |
| 4 | Camera Management & Selection | Camera enumeration, selection, search filtering, and mode-based suppression of local webcam | M1 | DONE |
| 5 | Google Sign-In & Auth Flows | Google OAuth and standard credential authentication with JWT session persistence | M1 | DONE |
| 6 | Real-Time Telemetry HUD | Live HUD displaying FPS, resolution, bitrate, RTT latency, and ICE connection state | M2 | DONE |
| 7 | Dynamic Hostname & Origin Resolution | Dynamic protocol, hostname, and port resolution for API endpoints and WebSockets (`/client`, `/signaling`) across LAN/WAN/proxies | M2 | DONE |
| 8 | End-to-End WebRTC Streaming & STUN/TURN | Seamless local/remote WebRTC playback with dynamic RFC 5766 TURN credentials, trickle ICE candidate buffering, and cleanup on unmount | M2 | DONE |
| 9 | Autoplay & Track Lifecycle | Autoplay without gesture errors, graceful stream reattachment, and strict track teardown | M2 | DONE |
| 10 | Backend Endpoint Hardening | Clean configuration binding for all signaling endpoints in `cmd/hss/main.go` | M2 | DONE |
| 11 | Comprehensive Build & Regression Verification | Verify `npm run build` (0 errors), Vitest suite (100% pass), `go build ./...` (0 errors), and Go test suite (100% pass) | M3 | DONE |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | Dashboard Unification & Viewer Experience | Port Theater Mode, ensure seamless mode toggle, remove `viewer_dashboard`, camera management, Google auth | none | DONE |
| 2 | WebRTC Remote Streaming & Telemetry HUD | Implement real-time `pc.getStats()` Telemetry HUD in `web_dashboard`, dynamic origin verification, fix backend signaling endpoint binding | M1 | DONE |
| 3 | Final Quality, E2E Build & Regression Verification | Comprehensive validation: `npm run build`, Vitest test suite, `go build ./...`, `go test ./...`, code layout verification | M1, M2 | DONE |

## Interface Contracts
### `web_dashboard/src/lib/config.ts` ↔ Backend Services
- `getApiUrl(path: string)`: Resolves `${protocol}//${hostname}:${port}${path}` dynamically based on `window.location`.
- `getWsUrl(path: string)`: Resolves `${wsProtocol}//${hostname}:${port}${path}` dynamically.
- `/client`: WebSocket endpoint requiring Authorization JWT, handles WebRTC SDP offers/answers and ICE candidates with Valkey session routing.
- `/api/turn/credentials`: REST endpoint returning ephemeral HMAC-SHA1 TURN credentials (RFC 5766).

### `CameraStream.svelte` ↔ `StreamTelemetry.svelte`
- `CameraStream` polls `pc.getStats()` every 1000ms.
- Extracts: `fps: number`, `resolution: string` (width x height), `bitrateKbps: number`, `rttMs: number`, `iceState: string`, `candidateType: 'relay' | 'host' | 'srflx' | 'prflx'`.
- Passes metrics to `StreamTelemetry` HUD overlay component.

## Code Layout
- Frontend: `hss-phase1a/web_dashboard/`
  - `src/App.svelte`
  - `src/lib/components/Dashboard.svelte`
  - `src/lib/components/CameraStream.svelte`
  - `src/lib/components/StreamTelemetry.svelte`
  - `src/lib/components/Login.svelte`
  - `src/lib/config.ts`
  - `src/lib/auth.ts`
  - `src/lib/cameras.ts`
- Backend: `hss-phase1a/`
  - `cmd/hss/main.go`
  - `internal/api/`
  - `internal/camera/`
  - `internal/signaling/`
  - `internal/webrtc/`
