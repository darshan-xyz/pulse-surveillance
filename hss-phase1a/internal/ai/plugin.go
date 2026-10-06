package ai

import (
	"context"
	"log"
	"math/rand"
	"time"

	"hss/internal/events"
)

type Plugin interface {
	Name() string
	Start(ctx context.Context) error
}

type MockMotionDetector struct {
	cameraID string
}

func NewMockMotionDetector(cameraID string) *MockMotionDetector {
	return &MockMotionDetector{
		cameraID: cameraID,
	}
}

func (m *MockMotionDetector) Name() string {
	return "MockMotionDetector"
}

func (m *MockMotionDetector) Start(ctx context.Context) error {
	log.Printf("Starting AI Plugin: %s for camera: %s", m.Name(), m.cameraID)
	
	go func() {
		// Mock logic: randomly trigger motion every 2 to 5 minutes
		for {
			delay := time.Duration(rand.Intn(3)+2) * time.Minute
			
			select {
			case <-ctx.Done():
				log.Printf("Shutting down AI Plugin: %s", m.Name())
				return
			case <-time.After(delay):
				evt := events.Event{
					CameraID:   m.cameraID,
					Type:       events.EventMotionDetected,
					StartedAt:  time.Now(),
					Confidence: 0.95, // 95% confident mock
				}
				events.DefaultEngine.Emit(evt)
				log.Printf("AI Plugin %s emitted Motion Detected event", m.Name())
			}
		}
	}()

	return nil
}
