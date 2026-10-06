package events

import (
	"context"
	"fmt"
	"log"
	"time"

	"hss/internal/cloud/db"
	"hss/internal/cloud/push"
	"hss/internal/database"
	"github.com/google/uuid"
)

type EventType string

const (
	EventMotionDetected EventType = "MOTION_DETECTED"
	EventPersonDetected EventType = "PERSON_DETECTED"
	EventFireDetected   EventType = "FIRE_DETECTED"
	EventSmokeDetected  EventType = "SMOKE_DETECTED"
	EventCameraOffline  EventType = "CAMERA_OFFLINE"
	EventCameraOnline   EventType = "CAMERA_ONLINE"
)

type Event struct {
	ID          string                 `json:"id"`
	CameraID    string                 `json:"camera_id"`
	Type        EventType              `json:"type"`
	StartedAt   time.Time              `json:"started_at"`
	Confidence  float64                `json:"confidence"`
	RecordingID *string                `json:"recording_id,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

type Engine struct {
	eventChan chan Event
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
}

var DefaultEngine = NewEngine()

func NewEngine() *Engine {
	return &Engine{
		eventChan: make(chan Event, 256),
	}
}

func InitEngine(ctx context.Context) {
	if DefaultEngine == nil {
		DefaultEngine = NewEngine()
	}
	if !DefaultEngine.running {
		DefaultEngine.ctx, DefaultEngine.cancel = context.WithCancel(ctx)
		DefaultEngine.running = true
		go DefaultEngine.processLoop(DefaultEngine.ctx)
	}
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
			e.running = false
			return
		case evt := <-e.eventChan:
			e.saveEvent(evt)
			e.triggerActions(evt)
		}
	}
}

func (e *Engine) saveEvent(evt Event) {
	if database.DB == nil {
		return
	}

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
	// Look up authorized push notification tokens for the camera
	tokens, err := db.GetPushTokensForDevice(evt.CameraID)
	if err != nil {
		log.Printf("[Events] Error retrieving push tokens for camera %s: %v", evt.CameraID, err)
	}

	if len(tokens) == 0 {
		return
	}

	var title string
	var body string

	switch evt.Type {
	case EventFireDetected:
		title = fmt.Sprintf("CRITICAL FIRE ALERT: %s", evt.CameraID)
		body = fmt.Sprintf("Fire detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	case EventSmokeDetected:
		title = fmt.Sprintf("WARNING SMOKE ALERT: %s", evt.CameraID)
		body = fmt.Sprintf("Smoke detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	case EventMotionDetected:
		title = fmt.Sprintf("Motion Alert: %s", evt.CameraID)
		body = fmt.Sprintf("Motion detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	case EventPersonDetected:
		title = fmt.Sprintf("Person Alert: %s", evt.CameraID)
		body = fmt.Sprintf("Person detected on camera %s (Confidence: %.0f%%)", evt.CameraID, evt.Confidence*100)
	case EventCameraOffline:
		title = fmt.Sprintf("Camera Offline: %s", evt.CameraID)
		body = fmt.Sprintf("Camera %s is offline", evt.CameraID)
	case EventCameraOnline:
		title = fmt.Sprintf("Camera Online: %s", evt.CameraID)
		body = fmt.Sprintf("Camera %s is online", evt.CameraID)
	default:
		title = fmt.Sprintf("Camera Alert: %s", evt.Type)
		body = fmt.Sprintf("Event %s detected on camera %s (Confidence: %.0f%%)", evt.Type, evt.CameraID, evt.Confidence*100)
	}

	for _, token := range tokens {
		dataPayload := map[string]interface{}{
			"camera_id":  evt.CameraID,
			"event_id":   evt.ID,
			"event_type": string(evt.Type),
			"confidence": evt.Confidence,
			"timestamp":  evt.StartedAt.Format(time.RFC3339),
		}
		if evt.Data != nil {
			for k, v := range evt.Data {
				dataPayload[k] = v
			}
		}

		msg := push.ExpoPushMessage{
			To:        token,
			Title:     title,
			Body:      body,
			Sound:     "default",
			Priority:  "high",
			ChannelID: "alerts",
			Data:      dataPayload,
		}

		go func(m push.ExpoPushMessage) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			svc := push.GetDefaultService()
			if svc != nil {
				if err := svc.SendNotification(ctx, m); err != nil {
					log.Printf("[Events] Push notification delivery failed for token %s: %v", m.To, err)
				} else {
					log.Printf("[Events] Push notification successfully dispatched to token %s for event %s", m.To, evt.ID)
				}
			}
		}(msg)
	}
}
