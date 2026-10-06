# Project: Pulse YOLO11 Fire & Smoke Surveillance Pipeline Hardening

## Architecture
Pulse is a hybrid surveillance platform comprising an edge Go daemon (`hss`), a central Go cloud server (`pulse-cloud`), a Python YOLO11 AI inference service (`ai_service`), and a Svelte 5 web dashboard (`web_dashboard`).

### Data Flow
1. **Video Ingestion**: `internal/media/engine.go` ingests video frames from active cameras (`webcam-live-01`, `test-camera-1`, RTSP feeds, synthetic fallback).
2. **AI Inference Pipeline**: `ai_service` with trained `best.pt` identifies `fire` (class 0) and `smoke` (class 1) with configurable confidence thresholds (>= 0.30 - 0.35).
3. **Event Alerting**: `internal/events/engine.go` emits `EventFireDetected` and `EventSmokeDetected` alerts, persisting to DB and dispatching notifications.
4. **WebSocket Signaling Hub**: `internal/cloud/signaling/hub.go` broadcasts detection packets `[x, y, w, h]` and confidence scores to `/client` viewer WebSockets and Valkey.
5. **Frontend Canvas & Emergency HUD**: `CameraStream.svelte` renders bounding box canvas overlays and pulsating badges; bubbles alerts to `Dashboard.svelte` for the top-level Emergency Warning Banner.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| F1 | YOLO11 Model Inference & Threshold Config | Load `best.pt`, detect fire (class 0) & smoke (class 1), support configurable threshold (0.30 - 0.35) | M1 | ORIGINAL_REQUEST R1 |
| F2 | Active Camera Ingestion Pipeline | Ingest frames from `test-camera-1`, `webcam-live-01`, and RTSP feeds into detection pipeline | M1 | ORIGINAL_REQUEST R1 |
| F3 | Go Backend Test Suite & Multi-Device Isolation | Fix test assertions in `api_test.go` and `db_chaos_test.go` for `webcam-live-01` multi-camera support so `go test -mod=readonly ./...` passes 100% | M1 | ORIGINAL_REQUEST R4 |
| F4 | Event Alert Dispatching & Signaling Hub | Structure alerts in `internal/events` and broadcast via WebSocket signaling relays (`hub.go`, `client.go`) | M1 | ORIGINAL_REQUEST R3 |
| F5 | Canvas Bounding Box Overlays & HUD Badges | Render normalized `[x, y, w, h]` bounding boxes, labels, and pulsating "FIRE DETECTED" badges in `CameraStream.svelte` | M1 | ORIGINAL_REQUEST R2 |
| F6 | Off-Screen Alert Bubbling & Emergency Banner | Ensure Theater/Single mode forwards off-screen alerts to `Dashboard.svelte` Emergency Warning Banner; align confidence thresholds to 0.30 | M1 | ORIGINAL_REQUEST R2, R3 |
| F7 | Automated Test Suites & Production Builds | Verify `npm test` (202+ tests), `pytest ai_service/test_service.py`, `go test -mod=readonly ./...`, `npm run build`, `go build ./...` | M1 | ORIGINAL_REQUEST R4 |
| F8 | Adversarial Verification & Integrity Forensics | Empirical adversarial challenge testing and forensic audit validation | M1 | ORIGINAL_REQUEST R4 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M0 | Phase 0: Full System Survey | Survey AI service, Go backend, and Svelte dashboard; identify defects & test status | none | DONE |
| M1 | End-to-End Pipeline Hardening & Test Remediation | Remediate Go test assertions (`api_test.go`, `db_chaos_test.go`), fix `CameraStream.svelte` off-screen alert bubbling, align thresholds to 0.30, verify builds & all test suites | M0 | DONE |

## Interface Contracts

### AI Inference Service ↔ Go Backend (`internal/ai`)
- **Request**: POST `http://127.0.0.1:8000/detect?camera_id=<id>&confidence_threshold=<0.30-0.35>`
- **Response JSON**:
  ```json
  {
    "camera_id": "test-camera-1",
    "detections": [
      {
        "box": [0.1, 0.2, 0.3, 0.4],
        "class_id": 0,
        "class_name": "fire",
        "confidence": 0.88
      }
    ],
    "has_fire": true,
    "has_smoke": false,
    "inference_time_ms": 12.5
  }
  ```

### Go Signaling Hub ↔ Web Dashboard
- **WebSocket Protocol**: `/client?token=<JWT>` or `/signaling`
- **Detection Message**:
  ```json
  {
    "type": "detection",
    "camera_id": "test-camera-1",
    "timestamp": 1772465000000,
    "detections": [
      {
        "class": "fire",
        "class_id": 0,
        "confidence": 0.88,
        "box": [0.1, 0.2, 0.3, 0.4]
      }
    ]
  }
  ```

### Dashboard Off-Screen Alert Contract
- In `CameraStream.svelte`: On receiving `type === 'detection'`, regardless of whether `data.camera_id === deviceId`, always dispatch `detection` event to parent `Dashboard.svelte` so `activeAlerts` and the Emergency Warning Banner activate globally.
- Only render bounding boxes on the local `<canvas>` if `!data.camera_id || data.camera_id === deviceId`.

## Code Layout
- AI Service: `hss-phase1a/ai_service/`
- Go Backend: `hss-phase1a/cmd/`, `hss-phase1a/internal/`, `hss-phase1a/tests/`
- Svelte Web Dashboard: `hss-phase1a/web_dashboard/src/`
- Frontend Tests: `hss-phase1a/web_dashboard/src/__tests__/`
