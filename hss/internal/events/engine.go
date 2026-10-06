package events

import (
	"context"
	"log"
	"time"

	"hss/internal/database"
	"github.com/google/uuid"
)

type EventType string

const (
	EventMotionDetected EventType = "MOTION_DETECTED"
	EventPersonDetected EventType = "PERSON_DETECTED"
	EventCameraOffline  EventType = "CAMERA_OFFLINE"
	EventCameraOnline   EventType = "CAMERA_ONLINE"
)

type Event struct {
	ID          string    `json:"id"`
	CameraID    string    `json:"camera_id"`
	Type        EventType `json:"type"`
	StartedAt   time.Time `json:"started_at"`
	Confidence  float64   `json:"confidence"`
	RecordingID *string   `json:"recording_id,omitempty"`
}

type Engine struct {
	eventChan chan Event
}

var DefaultEngine *Engine

func InitEngine(ctx context.Context) {
	DefaultEngine = &Engine{
		eventChan: make(chan Event, 100),
	}
	go DefaultEngine.processLoop(ctx)
	log.Println("Event Engine initialized.")
}

func (e *Engine) Emit(evt Event) {
	if evt.ID == "" {
		evt.ID = uuid.New().String()
	}
	if evt.StartedAt.IsZero() {
		evt.StartedAt = time.Now()
	}

	select {
	case e.eventChan <- evt:
	default:
		log.Println("WARNING: Event channel full, dropping event:", evt.Type)
	}
}

func (e *Engine) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Event Engine shutting down.")
			return
		case evt := <-e.eventChan:
			e.saveEvent(evt)
			e.triggerActions(evt)
		}
	}
}

func (e *Engine) saveEvent(evt Event) {
	query := `INSERT INTO events (id, camera_id, type, started_at, confidence, recording_id, created_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?)`
	
	startedStr := evt.StartedAt.Format(time.RFC3339)
	createdStr := time.Now().Format(time.RFC3339)

	_, err := database.DB.Exec(query,
		evt.ID,
		evt.CameraID,
		evt.Type,
		startedStr,
		evt.Confidence,
		evt.RecordingID,
		createdStr,
	)
	if err != nil {
		log.Printf("Error saving event to DB: %v", err)
	} else {
		log.Printf("Event %s stored: %s on camera %s", evt.ID, evt.Type, evt.CameraID)
	}
}

func (e *Engine) triggerActions(evt Event) {
	// E.g., if motion is detected, maybe trigger an immediate recording segment boundary
	// or send a push notification via the control plane.
}
