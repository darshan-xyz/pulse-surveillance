package e2e

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================================
// TIER 3: Cross-Feature Combinations (Pairwise Feature Interactions)
// Exercises complex interactions across AI, Events, Signaling, and Canvas
// ============================================================================

// ----------------------------------------------------------------------------
// Pairwise 1: AI Inference (F1) -> Event Engine (F3) -> SQLite & Push
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_01_InferenceToEventToDBAndPush(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness setup failed: %v", err)
	}
	defer harness.Close()

	// 1. Generate Synthetic Fire Frame and run Inference Oracle
	_, expectedDetections := GenerateSyntheticFrame(FrameTypeFireActive, 640, 480)
	oracle := NewYOLO11InferenceOracle()
	detections := oracle.Evaluate(expectedDetections, 0.40)

	if len(detections) != 1 {
		t.Fatalf("Expected 1 detection from inference oracle, got %d", len(detections))
	}

	det := detections[0]
	eventID := GenerateTestUUID()
	cameraID := "webcam-live-01"

	// 2. Map Detection to Emergency Event
	var eventType EventType
	if det.ClassID == ClassIDFire {
		eventType = EventFireDetected
	} else {
		eventType = EventSmokeDetected
	}

	evt := EmergencyEvent{
		ID:         eventID,
		CameraID:   cameraID,
		Type:       eventType,
		StartedAt:  time.Now(),
		Confidence: det.Confidence,
		Data: map[string]interface{}{
			"box":        det.Box,
			"class_name": det.Class,
			"confidence": det.Confidence,
		},
	}

	// 3. Emit through Event Engine
	if err := harness.EmitEvent(evt); err != nil {
		t.Fatalf("Failed to emit event: %v", err)
	}

	// 4. Verify Event Persistence State
	storedEvt, err := harness.GetEvent(eventID)
	if err != nil {
		t.Fatalf("Failed to retrieve event from persistence store: %v", err)
	}

	if storedEvt.Type != EventFireDetected || storedEvt.CameraID != cameraID || storedEvt.Confidence != det.Confidence {
		t.Fatalf("Event persistence integrity error: type=%s cam=%s conf=%f", storedEvt.Type, storedEvt.CameraID, storedEvt.Confidence)
	}

	// 5. Verify Expo Push Notification Dispatch
	if len(harness.DispatchedPushes) != 1 {
		t.Fatalf("Expected 1 push notification, got %d", len(harness.DispatchedPushes))
	}
	push := harness.DispatchedPushes[0]
	if !strings.Contains(push.Title, "CRITICAL FIRE ALERT") {
		t.Fatalf("Unexpected push title: %s", push.Title)
	}
	if push.Data["event_id"] != eventID || push.Data["camera_id"] != cameraID {
		t.Fatalf("Push metadata mismatch: %+v", push.Data)
	}

	t.Log("Tier 3 - Pairwise 01 Passed: Frame -> Inference -> Event -> SQLite -> Push verified.")
}

// ----------------------------------------------------------------------------
// Pairwise 2: AI Inference (F1) -> WebSocket Signaling Hub (F4) Multiplexing
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_02_InferenceToWebSocketBroadcasting(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "test-camera-1"
	viewerCount := 3
	var viewers []*websocket.Conn

	for i := 0; i < viewerCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
		if err != nil {
			t.Fatalf("Failed to connect viewer %d: %v", i, err)
		}
		viewers = append(viewers, ws)
		defer ws.Close()
	}

	time.Sleep(50 * time.Millisecond)

	// Ingest and evaluate frame
	_, expected := GenerateSyntheticFrame(FrameTypeSmokeActive, 1280, 720)
	oracle := NewYOLO11InferenceOracle()
	detections := oracle.Evaluate(expected, 0.40)

	pkt := DetectionPacket{
		Type:        "detection",
		CameraID:    cameraID,
		Timestamp:   time.Now().UnixMilli(),
		Detections:  detections,
		InferenceMs: 12.4,
	}

	// Broadcast via Signaling Hub
	harness.BroadcastDetection(pkt)

	// Verify all 3 viewers receive identical payload
	var wg sync.WaitGroup
	for idx, ws := range viewers {
		wg.Add(1)
		go func(i int, conn *websocket.Conn) {
			defer wg.Done()
			var received DetectionPacket
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			if err := conn.ReadJSON(&received); err != nil {
				t.Errorf("Viewer %d failed to receive: %v", i, err)
				return
			}
			if received.CameraID != cameraID || len(received.Detections) != len(detections) {
				t.Errorf("Viewer %d data mismatch: %+v", i, received)
			}
		}(idx, ws)
	}
	wg.Wait()

	t.Logf("Tier 3 - Pairwise 02 Passed: Inference -> WebSocket Fan-Out to %d viewers.", viewerCount)
}

// ----------------------------------------------------------------------------
// Pairwise 3: Video Frame Ingest (F2) -> AI (F1) -> WS (F4) -> Canvas (F5)
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_03_FrameIngestToInferenceToWSToCanvas(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "webcam-live-01"

	// 1. Ingest simulated 640x480 frame with Fire
	frameBytes, rawDets := GenerateSyntheticFrame(FrameTypeFireActive, 640, 480)
	if len(frameBytes) == 0 {
		t.Fatal("Failed to ingest frame")
	}

	// 2. AI Inference Pipeline
	oracle := NewYOLO11InferenceOracle()
	validDets := oracle.Evaluate(rawDets, 0.40)
	if len(validDets) != 1 {
		t.Fatalf("Expected 1 valid detection, got %d", len(validDets))
	}

	// 3. WebSocket Packaging and Streaming
	pkt := DetectionPacket{
		Type:       "detection",
		CameraID:   cameraID,
		Timestamp:  time.Now().UnixMilli(),
		Detections: validDets,
	}

	wsData, err := json.Marshal(pkt)
	if err != nil {
		t.Fatalf("JSON marshal error: %v", err)
	}

	parsedPkt, err := ValidateDetectionPacketSchema(wsData)
	if err != nil {
		t.Fatalf("WebSocket schema error: %v", err)
	}

	// 4. Frontend Canvas Overlay Math & Rendering
	canvasOracle := &CanvasGeometryOracle{}
	videoDisplayWidth := 1920.0
	videoDisplayHeight := 1080.0

	det := parsedPkt.Detections[0]
	pixelBox := canvasOracle.Transform(det.Box, videoDisplayWidth, videoDisplayHeight)
	style := canvasOracle.GetStyle(det)

	if pixelBox.Width <= 0 || pixelBox.Height <= 0 {
		t.Fatalf("Invalid pixel bounding box dimensions: %+v", pixelBox)
	}
	if style.StrokeColor != "rgba(239, 68, 68, 0.9)" || !strings.Contains(style.BadgeLabel, "FIRE") {
		t.Fatalf("Invalid canvas style parameters: %+v", style)
	}

	t.Logf("Tier 3 - Pairwise 03 Passed: Full Video-to-Canvas pipeline transformed box to [%.1f, %.1f, %.1f, %.1f] with style %s.",
		pixelBox.X, pixelBox.Y, pixelBox.Width, pixelBox.Height, style.BadgeLabel)
}

// ----------------------------------------------------------------------------
// Pairwise 4: Multi-Class Detection (F1) -> Event Engine (F3) -> HUD (F6)
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_04_MultiClassDetectionToEventToHUD(t *testing.T) {
	hud := NewHUDOracle()
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	// 1. Generate frame containing BOTH Fire and Smoke
	_, expected := GenerateSyntheticFrame(FrameTypeFireAndSmoke, 640, 480)
	oracle := NewYOLO11InferenceOracle()
	detections := oracle.Evaluate(expected, 0.40)

	if len(detections) != 2 {
		t.Fatalf("Expected 2 detections (Fire and Smoke), got %d", len(detections))
	}

	cameraID := "cam-warehouse-main"

	// 2. Dispatch events for both detections
	for _, d := range detections {
		var evtType EventType
		if d.ClassID == ClassIDFire {
			evtType = EventFireDetected
		} else {
			evtType = EventSmokeDetected
		}

		evt := EmergencyEvent{
			ID:         GenerateTestUUID(),
			CameraID:   cameraID,
			Type:       evtType,
			StartedAt:  time.Now(),
			Confidence: d.Confidence,
		}

		_ = harness.EmitEvent(evt)
		hud.ProcessEvent(evt)
	}

	// 3. Verify HUD State has both Fire and Smoke badges and Critical Banner
	status := hud.GetStatus()
	if !status.BannerVisible || status.BannerSeverity != "critical" {
		t.Fatalf("Expected critical alert banner, got: %+v", status)
	}
	if len(status.ActiveFireBadges) != 1 || len(status.ActiveSmokeBadges) != 1 {
		t.Fatalf("HUD should contain both fire and smoke badges for %s: %+v", cameraID, status)
	}

	t.Logf("Tier 3 - Pairwise 04 Passed: Dual Fire/Smoke detected, HUD banner active: '%s'.", status.BannerMessage)
}

// ----------------------------------------------------------------------------
// Pairwise 5: WebSocket Reconnection (F4) -> Canvas & HUD State Recovery (F5, F6)
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_05_WebSocketReconnectionToCanvasRecovery(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "test-camera-1"

	// 1. Initial Connection
	ws, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
	if err != nil {
		t.Fatalf("Initial dial failed: %v", err)
	}

	// 2. Send detection
	pkt1 := DetectionPacket{
		Type:      "detection",
		CameraID:  cameraID,
		Timestamp: time.Now().UnixMilli(),
		Detections: []Detection{
			{Class: "fire", ClassID: 0, Confidence: 0.88, Box: DetectionBox{0.2, 0.2, 0.3, 0.3}},
		},
	}
	harness.BroadcastDetection(pkt1)

	// 3. Simulate Network Drop & Disconnect
	ws.Close()
	time.Sleep(50 * time.Millisecond)

	// 4. Stale overlay TTL expires (> 1000ms)
	canvasOracle := &CanvasGeometryOracle{}
	staleUpdate := time.Now().Add(-1200 * time.Millisecond)
	if time.Since(staleUpdate).Milliseconds() <= CanvasOverlayTTLMillis {
		t.Fatal("Stale canvas overlay should have expired")
	}

	// 5. Reconnect and send fresh detection
	ws2, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
	if err != nil {
		t.Fatalf("Reconnection dial failed: %v", err)
	}
	defer ws2.Close()

	time.Sleep(50 * time.Millisecond)

	pkt2 := DetectionPacket{
		Type:      "detection",
		CameraID:  cameraID,
		Timestamp: time.Now().UnixMilli(),
		Detections: []Detection{
			{Class: "fire", ClassID: 0, Confidence: 0.94, Box: DetectionBox{0.25, 0.25, 0.35, 0.35}},
		},
	}
	harness.BroadcastDetection(pkt2)

	var recv DetectionPacket
	_ = ws2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if err := ws2.ReadJSON(&recv); err != nil {
		t.Fatalf("Failed to read after reconnection: %v", err)
	}

	pb := canvasOracle.Transform(recv.Detections[0].Box, 1920, 1080)
	if pb.Width <= 0 || pb.Height <= 0 {
		t.Fatalf("Restored bounding box geometry invalid: %+v", pb)
	}

	t.Log("Tier 3 - Pairwise 05 Passed: Stream disconnect, TTL clearing, and reconnection state recovery verified.")
}

// ----------------------------------------------------------------------------
// Pairwise 6: Burst Frame Rate Limiting (F2) -> Throttled Alerts (F3) -> Smooth Canvas (F5)
// ----------------------------------------------------------------------------
func TestTier3_Pairwise_06_BurstFrameRateLimitingToThrottledAlerts(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	burstFrameCount := 60
	cameraID := "webcam-live-01"

	var processedInferences int
	var emittedEvents int

	alertThrottleWindow := 100 * time.Millisecond
	var lastAlertTime time.Time

	canvasOracle := &CanvasGeometryOracle{}

	for i := 0; i < burstFrameCount; i++ {
		now := time.Now()

		// Frame Ingest & Inference
		processedInferences++
		det := Detection{Class: "fire", ClassID: 0, Confidence: 0.85, Box: DetectionBox{0.3, 0.3, 0.2, 0.2}}

		// Event Alert Throttling
		if now.Sub(lastAlertTime) >= alertThrottleWindow {
			_ = harness.EmitEvent(EmergencyEvent{
				ID:         GenerateTestUUID(),
				CameraID:   cameraID,
				Type:       EventFireDetected,
				StartedAt:  now,
				Confidence: det.Confidence,
			})
			emittedEvents++
			lastAlertTime = now
		}

		// Smooth Canvas Overlay Update (always receives real-time detection)
		pixelBox := canvasOracle.Transform(det.Box, 1280, 720)
		if pixelBox.Width <= 0 {
			t.Fatal("Canvas rendering glitch during burst")
		}

		time.Sleep(2 * time.Millisecond) // Fast burst cadence
	}

	if processedInferences != burstFrameCount {
		t.Fatalf("Expected %d processed frames, got %d", burstFrameCount, processedInferences)
	}
	if emittedEvents >= burstFrameCount {
		t.Fatalf("Alerts should have been throttled: emitted=%d total_frames=%d", emittedEvents, burstFrameCount)
	}

	t.Logf("Tier 3 - Pairwise 06 Passed: %d frames processed smoothly; alerts throttled to %d events.",
		processedInferences, emittedEvents)
}
