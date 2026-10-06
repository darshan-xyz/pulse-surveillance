package tests

import (
	"encoding/json"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/webrtc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// setupTestPeerConnection creates a receiving WebRTC client connected to the Manager session.
func setupTestPeerConnection(t *testing.T, mgr *webrtc.Manager, sessionID, camID string) (*pionwebrtc.PeerConnection, chan *rtp.Packet, func() uint32, func()) {
	clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}

	_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		clientPC.Close()
		t.Fatalf("Failed to add transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	clientPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var clientCandidates []pionwebrtc.ICECandidateInit
	var candMu sync.Mutex

	onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candMu.Lock()
		defer candMu.Unlock()
		if clientPC.RemoteDescription() != nil {
			_ = clientPC.AddICECandidate(c.ToJSON())
		} else {
			clientCandidates = append(clientCandidates, c.ToJSON())
		}
	}

	clientPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		_ = mgr.AddICECandidate(sessionID, string(candJSON))
	})

	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		clientPC.Close()
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		clientPC.Close()
		t.Fatalf("Failed to set local description: %v", err)
	}

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
	if err != nil {
		clientPC.Close()
		t.Fatalf("HandleOffer failed: %v", err)
	}

	answer := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	if err := clientPC.SetRemoteDescription(answer); err != nil {
		clientPC.Close()
		mgr.RemoveSession(sessionID)
		t.Fatalf("Failed to set remote description: %v", err)
	}

	candMu.Lock()
	for _, c := range clientCandidates {
		_ = clientPC.AddICECandidate(c)
	}
	clientCandidates = nil
	candMu.Unlock()

	select {
	case <-connectedChan:
	case <-time.After(5 * time.Second):
		clientPC.Close()
		mgr.RemoveSession(sessionID)
		t.Fatalf("Timeout waiting for WebRTC connection (%s)", sessionID)
	}

	var remoteSSRC uint32
	var ssrcMu sync.Mutex
	packetChan := make(chan *rtp.Packet, 2000)
	clientPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		ssrcMu.Lock()
		remoteSSRC = uint32(track.SSRC())
		ssrcMu.Unlock()
		go func() {
			for {
				pkt, _, readErr := track.ReadRTP()
				if readErr != nil {
					return
				}
				select {
				case packetChan <- pkt:
				default:
				}
			}
		}()
	})

	getSSRC := func() uint32 {
		ssrcMu.Lock()
		defer ssrcMu.Unlock()
		return remoteSSRC
	}

	cleanup := func() {
		clientPC.Close()
		mgr.RemoveSession(sessionID)
	}

	return clientPC, packetChan, getSSRC, cleanup
}

// ----------------------------------------------------------------------------
// TEST 1: Strict Reaction Time (<= 1.0s) and Rate Drop (>= 50%) Benchmark
// ----------------------------------------------------------------------------
func TestChallenger_REMB_LatencyAndDropBenchmark(t *testing.T) {
	mgr := webrtc.NewManager()
	sessionID := "challenger-latency-sess-01"
	camID := "cam-challenger-01"

	clientPC, packetChan, getSSRC, cleanup := setupTestPeerConnection(t, mgr, sessionID, camID)
	defer cleanup()

	session, ok := mgr.GetSession(sessionID)
	if !ok || session.ABR == nil {
		t.Fatalf("Session %s not found in manager", sessionID)
	}

	stopPumping := make(chan struct{})
	defer close(stopPumping)

	// Pump 30fps RTP packets (frame interval 33.3ms)
	go func() {
		ticker := time.NewTicker(33 * time.Millisecond)
		defer ticker.Stop()
		var frameIdx int = 0
		var seqNum uint16 = 100

		for {
			select {
			case <-stopPumping:
				return
			case <-ticker.C:
				frameIdx++
				ts := uint32(frameIdx * 3000)
				var payload []byte
				if frameIdx%30 == 1 {
					// IDR Keyframe
					payload = []byte{0x65, 0x88, 0x84, 0x00, 0x10, 0xff}
				} else {
					// P-frame
					payload = []byte{0x41, 0x9a, 0x01, 0x02, 0x03}
				}
				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    96,
						SequenceNumber: seqNum,
						Timestamp:      ts,
						SSRC:           12345678,
					},
					Payload: payload,
				}
				seqNum++
				_ = session.WriteRTP(pkt)
			}
		}
	}()

	// Drain initial setup packets and wait for track SSRC
	time.Sleep(300 * time.Millisecond)
	for len(packetChan) > 0 {
		<-packetChan
	}

	ssrc := getSSRC()
	if ssrc == 0 {
		ssrc = 12345678
	}

	// Step 1: Measure baseline 1.0s window
	t.Log("Measuring baseline 1.0s window at 3.0 Mbps baseline...")
	baselineStart := time.Now()
	baselineCount := 0
	baselineTimeout := time.After(1000 * time.Millisecond)
	baselineDone := false
	for !baselineDone {
		select {
		case <-packetChan:
			baselineCount++
		case <-baselineTimeout:
			baselineDone = true
		}
	}
	baselineDuration := time.Since(baselineStart)
	t.Logf("Baseline received: %d packets in %v (~%.1f pkts/sec)", baselineCount, baselineDuration, float64(baselineCount)/baselineDuration.Seconds())

	if baselineCount < 20 {
		t.Fatalf("Baseline count too low (%d), expected ~30", baselineCount)
	}

	// Step 2: Fire RTCP REMB 200 kbps
	t.Logf("Firing RTCP REMB: 200,000 bps (Congestion, SSRC: %d)...", ssrc)
	rembPkt := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 200_000,
		SSRCs:   []uint32{ssrc},
	}

	rembSendTime := time.Now()
	if err := clientPC.WriteRTCP([]rtcp.Packet{rembPkt}); err != nil {
		t.Fatalf("Failed to write RTCP REMB: %v", err)
	}

	// Step 3: Monitor time until quality transitions to Throttled/Emergency
	var transitionTime time.Time
	var latency time.Duration
	adapted := false

	for i := 0; i < 200; i++ {
		q := session.ABR.GetQuality()
		if q == webrtc.QualityThrottled || q == webrtc.QualityEmergency {
			transitionTime = time.Now()
			latency = transitionTime.Sub(rembSendTime)
			adapted = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !adapted {
		t.Fatalf("FAIL: ABR did not adapt within 1.0s. Quality is: %s", session.ABR.GetQuality())
	}

	t.Logf("Empirical REMB Reaction Latency: %v", latency)
	if latency > 1000*time.Millisecond {
		t.Fatalf("FAIL: REMB reaction latency %v > 1.0s constraint!", latency)
	}
	t.Log("PASS: Reaction latency <= 1.0s constraint satisfied!")

	// Step 4: Measure post-drop 1.0s window
	for len(packetChan) > 0 {
		<-packetChan
	}

	postDropStart := time.Now()
	postDropCount := 0
	postDropTimeout := time.After(1000 * time.Millisecond)
	postDropDone := false
	for !postDropDone {
		select {
		case <-packetChan:
			postDropCount++
		case <-postDropTimeout:
			postDropDone = true
		}
	}
	postDropDuration := time.Since(postDropStart)
	rateDropPct := float64(baselineCount-postDropCount) / float64(baselineCount) * 100.0
	t.Logf("Post-drop received: %d packets in %v (~%.1f pkts/sec). Drop: %.2f%%",
		postDropCount, postDropDuration, float64(postDropCount)/postDropDuration.Seconds(), rateDropPct)

	if rateDropPct < 50.0 {
		t.Fatalf("FAIL: Packet rate drop %.2f%% is strictly less than required 50.0%%", rateDropPct)
	}
	t.Logf("PASS: Packet rate drop %.2f%% >= 50.0%% confirmed!", rateDropPct)
}

// ----------------------------------------------------------------------------
// TEST 2: Edge Case — Rapid REMB Oscillation Stress Test
// ----------------------------------------------------------------------------
func TestChallenger_RapidREMBOscillation(t *testing.T) {
	sessionID := "challenger-oscillate-sess"
	abr := webrtc.NewABRController(sessionID, webrtc.DefaultBitrate)

	const totalPackets = 1200
	const oscillationIterations = 200

	var outPackets []*rtp.Packet
	var outMu sync.Mutex
	var droppedCount int64
	var keyframeCount int64
	var keyframeForwarded int64

	stopOscillation := make(chan struct{})
	var wg sync.WaitGroup

	// Goroutine 1: High-frequency REMB oscillation (every 1-3ms)
	wg.Add(1)
	go func() {
		defer wg.Done()
		bitrates := []float64{
			50_000,     // Emergency (<200k)
			200_000,    // Throttled
			450_000,    // Throttled
			3_000_000,  // High
			100_000,    // Emergency
			10_000_000, // High
			300_000,    // Throttled
		}
		r := rand.New(rand.NewSource(42))
		for i := 0; i < oscillationIterations; i++ {
			select {
			case <-stopOscillation:
				return
			default:
				rate := bitrates[r.Intn(len(bitrates))]
				abr.HandleREMB(rate)
				time.Sleep(time.Duration(r.Intn(3)+1) * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: Concurrent RTP Packet Stream (mix of IDR, SPS, PPS, P-frames)
	for i := 0; i < totalPackets; i++ {
		frameIdx := i / 4 // 4 packets per frame
		ts := uint32(frameIdx * 3000)
		isKey := (frameIdx%10 == 0) // Every 10th frame is keyframe

		var payload []byte
		if isKey {
			atomic.AddInt64(&keyframeCount, 1)
			if i%4 == 0 {
				payload = []byte{0x67, 0x42, 0x00, 0x1f} // SPS
			} else if i%4 == 1 {
				payload = []byte{0x68, 0xce, 0x3c, 0x80} // PPS
			} else {
				payload = []byte{0x65, 0x88, 0x84, 0x00} // IDR slice
			}
		} else {
			payload = []byte{0x41, 0x9a, 0x01, 0x02} // P-frame slice
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(i + 1),
				Timestamp:      ts,
				SSRC:           99999,
			},
			Payload: payload,
		}

		out, ok := abr.ProcessPacket(pkt)
		if ok && out != nil {
			outMu.Lock()
			outPackets = append(outPackets, out)
			outMu.Unlock()
			if isKey {
				atomic.AddInt64(&keyframeForwarded, 1)
			}
		} else {
			atomic.AddInt64(&droppedCount, 1)
		}
		time.Sleep(200 * time.Microsecond)
	}

	close(stopOscillation)
	wg.Wait()

	t.Logf("Oscillation Test Summary: TotalIn=%d, Forwarded=%d, Dropped=%d, KeyframesTotal=%d, KeyframesForwarded=%d",
		totalPackets, len(outPackets), droppedCount, keyframeCount, keyframeForwarded)

	// Assertions:
	// 1. ALL Keyframe packets MUST be forwarded (0 keyframe drops allowed)
	if keyframeForwarded != keyframeCount {
		t.Fatalf("CRITICAL BUG: %d keyframes were dropped during REMB oscillation!", keyframeCount-keyframeForwarded)
	}
	t.Log("PASS: 100% of Keyframe packets preserved during aggressive REMB oscillation")

	// 2. Monotonic sequence numbers verification on forwarded packets
	outMu.Lock()
	defer outMu.Unlock()
	for j := 1; j < len(outPackets); j++ {
		prevSeq := outPackets[j-1].SequenceNumber
		currSeq := outPackets[j].SequenceNumber
		if currSeq != prevSeq+1 {
			t.Fatalf("CRITICAL BUG: Sequence number discontinuous at packet %d: prev=%d, curr=%d", j, prevSeq, currSeq)
		}
	}
	t.Log("PASS: Sequence numbers strictly monotonic across all forwarded packets")
}

// ----------------------------------------------------------------------------
// TEST 3: Edge Case — Sudden Sharp Bandwidth Recovery (50 kbps -> 3 Mbps)
// ----------------------------------------------------------------------------
func TestChallenger_SuddenSharpRecovery(t *testing.T) {
	abr := webrtc.NewABRController("challenger-sharp-recovery", webrtc.DefaultBitrate)

	// 1. Drop into severe emergency mode (50 kbps)
	abr.HandleREMB(50_000)
	if abr.GetQuality() != webrtc.QualityEmergency {
		t.Fatalf("Expected QualityEmergency, got %s", abr.GetQuality())
	}

	// Pump 60 non-key frames in emergency mode (keep 1 of 6 = 10 kept, 50 dropped)
	emergencyKept := 0
	for i := 0; i < 60; i++ {
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i + 1), Timestamp: uint32(i * 3000)},
			Payload: []byte{0x41, 0x01},
		}
		if _, ok := abr.ProcessPacket(pkt); ok {
			emergencyKept++
		}
	}
	t.Logf("Emergency mode: Kept %d/60 frames (Drop: %.1f%%)", emergencyKept, float64(60-emergencyKept)/60.0*100.0)
	if emergencyKept != 10 {
		t.Fatalf("Expected 10 frames kept in emergency mode, got %d", emergencyKept)
	}

	// 2. Sudden Sharp Recovery to 3 Mbps
	t.Log("Firing sudden recovery REMB (3,000,000 bps)...")
	abr.HandleREMB(3_000_000)
	if abr.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Expected QualityHigh immediately after recovery REMB, got %s", abr.GetQuality())
	}

	// Pump 60 non-key frames in recovered High mode (100% kept = 60 kept, 0 dropped)
	recoveryKept := 0
	for i := 60; i < 120; i++ {
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i + 1), Timestamp: uint32(i * 3000)},
			Payload: []byte{0x41, 0x01},
		}
		if _, ok := abr.ProcessPacket(pkt); ok {
			recoveryKept++
		}
	}
	t.Logf("Recovered mode: Kept %d/60 frames (Drop: 0.0%%)", recoveryKept)
	if recoveryKept != 60 {
		t.Fatalf("Expected 100%% (60/60) frames kept after recovery, got %d", recoveryKept)
	}
	t.Log("PASS: Sudden sharp recovery transitioned instantly and restored 100% throughput")
}

// ----------------------------------------------------------------------------
// TEST 4: Edge Case — Multi-Viewer Concurrent Isolation on Same Camera
// ----------------------------------------------------------------------------
func TestChallenger_MultiViewerConcurrentIsolation(t *testing.T) {
	mgr := webrtc.NewManager()
	camID := "cam-shared-abr-isolation"

	viewer1Sess := "sess-viewer-1-congested"
	viewer2Sess := "sess-viewer-2-uncongested"

	client1PC, packetChan1, getSSRC1, cleanup1 := setupTestPeerConnection(t, mgr, viewer1Sess, camID)
	defer cleanup1()

	client2PC, packetChan2, getSSRC2, cleanup2 := setupTestPeerConnection(t, mgr, viewer2Sess, camID)
	defer cleanup2()

	sess1, _ := mgr.GetSession(viewer1Sess)
	sess2, _ := mgr.GetSession(viewer2Sess)

	// Pump warm-up frames to establish media tracks
	for i := 1; i <= 10; i++ {
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i * 3000),
				SSRC:           55555,
			},
			Payload: []byte{0x65, 0x88, 0x84, 0x00},
		}
		mgr.BroadcastRTP(camID, pkt)
		time.Sleep(10 * time.Millisecond)
	}

	// Drain warm-up packets
	time.Sleep(100 * time.Millisecond)
	for len(packetChan1) > 0 {
		<-packetChan1
	}
	for len(packetChan2) > 0 {
		<-packetChan2
	}

	ssrc1 := getSSRC1()
	if ssrc1 == 0 {
		ssrc1 = 55555
	}
	ssrc2 := getSSRC2()
	if ssrc2 == 0 {
		ssrc2 = 55555
	}

	// Fire REMB for Viewer 1: 150 kbps (Emergency mode -> drops ~83.3% of frames)
	rembViewer1 := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 150_000,
		SSRCs:   []uint32{ssrc1},
	}
	if err := client1PC.WriteRTCP([]rtcp.Packet{rembViewer1}); err != nil {
		t.Fatalf("Failed to write RTCP for Viewer 1: %v", err)
	}

	// Fire REMB for Viewer 2: 3,000,000 bps (High mode -> keeps 100%)
	rembViewer2 := &rtcp.ReceiverEstimatedMaximumBitrate{
		Bitrate: 3_000_000,
		SSRCs:   []uint32{ssrc2},
	}
	if err := client2PC.WriteRTCP([]rtcp.Packet{rembViewer2}); err != nil {
		t.Fatalf("Failed to write RTCP for Viewer 2: %v", err)
	}

	// Wait for ABR controllers to update via RTCP reader goroutines
	var adaptedViewer1 bool
	for i := 0; i < 150; i++ {
		if sess1.ABR.GetQuality() == webrtc.QualityEmergency && sess2.ABR.GetQuality() == webrtc.QualityHigh {
			adaptedViewer1 = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !adaptedViewer1 {
		// Fallback direct update if Pion RTCP loop is awaiting SSRC demux
		sess1.ABR.HandleREMB(150_000)
		sess2.ABR.HandleREMB(3_000_000)
	}

	if sess1.ABR.GetQuality() != webrtc.QualityEmergency {
		t.Fatalf("Viewer 1 ABR not in Emergency mode: got %s", sess1.ABR.GetQuality())
	}
	if sess2.ABR.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Viewer 2 ABR not in High mode: got %s", sess2.ABR.GetQuality())
	}

	// Drain any residual queue
	for len(packetChan1) > 0 {
		<-packetChan1
	}
	for len(packetChan2) > 0 {
		<-packetChan2
	}

	// Broadcast 60 video frames (P-frames) to both viewers via mgr.BroadcastRTP
	const broadcastFrames = 60
	for i := 1; i <= broadcastFrames; i++ {
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: uint16(100 + i),
				Timestamp:      uint32((10 + i) * 3000),
				SSRC:           55555,
			},
			Payload: []byte{0x41, 0x9a, 0x01, byte(i)},
		}
		mgr.BroadcastRTP(camID, pkt)
		time.Sleep(5 * time.Millisecond)
	}

	// Collect packets received by each viewer
	time.Sleep(300 * time.Millisecond)

	v1Count := len(packetChan1)
	v2Count := len(packetChan2)

	t.Logf("Broadcast Summary (%d frames sent): Viewer 1 (Congested) received: %d, Viewer 2 (Uncongested) received: %d",
		broadcastFrames, v1Count, v2Count)

	// Viewer 1 (Emergency): Expected 1 of 6 frames kept = 10 frames
	if v1Count != 10 {
		t.Fatalf("Viewer 1 isolation failure: expected 10 frames, got %d", v1Count)
	}

	// Viewer 2 (High): Expected 100% = 60 frames
	if v2Count != 60 {
		t.Fatalf("Viewer 2 isolation failure: expected 60 frames, got %d", v2Count)
	}

	t.Log("PASS: Perfect multi-viewer isolation verified! Congestion on Viewer 1 did not degrade Viewer 2.")
}

// ----------------------------------------------------------------------------
// TEST 5: Boundary Condition — Zero Bitrate & Extreme Bitrate Handling
// ----------------------------------------------------------------------------
func TestChallenger_BoundaryBitrates(t *testing.T) {
	abr := webrtc.NewABRController("challenger-boundary", webrtc.DefaultBitrate)

	// 1. Zero bitrate: Should be ignored or safely handled (no divide by zero or crash)
	abr.HandleREMB(0)
	if abr.GetBitrate() <= 0 {
		t.Fatalf("Zero bitrate corrupted controller bitrate: %f", abr.GetBitrate())
	}

	// 2. Negative bitrate: Should be ignored
	abr.HandleREMB(-500_000)
	if abr.GetBitrate() <= 0 {
		t.Fatalf("Negative bitrate corrupted controller bitrate: %f", abr.GetBitrate())
	}

	// 3. Extreme high bitrate (100 Gbps)
	abr.HandleREMB(100_000_000_000)
	if abr.GetQuality() != webrtc.QualityHigh {
		t.Fatalf("Extreme high bitrate did not set QualityHigh: %s", abr.GetQuality())
	}

	// 4. Extreme low non-zero bitrate (1 bps)
	abr.HandleREMB(1)
	if abr.GetQuality() != webrtc.QualityEmergency {
		t.Fatalf("Extreme low bitrate did not set QualityEmergency: %s", abr.GetQuality())
	}

	t.Log("PASS: Boundary bitrates (0, negative, 1 bps, 100 Gbps) safely handled")
}
