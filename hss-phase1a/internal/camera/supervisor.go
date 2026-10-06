package camera

import (
	"context"
	"log"
	"time"
)

type Supervisor struct {
	cam Camera
}

func NewSupervisor(cam Camera) *Supervisor {
	return &Supervisor{
		cam: cam,
	}
}

// Run executes the camera connection lifecycle with exponential backoff.
func (s *Supervisor) Run(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Printf("Supervisor shutting down camera: %s", s.cam.Name())
			s.cam.Disconnect()
			return
		default:
			log.Printf("Attempting to connect camera: %s", s.cam.Name())
			err := s.cam.Connect()

			if err == nil {
				// Connection successful, reset backoff
				log.Printf("Camera %s connected successfully.", s.cam.Name())
				backoff = 1 * time.Second
				
				// Here we would block while the stream is healthy
				// For now, simulate blocking until an error occurs or ctx cancels
				s.monitorStream(ctx)
			} else {
				log.Printf("Camera %s connection failed: %v", s.cam.Name(), err)
			}

			// Disconnect to clean up resources before retry
			s.cam.Disconnect()

			log.Printf("Camera %s reconnecting in %v...", s.cam.Name(), backoff)
			
			// Wait for backoff duration or cancellation
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (s *Supervisor) monitorStream(ctx context.Context) {
	// In a real implementation, this blocks while reading frames or checking health pings.
	// Returns when stream fails.
	<-ctx.Done()
}
