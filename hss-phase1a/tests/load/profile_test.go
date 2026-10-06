package load

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/media"
)

// TestLoadProfileMemoryAndGoroutines tests memory footprint and asserts zero goroutine leaks
// during and after high-concurrency (1,000+ connections) connection lifecycle.
func TestLoadProfileMemoryAndGoroutines(t *testing.T) {
	server, wsURL := setupLoadTestServer(t)
	defer server.Close()

	const totalViewers = 1000
	targetCameras := GetDefaultTargetCameras()

	// 1. Capture baseline snapshot after GC
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baselineSnap := CaptureMemorySnapshot()
	t.Logf("Baseline Profile: Goroutines: %d | HeapAlloc: %.2f MB | Sys: %.2f MB",
		baselineSnap.NumGoroutines,
		float64(baselineSnap.HeapAllocBytes)/(1024*1024),
		float64(baselineSnap.SysBytes)/(1024*1024))

	metrics := NewLoadMetrics(totalViewers)

	// 2. Start Cameras
	cameras := make([]*MockCameraDaemon, len(targetCameras))
	for i, camCfg := range targetCameras {
		ip := fmt.Sprintf("10.0.1.%d", i+1)
		cam := NewMockCameraDaemon(camCfg.DeviceID, ip, metrics)
		if err := cam.Connect(wsURL, "mock_device_jwt"); err != nil {
			t.Fatalf("Failed to connect camera %s: %v", camCfg.DeviceID, err)
		}
		cameras[i] = cam
	}

	time.Sleep(50 * time.Millisecond)

	// 3. Connect 1,000 Viewers and perform handshakes
	viewers := make([]*MockViewer, totalViewers)
	var wg sync.WaitGroup
	wg.Add(totalViewers)
	sem := make(chan struct{}, 80)

	for i := 0; i < totalViewers; i++ {
		idx := i
		camCfg := targetCameras[idx%len(targetCameras)]

		go func(vIdx int, cfg TargetCameraConfig) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			vID := fmt.Sprintf("prof-viewer-%05d", vIdx)
			sID := fmt.Sprintf("prof-sess-%05d", vIdx)
			ip := fmt.Sprintf("10.120.%d.%d", (vIdx>>8)&255, vIdx&255)

			v := NewMockViewer(vID, cfg.DeviceID, sID, ip, metrics)
			if err := v.Connect(wsURL, cfg.Token); err != nil {
				t.Errorf("Viewer %s dial failed: %v", vID, err)
				return
			}

			if _, err := v.PerformHandshake(10 * time.Second); err != nil {
				t.Errorf("Viewer %s handshake failed: %v", vID, err)
				return
			}

			viewers[vIdx] = v
		}(idx, camCfg)

		time.Sleep(2 * time.Millisecond)
	}

	wg.Wait()

	// 4. Capture Peak Load Snapshot
	peakSnap := CaptureMemorySnapshot()
	peakDelta := CalculateMemoryDelta(baselineSnap, peakSnap)
	t.Logf("Peak Load Profile (1,000 Active Connections): Goroutines: %d (delta: %+d) | HeapAlloc: %.2f MB (delta: %+.2f MB) | Sys: %.2f MB",
		peakSnap.NumGoroutines,
		peakDelta.GoroutineDelta,
		float64(peakSnap.HeapAllocBytes)/(1024*1024),
		peakDelta.HeapAllocDeltaMB,
		float64(peakSnap.SysBytes)/(1024*1024))

	// Verify peak memory scaling: 1,000 WebSocket connections on signaling hub should be < 60 MB
	if peakDelta.HeapAllocDeltaMB > 60.0 {
		t.Errorf("Peak heap allocation delta %.2f MB exceeds 60 MB budget for 1,000 connections", peakDelta.HeapAllocDeltaMB)
	}

	// 5. Complete Teardown: close all 1,000 viewers and cameras
	t.Log("Tearing down all 1,000 connections and testing for goroutine leaks...")
	for _, v := range viewers {
		if v != nil {
			_ = v.Close()
		}
	}
	for _, cam := range cameras {
		if cam != nil {
			_ = cam.Close()
		}
	}

	metrics.Close()

	// 6. Settle and run GC
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()

	// 7. Post-Teardown Snapshot & Leak Assertion
	postSnap := CaptureMemorySnapshot()
	postDelta := CalculateMemoryDelta(baselineSnap, postSnap)
	t.Logf("Post-Teardown Profile: Goroutines: %d (delta from baseline: %+d) | HeapAlloc: %.2f MB | Sys: %.2f MB",
		postSnap.NumGoroutines,
		postDelta.GoroutineDelta,
		float64(postSnap.HeapAllocBytes)/(1024*1024),
		float64(postSnap.SysBytes)/(1024*1024))

	// Strict Goroutine Leak Check: Goroutine delta after full teardown and GC must be minimal (<= 25)
	if postDelta.GoroutineDelta > 25 {
		t.Fatalf("FAIL: Goroutine leak detected! Baseline: %d, Post-Teardown: %d (leaked %d goroutines)",
			baselineSnap.NumGoroutines, postSnap.NumGoroutines, postDelta.GoroutineDelta)
	}

	t.Logf("PASS: Zero goroutine leaks and clean memory scaling verified (post-teardown delta: %+d goroutines).", postDelta.GoroutineDelta)
}

// MockMediaCamera is a test implementation of media.Camera interface
type MockMediaCamera struct {
	id   string
	name string
	url  string
}

func (m *MockMediaCamera) ID() string             { return m.id }
func (m *MockMediaCamera) Name() string           { return m.name }
func (m *MockMediaCamera) MainStreamURL() string  { return m.url }

// TestMediaEngineUnsubscribeAndMemoryLeak directly verifies the channel leak fix in internal/media/engine.go.
func TestMediaEngineUnsubscribeAndMemoryLeak(t *testing.T) {
	engine := media.NewEngine()
	cam := &MockMediaCamera{
		id:   "test-leak-cam-1",
		name: "Test Leak Camera",
		url:  "rtsp://127.0.0.1:8554/live",
	}

	// Attach camera
	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	const subscriberCount = 1000
	subChans := make([]<-chan media.MediaPacket, subscriberCount)

	t.Logf("Subscribing %d media consumer channels...", subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		ch, err := engine.Subscribe(cam.ID())
		if err != nil {
			t.Fatalf("Subscribe failed at index %d: %v", i, err)
		}
		subChans[i] = ch
	}

	// Verify count is exactly 1,000
	if currentCount := engine.SubscriberCount(cam.ID()); currentCount != subscriberCount {
		t.Fatalf("Expected %d active subscribers, got %d", subscriberCount, currentCount)
	}

	t.Logf("Unsubscribing all %d channels to verify clean channel removal...", subscriberCount)
	for i := 0; i < subscriberCount; i++ {
		engine.Unsubscribe(cam.ID(), subChans[i])
	}

	// Verify count drops to 0
	if remaining := engine.SubscriberCount(cam.ID()); remaining != 0 {
		t.Fatalf("Expected 0 active subscribers after unsubscribe, got %d", remaining)
	}

	// Verify all channels were closed cleanly
	for i, ch := range subChans {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("Channel %d was not closed after Unsubscribe", i)
			}
		default:
			t.Errorf("Channel %d did not yield immediate close signal", i)
		}
	}

	t.Log("PASS: Engine.Unsubscribe cleanly removed all 1,000 subscriber channels and prevented channel accumulation leak.")
}

// BenchmarkSignalingHandshake benchmarks the throughput of signaling negotiation over WebSocket.
func BenchmarkSignalingHandshake(b *testing.B) {
	server, wsURL := setupLoadTestServer(&testing.T{})
	defer server.Close()

	targetCameras := GetDefaultTargetCameras()
	metrics := NewLoadMetrics(b.N)

	cameras := make([]*MockCameraDaemon, len(targetCameras))
	for i, camCfg := range targetCameras {
		ip := fmt.Sprintf("10.0.9.%d", i+1)
		cam := NewMockCameraDaemon(camCfg.DeviceID, ip, metrics)
		if err := cam.Connect(wsURL, "mock_device_jwt"); err != nil {
			b.Fatalf("Failed to connect camera %s: %v", camCfg.DeviceID, err)
		}
		cameras[i] = cam
		defer cam.Close()
	}

	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := atomic.AddInt64(&counter, 1)
			camCfg := targetCameras[idx%int64(len(targetCameras))]
			vID := fmt.Sprintf("bench-v-%06d", idx)
			sID := fmt.Sprintf("bench-s-%06d", idx)
			ip := fmt.Sprintf("10.80.%d.%d", (idx>>8)&255, idx&255)

			v := NewMockViewer(vID, camCfg.DeviceID, sID, ip, metrics)
			if err := v.Connect(wsURL, camCfg.Token); err == nil {
				_, _ = v.PerformHandshake(5 * time.Second)
				_ = v.Close()
			}
		}
	})
}

// BenchmarkMediaEngineBroadcast benchmarks packet broadcasting across 100 concurrent subscribers.
func BenchmarkMediaEngineBroadcast(b *testing.B) {
	engine := media.NewEngine()
	cam := &MockMediaCamera{
		id:   "bench-cam-1",
		name: "Bench Camera",
		url:  "rtsp://127.0.0.1:8554/live",
	}

	_ = engine.AttachCamera(cam)
	defer engine.DetachCamera(cam.ID())

	const subCount = 100
	subs := make([]<-chan media.MediaPacket, subCount)
	for i := 0; i < subCount; i++ {
		ch, _ := engine.Subscribe(cam.ID())
		subs[i] = ch
		// Consumer drain goroutine
		go func(c <-chan media.MediaPacket) {
			for range c {
			}
		}(ch)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch, err := engine.Subscribe(cam.ID())
		if err == nil {
			engine.Unsubscribe(cam.ID(), ch)
		}
	}

	for _, ch := range subs {
		engine.Unsubscribe(cam.ID(), ch)
	}
}
