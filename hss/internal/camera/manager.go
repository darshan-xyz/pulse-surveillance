package camera

import (
	"context"
	"log"

	"hss/internal/ai"
	"hss/internal/database"
)

type Camera struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Status   string `json:"status"`
}

func InitManager(ctx context.Context) error {
	log.Println("Initializing Camera Manager...")
	
	// Inject Mock RTSP Camera for Phase 1 Media Foundation Flow
	mockCam := NewGenericCamera("cam_001_mock", "Test Pattern Stream", "rtsp://localhost:8554/cam1")
	
	// Boot the supervisor for this camera in the background
	sup := NewSupervisor(mockCam)
	go sup.Run(ctx)
	
	// Boot the AI Plugin for this camera
	motionDetector := ai.NewMockMotionDetector(mockCam.ID())
	motionDetector.Start(ctx)

	log.Printf("Bootstrapped mock camera %s. Supervisor & AI Plugin running.", mockCam.Name())
	return nil
}

func GetCameras() ([]Camera, error) {
	rows, err := database.DB.Query("SELECT id, name, protocol, address, status FROM cameras")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []Camera
	for rows.Next() {
		var c Camera
		if err := rows.Scan(&c.ID, &c.Name, &c.Protocol, &c.Address, &c.Status); err != nil {
			return nil, err
		}
		cameras = append(cameras, c)
	}
	if cameras == nil {
		cameras = make([]Camera, 0)
	}
	return cameras, nil
}
