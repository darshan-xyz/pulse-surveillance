package signaling

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSignalingHub_RegisterAndUnregister(t *testing.T) {
	hub := NewSignalingHub()

	s1 := &ViewerSession{
		ID:       "sess-1",
		Username: "user1",
		CameraID: "cam-1",
	}

	hub.RegisterViewer(s1)
	if count := hub.ActiveViewerCount(); count != 1 {
		t.Fatalf("Expected 1 viewer, got %d", count)
	}

	hub.UnregisterViewer(s1.ID)
	if count := hub.ActiveViewerCount(); count != 0 {
		t.Fatalf("Expected 0 viewers, got %d", count)
	}
}

func TestSignalingHub_UpdateCamera(t *testing.T) {
	hub := NewSignalingHub()

	s := &ViewerSession{
		ID:       "sess-update",
		Username: "user1",
		CameraID: "cam-initial",
	}
	hub.RegisterViewer(s)

	hub.UpdateViewerCamera(s.ID, "cam-updated")

	hub.mu.RLock()
	retrieved := hub.viewers[s.ID]
	camMap := hub.camViewers["cam-updated"]
	oldCamMap := hub.camViewers["cam-initial"]
	hub.mu.RUnlock()

	if retrieved.CameraID != "cam-updated" {
		t.Fatalf("Expected camera cam-updated, got %s", retrieved.CameraID)
	}
	if camMap == nil || camMap[s.ID] == nil {
		t.Fatal("Expected viewer in cam-updated mapping")
	}
	if oldCamMap != nil && oldCamMap[s.ID] != nil {
		t.Fatal("Expected viewer removed from old camera mapping")
	}
}

func TestSignalingHub_DetectionPacketFormat(t *testing.T) {
	msg := DetectionMessage{
		Type:      "detection",
		CameraID:  "test-camera-1",
		Timestamp: 1772465000000,
		Detections: []DetectionItem{
			{
				Class:      "fire",
				ClassID:    0,
				Confidence: 0.85,
				Box:        []float64{0.35, 0.42, 0.15, 0.28},
			},
		},
		InferenceTimeMs: 14.2,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var parsed DetectionMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if parsed.Type != "detection" || parsed.CameraID != "test-camera-1" {
		t.Fatalf("Parsed metadata mismatch: %+v", parsed)
	}
	if len(parsed.Detections) != 1 || parsed.Detections[0].Class != "fire" || parsed.Detections[0].Confidence != 0.85 {
		t.Fatalf("Parsed detection item mismatch: %+v", parsed.Detections)
	}
}

func TestSignalingHub_ViewerDetectionStreamingIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(HandleViewerConnection))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/client?token=admin&camera_id=test-camera-1"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	detMsg := DetectionMessage{
		Type:      "detection",
		CameraID:  "test-camera-1",
		Timestamp: time.Now().UnixMilli(),
		Detections: []DetectionItem{
			{
				Class:      "fire",
				ClassID:    0,
				Confidence: 0.92,
				Box:        []float64{0.1, 0.2, 0.3, 0.4},
			},
		},
	}

	BroadcastDetection(detMsg)

	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	var received DetectionMessage
	if err := conn.ReadJSON(&received); err != nil {
		t.Fatalf("Failed to read broadcast message: %v", err)
	}

	if received.Type != "detection" || received.CameraID != "test-camera-1" {
		t.Fatalf("Received message mismatch: %+v", received)
	}
	if len(received.Detections) != 1 || received.Detections[0].Confidence != 0.92 {
		t.Fatalf("Detection payload mismatch: %+v", received.Detections)
	}
}

func TestSignalingHub_MultiViewerCameraFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(HandleViewerConnection))
	defer server.Close()

	wsBase := "ws" + strings.TrimPrefix(server.URL, "http")

	// Viewer 1 subscribed to cam-A
	connA, _, err := websocket.DefaultDialer.Dial(wsBase+"/client?token=admin&camera_id=cam-A", nil)
	if err != nil {
		t.Fatalf("ConnA failed: %v", err)
	}
	defer connA.Close()

	// Viewer 2 subscribed to cam-B
	connB, _, err := websocket.DefaultDialer.Dial(wsBase+"/client?token=admin&camera_id=cam-B", nil)
	if err != nil {
		t.Fatalf("ConnB failed: %v", err)
	}
	defer connB.Close()

	time.Sleep(50 * time.Millisecond)

	// Broadcast only to cam-A
	detA := DetectionMessage{
		Type:      "detection",
		CameraID:  "cam-A",
		Timestamp: time.Now().UnixMilli(),
		Detections: []DetectionItem{
			{Class: "smoke", ClassID: 1, Confidence: 0.78, Box: []float64{0.2, 0.2, 0.4, 0.4}},
		},
	}
	BroadcastDetection(detA)

	_ = connA.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var recvA DetectionMessage
	if err := connA.ReadJSON(&recvA); err != nil {
		t.Fatalf("ConnA failed to receive cam-A packet: %v", err)
	}
	if recvA.CameraID != "cam-A" {
		t.Fatalf("Expected cam-A packet, got: %s", recvA.CameraID)
	}

	// connB should NOT receive cam-A packet
	_ = connB.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var recvB DetectionMessage
	if err := connB.ReadJSON(&recvB); err == nil {
		t.Fatalf("ConnB unexpectedly received cam-A packet: %+v", recvB)
	}
}

func TestSignalingHub_ConcurrentBroadcast(t *testing.T) {
	hub := NewSignalingHub()

	var conns []*websocket.Conn
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		safe := newSafeWS(conn)
		s := &ViewerSession{
			ID:       r.URL.Query().Get("id"),
			CameraID: "test-cam",
			WS:       safe,
		}
		hub.RegisterViewer(s)
		defer hub.UnregisterViewer(s.ID)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	for i := 0; i < 5; i++ {
		c, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("%s?id=viewer-%d", wsURL, i), nil)
		if err != nil {
			t.Fatalf("Failed to dial client %d: %v", i, err)
		}
		conns = append(conns, c)
		defer c.Close()
	}

	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	workers := 10
	broadcasts := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < broadcasts; j++ {
				hub.BroadcastDetection(DetectionMessage{
					Type:      "detection",
					CameraID:  "test-cam",
					Timestamp: time.Now().UnixMilli(),
					Detections: []DetectionItem{
						{Class: "fire", ClassID: 0, Confidence: 0.88, Box: []float64{0.1, 0.1, 0.2, 0.2}},
					},
				})
			}
		}(i)
	}

	wg.Wait()
}

func TestHandleViewerConnection_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(HandleViewerConnection))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/client"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("Expected unauthorized error, got success")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got: %d", resp.StatusCode)
	}
}

func TestHandleDeviceConnection_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(HandleDeviceConnection))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/signaling"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("Expected unauthorized error for device, got success")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Expected 401 Unauthorized, got: %d", resp.StatusCode)
	}
}
