package camera

import (
	"context"
	"log"
	"sync"
	"time"

	"hss/internal/ai"
	"hss/internal/cloud/signaling"
	"hss/internal/control"
	"hss/internal/database"
	"hss/internal/security"
)

type CameraConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Status   string `json:"status"`
}

var (
	detectorsMu     sync.RWMutex
	activeDetectors = make(map[string]*ai.YOLO11Detector)
)

// GetActiveDetector retrieves the YOLO11Detector instance for a given camera ID.
func GetActiveDetector(cameraID string) (*ai.YOLO11Detector, bool) {
	detectorsMu.RLock()
	defer detectorsMu.RUnlock()
	d, ok := activeDetectors[cameraID]
	return d, ok
}

// InitManager initializes primary camera streaming, supervisor, and AI detection pipeline.
func InitManager(ctx context.Context) error {
	log.Println("Initializing Camera Manager...")
	
	camID := "test-camera-1"
	if security.DeviceID != "" {
		camID = security.DeviceID
	}
	
	// Register Primary Camera Stream aligned with test-camera-1 / security.DeviceID
	primaryCam := NewSyntheticCamera(camID, "Warehouse North Wing (Synthetic H.264)")
	
	// Boot the supervisor for this camera in the background
	sup := NewSupervisor(primaryCam)
	go sup.Run(ctx)
	
	// Boot the YOLO11 AI Fire & Smoke Detector for this camera (real inference only)
	yoloDetector := ai.NewYOLO11Detector(primaryCam.ID(),
		ai.WithConfidenceThreshold(0.35),
		ai.WithMockFallback(false),
		ai.WithSampleInterval(500*time.Millisecond),
	)

	// Wire detection listener to broadcast detection metadata packets to active WebSocket viewers
	yoloDetector.OnDetection(func(cameraID string, resp *ai.DetectionResponse) {
		if resp == nil || len(resp.Detections) == 0 {
			return
		}

		dets := make([]signaling.DetectionItem, len(resp.Detections))
		for i, d := range resp.Detections {
			className := d.ClassName
			classID := d.ClassID
			if className == "" {
				if classID == 0 {
					className = "fire"
				} else if classID == 1 {
					className = "smoke"
				}
			}
			dets[i] = signaling.DetectionItem{
				Class:      className,
				ClassID:    classID,
				Confidence: d.Confidence,
				Box:        d.Box,
			}
		}

		ts := int64(resp.Timestamp * 1000)
		if ts <= 0 {
			ts = time.Now().UnixMilli()
		}

		msg := signaling.DetectionMessage{
			Type:            "detection",
			CameraID:        cameraID,
			Timestamp:       ts,
			Detections:      dets,
			InferenceTimeMs: resp.InferenceTimeMs,
		}

		signaling.BroadcastDetection(msg)
		_ = control.SendSignalingMessage(msg)
	})

	if err := yoloDetector.Start(ctx); err != nil {
		log.Printf("[CameraManager] Warning: could not start YOLO11 detector for %s: %v", primaryCam.ID(), err)
	}

	detectorsMu.Lock()
	activeDetectors[primaryCam.ID()] = yoloDetector
	detectorsMu.Unlock()

	// Also keep mock motion detector active for motion alerts
	motionDetector := ai.NewMockMotionDetector(primaryCam.ID())
	motionDetector.Start(ctx)

	log.Printf("Bootstrapped primary camera %s (%s). Supervisor, YOLO11 Detector & Motion Detector running.", primaryCam.ID(), primaryCam.Name())
	return nil
}

func GetCameras() ([]CameraConfig, error) {
	if database.DB == nil {
		return make([]CameraConfig, 0), nil
	}

	rows, err := database.DB.Query("SELECT id, name, protocol, address, status FROM cameras")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []CameraConfig
	for rows.Next() {
		var c CameraConfig
		if err := rows.Scan(&c.ID, &c.Name, &c.Protocol, &c.Address, &c.Status); err != nil {
			return nil, err
		}
		cameras = append(cameras, c)
	}
	if cameras == nil {
		cameras = make([]CameraConfig, 0)
	}
	return cameras, nil
}
