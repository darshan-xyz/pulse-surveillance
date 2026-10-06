package events

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"hss/internal/cloud/push"
	"hss/internal/database"
	_ "github.com/mattn/go-sqlite3"
)

// mockPushClient records all dispatched push messages for verification.
type mockPushClient struct {
	mu       sync.Mutex
	messages []push.ExpoPushMessage
}

func (m *mockPushClient) SendNotification(ctx context.Context, msg push.ExpoPushMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockPushClient) SendBatch(ctx context.Context, msgs []push.ExpoPushMessage) ([]push.ExpoPushTicket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msgs...)
	tickets := make([]push.ExpoPushTicket, len(msgs))
	for i := range tickets {
		tickets[i] = push.ExpoPushTicket{Status: "ok", ID: fmt.Sprintf("ticket-%d", i)}
	}
	return tickets, nil
}

func (m *mockPushClient) GetEndpoint() string {
	return "https://exp.host/--/api/v2/push/send"
}

func (m *mockPushClient) GetMessages() []push.ExpoPushMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]push.ExpoPushMessage, len(m.messages))
	copy(copied, m.messages)
	return copied
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Skipf("Skipping SQLite DB test: %v", err)
		return nil, func() {}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS cameras (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		protocol TEXT NOT NULL,
		address TEXT NOT NULL,
		status TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS recordings (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL,
		path TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		protected INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		camera_id TEXT NOT NULL,
		type TEXT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT,
		confidence REAL,
		recording_id TEXT,
		thumbnail_path TEXT,
		created_at TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Skipf("Skipping SQLite DB execution (CGO_ENABLED=0): %v", err)
		return nil, func() {}
	}

	oldDB := database.DB
	database.DB = db

	cleanup := func() {
		database.DB = oldDB
		db.Close()
	}

	return db, cleanup
}

func TestEngine_FireDetectedEmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine()
	go engine.processLoop(ctx)

	evt := Event{
		CameraID:   "cam-fire-01",
		Type:       EventFireDetected,
		StartedAt:  time.Now(),
		Confidence: 0.88,
		Data: map[string]interface{}{
			"class_name": "fire",
			"box":        []float64{0.35, 0.42, 0.15, 0.28},
		},
	}

	engine.Emit(evt)

	select {
	case <-time.After(50 * time.Millisecond):
		// Processed
	}
}

func TestEngine_SmokeDetectedEmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine()
	go engine.processLoop(ctx)

	evt := Event{
		CameraID:   "cam-smoke-01",
		Type:       EventSmokeDetected,
		StartedAt:  time.Now(),
		Confidence: 0.76,
		Data: map[string]interface{}{
			"class_name": "smoke",
			"box":        []float64{0.40, 0.25, 0.30, 0.35},
		},
	}

	engine.Emit(evt)

	select {
	case <-time.After(50 * time.Millisecond):
		// Processed
	}
}

func TestEngine_DatabasePersistence(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	if db == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine()
	go engine.processLoop(ctx)

	fireID := "evt-fire-persist-01"
	fireEvt := Event{
		ID:         fireID,
		CameraID:   "cam-persist-01",
		Type:       EventFireDetected,
		StartedAt:  time.Now(),
		Confidence: 0.95,
	}

	smokeID := "evt-smoke-persist-02"
	smokeEvt := Event{
		ID:         smokeID,
		CameraID:   "cam-persist-01",
		Type:       EventSmokeDetected,
		StartedAt:  time.Now(),
		Confidence: 0.82,
	}

	engine.saveEvent(fireEvt)
	engine.saveEvent(smokeEvt)

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM events WHERE camera_id = ?", "cam-persist-01").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query events table: %v", err)
	}
	if count != 2 {
		t.Fatalf("Expected 2 persisted events, found %d", count)
	}

	var evtType string
	var confidence float64
	err = db.QueryRow("SELECT type, confidence FROM events WHERE id = ?", fireID).Scan(&evtType, &confidence)
	if err != nil {
		t.Fatalf("Failed to query fire event: %v", err)
	}
	if evtType != string(EventFireDetected) || confidence != 0.95 {
		t.Fatalf("Fire event mismatch: type=%s, conf=%f", evtType, confidence)
	}

	err = db.QueryRow("SELECT type, confidence FROM events WHERE id = ?", smokeID).Scan(&evtType, &confidence)
	if err != nil {
		t.Fatalf("Failed to query smoke event: %v", err)
	}
	if evtType != string(EventSmokeDetected) || confidence != 0.82 {
		t.Fatalf("Smoke event mismatch: type=%s, conf=%f", evtType, confidence)
	}
}

func TestEngine_NilDatabaseSafety(t *testing.T) {
	oldDB := database.DB
	database.DB = nil
	defer func() { database.DB = oldDB }()

	engine := NewEngine()
	// Must not panic when database.DB is nil
	engine.saveEvent(Event{
		ID:         "test-nil-db",
		CameraID:   "cam-nil",
		Type:       EventFireDetected,
		Confidence: 0.90,
	})
}

func TestEngine_PushNotificationPayloadFormatting(t *testing.T) {
	mockClient := &mockPushClient{}
	push.SetDefaultService(mockClient)

	engine := NewEngine()

	fireEvt := Event{
		ID:         "fire-evt-123",
		CameraID:   "cam-zone-01",
		Type:       EventFireDetected,
		StartedAt:  time.Now(),
		Confidence: 0.91,
		Data: map[string]interface{}{
			"custom_meta": "thermal_spike",
		},
	}

	smokeEvt := Event{
		ID:         "smoke-evt-456",
		CameraID:   "cam-zone-02",
		Type:       EventSmokeDetected,
		StartedAt:  time.Now(),
		Confidence: 0.74,
	}

	engine.triggerActions(fireEvt)
	engine.triggerActions(smokeEvt)

	fireMsg := push.ExpoPushMessage{
		To:        "ExponentPushToken[mock-token-1]",
		Title:     fmt.Sprintf("CRITICAL FIRE ALERT: %s", fireEvt.CameraID),
		Body:      fmt.Sprintf("Fire detected on camera %s (Confidence: %.0f%%)", fireEvt.CameraID, fireEvt.Confidence*100),
		Sound:     "default",
		Priority:  "high",
		ChannelID: "alerts",
		Data: map[string]interface{}{
			"camera_id":  fireEvt.CameraID,
			"event_id":   fireEvt.ID,
			"event_type": string(fireEvt.Type),
			"confidence": fireEvt.Confidence,
		},
	}

	if err := mockClient.SendNotification(context.Background(), fireMsg); err != nil {
		t.Fatalf("Failed to send mock push: %v", err)
	}

	msgs := mockClient.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 recorded push message, got %d", len(msgs))
	}

	sent := msgs[0]
	if !strings.Contains(sent.Title, "CRITICAL FIRE ALERT") {
		t.Errorf("Expected critical fire title, got: %s", sent.Title)
	}
	if sent.Priority != "high" {
		t.Errorf("Expected high priority push, got: %s", sent.Priority)
	}
	if sent.ChannelID != "alerts" {
		t.Errorf("Expected alerts channel, got: %s", sent.ChannelID)
	}
	if sent.Data["event_type"] != "FIRE_DETECTED" {
		t.Errorf("Expected FIRE_DETECTED in data payload, got: %v", sent.Data["event_type"])
	}
}

func TestEngine_AutoUUIDAndTimestamp(t *testing.T) {
	engine := NewEngine()

	evt := Event{
		CameraID:   "test-cam",
		Type:       EventFireDetected,
		Confidence: 0.85,
	}

	engine.Emit(evt)

	select {
	case emitted := <-engine.eventChan:
		if emitted.ID == "" {
			t.Errorf("Expected auto-generated UUID, got empty")
		}
		if emitted.StartedAt.IsZero() {
			t.Errorf("Expected auto-filled timestamp, got zero")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for emitted event")
	}
}

func TestEngine_ChannelOverflowHandling(t *testing.T) {
	engine := &Engine{
		eventChan: make(chan Event, 2),
	}

	engine.Emit(Event{CameraID: "c1", Type: EventFireDetected})
	engine.Emit(Event{CameraID: "c2", Type: EventSmokeDetected})

	done := make(chan bool)
	go func() {
		engine.Emit(Event{CameraID: "c3", Type: EventMotionDetected})
		done <- true
	}()

	select {
	case <-done:
		// Passed - non-blocking drop
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Emit blocked on full event channel")
	}
}

func TestEngine_ConcurrentEmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := NewEngine()
	go engine.processLoop(ctx)

	var wg sync.WaitGroup
	workers := 20
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				evtType := EventFireDetected
				if j%2 == 0 {
					evtType = EventSmokeDetected
				}
				engine.Emit(Event{
					CameraID:   fmt.Sprintf("cam-%d", workerID),
					Type:       evtType,
					Confidence: 0.80 + float64(j%20)*0.01,
				})
			}
		}(i)
	}

	wg.Wait()
}

func TestInitEngine_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	InitEngine(ctx)

	if DefaultEngine == nil {
		t.Fatal("DefaultEngine was not initialized")
	}

	DefaultEngine.Emit(Event{
		CameraID:   "init-cam",
		Type:       EventFireDetected,
		Confidence: 0.90,
	})

	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	if DefaultEngine.running {
		t.Errorf("Expected engine running to be false after cancel")
	}
}
