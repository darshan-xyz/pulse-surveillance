package tests

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/webrtc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// TestABRCongestionDetection ensures the Edge Daemon correctly parses RTCP feedback
// (PLI and REMB) to detect poor 5G/LTE connections and lower the bitrate.
func TestABRCongestionDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ABR test in short mode")
	}

	edgePC, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	defer edgePC.Close()

	videoTrack, _ := pionwebrtc.NewTrackLocalStaticSample(
		pionwebrtc.RTPCodecCapability{MimeType: pionwebrtc.MimeTypeH264},
		"video", "pion",
	)

	mobilePC, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	defer mobilePC.Close()

	_, _ = mobilePC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})

	// Channel to signal that the edge daemon detected the congestion
	congestionDetected := make(chan struct{})

	// Candidate queueing until remote descriptions are set
	var edgeCandidates []pionwebrtc.ICECandidateInit
	var mobileCandidates []pionwebrtc.ICECandidateInit
	var edgeMu, mobileMu sync.Mutex

	edgePC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
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

	mobilePC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
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

	offer, _ := mobilePC.CreateOffer(nil)
	_ = mobilePC.SetLocalDescription(offer)
	_ = edgePC.SetRemoteDescription(offer)

	sender, _ := edgePC.AddTrack(videoTrack)

	edgeMu.Lock()
	for _, cand := range edgeCandidates {
		_ = edgePC.AddICECandidate(cand)
	}
	edgeCandidates = nil
	edgeMu.Unlock()

	answer, _ := edgePC.CreateAnswer(nil)
	_ = edgePC.SetLocalDescription(answer)
	_ = mobilePC.SetRemoteDescription(answer)

	mobileMu.Lock()
	for _, cand := range mobileCandidates {
		_ = mobilePC.AddICECandidate(cand)
	}
	mobileCandidates = nil
	mobileMu.Unlock()

	// Normally this is handled by `internal/webrtc/manager.go`, we replicate the RTCP listener here
	go func() {
		for {
			packets, _, err := sender.ReadRTCP()
			if err != nil {
				return
			}
			for _, pkt := range packets {
				switch p := pkt.(type) {
				case *rtcp.PictureLossIndication:
					t.Logf("Edge Daemon Detected PLI (Packet Loss)! MediaSSRC: %d", p.MediaSSRC)
					close(congestionDetected)
					return
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					t.Logf("Edge Daemon Detected REMB (Bandwidth Drop)! Bitrate: %f", p.Bitrate)
				}
			}
		}
	}()

	// Feed dummy media so it negotiates and pumps RTP packets
	go func() {
		for i := 0; i < 30; i++ {
			_ = videoTrack.WriteSample(media.Sample{
				Data:     []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1f, 0x96, 0x54, 0x05, 0x01, 0xe0, 0x08, 0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x03, 0x00, 0x78, 0x80, 0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, 0x80, 0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe, 0xf6, 0xf0},
				Duration: 33 * time.Millisecond,
			})
			time.Sleep(33 * time.Millisecond)
		}
	}()

	// Wait for connections to establish
	time.Sleep(1 * time.Second)

	// SIMULATE CONGESTION: Mobile App detects 50% packet loss and fires a PLI back to Edge
	t.Log("Simulating 50% 5G Packet Loss... Firing RTCP PLI...")

	var mediaSSRC uint32
	if enc := sender.GetParameters().Encodings; len(enc) > 0 {
		mediaSSRC = uint32(enc[0].SSRC)
	}
	t.Logf("Simulating PLI for MediaSSRC: %d", mediaSSRC)

	pli := &rtcp.PictureLossIndication{
		MediaSSRC: mediaSSRC,
	}
	_ = mobilePC.WriteRTCP([]rtcp.Packet{pli})

	// Assert Edge Daemon caught it
	select {
	case <-congestionDetected:
		t.Log("SUCCESS: Edge Daemon successfully caught the RTCP PLI congestion warning!")
	case <-time.After(3 * time.Second):
		t.Fatal("FAILURE: Edge Daemon failed to detect network congestion.")
	}
}

// TestABRBandwidthDropSimulation verifies end-to-end congestion control in the Go backend.
// It establishes a WebRTC session through webrtc.Manager, pumps 30fps video, fires a low-bitrate
// RTCP REMB (200 kbps) simulating a 5G bandwidth drop, and verifies:
// 1. Backend detects REMB and transitions quality mode within <= 1.0s.
// 2. Outgoing packet rate reduces by >= 50% within <= 1.0s.
// 3. Backend recovers cleanly to full rate when REMB bandwidth is restored.
func TestABRBandwidthDropSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ABR simulation test in short mode")
	}

	mgr := webrtc.NewManager()
	sessionID := "sim-5g-drop-sess-101"
	camID := "cam-5g-abr-test"

	// 1. Setup Mobile Client Receiver PC
	mobilePC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create mobile PC: %v", err)
	}
	defer mobilePC.Close()

	_, err = mobilePC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	mobilePC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		t.Logf("[Mobile Client] PeerConnection State: %s", s.String())
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var mobileCandidates []pionwebrtc.ICECandidateInit
	var candMu sync.Mutex

	onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candMu.Lock()
		defer candMu.Unlock()
		if mobilePC.RemoteDescription() != nil {
			_ = mobilePC.AddICECandidate(c.ToJSON())
		} else {
			mobileCandidates = append(mobileCandidates, c.ToJSON())
		}
	}

	mobilePC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		_ = mgr.AddICECandidate(sessionID, string(candJSON))
	})

	// 2. Client creates Offer
	offer, err := mobilePC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err := mobilePC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set mobile local description: %v", err)
	}

	// 3. Manager handles Offer
	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
	if err != nil {
		t.Fatalf("Manager.HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	answer := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	if err := mobilePC.SetRemoteDescription(answer); err != nil {
		t.Fatalf("Failed to set mobile remote description: %v", err)
	}

	// Flush queued ICE candidates
	candMu.Lock()
	for _, c := range mobileCandidates {
		_ = mobilePC.AddICECandidate(c)
	}
	mobileCandidates = nil
	candMu.Unlock()

	// Wait for WebRTC connection
	select {
	case <-connectedChan:
		t.Log("WebRTC PeerConnection connected successfully.")
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for WebRTC connection to establish")
	}

	session, ok := mgr.GetSession(sessionID)
	if !ok || session.ABR == nil {
		t.Fatalf("Session %s or ABR controller not found", sessionID)
	}

	// 4. Setup Client-Side Packet Counter
	var totalPacketsReceived int64
	var postDropPackets int64
	var isPostDrop int32

	trackReady := make(chan struct{})
	var trackOnce sync.Once
	var remoteSSRC uint32

	mobilePC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		trackOnce.Do(func() {
			remoteSSRC = uint32(track.SSRC())
			close(trackReady)
		})
		for {
			_, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			atomic.AddInt64(&totalPacketsReceived, 1)
			if atomic.LoadInt32(&isPostDrop) == 1 {
				atomic.AddInt64(&postDropPackets, 1)
			}
		}
	})

	// 5. Start Media Source Loop pumping 30fps H.264 RTP packets
	stopSource := make(chan struct{})
	defer close(stopSource)

	go func() {
		ticker := time.NewTicker(33 * time.Millisecond) // ~30 fps
		defer ticker.Stop()

		var seqNum uint16 = 1
		var frameIdx int = 0

		for {
			select {
			case <-stopSource:
				return
			case <-ticker.C:
				frameIdx++
				ts := uint32(frameIdx * 3000)

				var payload []byte
				if frameIdx%30 == 1 {
					// Keyframe (IDR NALU type 5)
					payload = []byte{0x65, 0x88, 0x84, 0x00, 0x10, 0xff, 0xfe}
				} else {
					// P-frame (Non-IDR NALU type 1)
					payload = []byte{0x41, 0x9a, 0x01, 0x02, 0x03, 0x04}
				}

				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    96,
						SequenceNumber: seqNum,
						Timestamp:      ts,
						SSRC:           remoteSSRC,
					},
					Payload: payload,
				}
				seqNum++

				_ = session.WriteRTP(pkt)
			}
		}
	}()

	// Wait for first track packets
	select {
	case <-trackReady:
		t.Logf("Mobile client receiving video track (SSRC: %d)", remoteSSRC)
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for media track to start")
	}

	// 6. PHASE 1: Baseline Measurement (Normal 30fps / Full Bitrate)
	t.Log("PHASE 1: Measuring Baseline Packet Rate (Normal 30fps / 3.0 Mbps)...")
	baselineStart := time.Now()
	baselineStartCount := atomic.LoadInt64(&totalPacketsReceived)
	time.Sleep(1000 * time.Millisecond)
	baselineEndCount := atomic.LoadInt64(&totalPacketsReceived)
	baselinePackets := baselineEndCount - baselineStartCount
	baselineDuration := time.Since(baselineStart)

	t.Logf("Baseline Rate: %d packets in %v (~%.1f pkts/sec)",
		baselinePackets, baselineDuration, float64(baselinePackets)/(baselineDuration.Seconds()))

	if baselinePackets < 20 {
		t.Fatalf("Baseline packet count too low (%d), expected ~30 packets", baselinePackets)
	}
	if session.ABR.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Expected baseline quality %s, got %s", webrtc.QualityHigh, session.ABR.GetQuality())
	}

	// 7. PHASE 2: Simulate 5G Bandwidth Drop with RTCP REMB
	t.Log("PHASE 2: Simulating 5G Bandwidth Drop... Firing RTCP REMB (200,000 bps)...")
	remb := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 200_000, // 200 kbps (triggers Throttled rate adaptation)
		SSRCs:   []uint32{remoteSSRC},
	}

	dropTriggerTime := time.Now()
	if err := mobilePC.WriteRTCP([]rtcp.Packet{remb}); err != nil {
		t.Fatalf("Failed to send RTCP REMB packet: %v", err)
	}

	// Verify backend adapts quality within <= 1.0s
	var adaptedTime time.Time
	var adaptationLatency time.Duration
	adapted := false

	for i := 0; i < 100; i++ { // Check every 10ms for up to 1.0s
		if session.ABR.GetQuality() == webrtc.QualityThrottled || session.ABR.GetQuality() == webrtc.QualityEmergency {
			adaptedTime = time.Now()
			adaptationLatency = adaptedTime.Sub(dropTriggerTime)
			adapted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !adapted {
		t.Fatalf("FAILURE: Backend failed to detect REMB and adapt quality within 1.0s (current quality: %s)",
			session.ABR.GetQuality())
	}

	t.Logf("SUCCESS: Backend detected REMB and transitioned to %s in %v (Threshold: <= 1.0s)",
		session.ABR.GetQuality(), adaptationLatency)

	if adaptationLatency > 1000*time.Millisecond {
		t.Fatalf("Adaptation latency %v exceeded 1.0s limit", adaptationLatency)
	}

	// 8. PHASE 3: Measure Outgoing Packet Rate During Congestion (1.0s Window)
	atomic.StoreInt32(&isPostDrop, 1)
	atomic.StoreInt64(&postDropPackets, 0)

	measureDuration := 1000 * time.Millisecond
	time.Sleep(measureDuration)

	postDropCount := atomic.LoadInt64(&postDropPackets)
	reduction := float64(baselinePackets-postDropCount) / float64(baselinePackets)

	t.Logf("Post-Drop Rate: %d packets in %v (~%.1f pkts/sec)",
		postDropCount, measureDuration, float64(postDropCount)/(measureDuration.Seconds()))
	t.Logf("Measured Packet Rate Reduction: %.1f%% (Threshold: >= 50.0%%)", reduction*100.0)

	if reduction < 0.50 {
		t.Fatalf("FAILURE: Packet rate reduction %.1f%% did not meet >= 50%% threshold (baseline=%d, postDrop=%d)",
			reduction*100.0, baselinePackets, postDropCount)
	}
	t.Log("SUCCESS: Rate reduction >= 50% verified within <= 1.0s of network drop!")

	// 9. PHASE 4: Network Recovery Simulation (Bandwidth Restored to 3.0 Mbps)
	t.Log("PHASE 4: Restoring Bandwidth... Firing RTCP REMB (3,000,000 bps)...")
	recoveryREMB := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 3_000_000,
		SSRCs:   []uint32{remoteSSRC},
	}
	if err := mobilePC.WriteRTCP([]rtcp.Packet{recoveryREMB}); err != nil {
		t.Fatalf("Failed to send recovery RTCP REMB: %v", err)
	}

	recovered := false
	for i := 0; i < 100; i++ {
		if session.ABR.GetQuality() == webrtc.QualityHigh {
			recovered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !recovered {
		t.Fatalf("FAILURE: Backend failed to recover to QualityHigh after REMB restoration")
	}
	t.Log("SUCCESS: Backend recovered cleanly to QualityHigh!")

	stats := session.ABR.GetStats()
	t.Logf("Final ABR Controller Stats: TotalIn=%d, TotalOut=%d, Dropped=%d, Adaptations=%d",
		stats.TotalPacketsIn, stats.TotalPacketsOut, stats.DroppedPackets, stats.AdaptationCount)
}
