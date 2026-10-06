package media

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockCamera struct {
	id     string
	name   string
	stream string
}

func (m *mockCamera) ID() string             { return m.id }
func (m *mockCamera) Name() string           { return m.name }
func (m *mockCamera) MainStreamURL() string  { return m.stream }

func TestEngine_AttachAndSubscribeImmediateKeyFrame(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-1",
		name:   "Test Camera 1",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	err := engine.AttachCamera(cam)
	if err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// Subscribe immediately
	ch, err := engine.Subscribe("test-camera-1")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer engine.Unsubscribe("test-camera-1", ch)

	// We MUST receive the initial cached KeyFrame immediately (< 50ms)
	select {
	case pkt, ok := <-ch:
		if !ok {
			t.Fatal("Subscriber channel closed unexpectedly")
		}
		if !pkt.IsKeyFrame {
			t.Fatal("Expected first packet from Subscribe to be a KeyFrame")
		}
		if len(pkt.Payload) < 3 {
			t.Fatalf("Expected at least 3 NALUs (SPS, PPS, IDR) in cached KeyFrame, got: %d", len(pkt.Payload))
		}
		if pkt.Payload[0][0]&0x1F != 7 {
			t.Errorf("Expected NALU 0 to be SPS (type 7), got: %d", pkt.Payload[0][0]&0x1F)
		}
		if pkt.Payload[1][0]&0x1F != 8 {
			t.Errorf("Expected NALU 1 to be PPS (type 8), got: %d", pkt.Payload[1][0]&0x1F)
		}
		if pkt.Payload[2][0]&0x1F != 5 {
			t.Errorf("Expected NALU 2 to be IDR (type 5), got: %d", pkt.Payload[2][0]&0x1F)
		}
		t.Log("SUCCESS: Client received immediate cached KeyFrame upon Subscribe!")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout: Did not receive cached KeyFrame on Subscribe")
	}
}

func TestEngine_AliasAndFallbackResolution(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-1",
		name:   "Primary Test Camera",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// Test 1: Subscribe with alias "cam_001_mock"
	ch1, err := engine.Subscribe("cam_001_mock")
	if err != nil {
		t.Fatalf("Failed to subscribe via alias cam_001_mock: %v", err)
	}
	engine.Unsubscribe("cam_001_mock", ch1)

	// Test 2: Subscribe with empty string ""
	ch2, err := engine.Subscribe("")
	if err != nil {
		t.Fatalf("Failed to subscribe via default empty ID: %v", err)
	}
	engine.Unsubscribe("", ch2)

	// Test 3: GetTrack with alias
	track, err := engine.GetTrack("cam_001_mock", "high")
	if err != nil || track == nil {
		t.Fatalf("Failed to get track via alias: %v", err)
	}

	t.Log("SUCCESS: Engine successfully resolved aliases and single-camera fallback")
}

func TestEngine_RequestKeyFrame(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-1",
		name:   "Primary Test Camera",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// Request KeyFrame
	engine.RequestKeyFrame("test-camera-1")

	// Check that synthetic generator forceKeyframe flag was set
	gen := engine.syntheticGens["test-camera-1"]
	if gen == nil {
		t.Fatal("Synthetic generator not registered on engine")
	}

	nextFrame := gen.GenerateNextFrame()
	if !nextFrame.IsKeyFrame {
		t.Fatal("Expected next frame after RequestKeyFrame to be a KeyFrame")
	}

	t.Log("SUCCESS: RequestKeyFrame correctly triggered immediate IDR keyframe")
}

func TestEngine_UnsubscribeCleanup(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-1",
		name:   "Primary Test Camera",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	ch1, _ := engine.Subscribe("test-camera-1")
	ch2, _ := engine.Subscribe("test-camera-1")

	if engine.SubscriberCount("test-camera-1") != 2 {
		t.Fatalf("Expected 2 subscribers, got: %d", engine.SubscriberCount("test-camera-1"))
	}

	engine.Unsubscribe("test-camera-1", ch1)
	if engine.SubscriberCount("test-camera-1") != 1 {
		t.Fatalf("Expected 1 subscriber after unsubscribe, got: %d", engine.SubscriberCount("test-camera-1"))
	}

	engine.Unsubscribe("test-camera-1", ch2)
	if engine.SubscriberCount("test-camera-1") != 0 {
		t.Fatalf("Expected 0 subscribers after unsubscribe all, got: %d", engine.SubscriberCount("test-camera-1"))
	}
}

func TestEngine_LockFreeDispatchConcurrency(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-concurrent",
		name:   "Concurrent Test Camera",
		stream: "rtsp://127.0.0.1:8554/cam_conc",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// Subscribe 50 concurrent readers
	const numSubscribers = 50
	subs := make([]<-chan MediaPacket, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		ch, err := engine.Subscribe(cam.ID())
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		defer engine.Unsubscribe(cam.ID(), ch)
		subs[i] = ch
	}

	// Concurrently process media packets while subscribers consume
	gen := NewSyntheticVideoGenerator(320, 240, 30)
	track, _ := engine.GetTrack(cam.ID(), "high")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 60; i++ {
			pkt := gen.GenerateNextFrame()
			engine.processMediaPacket(cam.ID(), pkt, track)
		}
	}()

	var readCount int64
	for i := 0; i < numSubscribers; i++ {
		wg.Add(1)
		go func(ch <-chan MediaPacket) {
			defer wg.Done()
			timeout := time.After(500 * time.Millisecond)
			for {
				select {
				case _, ok := <-ch:
					if ok {
						atomic.AddInt64(&readCount, 1)
					}
				case <-timeout:
					return
				}
			}
		}(subs[i])
	}

	wg.Wait()
	if readCount == 0 {
		t.Fatal("Expected subscribers to receive packets concurrently")
	}
	t.Logf("PASS: Successfully received %d packets across %d subscribers under concurrent lock-free dispatch",
		readCount, numSubscribers)
}

// Suppress unused imports
var _ = context.Background

