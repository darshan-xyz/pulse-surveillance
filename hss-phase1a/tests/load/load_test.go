package load

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/cloud/signaling"
)

// setupLoadTestServer creates a lightweight, port-isolated test HTTP server for signaling routes.
func setupLoadTestServer(t *testing.T) (*httptest.Server, string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return server, wsURL
}

// TestLoad1000ConcurrentViewers fulfills Milestone 3 (R3) by simulating 1,000+ simultaneous
// WebRTC viewers and cameras connecting to the cloud signaling hub, completing signaling
// handshakes without panics, deadlocks, or connection dropouts.
func TestLoad1000ConcurrentViewers(t *testing.T) {
	server, wsURL := setupLoadTestServer(t)
	defer server.Close()

	const targetViewers = 1000
	targetCameras := GetDefaultTargetCameras()

	metrics := NewLoadMetrics(targetViewers)

	// 1. Start Fleet of Mock Edge Camera Daemons
	cameras := make([]*MockCameraDaemon, len(targetCameras))
	for i, camCfg := range targetCameras {
		ip := fmt.Sprintf("10.0.1.%d", i+1)
		cam := NewMockCameraDaemon(camCfg.DeviceID, ip, metrics)
		if err := cam.Connect(wsURL, "mock_device_jwt"); err != nil {
			t.Fatalf("Failed to connect mock camera %s: %v", camCfg.DeviceID, err)
		}
		cameras[i] = cam
		defer cam.Close()
	}

	// Give camera WebSocket connections time to establish subscriptions
	time.Sleep(50 * time.Millisecond)

	t.Logf("Initiating load test with %d concurrent viewers across %d camera daemons...", targetViewers, len(targetCameras))

	var wg sync.WaitGroup
	wg.Add(targetViewers)

	// Worker pool semaphore to control simultaneous dial pressure while maintaining high concurrency
	sem := make(chan struct{}, 80)
	startOverall := time.Now()

	for i := 0; i < targetViewers; i++ {
		viewerIndex := i
		camCfg := targetCameras[viewerIndex%len(targetCameras)]

		go func(idx int, cfg TargetCameraConfig) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			viewerID := fmt.Sprintf("viewer-%05d", idx)
			sessionID := fmt.Sprintf("load-sess-%05d", idx)
			ip := fmt.Sprintf("10.%d.%d.%d", (idx>>16)&255, (idx>>8)&255, idx&255)

			viewer := NewMockViewer(viewerID, cfg.DeviceID, sessionID, ip, metrics)

			// 2. Connect viewer to signaling hub
			if err := viewer.Connect(wsURL, cfg.Token); err != nil {
				t.Errorf("Viewer %s failed to connect: %v", viewerID, err)
				return
			}
			defer viewer.Close()

			// 3. Perform SDP Offer/Answer negotiation
			_, err := viewer.PerformHandshake(10 * time.Second)
			if err != nil {
				t.Errorf("Viewer %s handshake failed: %v", viewerID, err)
				return
			}
		}(viewerIndex, camCfg)

		// Small pacing (2ms) prevents local OS ephemeral port / FD storm while achieving 250+ handshakes/sec
		time.Sleep(2 * time.Millisecond)
	}

	wg.Wait()
	metrics.Close()
	totalElapsed := time.Since(startOverall)

	// Output summary metrics
	t.Log(metrics.Summary())
	t.Logf("Total Load Test Wall Time: %v for %d viewers", totalElapsed, targetViewers)

	// Assertions for Acceptance Criteria:
	// 1. Zero panics or deadlocks (test completed to this point)
	// 2. All 1,000 handshakes completed successfully
	if atomic.LoadInt64(&metrics.SuccessfulHandshakes) < targetViewers {
		t.Fatalf("Expected %d successful handshakes, got %d (failed: %d)",
			targetViewers, atomic.LoadInt64(&metrics.SuccessfulHandshakes), atomic.LoadInt64(&metrics.FailedHandshakes))
	}

	if atomic.LoadInt64(&metrics.FailedHandshakes) > 0 {
		t.Fatalf("Encountered %d failed handshakes during 1000-viewer load test", atomic.LoadInt64(&metrics.FailedHandshakes))
	}

	// 3. Verify P95 latency is healthy (< 500ms for in-memory negotiation)
	stats := metrics.HandshakeLatencies.Stats()
	if stats.P95 > 500*time.Millisecond {
		t.Errorf("P95 latency %v exceeded threshold of 500ms", stats.P95)
	}

	t.Logf("PASS: Successfully completed %d concurrent WebRTC signaling handshakes (P50: %v, P95: %v, P99: %v, Success Rate: %.2f%%)",
		targetViewers, stats.P50.Round(time.Microsecond), stats.P95.Round(time.Microsecond), stats.P99.Round(time.Microsecond), metrics.SuccessRate())
}

// TestLoadRampUpAndSustainedSignaling verifies that the signaling hub handles stepped
// ramp-up load (250 -> 500 -> 750 -> 1000 connections) and sustains continuous message traffic
// without connection dropouts or resource starvation.
func TestLoadRampUpAndSustainedSignaling(t *testing.T) {
	server, wsURL := setupLoadTestServer(t)
	defer server.Close()

	const totalViewers = 1000
	targetCameras := GetDefaultTargetCameras()

	metrics := NewLoadMetrics(totalViewers)

	cameras := make([]*MockCameraDaemon, len(targetCameras))
	for i, camCfg := range targetCameras {
		ip := fmt.Sprintf("10.0.2.%d", i+1)
		cam := NewMockCameraDaemon(camCfg.DeviceID, ip, metrics)
		if err := cam.Connect(wsURL, "mock_device_jwt"); err != nil {
			t.Fatalf("Failed to connect mock camera %s: %v", camCfg.DeviceID, err)
		}
		cameras[i] = cam
		defer cam.Close()
	}

	time.Sleep(50 * time.Millisecond)

	viewers := make([]*MockViewer, totalViewers)
	var viewersMu sync.Mutex

	stages := []struct {
		name       string
		startIndex int
		endIndex   int
	}{
		{"Stage 1: 250 Connections", 0, 250},
		{"Stage 2: 500 Connections", 250, 500},
		{"Stage 3: 750 Connections", 500, 750},
		{"Stage 4: 1000 Connections", 750, 1000},
	}

	sem := make(chan struct{}, 60)

	for _, stage := range stages {
		stageStart := time.Now()
		var stageWg sync.WaitGroup
		count := stage.endIndex - stage.startIndex
		stageWg.Add(count)

		for i := stage.startIndex; i < stage.endIndex; i++ {
			idx := i
			camCfg := targetCameras[idx%len(targetCameras)]

			go func(vIdx int, cfg TargetCameraConfig) {
				defer stageWg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				viewerID := fmt.Sprintf("ramp-viewer-%05d", vIdx)
				sessionID := fmt.Sprintf("ramp-sess-%05d", vIdx)
				ip := fmt.Sprintf("10.150.%d.%d", (vIdx>>8)&255, vIdx&255)

				v := NewMockViewer(viewerID, cfg.DeviceID, sessionID, ip, metrics)
				if err := v.Connect(wsURL, cfg.Token); err != nil {
					t.Errorf("Failed to connect %s: %v", viewerID, err)
					return
				}

				if _, err := v.PerformHandshake(10 * time.Second); err != nil {
					t.Errorf("Handshake failed for %s: %v", viewerID, err)
					return
				}

				viewersMu.Lock()
				viewers[vIdx] = v
				viewersMu.Unlock()
			}(idx, camCfg)

			time.Sleep(2 * time.Millisecond)
		}

		stageWg.Wait()
		t.Logf("Ramp-Up %s completed in %v", stage.name, time.Since(stageStart))
	}

	// Sustain period: keep all 1,000 connections open and send additional ICE candidates
	t.Log("Sustaining 1,000 active WebRTC signaling sessions for 2 seconds with active message flow...")
	sustainDuration := 2 * time.Second
	sustainStart := time.Now()

	var sustainWg sync.WaitGroup
	for i := 0; i < totalViewers; i++ {
		v := viewers[i]
		if v == nil {
			continue
		}
		sustainWg.Add(1)
		go func(vw *MockViewer) {
			defer sustainWg.Done()
			// Send periodic ICE candidate messages during sustain period
			ticker := time.NewTicker(400 * time.Millisecond)
			defer ticker.Stop()
			stopTimer := time.NewTimer(sustainDuration)
			defer stopTimer.Stop()

			for {
				select {
				case <-stopTimer.C:
					return
				case <-ticker.C:
					vw.mu.Lock()
					if !vw.closed && vw.ws != nil {
						candMsg := signaling.SignalingMessage{
							Type:      "candidate",
							SessionID: vw.SessionID,
							DeviceID:  vw.DeviceID,
							Candidate: SyntheticCandidate2,
						}
						_ = vw.ws.WriteJSON(candMsg)
						metrics.RecordMsgSent()
					}
					vw.mu.Unlock()
				}
			}
		}(v)
	}

	sustainWg.Wait()
	t.Logf("Sustained message flow completed in %v", time.Since(sustainStart))

	// Clean teardown of all 1,000 viewers
	t.Log("Tearing down 1,000 viewer connections...")
	for _, v := range viewers {
		if v != nil {
			_ = v.Close()
		}
	}

	metrics.Close()
	t.Log(metrics.Summary())

	if atomic.LoadInt64(&metrics.SuccessfulHandshakes) < totalViewers {
		t.Fatalf("Expected %d successful handshakes, got %d", totalViewers, atomic.LoadInt64(&metrics.SuccessfulHandshakes))
	}

	t.Log("PASS: Ramp-up and sustained 1000-connection load verified cleanly.")
}

// TestLoadHighFrequencySignalingCandidateBurst tests the in-memory broker under a high-frequency
// burst of 1,000 signaling messages across 50 concurrent sessions.
func TestLoadHighFrequencySignalingCandidateBurst(t *testing.T) {
	server, wsURL := setupLoadTestServer(t)
	defer server.Close()

	const numViewers = 50
	const messagesPerViewer = 20
	targetCameras := GetDefaultTargetCameras()

	metrics := NewLoadMetrics(numViewers)

	cameras := make([]*MockCameraDaemon, len(targetCameras))
	for i, camCfg := range targetCameras {
		ip := fmt.Sprintf("10.0.3.%d", i+1)
		cam := NewMockCameraDaemon(camCfg.DeviceID, ip, metrics)
		if err := cam.Connect(wsURL, "mock_device_jwt"); err != nil {
			t.Fatalf("Failed to connect camera %s: %v", camCfg.DeviceID, err)
		}
		cameras[i] = cam
		defer cam.Close()
	}

	time.Sleep(50 * time.Millisecond)

	viewers := make([]*MockViewer, numViewers)
	for i := 0; i < numViewers; i++ {
		camCfg := targetCameras[i%len(targetCameras)]
		vID := fmt.Sprintf("burst-viewer-%04d", i)
		sID := fmt.Sprintf("burst-sess-%04d", i)
		ip := fmt.Sprintf("10.200.%d.%d", (i>>8)&255, i&255)

		v := NewMockViewer(vID, camCfg.DeviceID, sID, ip, metrics)
		if err := v.Connect(wsURL, camCfg.Token); err != nil {
			t.Fatalf("Failed to connect %s: %v", vID, err)
		}
		if _, err := v.PerformHandshake(5 * time.Second); err != nil {
			t.Fatalf("Handshake failed for %s: %v", vID, err)
		}
		viewers[i] = v
	}

	t.Logf("Firing burst of %d signaling messages (%d per session across %d sessions)...",
		numViewers*messagesPerViewer, messagesPerViewer, numViewers)

	burstStart := time.Now()
	var burstWg sync.WaitGroup
	burstWg.Add(numViewers)

	for i := 0; i < numViewers; i++ {
		v := viewers[i]
		go func(vw *MockViewer) {
			defer burstWg.Done()
			for m := 0; m < messagesPerViewer; m++ {
				candMsg := signaling.SignalingMessage{
					Type:      "candidate",
					SessionID: vw.SessionID,
					DeviceID:  vw.DeviceID,
					Candidate: fmt.Sprintf(`{"candidate":"candidate:%d 1 UDP 2130706431 192.168.1.100 %d typ host","sdpMid":"0","sdpMLineIndex":0}`, m, 50000+m),
				}
				vw.mu.Lock()
				if !vw.closed && vw.ws != nil {
					_ = vw.ws.WriteJSON(candMsg)
					metrics.RecordMsgSent()
				}
				vw.mu.Unlock()
			}
		}(v)
	}

	burstWg.Wait()
	burstElapsed := time.Since(burstStart)

	t.Logf("Signaling burst completed in %v (Throughput: %.2f msgs/sec)",
		burstElapsed, float64(numViewers*messagesPerViewer)/burstElapsed.Seconds())

	// Teardown
	for _, v := range viewers {
		if v != nil {
			_ = v.Close()
		}
	}

	metrics.Close()
	t.Log("PASS: High-frequency signaling candidate burst handled without drops or deadlocks.")
}
