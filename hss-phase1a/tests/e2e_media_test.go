package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// TestE2EMediaPipeline verifies that an actual UDP RTP packet successfully routes from
// a mock edge daemon (RTP Sender) directly to a headless mobile client (RTP Receiver).
func TestE2EMediaPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E Media test in short mode")
	}

	// 1. Setup Headless Edge Daemon (Sender)
	edgePC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create edge PC: %v", err)
	}
	defer edgePC.Close()

	// Create a dummy video track
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "pion",
	)
	if err != nil {
		t.Fatalf("Failed to create video track: %v", err)
	}

	// 2. Setup Headless Mobile Client (Receiver)
	mobilePC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create mobile PC: %v", err)
	}
	defer mobilePC.Close()

	_, err = mobilePC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	edgePC.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		t.Logf("edgePC connection state: %s", s)
	})
	mobilePC.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		t.Logf("mobilePC connection state: %s", s)
	})
	edgePC.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		t.Logf("edgePC ICE state: %s", s)
	})
	mobilePC.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		t.Logf("mobilePC ICE state: %s", s)
	})

	// Channel to signal when we receive the first video packet
	videoReceived := make(chan struct{})

	mobilePC.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		t.Logf("Mobile Client received track: %s (%s)", track.ID(), track.Codec().MimeType)
		
		// Wait for the first RTP packet
		_, _, readErr := track.ReadRTP()
		if readErr == nil {
			close(videoReceived) // Signal success
		}
	})

	// 3. ICE Candidate Routing (Queued until remote description is set)
	var edgeCandidates []webrtc.ICECandidateInit
	var mobileCandidates []webrtc.ICECandidateInit
	var edgeMu, mobileMu sync.Mutex

	edgePC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		mobileMu.Lock()
		defer mobileMu.Unlock()
		if mobilePC.RemoteDescription() != nil {
			_ = mobilePC.AddICECandidate(c.ToJSON())
		} else {
			mobileCandidates = append(mobileCandidates, c.ToJSON())
		}
	})

	mobilePC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		edgeMu.Lock()
		defer edgeMu.Unlock()
		if edgePC.RemoteDescription() != nil {
			_ = edgePC.AddICECandidate(c.ToJSON())
		} else {
			edgeCandidates = append(edgeCandidates, c.ToJSON())
		}
	})

	// 4. SDP Negotiation (Normally happens via pulse-cloud + Valkey)
	offer, err := mobilePC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}

	err = mobilePC.SetLocalDescription(offer)
	if err != nil {
		t.Fatalf("Failed to set mobile local desc: %v", err)
	}

	err = edgePC.SetRemoteDescription(offer)
	if err != nil {
		t.Fatalf("Failed to set edge remote desc: %v", err)
	}

	_, err = edgePC.AddTrack(videoTrack)
	if err != nil {
		t.Fatalf("Failed to add track to edge PC: %v", err)
	}

	edgeMu.Lock()
	for _, cand := range edgeCandidates {
		_ = edgePC.AddICECandidate(cand)
	}
	edgeCandidates = nil
	edgeMu.Unlock()

	answer, err := edgePC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("Failed to create answer: %v", err)
	}

	err = edgePC.SetLocalDescription(answer)
	if err != nil {
		t.Fatalf("Failed to set edge local desc: %v", err)
	}

	err = mobilePC.SetRemoteDescription(answer)
	if err != nil {
		t.Fatalf("Failed to set mobile remote desc: %v", err)
	}

	mobileMu.Lock()
	for _, cand := range mobileCandidates {
		_ = mobilePC.AddICECandidate(cand)
	}
	mobileCandidates = nil
	mobileMu.Unlock()

	// 5. Inject Fake Media Packets
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Send a dummy frame every 33ms (30fps)
		ticker := time.NewTicker(33 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := videoTrack.WriteSample(media.Sample{
					Data:     []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05, 0x01, 0xe0, 0x08, 0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x78, 0x80, 0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, 0x80, 0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe, 0xf6, 0xf0},
					Duration: 33 * time.Millisecond,
				})
				if err != nil {
					t.Logf("WriteSample returned error: %v", err)
				}
			}
		}
	}()

	// 6. Assert Media Delivery
	select {
	case <-videoReceived:
		t.Log("SUCCESS: Headless Mobile Client successfully received the H.264 RTP UDP Packet!")
	case <-time.After(5 * time.Second):
		t.Fatal("FAILURE: Timeout waiting for media packet. UDP Hole Punching failed.")
	}
}
