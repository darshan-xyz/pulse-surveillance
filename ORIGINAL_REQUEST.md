# Original User Request

## 2026-08-30T14:31:20Z

# Teamwork Project Prompt — Draft

> Status: Launched.
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: A full autonomous team

Automate and configure the entire remote streaming pipeline to provide a single, zero-configuration public URL that allows any device on any network to instantly open and watch live WebRTC video streams with zero manual setup.

Working directory: `d:\projects\pulse-landing(3)\hss-phase1a`
Integrity mode: demo

## Requirements

### R1. Zero-Configuration Remote Tunnel & Public URL
Set up an automated public ingress tunnel (e.g. Cloudflare Quick Tunnel / ngrok) that securely exposes both the frontend dashboard (`:5173`) and cloud signaling backend (`:9091`) under public HTTPS/WSS URLs, printing a single clickable shareable viewer link.

### R2. One-Click Instant Viewer Link (Direct Stream Access)
Implement a direct viewer URL route (e.g. `https://<public-url>/?viewer=true&autostart=true`) with automatic authenticated session bootstrapping, bypassing login hurdles on remote devices so that simply clicking or scanning the link immediately opens the live video feed.

### R3. WebRTC Cross-Network & Firewall Traversal
Ensure Pion WebRTC and browser client configurations utilize high-availability STUN/TURN relays (`openrelay.metered.ca`) so that symmetric NATs and cellular mobile networks connect with sub-second latency and zero black screens.

## Acceptance Criteria

### Automated Verification
- [ ] `npm test` inside `web_dashboard` completes with 100% tests passing (119+ tests).
- [ ] Backend Go test suite (`go test ./...`) passes with 0 errors.
- [ ] Running the tunnel provisioner successfully generates a public `https://*.trycloudflare.com` or local network URL.
- [ ] Navigating to the generated viewer URL on an external network or mobile phone immediately initializes WebRTC and renders the live video stream without manual intervention.

## 2026-08-31T06:43:43Z

# Teamwork Project Prompt — Draft

> Status: Launched.
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: A full autonomous team

Conduct a comprehensive performance, latency, and reliability optimization across the entire WebRTC surveillance stack: eliminate signaling bottlenecks, minimize glass-to-glass latency to sub-200ms, optimize browser video playout pipelines, and harden cross-network connection stability.

Working directory: `d:\projects\pulse-landing(3)\hss-phase1a`
Integrity mode: demo

## Requirements

### R1. Ultra-Low Latency Video Playout Pipeline
- Optimize browser `<video>` element playout buffer settings (`latencyHint: 'interactive'`, `playoutDelayHint: 0`, and unbuffered low-latency playback).
- Tune WebRTC SDP parameters (e.g. `x-google-min-bitrate`, `max-bitrate`, `cbr=1`, and low packetization delay) to achieve real-time streaming with sub-200ms latency.

### R2. WebRTC Negotiation Speed & ICE Optimization
- Parallelize and accelerate ICE candidate gathering using aggressive trickle ICE.
- Optimize STUN/TURN server selection and fast-path host candidate checks to reduce initial stream negotiation and connection time to under 1.5 seconds.

### R3. Media Ingest & Frame Encoding Throughput
- Optimize Go backend H.264 synthetic generation and browser `getUserMedia` encoding constraints to deliver consistent 30+ FPS at optimal bitrate without frame drops or GC stalls.
- Ensure efficient memory recycling and zero buffer allocations along the critical RTP dispatch path.

### R4. Automated Performance & Latency Benchmarks
- Expand the automated test suite with latency and throughput regression tests verifying sub-200ms target latency, zero memory leaks, and resilient reconnection across simulated network packet loss.

## Acceptance Criteria

### Automated Verification
- [ ] `npm test` inside `web_dashboard` completes with 100% tests passing.
- [ ] Backend test suite (`go test ./...`) completes with 0 errors and zero race conditions (`-race`).
- [ ] Video playback starts in under 2 seconds from page load.
- [ ] Round-trip latency (RTT) and frame jitter remain within sub-second interactive thresholds under load.

## 2026-09-01T14:54:49Z

# Teamwork Project Prompt — Draft

> Status: Launched.
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: A full autonomous team

Integrate the YOLO11 fire and smoke detection model (`best.pt`) into the Pulse surveillance platform to perform real-time AI fire detection on live video feeds, render live bounding boxes on the dashboard, and emit immediate alert notifications upon detection.

Working directory: `d:\projects\pulse-landing(3)\hss-phase1a`
Integrity mode: demo

## Requirements

### R1. Real-Time Fire & Smoke AI Inference Pipeline
- Deploy an inference engine using the trained `best.pt` model (or optimized ONNX export) that continuously ingests live video frames from active cameras (`webcam-live-01`, `test-camera-1`, and RTSP feeds).
- Run object detection to identify `fire` (class 0) and `smoke` (class 1) with configurable confidence thresholds (default: >= 0.40).

### R2. Live Bounding Box Overlay on Dashboard
- Stream detected bounding boxes `[x, y, w, h]`, class names, and confidence scores in real time to the frontend dashboard.
- Render smooth, low-latency bounding box canvas overlays and telemetry labels directly over the live video player in `CameraStream.svelte`.

### R3. Emergency Event Alerts & Notification System
- When fire or smoke is detected, emit structured alerts through the `internal/events` engine.
- Display a prominent emergency warning banner and trigger audio/visual indicator badges on the dashboard HUD.

### R4. Automated Testing & Verification Suite
- Provide programmatic unit and integration tests verifying:
  - Model loading and inference execution on sample fire/smoke test frames.
  - Correct event dispatch and WebSocket metadata streaming.
  - Bounding box calculation and frontend component rendering under load.

## Acceptance Criteria

### Automated Verification
- [ ] `npm test` in `web_dashboard` passes with 100% success rate.
- [ ] Backend Go test suite (`go test ./...`) passes with 0 errors.
- [ ] Feeding a video frame with fire/smoke triggers an AI detection event with confidence score >= 0.40.
- [ ] The dashboard renders live bounding boxes and an emergency fire alert badge over the video feed.

## 2026-09-03T15:43:31Z

# Teamwork Project Prompt — Draft

> Status: Launched.
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: A full autonomous team

Perform an exhaustive, adversarial end-to-end verification and hardening of the real-time YOLO11 fire and smoke detection pipeline across the Pulse surveillance platform: ensure live video feeds (`test-camera-1`, `webcam-live-01`, and RTSP feeds) reliably detect fire in real time, render bounding box overlays on the Svelte dashboard, emit instant emergency alerts, and pass all automated regression and stress test suites.

Working directory: `d:\projects\pulse-landing(3)\hss-phase1a`
Integrity mode: demo

## Requirements

### R1. Real-Time Fire & Smoke AI Inference Pipeline
- Ensure the inference engine using the trained `best.pt` model continuously ingests live video frames from active cameras (`webcam-live-01`, `test-camera-1`, and RTSP feeds).
- Verify object detection identifies `fire` (class 0) and `smoke` (class 1) with configurable confidence thresholds (>= 0.30 - 0.35).

### R2. Live Bounding Box Overlay & Visual Alerts on Dashboard
- Stream detected bounding boxes `[x, y, w, h]`, class names, and confidence scores in real time to the frontend dashboard.
- Render smooth, low-latency bounding box canvas overlays and pulsating "FIRE DETECTED" badges directly over the live video player in `CameraStream.svelte` and the top-level Emergency Warning Banner in `Dashboard.svelte`.

### R3. Emergency Event Alerts & Signaling Relay
- When fire or smoke is detected, verify structured alerts pass through `internal/events` and WebSocket signaling relays (`hub.go` & `client.go`) so all connected local and remote viewers receive real-time detection packets.

### R4. Automated Testing & Production Build Verification
- Execute and verify the complete test suite:
  - `npm test` inside `web_dashboard` passes with 100% of tests (202+ tests).
  - Python AI service test suite (`pytest ai_service/test_service.py`) passes 100%.
  - Backend Go test suite (`go test -mod=readonly ./...`) passes with 0 errors.
  - Production builds (`npm run build` and `go build ./...`) complete with 0 errors.

## Acceptance Criteria

### Automated Verification
- [ ] `npm test` in `web_dashboard` passes with 100% success rate (202+ tests).
- [ ] Backend Go test suite (`go test -mod=readonly ./...`) passes with 0 errors.
- [ ] Python YOLO11 test suite (`pytest ai_service/test_service.py`) passes 5/5 tests.
- [ ] Feeding a video frame with fire/smoke triggers an AI detection event and updates the dashboard HUD.
- [ ] The dashboard renders live bounding boxes and an emergency fire alert badge over the video feed.
