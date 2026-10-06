package webrtc

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"hss/internal/media"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

func TestManager_HandleOfferAndAnswer(t *testing.T) {
	mgr := NewManager()

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client peer connection: %v", err)
	}
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add transceiver: %v", err)
	}

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set local description: %v", err)
	}

	sessionID := "test-session-1"
	camID := "test-camera-1"

	var localCandidates []string
	var candMu sync.Mutex
	onCandidate := func(c *webrtc.ICECandidate) {
		candMu.Lock()
		defer candMu.Unlock()
		candJSON, _ := json.Marshal(c.ToJSON())
		localCandidates = append(localCandidates, string(candJSON))
	}

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onCandidate)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}

	if answerSDP == "" {
		t.Fatal("Expected non-empty SDP answer")
	}

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	if err := clientPC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("Failed to set remote description on client: %v", err)
	}

	defer mgr.RemoveSession(sessionID)
}

func TestManager_AddICECandidate_Formats(t *testing.T) {
	mgr := NewManager()

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}
	defer clientPC.Close()

	_, _ = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	offer, _ := clientPC.CreateOffer(nil)
	_ = clientPC.SetLocalDescription(offer)

	sessionID := "test-cand-session"
	camID := "test-cam-cand"

	_, err = mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", nil)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	// 1. Test empty/whitespace/null candidate (should succeed without error)
	if err := mgr.AddICECandidate(sessionID, ""); err != nil {
		t.Errorf("Empty candidate returned error: %v", err)
	}
	if err := mgr.AddICECandidate(sessionID, "null"); err != nil {
		t.Errorf("Null candidate returned error: %v", err)
	}
	if err := mgr.AddICECandidate(sessionID, "   "); err != nil {
		t.Errorf("Whitespace candidate returned error: %v", err)
	}

	// 2. Test JSON object format (e.g. from RTCIceCandidate.toJSON())
	mid := "0"
	lineIndex := uint16(0)
	ufrag := "testfrag"
	candInit := webrtc.ICECandidateInit{
		Candidate:        "candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host",
		SDPMid:           &mid,
		SDPMLineIndex:    &lineIndex,
		UsernameFragment: &ufrag,
	}
	candBytes, _ := json.Marshal(candInit)
	if err := mgr.AddICECandidate(sessionID, string(candBytes)); err != nil {
		t.Errorf("JSON candidate returned error: %v", err)
	}

	// 3. Test double-encoded JSON format
	doubleEncoded, _ := json.Marshal(string(candBytes))
	if err := mgr.AddICECandidate(sessionID, string(doubleEncoded)); err != nil {
		t.Errorf("Double-encoded candidate returned error: %v", err)
	}

	// 4. Test non-existent session
	if err := mgr.AddICECandidate("non-existent-session", string(candBytes)); err == nil {
		t.Errorf("Expected error for non-existent session, got nil")
	}
}

func TestManager_ConcurrencyLimitAndCleanup(t *testing.T) {
	mgr := NewManager()
	camID := "busy-camera"

	var sessionIDs []string
	for i := 0; i < 3; i++ {
		clientPC, _ := webrtc.NewPeerConnection(webrtc.Configuration{})
		defer clientPC.Close()
		_, _ = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		})
		offer, _ := clientPC.CreateOffer(nil)
		_ = clientPC.SetLocalDescription(offer)

		sessID := "sess-" + string(rune('A'+i))
		sessionIDs = append(sessionIDs, sessID)

		_, err := mgr.HandleOffer(sessID, camID, offer.SDP, "", "", "", nil)
		if err != nil {
			t.Fatalf("Session %d failed to establish: %v", i, err)
		}
	}

	clientPC4, _ := webrtc.NewPeerConnection(webrtc.Configuration{})
	defer clientPC4.Close()
	_, _ = clientPC4.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	offer4, _ := clientPC4.CreateOffer(nil)
	_ = clientPC4.SetLocalDescription(offer4)

	_, err := mgr.HandleOffer("sess-4th", camID, offer4.SDP, "", "", "", nil)
	if err == nil {
		t.Fatal("Expected 4th concurrent session to fail, but succeeded")
	}
	if !strings.Contains(err.Error(), "maximum of 3 concurrent viewers") {
		t.Errorf("Unexpected error message: %v", err)
	}

	mgr.RemoveSession(sessionIDs[0])

	_, err = mgr.HandleOffer("sess-4th-retry", camID, offer4.SDP, "", "", "", nil)
	if err != nil {
		t.Fatalf("Expected 4th session to succeed after cleanup, got error: %v", err)
	}
	mgr.RemoveSession("sess-4th-retry")
	for _, s := range sessionIDs[1:] {
		mgr.RemoveSession(s)
	}
}

type testCam struct {
	id     string
	name   string
	stream string
}

func (t *testCam) ID() string            { return t.id }
func (t *testCam) Name() string          { return t.name }
func (t *testCam) MainStreamURL() string { return t.stream }

func TestManager_MediaEngineStreamingIntegration(t *testing.T) {
	cam := &testCam{
		id:     "test-camera-1",
		name:   "Test Cam",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	_ = media.DefaultEngine.AttachCamera(cam)
	defer media.DefaultEngine.DetachCamera(cam.ID())

	mgr := NewManager()

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}
	defer clientPC.Close()

	_, _ = clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	_ = clientPC.SetLocalDescription(offer)

	sessionID := "sess-media-integration"
	answerSDP, err := mgr.HandleOffer(sessionID, "test-camera-1", offer.SDP, "", "", "", func(c *webrtc.ICECandidate) {
		if c != nil {
			_ = clientPC.AddICECandidate(c.ToJSON())
		}
	})
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			candJSON, _ := json.Marshal(c.ToJSON())
			_ = mgr.AddICECandidate(sessionID, string(candJSON))
		}
	})

	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	_ = clientPC.SetRemoteDescription(answer)

	// Verify session exists and ABR is initialized
	sess, ok := mgr.GetSession(sessionID)
	if !ok || sess == nil {
		t.Fatal("Expected session to exist")
	}

	abr, ok := mgr.GetABRController(sessionID)
	if !ok || abr == nil {
		t.Fatal("Expected ABR controller to exist")
	}

	// Verify PLI keyframe triggering
	media.DefaultEngine.RequestKeyFrame("test-camera-1")
	abr.HandlePLI()

	stats := abr.GetStats()
	if stats.SessionID != sessionID {
		t.Errorf("Expected session ID %s, got: %s", sessionID, stats.SessionID)
	}
}

// Suppress unused imports
var _ = time.Millisecond
var _ = rtcp.Header{}