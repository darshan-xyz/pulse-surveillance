# E2E Test Infrastructure: Pulse YOLO11 Fire & Smoke Detection

## Test Philosophy & Methodology
- **Requirements-Driven & Opaque-Box**: All test cases are strictly derived from `ORIGINAL_REQUEST.md` (R1..R4) and interface contracts in `PROJECT.md` (F1..F8).
- **4-Tier Test Case Architecture**:
  - **Tier 1: Feature Coverage** (>= 5 test cases per feature for F1..F8, 40 total cases).
  - **Tier 2: Boundary & Corner Cases** (>= 5 test cases per feature covering edge values, threshold boundaries, corrupt data, 40 total cases).
  - **Tier 3: Pairwise Cross-Feature Interactions** (6 complex pairwise integrations across AI inference, event engine, signaling hub, and canvas).
  - **Tier 4: Real-World Application Scenarios** (5 end-to-end multi-camera emergency surveillance workflows).

---

## Feature Inventory & Test Matrix

| # | Feature ID | Feature Name | Source | Tier 1 (Coverage) | Tier 2 (Boundary) | Tier 3 (Cross-Feature) | Tier 4 (Real-World) |
|---|------------|--------------|--------|:-----------------:|:-----------------:|:----------------------:|:-------------------:|
| 1 | F1 | YOLO11 Model Loading & Inference Engine | ORIGINAL_REQUEST R1 | 5 | 5 | ✓ | ✓ |
| 2 | F2 | Real-Time Video Frame Ingestion Pipeline | ORIGINAL_REQUEST R1 | 5 | 5 | ✓ | ✓ |
| 3 | F3 | Emergency Event Alert Dispatching | ORIGINAL_REQUEST R3 | 5 | 5 | ✓ | ✓ |
| 4 | F4 | Real-Time WebSocket Metadata Streaming | ORIGINAL_REQUEST R2 | 5 | 5 | ✓ | ✓ |
| 5 | F5 | Frontend Canvas Bounding Box Overlay | ORIGINAL_REQUEST R2 | 5 | 5 | ✓ | ✓ |
| 6 | F6 | Frontend Emergency HUD & Alert Banner | ORIGINAL_REQUEST R3 | 5 | 5 | ✓ | ✓ |
| 7 | F7 | Go Backend Automated Unit & Integration Tests | ORIGINAL_REQUEST R4 | 5 | 5 | ✓ | ✓ |
| 8 | F8 | Frontend Automated Unit & Integration Tests | ORIGINAL_REQUEST R4 | 5 | 5 | ✓ | ✓ |
| **Total** | | **Full E2E Verification Suite** | | **40** | **40** | **6** | **5** |

---

## Test Execution Commands

### Standalone E2E Test Runner (All Tiers)
```powershell
# From project root:
powershell -ExecutionPolicy Bypass -File .\run_e2e.ps1

# Or from hss-phase1a directory:
cd hss-phase1a
go run ./tests/e2e/runner/main.go
```

### Go Test Command
```bash
cd hss-phase1a
go test -v ./tests/e2e/...
```

### Frontend Vitest Suite
```bash
cd hss-phase1a/web_dashboard
npm test
```

### Go Backend Unit & Integration Tests
```bash
cd hss-phase1a
go test ./internal/...
```

---

## Tier Breakdown & Detailed Test Catalog

### Tier 1: Feature Coverage (40 Test Cases)
- **F1 (Model Loading & Inference)**:
  - `TestTier1_F1_01_ModelLoadAndWarmup`: Model initialization, tensor warmup, clean frame zero false-positives.
  - `TestTier1_F1_02_Class0FireDetectionAboveThreshold`: Class 0 (fire) detection with confidence >= 0.40.
  - `TestTier1_F1_03_Class1SmokeDetectionAboveThreshold`: Class 1 (smoke) detection with confidence >= 0.40.
  - `TestTier1_F1_04_SubThresholdFilteringBelow040`: Strict rejection of detections with confidence < 0.40.
  - `TestTier1_F1_05_InferenceMetadataAndTiming`: Validation of inference timing latency and camera metadata.
- **F2 (Frame Ingestion Pipeline)**:
  - `TestTier1_F2_01_WebcamLive01Ingest`: Frame ingestion from active webcam feed `webcam-live-01`.
  - `TestTier1_F2_02_TestCamera1Ingest`: 720p HD frame ingestion from `test-camera-1`.
  - `TestTier1_F2_03_RTSPFeedIngest`: Ingestion of RTP packet streams from simulated RTSP camera feeds.
  - `TestTier1_F2_04_SyntheticFrameFallback`: Synthetic frame generation fallback on camera disconnect.
  - `TestTier1_F2_05_SequentialMultiFrameCadence`: Multi-frame sequential stream cadence without buffer drift.
- **F3 (Emergency Event Alert Dispatching)**:
  - `TestTier1_F3_01_FireDetectedEventEmission`: Immediate emission of `FIRE_DETECTED` events.
  - `TestTier1_F3_02_SmokeDetectedEventEmission`: Immediate emission of `SMOKE_DETECTED` events.
  - `TestTier1_F3_03_SQLiteEventPersistence`: Event persistence to SQLite DB with UUID and timestamp.
  - `TestTier1_F3_04_PushNotificationDispatch`: Expo push notification formatting (`CRITICAL FIRE ALERT`, high priority).
  - `TestTier1_F3_05_EventDeduplicationAndThrottling`: Rapid multi-event burst capture and throttling.
- **F4 (WebSocket Metadata Streaming)**:
  - `TestTier1_F4_01_SignalingHubDetectionPacketFormat`: JSON payload validation (`type: "detection"`, `box`, `confidence`).
  - `TestTier1_F4_02_BroadcastToConnectedViewers`: Fan-out broadcasting to multiple concurrent viewer connections.
  - `TestTier1_F4_03_CameraSpecificViewerFiltering`: Camera-specific WebSocket stream routing.
  - `TestTier1_F4_04_TimestampMonotonicity`: Monotonic timestamp ordering on streamed packets.
  - `TestTier1_F4_05_ClientDisconnectAndGracefulCleanup`: Clean socket teardown on viewer disconnect.
- **F5 (Frontend Canvas Bounding Box Overlay)**:
  - `TestTier1_F5_01_NormalizedCoordinateScaling`: Scaling normalized coordinates `[0.35, 0.42, 0.15, 0.28]` to video canvas pixels.
  - `TestTier1_F5_02_FireBoxStylingRed`: Red-500 stroke `rgba(239, 68, 68, 0.9)`, fill `rgba(239, 68, 68, 0.15)`, badge `FIRE (85%)`.
  - `TestTier1_F5_03_SmokeBoxStylingAmber`: Amber-500 stroke `rgba(245, 158, 11, 0.9)`, fill `rgba(245, 158, 11, 0.15)`, badge `SMOKE (72%)`.
  - `TestTier1_F5_04_MultiBoxSimultaneousRendering`: Simultaneous rendering of multiple boxes on a single canvas.
  - `TestTier1_F5_05_TTLExpiryAutoClearing1000ms`: Automatic clearing of bounding boxes after 1000ms TTL.
- **F6 (Frontend Emergency HUD & Alert Banner)**:
  - `TestTier1_F6_01_CameraStreamFireBadge`: Pulsing emergency fire badge on active camera player.
  - `TestTier1_F6_02_CameraStreamSmokeBadge`: Amber smoke warning badge on active camera player.
  - `TestTier1_F6_03_GlobalDashboardAlertBanner`: Top-level critical emergency banner on dashboard.
  - `TestTier1_F6_04_AlertAcknowledgmentAndDismissal`: Operator alert dismissal and HUD resolution.
  - `TestTier1_F6_05_MultiCameraEmergencyAggregation`: Aggregated multi-camera status badges in HUD.
- **F7 (Go Backend Automated Unit & Integration Tests)**:
  - `TestTier1_F7_01_AIDetectorLifecycle`: AI detector goroutine startup, context cancellation, clean shutdown.
  - `TestTier1_F7_02_EventEngineDBTransactionIntegrity`: Atomic database transaction persistence.
  - `TestTier1_F7_03_SignalingHubConcurrentSafety`: Thread-safe client connection and broadcast handling.
  - `TestTier1_F7_04_MediaEngineFrameDispatchPipeline`: Frame queue dispatch from media engine to AI detector.
  - `TestTier1_F7_05_BackendIntegrationSmokeTest`: End-to-end backend integration via REST and WebSocket.
- **F8 (Frontend Automated Unit & Integration Tests)**:
  - `TestTier1_F8_01_DetectionPacketParser`: Schema parser validating required detection fields.
  - `TestTier1_F8_02_CanvasBoundingBoxRendererMath`: Canvas aspect ratio and coordinate transform math.
  - `TestTier1_F8_03_HUDAlertStateManagement`: Reactive alert state transitions.
  - `TestTier1_F8_04_CanvasResizeObserverSync`: Synchronized canvas scaling upon video resize.
  - `TestTier1_F8_05_StreamReconnectionDetectionState`: State reset and re-initialization on stream reconnect.

### Tier 2: Boundary & Corner Cases (40 Test Cases)
- **F1 Boundary**: Threshold boundary (0.399999 vs 0.400000), 0.00 vs 1.00 confidence, out-of-bounds bounding boxes, empty/nil frame buffers, dense crowding (25+ simultaneous boxes).
- **F2 Boundary**: Corrupted frame bytes, 4K UHD frames (3840x2160), 120 FPS burst ingest, camera feed drop/resume, zero-FPS frozen feed detection.
- **F3 Boundary**: Zero confidence rejection, UUID collision resistance (1,000 emissions), closed/nil DB error handling, alert storm rate-limiting, push dispatch timeout.
- **F4 Boundary**: 100-box massive WebSocket payload, rapid subscriber churn (30 connections), slow consumer backpressure, malformed WS packets, invalid JWT rejection.
- **F5 Boundary**: 0x0 canvas dimension clamping, negative/out-of-bounds coordinate clamping, zero-width/height box skipping, high-frequency style oscillation, canvas context loss/restoration.
- **F6 Boundary**: Minimum hold time anti-flicker, 20-camera alert overflow, alert dismissal during active detection, 32:9 and 9:16 extreme viewports, camera offline vs fire priority coexistence.
- **F7 Boundary**: 10-detector concurrent startup, SQLite busy transaction isolation, event channel buffer saturation, goroutine leak audit, model inference failure fallback.
- **F8 Boundary**: Truncated JSON parsing, missing camera_id, non-numeric confidence type, stale timestamp detection, unmounted component draw loop suppression.

### Tier 3: Pairwise Cross-Feature Interactions (6 Test Cases)
- `TestTier3_Pairwise_01_InferenceToEventToDBAndPush` (F1 x F3): AI detection -> Event emission -> SQLite storage -> Push notification dispatch.
- `TestTier3_Pairwise_02_InferenceToWebSocketBroadcasting` (F1 x F4): AI inference output -> WebSocket serialization -> Multiplexed viewer fan-out.
- `TestTier3_Pairwise_03_FrameIngestToInferenceToWSToCanvas` (F2 x F1 x F4 x F5): Camera frame ingest -> YOLO11 detection -> WebSocket stream -> Canvas overlay rendering.
- `TestTier3_Pairwise_04_MultiClassDetectionToEventToHUD` (F1 x F3 x F6): Simultaneous fire & smoke detection -> Dual event alerts -> Aggregated HUD badges and critical banner.
- `TestTier3_Pairwise_05_WebSocketReconnectionToCanvasRecovery` (F4 x F5 x F6): Network disruption -> Overlay TTL clearing -> Reconnection -> Instantaneous overlay restoration.
- `TestTier3_Pairwise_06_BurstFrameRateLimitingToThrottledAlerts` (F2 x F1 x F3 x F5): High-speed frame burst -> AI rate limiting -> Event alert throttling -> Smooth 30 FPS canvas updates.

### Tier 4: Real-World Application Scenarios (5 Test Cases)
- `TestTier4_Scenario_01_MultiCameraWarehouseFireOutbreak`: 4-camera facility surveillance with concurrent fire on Camera 1 and smoke on Camera 2.
- `TestTier4_Scenario_02_SmolderingFireEvolutionLifecycle`: Complete 4-phase incident progression: Faint smoke (<0.40) -> Dense smoke (0.76) -> Open flame (0.88) -> Extinguished (TTL clearing).
- `TestTier4_Scenario_03_Continuous247SurveillanceWithFalseAlarmFiltering`: 100 sequential frames with optical reflections/steam (conf 0.10..0.38) generating 0 false alarms, followed by immediate genuine fire detection.
- `TestTier4_Scenario_04_MultiViewerLiveIncidentResponse`: 5 concurrent security operator dashboards receiving synchronized live fire bounding boxes with low latency jitter (< 100ms).
- `TestTier4_Scenario_05_EdgeNetworkPartitionAndOfflineResync`: Edge camera buffering detections locally during WAN outage, auto-resyncing upon reconnection.

---

## Test Runner Architecture & Exit Codes
- **Execution Tool**: `hss-phase1a/tests/e2e/runner/main.go`
- **Output**: Real-time ASCII table with pass/fail counts per tier, overall completion time, and structured error logs.
- **Pass/Fail Semantics**: Returns exit code `0` on 100% test pass; returns exit code `1` on any failure.
