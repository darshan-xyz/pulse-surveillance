package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"hss/internal/events"
	"hss/internal/media"
)

// EventType constants for AI detections
const (
	EventFireDetected  events.EventType = "FIRE_DETECTED"
	EventSmokeDetected events.EventType = "SMOKE_DETECTED"
)

// Detection represents a single object detection instance.
type Detection struct {
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float64   `json:"confidence"`
	Box        []float64 `json:"box"` // Normalized [x, y, w, h] in range [0.0, 1.0]
	XYXY       []float64 `json:"xyxy,omitempty"`
	XYXYNorm   []float64 `json:"xyxy_norm,omitempty"`
}

// DetectionResponse represents the complete inference payload returned by the AI inference service.
type DetectionResponse struct {
	CameraID          string      `json:"camera_id"`
	Timestamp         float64     `json:"timestamp"`
	InferenceTimeMs   float64     `json:"inference_time_ms"`
	DetectionsCount   int         `json:"detections_count"`
	HasFire           bool        `json:"has_fire"`
	HasSmoke          bool        `json:"has_smoke"`
	FireStreak        int         `json:"fire_streak,omitempty"`
	SmokeStreak       int         `json:"smoke_streak,omitempty"`
	TemporalConfirmed bool        `json:"temporal_confirmed,omitempty"`
	MotionScore       float64     `json:"motion_score,omitempty"`
	Detections        []Detection `json:"detections"`
}

// DetectionCallback is invoked when new detections meeting the confidence threshold are produced.
type DetectionCallback func(cameraID string, resp *DetectionResponse)

// YOLO11DetectorConfig contains configuration parameters for the YOLO11Detector.
type YOLO11DetectorConfig struct {
	CameraID            string
	ServiceURL          string
	ConfidenceThreshold float64
	FireThreshold       float64
	SmokeThreshold      float64
	SampleInterval      time.Duration
	KeyframesOnly       bool
	MockFallback        bool
	HTTPTimeout         time.Duration
}

// Option configures the YOLO11Detector.
type Option func(*YOLO11Detector)

// WithServiceURL sets the AI inference service endpoint.
func WithServiceURL(url string) Option {
	return func(d *YOLO11Detector) {
		d.config.ServiceURL = url
	}
}

// WithConfidenceThreshold sets the minimum confidence score (default: 0.35).
func WithConfidenceThreshold(threshold float64) Option {
	return func(d *YOLO11Detector) {
		d.config.ConfidenceThreshold = threshold
	}
}

// WithFireThreshold sets specific confidence threshold for fire (class 0).
func WithFireThreshold(threshold float64) Option {
	return func(d *YOLO11Detector) {
		d.config.FireThreshold = threshold
	}
}

// WithSmokeThreshold sets specific confidence threshold for smoke (class 1).
func WithSmokeThreshold(threshold float64) Option {
	return func(d *YOLO11Detector) {
		d.config.SmokeThreshold = threshold
	}
}

// WithSampleInterval sets the minimum interval between frame inferences.
func WithSampleInterval(interval time.Duration) Option {
	return func(d *YOLO11Detector) {
		d.config.SampleInterval = interval
	}
}

// WithMockFallback configures whether to fallback to synthetic detection if the AI service is unreachable.
func WithMockFallback(fallback bool) Option {
	return func(d *YOLO11Detector) {
		d.config.MockFallback = fallback
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(d *YOLO11Detector) {
		d.httpClient = client
	}
}

// WithKeyframesOnly restricts inference sampling strictly to IDR keyframes.
func WithKeyframesOnly(kOnly bool) Option {
	return func(d *YOLO11Detector) {
		d.config.KeyframesOnly = kOnly
	}
}

// WithHTTPTimeout configures the HTTP request timeout.
func WithHTTPTimeout(timeout time.Duration) Option {
	return func(d *YOLO11Detector) {
		d.config.HTTPTimeout = timeout
		if d.httpClient != nil {
			d.httpClient.Timeout = timeout
		}
	}
}

// MockInferenceEngine provides deterministic synthetic detections for unit testing without live service.
type MockInferenceEngine struct {
	mu           sync.RWMutex
	alwaysFire   bool
	alwaysSmoke  bool
	customDets   []Detection
	simulateFail bool
}

// YOLO11Detector implements ai.Plugin and ingests camera frames from media.DefaultEngine.
type YOLO11Detector struct {
	config        YOLO11DetectorConfig
	httpClient    *http.Client
	mu            sync.RWMutex
	listeners     []DetectionCallback
	lastInferTime time.Time
	mockEngine    *MockInferenceEngine
	running       bool
	stopFunc      context.CancelFunc
}

// NewYOLO11Detector initializes a new YOLO11Detector for a given cameraID.
func NewYOLO11Detector(cameraID string, opts ...Option) *YOLO11Detector {
	confThresh := 0.35
	if v := os.Getenv("CONFIDENCE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			confThresh = f
		}
	}
	var fireThresh float64
	if v := os.Getenv("FIRE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			fireThresh = f
		}
	}
	var smokeThresh float64
	if v := os.Getenv("SMOKE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			smokeThresh = f
		}
	}

	d := &YOLO11Detector{
		config: YOLO11DetectorConfig{
			CameraID:            cameraID,
			ServiceURL:          "http://127.0.0.1:8000/detect",
			ConfidenceThreshold: confThresh,
			FireThreshold:       fireThresh,
			SmokeThreshold:      smokeThresh,
			SampleInterval:      500 * time.Millisecond,
			KeyframesOnly:       true,
			MockFallback:        false,
			HTTPTimeout:         3 * time.Second,
		},
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		mockEngine: &MockInferenceEngine{
			alwaysFire: false,
		},
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Name returns the plugin identifier.
func (d *YOLO11Detector) Name() string {
	return "YOLO11FireSmokeDetector"
}

// CameraID returns the associated camera ID.
func (d *YOLO11Detector) CameraID() string {
	return d.config.CameraID
}

// OnDetection registers a listener callback to receive real-time detection telemetry.
func (d *YOLO11Detector) OnDetection(cb DetectionCallback) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, cb)
}

// SetMockResponse configures the mock engine for testing.
func (d *YOLO11Detector) SetMockResponse(fire bool, smoke bool, customDets []Detection) {
	d.mockEngine.mu.Lock()
	defer d.mockEngine.mu.Unlock()
	d.mockEngine.alwaysFire = fire
	d.mockEngine.alwaysSmoke = smoke
	d.mockEngine.customDets = customDets
	d.mockEngine.simulateFail = false
}

// SetMockFailure simulates network or inference errors.
func (d *YOLO11Detector) SetMockFailure(fail bool) {
	d.mockEngine.mu.Lock()
	defer d.mockEngine.mu.Unlock()
	d.mockEngine.simulateFail = fail
}

// Start begins ingesting video frames from media.DefaultEngine and running inference.
func (d *YOLO11Detector) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("detector %s already running", d.config.CameraID)
	}

	subCtx, cancel := context.WithCancel(ctx)
	d.stopFunc = cancel
	d.running = true
	d.mu.Unlock()

	log.Printf("[YOLO11Detector] Starting for camera %s (service: %s, conf >= %.2f)",
		d.config.CameraID, d.config.ServiceURL, d.config.ConfidenceThreshold)

	// Subscribe to media engine
	frameCh, err := media.DefaultEngine.Subscribe(d.config.CameraID)
	if err != nil {
		log.Printf("[YOLO11Detector] Warning: could not subscribe to camera %s: %v (will retry or wait for camera attach)", d.config.CameraID, err)
	}

	go d.processLoop(subCtx, frameCh)
	return nil
}

// Stop terminates the detector.
func (d *YOLO11Detector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopFunc != nil {
		d.stopFunc()
	}
	d.running = false
}

// processLoop handles the continuous frame ingestion pipeline.
func (d *YOLO11Detector) processLoop(ctx context.Context, initialCh <-chan media.MediaPacket) {
	frameCh := initialCh
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[YOLO11Detector] Stopping frame ingestion for camera %s", d.config.CameraID)
			if frameCh != nil {
				media.DefaultEngine.Unsubscribe(d.config.CameraID, frameCh)
			}
			return

		case <-ticker.C:
			// If subscription was not ready at Start, attempt to subscribe
			if frameCh == nil {
				if ch, err := media.DefaultEngine.Subscribe(d.config.CameraID); err == nil {
					frameCh = ch
					log.Printf("[YOLO11Detector] Successfully subscribed to camera %s", d.config.CameraID)
				}
			}

		case pkt, ok := <-frameCh:
			if !ok {
				log.Printf("[YOLO11Detector] Stream channel closed for camera %s", d.config.CameraID)
				frameCh = nil
				continue
			}

			// Frame filtering: Check keyframe constraint and sample interval
			if d.config.KeyframesOnly && !pkt.IsKeyFrame {
				continue
			}

			d.mu.RLock()
			now := time.Now()
			if now.Sub(d.lastInferTime) < d.config.SampleInterval {
				d.mu.RUnlock()
				continue
			}
			d.mu.RUnlock()

			// Extract frame bytes
			frameBytes := d.extractFrameBytes(pkt)
			if len(frameBytes) == 0 {
				continue
			}

			d.mu.Lock()
			d.lastInferTime = now
			d.mu.Unlock()

			// Run inference asynchronously so video pipeline is never blocked
			go func(data []byte) {
				inferCtx, cancel := context.WithTimeout(context.Background(), d.config.HTTPTimeout)
				defer cancel()

				resp, err := d.InferFrame(inferCtx, data)
				if err != nil {
					log.Printf("[YOLO11Detector] Inference error for camera %s: %v", d.config.CameraID, err)
					return
				}

				if resp != nil && len(resp.Detections) > 0 {
					d.handleDetections(resp)
				}
			}(frameBytes)
		}
	}
}

// extractFrameBytes packages NALUs or raw payload into binary buffer.
func (d *YOLO11Detector) extractFrameBytes(pkt media.MediaPacket) []byte {
	if len(pkt.Payload) == 0 {
		return nil
	}

	// Flatten NALUs with 4-byte start codes (0x00000001) for standard H.264 stream representation
	var buf bytes.Buffer
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	for _, nalu := range pkt.Payload {
		if len(nalu) > 0 {
			buf.Write(startCode)
			buf.Write(nalu)
		}
	}
	return buf.Bytes()
}

// InferFrame executes fire and smoke inference on raw image or frame bytes.
func (d *YOLO11Detector) InferFrame(ctx context.Context, frameData []byte) (*DetectionResponse, error) {
	if len(frameData) == 0 {
		return nil, fmt.Errorf("empty frame data")
	}

	// Try live HTTP AI Service first
	if d.config.ServiceURL != "" {
		fireThresh := d.config.FireThreshold
		if fireThresh <= 0 {
			fireThresh = d.config.ConfidenceThreshold
		}
		smokeThresh := d.config.SmokeThreshold
		if smokeThresh <= 0 {
			smokeThresh = d.config.ConfidenceThreshold
		}
		minThresh := d.config.ConfidenceThreshold
		if fireThresh < minThresh {
			minThresh = fireThresh
		}
		if smokeThresh < minThresh {
			minThresh = smokeThresh
		}

		reqURL := fmt.Sprintf("%s?camera_id=%s&confidence_threshold=%.2f&fire_threshold=%.2f&smoke_threshold=%.2f",
			d.config.ServiceURL, d.config.CameraID, minThresh, fireThresh, smokeThresh)

		var reqErr error
		req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(frameData))
		if err != nil {
			reqErr = err
		} else {
			req.Header.Set("Content-Type", "application/octet-stream")
			httpResp, httpErr := d.httpClient.Do(req)
			if httpErr != nil {
				reqErr = httpErr
			} else {
				defer httpResp.Body.Close()
				if httpResp.StatusCode == http.StatusOK {
					var result DetectionResponse
					if err := json.NewDecoder(httpResp.Body).Decode(&result); err == nil {
						return d.filterDetections(&result), nil
					} else {
						reqErr = fmt.Errorf("failed to decode AI response: %w", err)
					}
				} else {
					body, _ := io.ReadAll(httpResp.Body)
					reqErr = fmt.Errorf("AI service returned HTTP %d: %s", httpResp.StatusCode, string(body))
				}
			}
		}

		if reqErr != nil && !d.config.MockFallback {
			return nil, reqErr
		}
	}

	// If live service failed or mock fallback is enabled, evaluate fallback
	if d.config.MockFallback {
		return d.evaluateMockInference(frameData)
	}

	return nil, fmt.Errorf("AI inference service unavailable and mock fallback disabled")
}

// evaluateMockInference produces deterministic mock detection for synthetic/unit tests.
func (d *YOLO11Detector) evaluateMockInference(frameData []byte) (*DetectionResponse, error) {
	d.mockEngine.mu.RLock()
	defer d.mockEngine.mu.RUnlock()

	if d.mockEngine.simulateFail {
		return nil, fmt.Errorf("simulated inference failure")
	}

	resp := &DetectionResponse{
		CameraID:        d.config.CameraID,
		Timestamp:       float64(time.Now().UnixNano()) / 1e9,
		InferenceTimeMs: 15.0,
		Detections:      make([]Detection, 0),
	}

	if len(d.mockEngine.customDets) > 0 {
		resp.Detections = append(resp.Detections, d.mockEngine.customDets...)
	} else {
		box := []float64{0.45, 0.50, 0.20, 0.30}
		if gen := media.DefaultEngine.GetSyntheticGenerator(d.config.CameraID); gen != nil {
			box = gen.GetCurrentBoxNormalized()
		}

		if d.mockEngine.alwaysFire {
			resp.Detections = append(resp.Detections, Detection{
				ClassID:    0,
				ClassName:  "fire",
				Confidence: 0.88,
				Box:        box,
				XYXY:       []float64{box[0] * 640.0, box[1] * 480.0, (box[0] + box[2]) * 640.0, (box[1] + box[3]) * 480.0},
				XYXYNorm:   []float64{box[0], box[1], box[0] + box[2], box[1] + box[3]},
			})
		}
		if d.mockEngine.alwaysSmoke {
			resp.Detections = append(resp.Detections, Detection{
				ClassID:    1,
				ClassName:  "smoke",
				Confidence: 0.76,
				Box:        []float64{0.40, 0.25, 0.30, 0.35},
				XYXY:       []float64{256.0, 120.0, 448.0, 288.0},
				XYXYNorm:   []float64{0.40, 0.25, 0.70, 0.60},
			})
		}
	}

	return d.filterDetections(resp), nil
}

// filterDetections applies strict thresholding to all detections.
func (d *YOLO11Detector) filterDetections(resp *DetectionResponse) *DetectionResponse {
	if resp == nil {
		return nil
	}

	fireThresh := d.config.FireThreshold
	if fireThresh <= 0 {
		fireThresh = d.config.ConfidenceThreshold
	}
	smokeThresh := d.config.SmokeThreshold
	if smokeThresh <= 0 {
		smokeThresh = d.config.ConfidenceThreshold
	}

	filtered := make([]Detection, 0, len(resp.Detections))
	hasFire := false
	hasSmoke := false

	for _, det := range resp.Detections {
		targetThresh := d.config.ConfidenceThreshold
		if det.ClassID == 0 || det.ClassName == "fire" {
			targetThresh = fireThresh
		} else if det.ClassID == 1 || det.ClassName == "smoke" {
			targetThresh = smokeThresh
		}

		if det.Confidence >= targetThresh {
			filtered = append(filtered, det)
			if det.ClassID == 0 || det.ClassName == "fire" {
				hasFire = true
			} else if det.ClassID == 1 || det.ClassName == "smoke" {
				hasSmoke = true
			}
		}
	}

	resp.Detections = filtered
	resp.DetectionsCount = len(filtered)
	resp.HasFire = hasFire
	resp.HasSmoke = hasSmoke
	return resp
}

// handleDetections dispatches alert events and invokes listeners.
func (d *YOLO11Detector) handleDetections(resp *DetectionResponse) {
	d.mu.RLock()
	listeners := make([]DetectionCallback, len(d.listeners))
	copy(listeners, d.listeners)
	d.mu.RUnlock()

	// 1. Notify listeners (e.g. WebSocket metadata stream hub)
	for _, cb := range listeners {
		cb(d.config.CameraID, resp)
	}

	// 2. Emit alert events via internal/events engine
	if events.DefaultEngine != nil {
		fireThresh := d.config.FireThreshold
		if fireThresh <= 0 {
			fireThresh = d.config.ConfidenceThreshold
		}
		smokeThresh := d.config.SmokeThreshold
		if smokeThresh <= 0 {
			smokeThresh = d.config.ConfidenceThreshold
		}

		var maxFireConf float64
		var maxSmokeConf float64

		for _, det := range resp.Detections {
			if (det.ClassID == 0 || det.ClassName == "fire") && det.Confidence > maxFireConf {
				maxFireConf = det.Confidence
			}
			if (det.ClassID == 1 || det.ClassName == "smoke") && det.Confidence > maxSmokeConf {
				maxSmokeConf = det.Confidence
			}
		}

		if resp.HasFire && maxFireConf >= fireThresh {
			evt := events.Event{
				CameraID:   d.config.CameraID,
				Type:       EventFireDetected,
				StartedAt:  time.Now(),
				Confidence: maxFireConf,
			}
			events.DefaultEngine.Emit(evt)
			log.Printf("[YOLO11Detector] EMITTED %s event on camera %s (conf: %.2f)", EventFireDetected, d.config.CameraID, maxFireConf)
		}

		if resp.HasSmoke && maxSmokeConf >= smokeThresh {
			evt := events.Event{
				CameraID:   d.config.CameraID,
				Type:       EventSmokeDetected,
				StartedAt:  time.Now(),
				Confidence: maxSmokeConf,
			}
			events.DefaultEngine.Emit(evt)
			log.Printf("[YOLO11Detector] EMITTED %s event on camera %s (conf: %.2f)", EventSmokeDetected, d.config.CameraID, maxSmokeConf)
		}
	}
}
