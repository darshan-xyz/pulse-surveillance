package camera

import (
	"context"
	"testing"
	"time"
)

func TestCameraManager_InitManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := InitManager(ctx); err != nil {
		t.Fatalf("InitManager returned error: %v", err)
	}

	// Verify YOLO11 detector was initialized and registered
	detector, found := GetActiveDetector("test-camera-1")
	if !found || detector == nil {
		t.Fatal("Expected active YOLO11 detector for test-camera-1")
	}

	if detector.CameraID() != "test-camera-1" {
		t.Fatalf("Expected detector camera ID test-camera-1, got %s", detector.CameraID())
	}
}

func TestCameraManager_GetCameras_NilDB(t *testing.T) {
	cams, err := GetCameras()
	if err != nil {
		t.Fatalf("GetCameras returned error: %v", err)
	}
	if cams == nil {
		t.Fatal("Expected non-nil slice from GetCameras")
	}
}

func TestCameraManager_SyntheticCamera(t *testing.T) {
	cam := NewSyntheticCamera("synth-01", "Synthetic Camera 1")
	if cam.ID() != "synth-01" {
		t.Fatalf("Expected ID synth-01, got %s", cam.ID())
	}
	if cam.Name() != "Synthetic Camera 1" {
		t.Fatalf("Expected Name Synthetic Camera 1, got %s", cam.Name())
	}
	if cam.Status() != "online" {
		t.Fatalf("Expected Status online, got %s", cam.Status())
	}
	if len(cam.Capabilities()) == 0 {
		t.Fatal("Expected non-empty capabilities")
	}
	if cam.MainStreamURL() != "synthetic://test-pattern" {
		t.Fatalf("Unexpected main stream URL: %s", cam.MainStreamURL())
	}
	if cam.SubStreamURL() != "synthetic://test-pattern" {
		t.Fatalf("Unexpected sub stream URL: %s", cam.SubStreamURL())
	}
}

func TestCameraManager_GenericCamera(t *testing.T) {
	cam := NewGenericCamera("gen-01", "Generic RTSP Cam", "rtsp://10.0.0.1/live")
	if cam.ID() != "gen-01" {
		t.Fatalf("Expected ID gen-01, got %s", cam.ID())
	}
	if cam.Name() != "Generic RTSP Cam" {
		t.Fatalf("Expected Name Generic RTSP Cam, got %s", cam.Name())
	}
	if cam.MainStreamURL() != "rtsp://10.0.0.1/live" {
		t.Fatalf("Unexpected MainStreamURL: %s", cam.MainStreamURL())
	}
	if cam.SubStreamURL() != "rtsp://10.0.0.1/live" {
		t.Fatalf("Unexpected SubStreamURL: %s", cam.SubStreamURL())
	}
}

func TestCameraManager_Supervisor(t *testing.T) {
	cam := NewSyntheticCamera("sup-cam", "Supervisor Test Cam")
	sup := NewSupervisor(cam)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)

	go func() {
		sup.Run(ctx)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Passed - cleanly shutdown
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Supervisor did not shut down in timely manner")
	}
}
