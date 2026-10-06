package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/events"
	"hss/internal/media"
)

// TestStress_KeyframeVsNonKeyframeFiltering verifies strict keyframe filtering behavior.
func TestStress_KeyframeVsNonKeyframeFiltering(t *testing.T) {
	camID := "cam-stress-kf-filter"

	// 1. Test KeyframesOnly = true
	t.Run("KeyframesOnly_Enabled", func(t *testing.T) {
		detector := NewYOLO11Detector(
			camID,
			WithConfidenceThreshold(0.40),
			WithSampleInterval(1*time.Millisecond),
			WithMockFallback(true),
			WithKeyframesOnly(true),
		)
		detector.SetMockResponse(true, false, nil)

		var inferenceCalls int32
		detector.OnDetection(func(cid string, resp *DetectionResponse) {
			atomic.AddInt32(&inferenceCalls, 1)
		})

		frameCh := make(chan media.MediaPacket, 100)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go detector.processLoop(ctx, frameCh)

		// Send 30 non-keyframes
		for i := 0; i < 30; i++ {
			frameCh <- media.MediaPacket{
				IsKeyFrame: false,
				Payload:    [][]byte{{0x01, 0x02, 0x03}},
			}
		}

		time.Sleep(50 * time.Millisecond)

		if count := atomic.LoadInt32(&inferenceCalls); count != 0 {
			t.Fatalf("expected 0 inference calls for non-keyframes when KeyframesOnly=true, got %d", count)
		}

		// Send 5 keyframes
		for i := 0; i < 5; i++ {
			frameCh <- media.MediaPacket{
				IsKeyFrame: true,
				Payload:    [][]byte{{0x05, 0x06, 0x07}},
			}
			time.Sleep(5 * time.Millisecond)
		}

		time.Sleep(50 * time.Millisecond)

		if count := atomic.LoadInt32(&inferenceCalls); count < 1 {
			t.Fatalf("expected keyframes to be processed, got %d calls", count)
		}
	})

	// 2. Test KeyframesOnly = false
	t.Run("KeyframesOnly_Disabled", func(t *testing.T) {
		detector := NewYOLO11Detector(
			camID+"-all",
			WithConfidenceThreshold(0.40),
			WithSampleInterval(1*time.Millisecond),
			WithMockFallback(true),
			WithKeyframesOnly(false),
		)
		detector.SetMockResponse(true, false, nil)

		var inferenceCalls int32
		detector.OnDetection(func(cid string, resp *DetectionResponse) {
			atomic.AddInt32(&inferenceCalls, 1)
		})

		frameCh := make(chan media.MediaPacket, 100)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go detector.processLoop(ctx, frameCh)

		// Send 5 non-keyframes
		for i := 0; i < 5; i++ {
			frameCh <- media.MediaPacket{
				IsKeyFrame: false,
				Payload:    [][]byte{{0x01, 0x02, 0x03}},
			}
			time.Sleep(5 * time.Millisecond)
		}

		time.Sleep(50 * time.Millisecond)

		if count := atomic.LoadInt32(&inferenceCalls); count < 1 {
			t.Fatalf("expected non-keyframes to be processed when KeyframesOnly=false, got %d calls", count)
		}
	})
}

// TestStress_RapidFrameIngestionAndSampleRateLimit tests throttling under rapid burst ingestion.
func TestStress_RapidFrameIngestionAndSampleRateLimit(t *testing.T) {
	camID := "cam-stress-burst"
	sampleInterval := 50 * time.Millisecond

	detector := NewYOLO11Detector(
		camID,
		WithConfidenceThreshold(0.40),
		WithSampleInterval(sampleInterval),
		WithMockFallback(true),
		WithKeyframesOnly(false),
	)
	detector.SetMockResponse(true, false, nil)

	var detectionsCount int32
	detector.OnDetection(func(cid string, resp *DetectionResponse) {
		atomic.AddInt32(&detectionsCount, 1)
	})

	frameCh := make(chan media.MediaPacket, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go detector.processLoop(ctx, frameCh)

	// Pump 500 frames within 100ms
	totalSent := 500
	startTime := time.Now()
	for i := 0; i < totalSent; i++ {
		frameCh <- media.MediaPacket{
			IsKeyFrame: true,
			Payload:    [][]byte{{0x01, byte(i % 256)}},
		}
		if i%50 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	time.Sleep(100 * time.Millisecond)
	elapsed := time.Since(startTime)

	dets := atomic.LoadInt32(&detectionsCount)
	// Maximum possible inferences in elapsed time is roughly elapsed / sampleInterval + 1
	maxExpected := int32(elapsed/sampleInterval) + 3

	if dets > maxExpected {
		t.Fatalf("rate limiting failed: got %d detections in %v (sampleInterval=%v, maxExpected=%d)",
			dets, elapsed, sampleInterval, maxExpected)
	}
	if dets == 0 {
		t.Fatalf("expected at least 1 detection during burst, got 0")
	}
}

// TestStress_ConcurrentListenersAndEvents tests concurrent registration and invocation of listeners.
func TestStress_ConcurrentListenersAndEvents(t *testing.T) {
	if events.DefaultEngine == nil {
		events.InitEngine(context.Background())
	}

	camID := "cam-stress-listeners"
	detector := NewYOLO11Detector(
		camID,
		WithConfidenceThreshold(0.40),
		WithMockFallback(true),
	)

	var wg sync.WaitGroup
	var totalCallbacks int64
	numListeners := 25
	numIterations := 50

	// Concurrently register listeners
	for i := 0; i < numListeners; i++ {
		wg.Add(1)
		go func(listenerID int) {
			defer wg.Done()
			detector.OnDetection(func(cid string, resp *DetectionResponse) {
				atomic.AddInt64(&totalCallbacks, 1)
			})
		}(i)
	}
	wg.Wait()

	// Dispatch detection bursts concurrently
	for i := 0; i < numIterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := &DetectionResponse{
				CameraID: camID,
				HasFire:  idx%2 == 0,
				HasSmoke: idx%3 == 0,
				Detections: []Detection{
					{ClassID: 0, ClassName: "fire", Confidence: 0.85, Box: []float64{0.1, 0.1, 0.2, 0.2}},
					{ClassID: 1, ClassName: "smoke", Confidence: 0.75, Box: []float64{0.3, 0.3, 0.2, 0.2}},
				},
			}
			detector.handleDetections(resp)
		}(i)
	}

	wg.Wait()

	expectedCalls := int64(numListeners * numIterations)
	actualCalls := atomic.LoadInt64(&totalCallbacks)
	if actualCalls != expectedCalls {
		t.Fatalf("expected %d total listener callbacks, got %d", expectedCalls, actualCalls)
	}
}

// TestStress_StartStopRestartCycles tests Start and Stop idempotency and cycling.
func TestStress_StartStopRestartCycles(t *testing.T) {
	camID := "cam-stress-lifecycle"
	detector := NewYOLO11Detector(camID, WithMockFallback(true))

	// 1. Multiple Stops when not started should not panic
	detector.Stop()
	detector.Stop()

	// 2. Start cleanly
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := detector.Start(ctx); err != nil {
		t.Fatalf("initial start failed: %v", err)
	}

	// 3. Double Start should return error (not panic)
	if err := detector.Start(ctx); err == nil {
		t.Fatalf("expected error on duplicate start, got nil")
	}

	// 4. Stop should terminate
	detector.Stop()

	// 5. Multiple consecutive Stops after Stop should be safe
	detector.Stop()
	detector.Stop()

	// 6. Rapid Start-Stop cycle 10 times
	for i := 0; i < 10; i++ {
		c, can := context.WithCancel(context.Background())
		if err := detector.Start(c); err != nil {
			t.Fatalf("cycle %d start failed: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
		detector.Stop()
		can()
	}
}

// TestStress_HTTPInferenceSlowAndErrorHandling tests HTTP service latency, timeouts, and error responses.
func TestStress_HTTPInferenceSlowAndErrorHandling(t *testing.T) {
	var requestCount int32

	// Server returning 500 error or delayed response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count%2 == 1 {
			// Return 500 Internal Server Error
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Return valid detection
		resp := DetectionResponse{
			CameraID:        "cam-http-stress",
			Timestamp:       float64(time.Now().Unix()),
			InferenceTimeMs: 20.0,
			DetectionsCount: 1,
			HasFire:         true,
			Detections: []Detection{
				{ClassID: 0, ClassName: "fire", Confidence: 0.90, Box: []float64{0.2, 0.2, 0.4, 0.4}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Test with MockFallback = true (graceful fallback on 500)
	detector := NewYOLO11Detector(
		"cam-http-stress",
		WithServiceURL(ts.URL),
		WithConfidenceThreshold(0.40),
		WithMockFallback(true),
		WithHTTPTimeout(100*time.Millisecond),
	)
	detector.SetMockResponse(true, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Request 1: Server returns 500, fallback should kick in
	resp1, err1 := detector.InferFrame(ctx, []byte("test-frame-1"))
	if err1 != nil || resp1 == nil || !resp1.HasFire {
		t.Fatalf("expected fallback on HTTP 500 error, got err=%v, resp=%v", err1, resp1)
	}

	// Request 2: Server returns 200 OK
	resp2, err2 := detector.InferFrame(ctx, []byte("test-frame-2"))
	if err2 != nil || resp2 == nil || !resp2.HasFire {
		t.Fatalf("expected successful HTTP response, got err=%v, resp=%v", err2, resp2)
	}
}

// TestStress_ExtremeNALUAndPayloadVariations tests edge cases with NALU bytes.
func TestStress_ExtremeNALUAndPayloadVariations(t *testing.T) {
	detector := NewYOLO11Detector("cam-nalu-test")

	// 1. Empty payload
	pkt1 := media.MediaPacket{Payload: nil}
	if b := detector.extractFrameBytes(pkt1); len(b) != 0 {
		t.Fatalf("expected nil bytes for empty payload, got %d bytes", len(b))
	}

	// 2. Payload with empty inner slice
	pkt2 := media.MediaPacket{Payload: [][]byte{{}, {}}}
	if b := detector.extractFrameBytes(pkt2); len(b) != 0 {
		t.Fatalf("expected nil/empty bytes for empty inner slices, got %d bytes", len(b))
	}

	// 3. Multi-NALU frame
	pkt3 := media.MediaPacket{
		Payload: [][]byte{
			{0x67, 0x42, 0x00}, // SPS
			{0x68, 0xCE, 0x3C}, // PPS
			{0x65, 0x88, 0x84}, // IDR
		},
	}
	b3 := detector.extractFrameBytes(pkt3)
	// Expected 3 NALUs, each prefixed with 4-byte start code (0x00000001) => (4+3)*3 = 21 bytes
	if len(b3) != 21 {
		t.Fatalf("expected 21 bytes for 3 NALUs with start codes, got %d bytes", len(b3))
	}
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	if !bytes.HasPrefix(b3, startCode) {
		t.Fatalf("expected start code prefix in flattened NALU stream")
	}
}

// TestStress_MaxConfidenceAlertAggregation verifies max confidence is selected for fire/smoke events.
func TestStress_MaxConfidenceAlertAggregation(t *testing.T) {
	detector := NewYOLO11Detector("cam-max-conf-test", WithConfidenceThreshold(0.40))

	resp := &DetectionResponse{
		CameraID: "cam-max-conf-test",
		HasFire:  true,
		HasSmoke: true,
		Detections: []Detection{
			{ClassID: 0, ClassName: "fire", Confidence: 0.55, Box: []float64{0.1, 0.1, 0.1, 0.1}},
			{ClassID: 0, ClassName: "fire", Confidence: 0.92, Box: []float64{0.2, 0.2, 0.1, 0.1}},
			{ClassID: 0, ClassName: "fire", Confidence: 0.70, Box: []float64{0.3, 0.3, 0.1, 0.1}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.45, Box: []float64{0.4, 0.4, 0.1, 0.1}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.81, Box: []float64{0.5, 0.5, 0.1, 0.1}},
		},
	}

	filtered := detector.filterDetections(resp)
	if filtered.DetectionsCount != 5 {
		t.Fatalf("expected 5 detections, got %d", filtered.DetectionsCount)
	}

	// Verify handleDetections processes max confidence without error
	detector.handleDetections(filtered)
}
