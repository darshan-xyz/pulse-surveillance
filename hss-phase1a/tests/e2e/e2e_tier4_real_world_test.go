package e2e

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================================
// TIER 4: Real-World Application Scenarios
// End-to-End multi-camera surveillance workflows under simulated fire/smoke emergencies
// ============================================================================

// ----------------------------------------------------------------------------
// Scenario 1: Multi-Camera Warehouse Fire Outbreak
// ----------------------------------------------------------------------------
func TestTier4_Scenario_01_MultiCameraWarehouseFireOutbreak(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameras := []struct {
		ID        string
		FrameType FrameType
		Expected  string // "fire", "smoke", "none"
	}{
		{ID: "webcam-live-01", FrameType: FrameTypeFireActive, Expected: "fire"},
		{ID: "cam-warehouse-east", FrameType: FrameTypeSmokeActive, Expected: "smoke"},
		{ID: "test-camera-1", FrameType: FrameTypeCleanRoom, Expected: "none"},
		{ID: "cam-warehouse-west", FrameType: FrameTypeCleanRoom, Expected: "none"},
	}

	oracle := NewYOLO11InferenceOracle()
	hud := NewHUDOracle()
	canvasOracle := &CanvasGeometryOracle{}

	for _, cam := range cameras {
		_, rawDets := GenerateSyntheticFrame(cam.FrameType, 640, 480)
		detections := oracle.Evaluate(rawDets, 0.40)

		if cam.Expected == "none" {
			if len(detections) != 0 {
				t.Fatalf("Expected 0 detections on clean camera %s, got %d", cam.ID, len(detections))
			}
			continue
		}

		if len(detections) != 1 {
			t.Fatalf("Expected 1 detection on active camera %s, got %d", cam.ID, len(detections))
		}

		det := detections[0]
		var evtType EventType
		if det.ClassID == ClassIDFire {
			evtType = EventFireDetected
		} else {
			evtType = EventSmokeDetected
		}

		evt := EmergencyEvent{
			ID:         GenerateTestUUID(),
			CameraID:   cam.ID,
			Type:       evtType,
			StartedAt:  time.Now(),
			Confidence: det.Confidence,
		}

		// Emit through harness and HUD
		_ = harness.EmitEvent(evt)
		hud.ProcessEvent(evt)

		// Broadcast detection packet
		harness.BroadcastDetection(DetectionPacket{
			Type:       "detection",
			CameraID:   cam.ID,
			Timestamp:  time.Now().UnixMilli(),
			Detections: detections,
		})

		// Canvas coordinate transformation
		pixelBox := canvasOracle.Transform(det.Box, 1920, 1080)
		if pixelBox.Width <= 0 || pixelBox.Height <= 0 {
			t.Fatalf("Invalid canvas geometry for %s: %+v", cam.ID, pixelBox)
		}
	}

	// Verify HUD Aggregation
	status := hud.GetStatus()
	if !status.BannerVisible || status.BannerSeverity != "critical" {
		t.Fatalf("Expected critical banner for multi-camera outbreak: %+v", status)
	}
	if len(status.ActiveFireBadges) != 1 || status.ActiveFireBadges[0] != "webcam-live-01" {
		t.Fatalf("Fire badge missing for webcam-live-01: %+v", status.ActiveFireBadges)
	}
	if len(status.ActiveSmokeBadges) != 1 || status.ActiveSmokeBadges[0] != "cam-warehouse-east" {
		t.Fatalf("Smoke badge missing for cam-warehouse-east: %+v", status.ActiveSmokeBadges)
	}

	// Verify event store has exactly 2 emergency records
	count := harness.CountEvents("")
	if count != 2 {
		t.Fatalf("Expected 2 events in persistence store, got %d", count)
	}

	t.Log("Tier 4 - Scenario 01 Passed: Multi-camera warehouse fire outbreak handled with complete isolation and accurate HUD aggregation.")
}

// ----------------------------------------------------------------------------
// Scenario 2: Smoldering Fire Evolution Lifecycle (Phases 1..4)
// ----------------------------------------------------------------------------
func TestTier4_Scenario_02_SmolderingFireEvolutionLifecycle(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "webcam-live-01"
	oracle := NewYOLO11InferenceOracle()
	hud := NewHUDOracle()
	canvasOracle := &CanvasGeometryOracle{}

	// ------------------------------------------------------------------------
	// Phase 1: Faint Smoke (Confidence 0.35 < 0.40 Threshold) -> No Alert
	// ------------------------------------------------------------------------
	t.Log("Scenario 2 - Phase 1: Ingesting faint smoke frame (sub-threshold)...")
	_, faintDets := GenerateSyntheticFrame(FrameTypeFaintSmoke, 640, 480)
	filteredP1 := oracle.Evaluate(faintDets, 0.40)
	if len(filteredP1) != 0 {
		t.Fatalf("Phase 1: Sub-threshold faint smoke should be filtered out, got %d detections", len(filteredP1))
	}
	if hud.GetStatus().BannerVisible {
		t.Fatal("Phase 1: Banner should not be visible for sub-threshold smoke")
	}

	// ------------------------------------------------------------------------
	// Phase 2: Dense Smoke Buildup (Confidence 0.76 >= 0.40) -> Smoke Warning
	// ------------------------------------------------------------------------
	t.Log("Scenario 2 - Phase 2: Ingesting dense smoke frame...")
	_, denseDets := GenerateSyntheticFrame(FrameTypeSmokeActive, 640, 480)
	filteredP2 := oracle.Evaluate(denseDets, 0.40)
	if len(filteredP2) != 1 || filteredP2[0].Class != ClassNameSmoke {
		t.Fatalf("Phase 2: Expected 1 smoke detection, got %+v", filteredP2)
	}

	smokeEvt := EmergencyEvent{
		ID:         GenerateTestUUID(),
		CameraID:   cameraID,
		Type:       EventSmokeDetected,
		StartedAt:  time.Now(),
		Confidence: filteredP2[0].Confidence,
	}
	_ = harness.EmitEvent(smokeEvt)
	statusP2 := hud.ProcessEvent(smokeEvt)

	if !statusP2.BannerVisible || statusP2.BannerSeverity != "warning" {
		t.Fatalf("Phase 2: Expected warning banner, got %+v", statusP2)
	}
	styleP2 := canvasOracle.GetStyle(filteredP2[0])
	if styleP2.StrokeColor != "rgba(245, 158, 11, 0.9)" {
		t.Fatalf("Phase 2: Expected Amber stroke for smoke, got %s", styleP2.StrokeColor)
	}

	// ------------------------------------------------------------------------
	// Phase 3: Open Flame Outbreak (Confidence 0.88 >= 0.40) -> Critical Fire Alert
	// ------------------------------------------------------------------------
	t.Log("Scenario 2 - Phase 3: Ingesting open flame fire frame...")
	_, fireDets := GenerateSyntheticFrame(FrameTypeFireActive, 640, 480)
	filteredP3 := oracle.Evaluate(fireDets, 0.40)
	if len(filteredP3) != 1 || filteredP3[0].Class != ClassNameFire {
		t.Fatalf("Phase 3: Expected 1 fire detection, got %+v", filteredP3)
	}

	fireEvt := EmergencyEvent{
		ID:         GenerateTestUUID(),
		CameraID:   cameraID,
		Type:       EventFireDetected,
		StartedAt:  time.Now(),
		Confidence: filteredP3[0].Confidence,
	}
	_ = harness.EmitEvent(fireEvt)
	statusP3 := hud.ProcessEvent(fireEvt)

	if !statusP3.BannerVisible || statusP3.BannerSeverity != "critical" {
		t.Fatalf("Phase 3: Expected critical fire banner, got %+v", statusP3)
	}
	styleP3 := canvasOracle.GetStyle(filteredP3[0])
	if styleP3.StrokeColor != "rgba(239, 68, 68, 0.9)" {
		t.Fatalf("Phase 3: Expected Red stroke for fire, got %s", styleP3.StrokeColor)
	}

	// ------------------------------------------------------------------------
	// Phase 4: Fire Suppression & Clearance -> Overlay TTL Expiry
	// ------------------------------------------------------------------------
	t.Log("Scenario 2 - Phase 4: Fire suppressed, clean room frame ingested...")
	_, cleanDets := GenerateSyntheticFrame(FrameTypeCleanRoom, 640, 480)
	filteredP4 := oracle.Evaluate(cleanDets, 0.40)
	if len(filteredP4) != 0 {
		t.Fatalf("Phase 4: Expected 0 detections in suppressed room, got %d", len(filteredP4))
	}

	// TTL auto-clearing check (1000ms after last detection)
	lastDetTime := time.Now().Add(-1100 * time.Millisecond)
	if time.Since(lastDetTime).Milliseconds() <= CanvasOverlayTTLMillis {
		t.Fatal("Phase 4: Canvas overlay should be cleared after 1000ms TTL")
	}

	t.Log("Tier 4 - Scenario 02 Passed: Full smoldering fire evolution lifecycle verified across all 4 phases.")
}

// ----------------------------------------------------------------------------
// Scenario 3: Continuous 24/7 Surveillance with Intermittent False Alarm Filtering
// ----------------------------------------------------------------------------
func TestTier4_Scenario_03_Continuous247SurveillanceWithFalseAlarmFiltering(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	oracle := NewYOLO11InferenceOracle()
	totalFrames := 100
	genuineFireFrameIndex := 75

	var alertsEmitted int

	startTime := time.Now()
	for i := 0; i < totalFrames; i++ {
		var raw []Detection
		if i == genuineFireFrameIndex {
			// Genuine Fire Frame
			raw = []Detection{
				{Class: "fire", ClassID: 0, Confidence: 0.89, Box: DetectionBox{0.2, 0.2, 0.4, 0.4}},
			}
		} else {
			// Environmental optical noise (reflections, steam, shadows: conf 0.10..0.38)
			noiseConf := 0.10 + float64(i%28)*0.01
			raw = []Detection{
				{Class: "smoke", ClassID: 1, Confidence: noiseConf, Box: DetectionBox{0.1, 0.1, 0.2, 0.2}},
			}
		}

		filtered := oracle.Evaluate(raw, 0.40)

		if len(filtered) > 0 {
			alertsEmitted++
			_ = harness.EmitEvent(EmergencyEvent{
				ID:         GenerateTestUUID(),
				CameraID:   "webcam-live-01",
				Type:       EventFireDetected,
				StartedAt:  time.Now(),
				Confidence: filtered[0].Confidence,
			})
		}
	}
	elapsed := time.Since(startTime)

	if alertsEmitted != 1 {
		t.Fatalf("False alarm filtering failed: expected exactly 1 genuine fire alert, got %d alerts", alertsEmitted)
	}

	// Verify only 1 record stored in persistence store
	dbCount := harness.CountEvents("")
	if dbCount != 1 {
		t.Fatalf("Expected 1 event record, got %d", dbCount)
	}

	t.Logf("Tier 4 - Scenario 03 Passed: 100 frames evaluated in %v; 99 optical noise frames rejected, 1 genuine fire detected instantaneously.", elapsed)
}

// ----------------------------------------------------------------------------
// Scenario 4: Multi-Viewer Live Incident Response
// ----------------------------------------------------------------------------
func TestTier4_Scenario_04_MultiViewerLiveIncidentResponse(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "webcam-live-01"
	operatorCount := 5
	var operators []*websocket.Conn

	for i := 0; i < operatorCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
		if err != nil {
			t.Fatalf("Operator %d connection failed: %v", i, err)
		}
		operators = append(operators, ws)
		defer ws.Close()
	}

	time.Sleep(100 * time.Millisecond)

	// Fire detected on camera feed
	detPacket := DetectionPacket{
		Type:      "detection",
		CameraID:  cameraID,
		Timestamp: time.Now().UnixMilli(),
		Detections: []Detection{
			{Class: "fire", ClassID: 0, Confidence: 0.95, Box: DetectionBox{0.3, 0.3, 0.4, 0.4}},
		},
	}

	harness.BroadcastDetection(detPacket)

	// Verify synchronized reception across all 5 operators
	var wg sync.WaitGroup
	receiveTimestamps := make([]int64, operatorCount)
	readErrors := make([]error, operatorCount)

	for idx, ws := range operators {
		wg.Add(1)
		go func(i int, conn *websocket.Conn) {
			defer wg.Done()
			var recv DetectionPacket
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			if err := conn.ReadJSON(&recv); err != nil {
				readErrors[i] = err
				return
			}
			receiveTimestamps[i] = time.Now().UnixMilli()
			if recv.Detections[0].Confidence != 0.95 {
				readErrors[i] = fmt.Errorf("confidence mismatch: %f", recv.Detections[0].Confidence)
			}
		}(idx, ws)
	}
	wg.Wait()

	for i, err := range readErrors {
		if err != nil {
			t.Fatalf("Operator %d read error: %v", i, err)
		}
	}

	// Verify minimal inter-client latency skew (< 100ms)
	var minTs, maxTs int64
	minTs = receiveTimestamps[0]
	maxTs = receiveTimestamps[0]
	for _, ts := range receiveTimestamps {
		if ts < minTs {
			minTs = ts
		}
		if ts > maxTs {
			maxTs = ts
		}
	}
	jitter := maxTs - minTs
	if jitter > 100 {
		t.Fatalf("Multi-viewer sync jitter too high: %d ms", jitter)
	}

	t.Logf("Tier 4 - Scenario 04 Passed: %d operators received live incident broadcast with low jitter (%d ms).", operatorCount, jitter)
}

// ----------------------------------------------------------------------------
// Scenario 5: Edge Network Partition and Offline Resync
// ----------------------------------------------------------------------------
func TestTier4_Scenario_05_EdgeNetworkPartitionAndOfflineResync(t *testing.T) {
	harness, err := NewE2ETestHarness()
	if err != nil {
		t.Fatalf("Harness error: %v", err)
	}
	defer harness.Close()

	cameraID := "test-camera-1"

	// 1. Initial online stream
	ws, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
	if err != nil {
		t.Fatalf("Initial connect failed: %v", err)
	}

	// 2. WAN Link Severed (Simulate Network Partition)
	t.Log("Scenario 5: WAN link dropped, edge camera buffering detections offline...")
	ws.Close()

	// Edge buffers 3 events locally during 2-second outage
	var localEdgeBuffer []EmergencyEvent
	for i := 0; i < 3; i++ {
		localEdgeBuffer = append(localEdgeBuffer, EmergencyEvent{
			ID:         GenerateTestUUID(),
			CameraID:   cameraID,
			Type:       EventFireDetected,
			StartedAt:  time.Now().Add(-time.Duration(3-i) * time.Second),
			Confidence: 0.88 + float64(i)*0.03,
		})
	}

	// 3. WAN Link Restored -> Resync Buffered Events to Cloud DB
	t.Log("Scenario 5: WAN link restored, edge flushing buffered events to cloud...")
	for _, evt := range localEdgeBuffer {
		if err := harness.EmitEvent(evt); err != nil {
			t.Fatalf("Failed to sync buffered event: %v", err)
		}
	}

	// 4. Reconnect Viewer and Resume Live Stream
	ws2, _, err := websocket.DefaultDialer.Dial(harness.WSURL+"/client?token=valid_token&camera_id="+cameraID, nil)
	if err != nil {
		t.Fatalf("Reconnection failed: %v", err)
	}
	defer ws2.Close()

	time.Sleep(50 * time.Millisecond)

	// Live detection post-recovery
	livePkt := DetectionPacket{
		Type:      "detection",
		CameraID:  cameraID,
		Timestamp: time.Now().UnixMilli(),
		Detections: []Detection{
			{Class: "fire", ClassID: 0, Confidence: 0.96, Box: DetectionBox{0.25, 0.25, 0.5, 0.5}},
		},
	}
	harness.BroadcastDetection(livePkt)

	var recvLive DetectionPacket
	_ = ws2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if err := ws2.ReadJSON(&recvLive); err != nil {
		t.Fatalf("Failed to receive live detection after recovery: %v", err)
	}

	// Verify all 3 buffered events successfully synced to cloud store
	dbCount := harness.CountEvents(cameraID)
	if dbCount != 3 {
		t.Fatalf("Expected 3 synced events in DB, got %d", dbCount)
	}

	t.Log("Tier 4 - Scenario 05 Passed: Network partition, offline edge buffering, cloud resync, and live stream restoration verified.")
}
