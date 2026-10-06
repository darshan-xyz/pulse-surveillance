package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/events"
	"hss/internal/media"
)

func TestYOLO11Detector_Creation(t *testing.T) {
	camID := "cam-test-001"
	detector := NewYOLO11Detector(
		camID,
		WithServiceURL("http://127.0.0.1:8000/detect"),
		WithConfidenceThreshold(0.45),
		WithSampleInterval(200*time.Millisecond),
		WithMockFallback(true),
		WithKeyframesOnly(true),
	)

	if detector.Name() != "YOLO11FireSmokeDetector" {
		t.Fatalf("expected name 'YOLO11FireSmokeDetector', got '%s'", detector.Name())
	}
	if detector.CameraID() != camID {
		t.Fatalf("expected camera ID '%s', got '%s'", camID, detector.CameraID())
	}
	if detector.config.ConfidenceThreshold != 0.45 {
		t.Fatalf("expected threshold 0.45, got %f", detector.config.ConfidenceThreshold)
	}
	if detector.config.SampleInterval != 200*time.Millisecond {
		t.Fatalf("expected sample interval 200ms, got %v", detector.config.SampleInterval)
	}
}

func TestYOLO11Detector_ConfidenceThresholding(t *testing.T) {
	detector := NewYOLO11Detector("cam-test-thresh", WithConfidenceThreshold(0.40))

	rawResp := &DetectionResponse{
		CameraID: "cam-test-thresh",
		Detections: []Detection{
			{ClassID: 0, ClassName: "fire", Confidence: 0.35, Box: []float64{0.1, 0.1, 0.2, 0.2}},
			{ClassID: 0, ClassName: "fire", Confidence: 0.48, Box: []float64{0.2, 0.2, 0.3, 0.3}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.22, Box: []float64{0.4, 0.4, 0.1, 0.1}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.72, Box: []float64{0.5, 0.5, 0.2, 0.2}},
		},
	}

	filtered := detector.filterDetections(rawResp)

	if filtered.DetectionsCount != 2 {
		t.Fatalf("expected 2 detections meeting threshold >= 0.40, got %d", filtered.DetectionsCount)
	}
	if !filtered.HasFire {
		t.Fatalf("expected HasFire to be true")
	}
	if !filtered.HasSmoke {
		t.Fatalf("expected HasSmoke to be true")
	}

	for _, d := range filtered.Detections {
		if d.Confidence < 0.40 {
			t.Fatalf("detection below threshold passed filter: conf=%f", d.Confidence)
		}
	}
}

func TestYOLO11Detector_MockFallback(t *testing.T) {
	detector := NewYOLO11Detector(
		"cam-test-mock",
		WithConfidenceThreshold(0.40),
		WithMockFallback(true),
		WithHTTPTimeout(50*time.Millisecond),
		WithServiceURL("http://invalid-non-existent-host:9999/detect"),
	)

	// Set mock engine to fire
	detector.SetMockResponse(true, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := detector.InferFrame(ctx, []byte("dummy-frame-bytes"))
	if err != nil {
		t.Fatalf("expected mock inference to succeed, got: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
	if !resp.HasFire {
		t.Fatalf("expected fire detection in mock response")
	}
	if resp.HasSmoke {
		t.Fatalf("did not expect smoke detection")
	}
	if len(resp.Detections) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(resp.Detections))
	}
	if resp.Detections[0].ClassID != 0 || resp.Detections[0].ClassName != "fire" {
		t.Fatalf("expected class 0/fire, got %d/%s", resp.Detections[0].ClassID, resp.Detections[0].ClassName)
	}
}

func TestYOLO11Detector_HTTPInference(t *testing.T) {
	// Create mock HTTP server simulating FastAPI AI service
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resp := DetectionResponse{
			CameraID:        r.URL.Query().Get("camera_id"),
			Timestamp:       float64(time.Now().Unix()),
			InferenceTimeMs: 12.5,
			DetectionsCount: 1,
			HasFire:         true,
			HasSmoke:        false,
			Detections: []Detection{
				{
					ClassID:    0,
					ClassName:  "fire",
					Confidence: 0.85,
					Box:        []float64{0.45, 0.50, 0.20, 0.30},
					XYXY:       []float64{288.0, 240.0, 416.0, 384.0},
					XYXYNorm:   []float64{0.45, 0.50, 0.65, 0.80},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	detector := NewYOLO11Detector(
		"cam-http-test",
		WithServiceURL(ts.URL),
		WithConfidenceThreshold(0.40),
		WithMockFallback(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := detector.InferFrame(ctx, []byte("sample-test-image-bytes"))
	if err != nil {
		t.Fatalf("HTTP inference failed: %v", err)
	}

	if resp.CameraID != "cam-http-test" {
		t.Fatalf("expected camera ID 'cam-http-test', got '%s'", resp.CameraID)
	}
	if !resp.HasFire {
		t.Fatalf("expected HasFire=true")
	}
	if len(resp.Detections) != 1 {
		t.Fatalf("expected 1 detection, got %d", len(resp.Detections))
	}
	box := resp.Detections[0].Box
	if len(box) != 4 || box[0] != 0.45 || box[1] != 0.50 || box[2] != 0.20 || box[3] != 0.30 {
		t.Fatalf("unexpected bounding box: %v", box)
	}
}

func TestYOLO11Detector_FrameFileInference(t *testing.T) {
	// Try loading sample frame from disk if available
	samplePaths := []string{
		"../../ai_service/test_data/frame_04_fire_smoke.jpg",
		"../ai_service/test_data/frame_04_fire_smoke.jpg",
		"ai_service/test_data/frame_04_fire_smoke.jpg",
	}

	var frameBytes []byte
	for _, p := range samplePaths {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			frameBytes = b
			break
		}
	}

	if len(frameBytes) == 0 {
		// Use synthetic bytes
		frameBytes = []byte("synthetic-fire-frame-bytes")
	}

	detector := NewYOLO11Detector(
		"cam-file-test",
		WithConfidenceThreshold(0.40),
		WithMockFallback(true),
		WithHTTPTimeout(50*time.Millisecond),
	)
	detector.SetMockResponse(true, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := detector.InferFrame(ctx, frameBytes)
	if err != nil {
		t.Fatalf("file frame inference failed: %v", err)
	}

	if resp == nil || !resp.HasFire {
		t.Fatalf("expected fire detection from frame inference")
	}
	if len(resp.Detections) == 0 || resp.Detections[0].Confidence < 0.40 {
		t.Fatalf("expected detection with conf >= 0.40")
	}
}

func TestYOLO11Detector_NormalizedBoundingBoxes(t *testing.T) {
	detector := NewYOLO11Detector("cam-norm-test")

	resp := &DetectionResponse{
		CameraID: "cam-norm-test",
		Detections: []Detection{
			{ClassID: 0, ClassName: "fire", Confidence: 0.90, Box: []float64{0.0, 0.0, 1.0, 1.0}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.80, Box: []float64{0.25, 0.35, 0.40, 0.50}},
		},
	}

	filtered := detector.filterDetections(resp)
	for i, d := range filtered.Detections {
		if len(d.Box) != 4 {
			t.Fatalf("detection %d: box must have length 4, got %d", i, len(d.Box))
		}
		x, y, w, h := d.Box[0], d.Box[1], d.Box[2], d.Box[3]
		if x < 0.0 || x > 1.0 || y < 0.0 || y > 1.0 || w <= 0.0 || w > 1.0 || h <= 0.0 || h > 1.0 {
			t.Fatalf("detection %d: invalid normalized box coordinates: %v", i, d.Box)
		}
	}
}

func TestYOLO11Detector_ListenersAndEventEmission(t *testing.T) {
	// Initialize events engine if not already initialized
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if events.DefaultEngine == nil {
		events.InitEngine(ctx)
	}

	detector := NewYOLO11Detector("cam-event-test", WithConfidenceThreshold(0.40))
	detector.SetMockResponse(true, true, nil)

	var callbackCount int32
	var lastReceivedCamID string
	var mu sync.Mutex

	detector.OnDetection(func(camID string, resp *DetectionResponse) {
		atomic.AddInt32(&callbackCount, 1)
		mu.Lock()
		lastReceivedCamID = camID
		mu.Unlock()
	})

	testResp := &DetectionResponse{
		CameraID: "cam-event-test",
		HasFire:  true,
		HasSmoke: true,
		Detections: []Detection{
			{ClassID: 0, ClassName: "fire", Confidence: 0.88, Box: []float64{0.5, 0.5, 0.2, 0.2}},
			{ClassID: 1, ClassName: "smoke", Confidence: 0.75, Box: []float64{0.4, 0.3, 0.3, 0.3}},
		},
	}

	detector.handleDetections(testResp)

	if atomic.LoadInt32(&callbackCount) != 1 {
		t.Fatalf("expected callback to be called once, got %d", callbackCount)
	}

	mu.Lock()
	if lastReceivedCamID != "cam-event-test" {
		t.Fatalf("expected camera ID 'cam-event-test', got '%s'", lastReceivedCamID)
	}
	mu.Unlock()
}

// MockCamera implements media.Camera for testing
type testMockCamera struct {
	id   string
	name string
	url  string
}

func (c *testMockCamera) ID() string            { return c.id }
func (c *testMockCamera) Name() string          { return c.name }
func (c *testMockCamera) MainStreamURL() string { return c.url }

func TestYOLO11Detector_MediaStreamIngestion(t *testing.T) {
	camID := "cam-media-ingest-test"
	mockCam := &testMockCamera{
		id:   camID,
		name: "Media Ingest Test Cam",
		url:  "rtsp://127.0.0.1:8554/live", // will fallback to synthetic generator
	}

	// Attach camera to media DefaultEngine
	_ = media.DefaultEngine.AttachCamera(mockCam)
	defer media.DefaultEngine.DetachCamera(camID)

	detector := NewYOLO11Detector(
		camID,
		WithConfidenceThreshold(0.40),
		WithSampleInterval(50*time.Millisecond),
		WithMockFallback(true),
		WithKeyframesOnly(false),
	)

	var detectionReceived int32
	detector.OnDetection(func(cid string, resp *DetectionResponse) {
		atomic.AddInt32(&detectionReceived, 1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := detector.Start(ctx)
	if err != nil {
		t.Fatalf("detector start failed: %v", err)
	}
	defer detector.Stop()

	// Wait for frame processing
	time.Sleep(300 * time.Millisecond)

	// Verify subscriber count
	subCount := media.DefaultEngine.SubscriberCount(camID)
	if subCount < 1 {
		t.Fatalf("expected at least 1 subscriber, got %d", subCount)
	}
}

func TestYOLO11Detector_EmptyFrameHandling(t *testing.T) {
	detector := NewYOLO11Detector("cam-empty-test")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	resp, err := detector.InferFrame(ctx, nil)
	if err == nil {
		t.Fatalf("expected error on nil frame data, got response: %v", resp)
	}

	resp2, err2 := detector.InferFrame(ctx, []byte{})
	if err2 == nil {
		t.Fatalf("expected error on empty frame data, got response: %v", resp2)
	}
}

func TestYOLO11Detector_ConcurrentInference(t *testing.T) {
	detector := NewYOLO11Detector("cam-concurrent-test", WithMockFallback(true))
	detector.SetMockResponse(true, false, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	numRoutines := 20
	errCount := int32(0)

	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := detector.InferFrame(ctx, []byte("concurrent-frame-bytes"))
			if err != nil || resp == nil || !resp.HasFire {
				atomic.AddInt32(&errCount, 1)
			}
		}()
	}

	wg.Wait()

	if errCount != 0 {
		t.Fatalf("expected 0 errors during concurrent inference, got %d", errCount)
	}
}

func TestYOLO11Detector_MultipleListeners(t *testing.T) {
	detector := NewYOLO11Detector("cam-multi-listener")

	var countA int32
	var countB int32

	detector.OnDetection(func(cid string, resp *DetectionResponse) {
		atomic.AddInt32(&countA, 1)
	})
	detector.OnDetection(func(cid string, resp *DetectionResponse) {
		atomic.AddInt32(&countB, 1)
	})

	testResp := &DetectionResponse{
		CameraID: "cam-multi-listener",
		HasFire:  true,
		Detections: []Detection{
			{ClassID: 0, ClassName: "fire", Confidence: 0.95, Box: []float64{0.1, 0.1, 0.2, 0.2}},
		},
	}

	detector.handleDetections(testResp)

	if atomic.LoadInt32(&countA) != 1 || atomic.LoadInt32(&countB) != 1 {
		t.Fatalf("expected both listeners to be called once: A=%d, B=%d", countA, countB)
	}
}

func TestYOLO11Detector_StopIdempotence(t *testing.T) {
	detector := NewYOLO11Detector("cam-stop-test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = detector.Start(ctx)
	detector.Stop()
	// Second stop should not panic
	detector.Stop()
}

