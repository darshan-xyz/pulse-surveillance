package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

// ============================================================================
// Core Data Models matching PROJECT.md Interface Contracts
// ============================================================================

// Class IDs and Names
const (
	ClassIDFire    = 0
	ClassIDSmoke   = 1
	ClassNameFire  = "fire"
	ClassNameSmoke = "smoke"

	DefaultConfidenceThreshold = 0.40
	CanvasOverlayTTLMillis     = 1000
)

// DetectionBox represents normalized [x, y, w, h] coordinates in range [0.0, 1.0].
type DetectionBox [4]float64

func (b DetectionBox) X() float64 { return b[0] }
func (b DetectionBox) Y() float64 { return b[1] }
func (b DetectionBox) W() float64 { return b[2] }
func (b DetectionBox) H() float64 { return b[3] }

// Detection represents a single detected object in a video frame.
type Detection struct {
	Class      string       `json:"class"`
	ClassID    int          `json:"class_id"`
	Confidence float64      `json:"confidence"`
	Box        DetectionBox `json:"box"`
}

// DetectionPacket represents the WebSocket payload streamed to frontend viewers.
type DetectionPacket struct {
	Type        string      `json:"type"`
	CameraID    string      `json:"camera_id"`
	Timestamp   int64       `json:"timestamp"`
	Detections  []Detection `json:"detections"`
	InferenceMs float64     `json:"inference_time_ms,omitempty"`
}

// InferenceRequest represents an image frame or tensor inference invocation.
type InferenceRequest struct {
	CameraID            string  `json:"camera_id"`
	FrameData           []byte  `json:"-"`
	FilePath            string  `json:"file_path,omitempty"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
	Timestamp           int64   `json:"timestamp"`
}

// InferenceResponse represents the output produced by the YOLO11 model.
type InferenceResponse struct {
	CameraID        string      `json:"camera_id"`
	Detections      []Detection `json:"detections"`
	InferenceTimeMs float64     `json:"inference_time_ms"`
	Timestamp       int64       `json:"timestamp"`
}

// EventType represents the category of emergency alert event.
type EventType string

const (
	EventFireDetected   EventType = "FIRE_DETECTED"
	EventSmokeDetected  EventType = "SMOKE_DETECTED"
	EventMotionDetected EventType = "MOTION_DETECTED"
	EventCameraOffline  EventType = "CAMERA_OFFLINE"
	EventCameraOnline   EventType = "CAMERA_ONLINE"
)

// EmergencyEvent represents an alert persisted in SQLite and pushed to mobile/HUD.
type EmergencyEvent struct {
	ID          string                 `json:"id"`
	CameraID    string                 `json:"camera_id"`
	Type        EventType              `json:"type"`
	StartedAt   time.Time              `json:"started_at"`
	Confidence  float64                `json:"confidence"`
	RecordingID *string                `json:"recording_id,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// PushNotificationPayload represents an Expo push notification sent on detection.
type PushNotificationPayload struct {
	To        string                 `json:"to"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Sound     string                 `json:"sound"`
	Priority  string                 `json:"priority"`
	ChannelID string                 `json:"channel_id"`
	Data      map[string]interface{} `json:"data"`
}

// CanvasPixelBox represents scaled pixel coordinates on an HTML5 canvas.
type CanvasPixelBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// CanvasRenderStyle represents the visual styling parameters for bounding boxes.
type CanvasRenderStyle struct {
	StrokeColor string `json:"stroke_color"`
	FillColor   string `json:"fill_color"`
	BadgeLabel  string `json:"badge_label"`
	TextColor   string `json:"text_color"`
}

// HUDAlertStatus represents the state of emergency indicators on the dashboard.
type HUDAlertStatus struct {
	BannerVisible     bool      `json:"banner_visible"`
	BannerMessage     string    `json:"banner_message"`
	BannerSeverity    string    `json:"banner_severity"` // "critical", "warning", "info"
	ActiveFireBadges  []string  `json:"active_fire_badges"`
	ActiveSmokeBadges []string  `json:"active_smoke_badges"`
	LastAlertTime     time.Time `json:"last_alert_time"`
}

// ============================================================================
// Authoritative E2E Verification Oracles & Reference Models
// ============================================================================

// YOLO11InferenceOracle provides authoritative evaluation of YOLO11 inference logic.
type YOLO11InferenceOracle struct {
	DefaultThreshold float64
}

func NewYOLO11InferenceOracle() *YOLO11InferenceOracle {
	return &YOLO11InferenceOracle{
		DefaultThreshold: DefaultConfidenceThreshold,
	}
}

func (o *YOLO11InferenceOracle) Evaluate(rawDetections []Detection, threshold float64) []Detection {
	if threshold <= 0 {
		threshold = o.DefaultThreshold
	}
	var valid []Detection
	for _, d := range rawDetections {
		// Filter out detections strictly below confidence threshold
		if d.Confidence < threshold {
			continue
		}
		// Normalize and validate bounding box coordinates
		box := d.Box
		box[0] = math.Max(0.0, math.Min(1.0, box[0]))
		box[1] = math.Max(0.0, math.Min(1.0, box[1]))
		box[2] = math.Max(0.0, math.Min(1.0-box[0], box[2]))
		box[3] = math.Max(0.0, math.Min(1.0-box[1], box[3]))

		// Standardize class mapping
		className := d.Class
		classID := d.ClassID
		if classID == ClassIDFire || strings.ToLower(className) == ClassNameFire {
			classID = ClassIDFire
			className = ClassNameFire
		} else if classID == ClassIDSmoke || strings.ToLower(className) == ClassNameSmoke {
			classID = ClassIDSmoke
			className = ClassNameSmoke
		}

		valid = append(valid, Detection{
			Class:      className,
			ClassID:    classID,
			Confidence: d.Confidence,
			Box:        box,
		})
	}
	return valid
}

// CanvasGeometryOracle calculates exact pixel bounding boxes for given video dimensions.
type CanvasGeometryOracle struct{}

func (c *CanvasGeometryOracle) Transform(box DetectionBox, canvasWidth, canvasHeight float64) CanvasPixelBox {
	if canvasWidth <= 0 || canvasHeight <= 0 {
		return CanvasPixelBox{0, 0, 0, 0}
	}
	clampedX := math.Max(0, math.Min(1.0, box[0]))
	clampedY := math.Max(0, math.Min(1.0, box[1]))
	clampedW := math.Max(0, math.Min(1.0-clampedX, box[2]))
	clampedH := math.Max(0, math.Min(1.0-clampedY, box[3]))

	return CanvasPixelBox{
		X:      math.Round(clampedX*canvasWidth*100) / 100,
		Y:      math.Round(clampedY*canvasHeight*100) / 100,
		Width:  math.Round(clampedW*canvasWidth*100) / 100,
		Height: math.Round(clampedH*canvasHeight*100) / 100,
	}
}

func (c *CanvasGeometryOracle) GetStyle(d Detection) CanvasRenderStyle {
	pct := int(math.Round(d.Confidence * 100))
	if d.ClassID == ClassIDFire || d.Class == ClassNameFire {
		return CanvasRenderStyle{
			StrokeColor: "rgba(239, 68, 68, 0.9)",
			FillColor:   "rgba(239, 68, 68, 0.15)",
			BadgeLabel:  fmt.Sprintf("FIRE (%d%%)", pct),
			TextColor:   "#ffffff",
		}
	}
	return CanvasRenderStyle{
		StrokeColor: "rgba(245, 158, 11, 0.9)",
		FillColor:   "rgba(245, 158, 11, 0.15)",
		BadgeLabel:  fmt.Sprintf("SMOKE (%d%%)", pct),
		TextColor:   "#ffffff",
	}
}

// ============================================================================
// Mock Test Environment Harness for E2E Verification
// ============================================================================

// InMemEventStore provides thread-safe in-memory event storage for pure Go testing.
type InMemEventStore struct {
	mu     sync.RWMutex
	events map[string]EmergencyEvent
	order  []string
}

func NewInMemEventStore() *InMemEventStore {
	return &InMemEventStore{
		events: make(map[string]EmergencyEvent),
	}
}

func (s *InMemEventStore) Insert(evt EmergencyEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.events[evt.ID]; !exists {
		s.order = append(s.order, evt.ID)
	}
	s.events[evt.ID] = evt
}

func (s *InMemEventStore) Get(id string) (EmergencyEvent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	evt, ok := s.events[id]
	return evt, ok
}

func (s *InMemEventStore) List(cameraID string) []EmergencyEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []EmergencyEvent
	for _, id := range s.order {
		evt := s.events[id]
		if cameraID == "" || evt.CameraID == cameraID {
			list = append(list, evt)
		}
	}
	return list
}

func (s *InMemEventStore) Count(cameraID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cameraID == "" {
		return len(s.events)
	}
	count := 0
	for _, evt := range s.events {
		if evt.CameraID == cameraID {
			count++
		}
	}
	return count
}

// E2ETestHarness provides a complete in-memory mock environment with SQLite support,
// WebSocket signaling server, mock AI detector, and event engine.
type E2ETestHarness struct {
	Store            *InMemEventStore
	DB               *sql.DB
	Server           *httptest.Server
	WSURL            string
	Clients          map[string]*websocket.Conn
	ClientMu         sync.Mutex
	ReceivedPackets  map[string][]DetectionPacket
	PacketMu         sync.Mutex
	EmittedEvents    []EmergencyEvent
	EventMu          sync.Mutex
	DispatchedPushes []PushNotificationPayload
	PushMu           sync.Mutex
	Ctx              context.Context
	Cancel           context.CancelFunc
}

func NewE2ETestHarness() (*E2ETestHarness, error) {
	ctx, cancel := context.WithCancel(context.Background())

	store := NewInMemEventStore()

	// Try initializing in-memory SQLite database if CGO is enabled
	var db *sql.DB
	sqliteDB, err := sql.Open("sqlite3", ":memory:")
	if err == nil {
		schema := `
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			camera_id TEXT NOT NULL,
			type TEXT NOT NULL,
			started_at TEXT NOT NULL,
			confidence REAL NOT NULL,
			recording_id TEXT,
			data TEXT,
			created_at TEXT NOT NULL
		);
		`
		if _, execErr := sqliteDB.Exec(schema); execErr == nil {
			db = sqliteDB
		} else {
			sqliteDB.Close()
		}
	}

	h := &E2ETestHarness{
		Store:           store,
		DB:              db,
		Clients:         make(map[string]*websocket.Conn),
		ReceivedPackets: make(map[string][]DetectionPacket),
		Ctx:             ctx,
		Cancel:          cancel,
	}

	// Initialize mock HTTP & WebSocket server
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "invalid_token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		cameraID := r.URL.Query().Get("camera_id")
		if cameraID == "" {
			cameraID = "default"
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		h.ClientMu.Lock()
		clientID := fmt.Sprintf("%s-%s-%d", cameraID, GenerateTestUUID(), time.Now().UnixNano())
		h.Clients[clientID] = conn
		h.ClientMu.Unlock()

		go func() {
			defer func() {
				h.ClientMu.Lock()
				delete(h.Clients, clientID)
				h.ClientMu.Unlock()
				conn.Close()
			}()

			for {
				var pkt DetectionPacket
				if err := conn.ReadJSON(&pkt); err != nil {
					break
				}
				h.PacketMu.Lock()
				h.ReceivedPackets[pkt.CameraID] = append(h.ReceivedPackets[pkt.CameraID], pkt)
				h.PacketMu.Unlock()
			}
		}()
	})

	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		cameraID := r.URL.Query().Get("camera_id")
		evts := h.Store.List(cameraID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(evts)
	})

	server := httptest.NewServer(mux)
	h.Server = server
	h.WSURL = "ws" + strings.TrimPrefix(server.URL, "http")

	return h, nil
}

func (h *E2ETestHarness) EmitEvent(evt EmergencyEvent) error {
	h.EventMu.Lock()
	h.EmittedEvents = append(h.EmittedEvents, evt)
	h.EventMu.Unlock()

	// Store in In-Memory Store
	h.Store.Insert(evt)

	// Persist to DB if available
	if h.DB != nil {
		query := `INSERT INTO events (id, camera_id, type, started_at, confidence, recording_id, created_at)
				  VALUES (?, ?, ?, ?, ?, ?, ?)`
		startedStr := evt.StartedAt.Format(time.RFC3339)
		createdStr := time.Now().Format(time.RFC3339)

		var recID *string
		if evt.RecordingID != nil {
			recID = evt.RecordingID
		}

		_, _ = h.DB.Exec(query, evt.ID, evt.CameraID, string(evt.Type), startedStr, evt.Confidence, recID, createdStr)
	}

	// Trigger Push Notification
	var title, body string
	switch evt.Type {
	case EventFireDetected:
		title = fmt.Sprintf("CRITICAL FIRE ALERT: %s", evt.CameraID)
		body = fmt.Sprintf("Fire detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	case EventSmokeDetected:
		title = fmt.Sprintf("WARNING SMOKE ALERT: %s", evt.CameraID)
		body = fmt.Sprintf("Smoke detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	default:
		title = fmt.Sprintf("Alert: %s", evt.Type)
		body = fmt.Sprintf("Event on camera %s", evt.CameraID)
	}

	pushMsg := PushNotificationPayload{
		To:        "test-push-token",
		Title:     title,
		Body:      body,
		Sound:     "default",
		Priority:  "high",
		ChannelID: "alerts",
		Data: map[string]interface{}{
			"camera_id":  evt.CameraID,
			"event_id":   evt.ID,
			"event_type": string(evt.Type),
			"confidence": evt.Confidence,
		},
	}

	h.PushMu.Lock()
	h.DispatchedPushes = append(h.DispatchedPushes, pushMsg)
	h.PushMu.Unlock()

	return nil
}

func (h *E2ETestHarness) GetEvent(id string) (EmergencyEvent, error) {
	evt, ok := h.Store.Get(id)
	if !ok {
		return EmergencyEvent{}, fmt.Errorf("event not found: %s", id)
	}
	return evt, nil
}

func (h *E2ETestHarness) CountEvents(cameraID string) int {
	return h.Store.Count(cameraID)
}

func (h *E2ETestHarness) BroadcastDetection(pkt DetectionPacket) {
	h.PacketMu.Lock()
	h.ReceivedPackets[pkt.CameraID] = append(h.ReceivedPackets[pkt.CameraID], pkt)
	h.PacketMu.Unlock()

	h.ClientMu.Lock()
	defer h.ClientMu.Unlock()

	for _, conn := range h.Clients {
		_ = conn.WriteJSON(pkt)
	}
}

func (h *E2ETestHarness) Close() {
	h.Cancel()
	h.ClientMu.Lock()
	for _, conn := range h.Clients {
		conn.Close()
	}
	h.ClientMu.Unlock()
	if h.Server != nil {
		h.Server.Close()
	}
	if h.DB != nil {
		h.DB.Close()
	}
}
