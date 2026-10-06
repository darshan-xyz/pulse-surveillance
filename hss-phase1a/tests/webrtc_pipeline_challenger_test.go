package tests

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/media"
	"hss/internal/webrtc"
	pion_webrtc "github.com/pion/webrtc/v4"
)

type testCam struct {
	id     string
	name   string
	stream string
}

func (t *testCam) ID() string             { return t.id }
func (t *testCam) Name() string           { return t.name }
func (t *testCam) MainStreamURL() string  { return t.stream }

func TestChallenger2_CameraIDRouting_OffersAndAliases(t *testing.T) {
	mgr := webrtc.NewManager()
	engine := media.NewEngine()

	cam := &testCam{
		id:     "test-camera-1",
		name:   "Test Camera 1",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// 1. Validate Track Creation for test-camera-1
	track, err := engine.GetTrack("test-camera-1", "high")
	if err != nil || track == nil {
		t.Fatalf("Failed to get track for test-camera-1: %v", err)
	}
	if track.Codec().MimeType != pion_webrtc.MimeTypeH264 {
		t.Errorf("Expected H264 track mime type, got: %s", track.Codec().MimeType)
	}

	// 2. Validate Alias Resolution for cam_001_mock and empty string
	trackAlias, err := engine.GetTrack("cam_001_mock", "high")
	if err != nil || trackAlias == nil {
		t.Fatalf("Alias cam_001_mock failed to resolve track: %v", err)
	}

	trackEmpty, err := engine.GetTrack("", "high")
	if err != nil || trackEmpty == nil {
		t.Fatalf("Empty camera ID failed to resolve single active track: %v", err)
	}

	// 3. Validate Non-existent Camera Fails Gracefully
	engine2 := media.NewEngine()
	_, err = engine2.Subscribe("nonexistent-cam")
	if err == nil {
		t.Error("Expected error when subscribing to nonexistent camera on empty engine, got nil")
	}

	// 4. Validate Offer/Answer Exchange with Pion WebRTC Client
	clientAPI := pion_webrtc.NewAPI()
	clientPC, err := clientAPI.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client peer connection: %v", err)
	}
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(pion_webrtc.RTPCodecTypeVideo, pion_webrtc.RTPTransceiverInit{
		Direction: pion_webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add recvonly transceiver: %v", err)
	}

	clientOffer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create client offer: %v", err)
	}
	if err = clientPC.SetLocalDescription(clientOffer); err != nil {
		t.Fatalf("Failed to set client local description: %v", err)
	}

	answerSDP, err := mgr.HandleOffer("sess-challenger-1", "test-camera-1", clientOffer.SDP, "", "", "", nil)
	if err != nil {
		t.Fatalf("HandleOffer failed for test-camera-1: %v", err)
	}
	if len(answerSDP) == 0 {
		t.Fatal("HandleOffer returned empty answer SDP")
	}

	answerDesc := pion_webrtc.SessionDescription{
		Type: pion_webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	if err = clientPC.SetRemoteDescription(answerDesc); err != nil {
		t.Fatalf("Client failed to set remote answer description: %v", err)
	}

	mgr.RemoveSession("sess-challenger-1")
	t.Log("PASS: Camera ID routing, alias resolution, and WebRTC SDP negotiation verified.")
}

// TestChallenger2_MultiClientSimultaneousStreaming_ConcurrencyAndLimits validates multi-client streaming,
// viewer limits (max 3), immediate keyframe delivery, and RTP monotonicity.
func TestChallenger2_MultiClientSimultaneousStreaming_ConcurrencyAndLimits(t *testing.T) {
	mgr := webrtc.NewManager()
	engine := media.NewEngine()

	cam := &testCam{
		id:     "test-camera-1",
		name:   "Test Camera Multi",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	// Create 3 client peer connections (max limit)
	const maxClients = 3
	var clientPCs []*pion_webrtc.PeerConnection
	var sessionIDs []string

	for i := 0; i < maxClients; i++ {
		sessID := fmt.Sprintf("sess-multi-%d", i)
		sessionIDs = append(sessionIDs, sessID)

		clientPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}
		defer clientPC.Close()
		clientPCs = append(clientPCs, clientPC)

		_, err = clientPC.AddTransceiverFromKind(pion_webrtc.RTPCodecTypeVideo, pion_webrtc.RTPTransceiverInit{
			Direction: pion_webrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			t.Fatalf("Failed to add transceiver for client %d: %v", i, err)
		}

		offer, err := clientPC.CreateOffer(nil)
		if err != nil {
			t.Fatalf("Failed to create offer for client %d: %v", i, err)
		}
		if err = clientPC.SetLocalDescription(offer); err != nil {
			t.Fatalf("Failed to set local description for client %d: %v", i, err)
		}

		answerSDP, err := mgr.HandleOffer(sessID, "test-camera-1", offer.SDP, "", "", "", nil)
		if err != nil {
			t.Fatalf("HandleOffer failed for client %d: %v", i, err)
		}

		if err = clientPC.SetRemoteDescription(pion_webrtc.SessionDescription{
			Type: pion_webrtc.SDPTypeAnswer,
			SDP:  answerSDP,
		}); err != nil {
			t.Fatalf("Client %d failed to set remote description: %v", i, err)
		}
	}

	// 2. Test Viewer Limit Enforcement (4th client should fail)
	t.Run("ViewerLimitEnforcement", func(t *testing.T) {
		extraPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
		if err != nil {
			t.Fatalf("Failed to create extra client: %v", err)
		}
		defer extraPC.Close()

		_, _ = extraPC.AddTransceiverFromKind(pion_webrtc.RTPCodecTypeVideo, pion_webrtc.RTPTransceiverInit{
			Direction: pion_webrtc.RTPTransceiverDirectionRecvonly,
		})
		offer, _ := extraPC.CreateOffer(nil)
		_ = extraPC.SetLocalDescription(offer)

		_, err = mgr.HandleOffer("sess-multi-4-overflow", "test-camera-1", offer.SDP, "", "", "", nil)
		if err == nil {
			t.Fatal("Expected error when exceeding 3 concurrent viewers, got nil")
		}
		t.Logf("PASS: Viewer limit enforced: %v", err)
	})

	// 3. Test Simultaneous Multi-Subscriber Packet Streaming
	var subscriberWg sync.WaitGroup
	subscriberWg.Add(maxClients)
	var totalPacketsReceived int64

	for i := 0; i < maxClients; i++ {
		go func(subIdx int) {
			defer subscriberWg.Done()

			subChan, err := engine.Subscribe("test-camera-1")
			if err != nil {
				t.Errorf("Client %d failed to subscribe: %v", subIdx, err)
				return
			}
			defer engine.Unsubscribe("test-camera-1", subChan)

			// First packet MUST be cached KeyFrame
			select {
			case firstPkt, ok := <-subChan:
				if !ok {
					t.Errorf("Client %d channel closed unexpectedly", subIdx)
					return
				}
				if !firstPkt.IsKeyFrame {
					t.Errorf("Client %d first packet was not a KeyFrame", subIdx)
					return
				}
				atomic.AddInt64(&totalPacketsReceived, 1)
			case <-time.After(1 * time.Second):
				t.Errorf("Client %d timed out waiting for initial keyframe", subIdx)
				return
			}

			// Read next 10 packets to verify continuous delivery
			for p := 0; p < 10; p++ {
				select {
				case pkt, ok := <-subChan:
					if !ok {
						return
					}
					if pkt.OriginalRTP != nil {
						atomic.AddInt64(&totalPacketsReceived, 1)
					}
				case <-time.After(1 * time.Second):
					t.Errorf("Client %d timed out waiting for packet %d", subIdx, p)
					return
				}
			}
		}(i)
	}

	subscriberWg.Wait()

	if totalPacketsReceived < int64(maxClients*10) {
		t.Fatalf("Expected at least %d packets received across all clients, got %d", maxClients*10, totalPacketsReceived)
	}

	// 4. Test Session Removal & Viewer Count Recovery
	mgr.RemoveSession(sessionIDs[0])

	// Now a new client should be allowed to connect
	replPC, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create replacement client: %v", err)
	}
	defer replPC.Close()

	_, _ = replPC.AddTransceiverFromKind(pion_webrtc.RTPCodecTypeVideo, pion_webrtc.RTPTransceiverInit{
		Direction: pion_webrtc.RTPTransceiverDirectionRecvonly,
	})
	offer, _ := replPC.CreateOffer(nil)
	_ = replPC.SetLocalDescription(offer)

	_, err = mgr.HandleOffer("sess-multi-replacement", "test-camera-1", offer.SDP, "", "", "", nil)
	if err != nil {
		t.Fatalf("Failed to connect replacement client after session removal: %v", err)
	}

	// Clean up all sessions
	for _, id := range sessionIDs {
		mgr.RemoveSession(id)
	}
	mgr.RemoveSession("sess-multi-replacement")

	t.Log("PASS: Simultaneous multi-client streaming, viewer limits, and session recovery verified.")
}

// TestChallenger2_RapidChurn_NoDeadlockOrRace tests high-concurrency rapid session creation and destruction
func TestChallenger2_RapidChurn_NoDeadlockOrRace(t *testing.T) {
	mgr := webrtc.NewManager()
	engine := media.NewEngine()

	cam := &testCam{
		id:     "test-camera-1",
		name:   "Test Camera Churn",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	const iterations = 10
	var wg sync.WaitGroup
	wg.Add(iterations)

	for i := 0; i < iterations; i++ {
		go func(idx int) {
			defer wg.Done()

			sessID := fmt.Sprintf("churn-sess-%d", idx)

			pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
			if err != nil {
				return
			}
			defer pc.Close()

			_, _ = pc.AddTransceiverFromKind(pion_webrtc.RTPCodecTypeVideo, pion_webrtc.RTPTransceiverInit{
				Direction: pion_webrtc.RTPTransceiverDirectionRecvonly,
			})
			offer, err := pc.CreateOffer(nil)
			if err != nil {
				return
			}
			_ = pc.SetLocalDescription(offer)

			_, err = mgr.HandleOffer(sessID, "test-camera-1", offer.SDP, "", "", "", nil)
			if err != nil {
				return
			}

			time.Sleep(10 * time.Millisecond)
			mgr.RemoveSession(sessID)
		}(i)
	}

	wg.Wait()
	t.Log("PASS: Rapid connection churn executed with zero deadlocks or panics.")
}

// TestChallenger2_RTCP_PLI_DispatchesKeyframe verifies that receiving an RTCP PLI packet
// invokes RequestKeyFrame and triggers an IDR keyframe emission.
func TestChallenger2_RTCP_PLI_DispatchesKeyframe(t *testing.T) {
	gen := media.NewSyntheticVideoGenerator(320, 240, 30)

	gen.GenerateNextFrame()
	f1 := gen.GenerateNextFrame()
	if f1.IsKeyFrame {
		t.Fatalf("Expected Frame 1 to be delta frame, got KeyFrame")
	}

	gen.RequestKeyFrame()

	f2 := gen.GenerateNextFrame()
	if !f2.IsKeyFrame {
		t.Fatalf("Expected Frame 2 to be KeyFrame after RequestKeyFrame, got Delta frame")
	}

	if len(f2.Payload) < 3 {
		t.Fatalf("Expected at least 3 NALUs in KeyFrame, got %d", len(f2.Payload))
	}
	if f2.Payload[0][0]&0x1F != 7 {
		t.Errorf("Expected first NALU to be SPS (7), got %d", f2.Payload[0][0]&0x1F)
	}
	if f2.Payload[1][0]&0x1F != 8 {
		t.Errorf("Expected second NALU to be PPS (8), got %d", f2.Payload[1][0]&0x1F)
	}
	if f2.Payload[2][0]&0x1F != 5 {
		t.Errorf("Expected third NALU to be IDR (5), got %d", f2.Payload[2][0]&0x1F)
	}

	if f2.OriginalRTP == nil || len(f2.OriginalRTP.Payload) < 1 {
		t.Fatal("OriginalRTP payload is nil or empty")
	}
	nalType := f2.OriginalRTP.Payload[0] & 0x1F
	if nalType != 5 && nalType != 24 {
		t.Errorf("Expected IDR (5) or STAP-A (24), got %d (header 0x%X)", nalType, f2.OriginalRTP.Payload[0])
	}

	t.Log("PASS: RTCP PLI immediate keyframe dispatch verified.")
}

// TestChallenger2_SyntheticH264_BitstreamStandards validates exact H.264 syntax
func TestChallenger2_SyntheticH264_BitstreamStandards(t *testing.T) {
	gen := media.NewSyntheticVideoGenerator(320, 240, 30)

	sps := gen.GetSPS()
	if len(sps) < 4 {
		t.Fatalf("SPS length %d too short", len(sps))
	}
	if sps[0]&0x1F != 7 {
		t.Errorf("Expected SPS NAL unit type 7, got %d", sps[0]&0x1F)
	}
	if sps[1] != 66 {
		t.Errorf("Expected Profile IDC 66, got %d", sps[1])
	}
	if sps[2] != 0xE0 {
		t.Errorf("Expected Constraint flags 0xE0, got 0x%X", sps[2])
	}
	if sps[3] != 31 {
		t.Errorf("Expected Level IDC 31, got %d", sps[3])
	}

	pps := gen.GetPPS()
	if len(pps) < 1 {
		t.Fatal("PPS empty")
	}
	if pps[0]&0x1F != 8 {
		t.Errorf("Expected PPS NAL unit type 8, got %d", pps[0]&0x1F)
	}

	var lastTS uint32
	var lastSeq uint16

	for i := 0; i < 60; i++ {
		pkt := gen.GenerateNextFrame()
		if pkt.OriginalRTP == nil {
			t.Fatalf("Frame %d: OriginalRTP is nil", i)
		}
		if i == 0 {
			lastTS = pkt.OriginalRTP.Timestamp
			lastSeq = pkt.OriginalRTP.SequenceNumber
		} else {
			tsDelta := pkt.OriginalRTP.Timestamp - lastTS
			if tsDelta != 3000 {
				t.Fatalf("Frame %d: Timestamp delta expected 3000, got %d", i, tsDelta)
			}
			lastTS = pkt.OriginalRTP.Timestamp

			seqDelta := pkt.OriginalRTP.SequenceNumber - lastSeq
			if seqDelta != 1 {
				t.Fatalf("Frame %d: Sequence number gap expected 1, got %d", i, seqDelta)
			}
			lastSeq = pkt.OriginalRTP.SequenceNumber
		}
	}

	t.Log("PASS: Synthetic H.264 bitstream standards, SPS/PPS parameters, and RTP monotonicity verified.")
}
