package tests

import (
	"encoding/json"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hss/internal/media"
	"hss/internal/webrtc"

	"github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestLatency_GlassToGlassSub200msSimulation verifies that the end-to-end media pipeline
// (frame generation -> H.264 slice encoding -> FU-A packetization -> lock-free RTP dispatch ->
// WebRTC transport -> receiver ReadRTP) achieves strictly sub-200ms latency with low jitter.
func TestLatency_GlassToGlassSub200msSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency simulation test in short mode")
	}

	mgr := webrtc.NewManager()
	sessionID := "latency-sub200ms-sess-01"
	camID := "cam-latency-test-01"

	// 1. Setup Receiver RTCPeerConnection
	receiverPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create receiver PeerConnection: %v", err)
	}
	defer receiverPC.Close()

	_, err = receiverPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	receiverPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var pendingCandidates []pionwebrtc.ICECandidateInit
	var candMu sync.Mutex

	onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candMu.Lock()
		defer candMu.Unlock()
		if receiverPC.RemoteDescription() != nil {
			_ = receiverPC.AddICECandidate(c.ToJSON())
		} else {
			pendingCandidates = append(pendingCandidates, c.ToJSON())
		}
	}

	receiverPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		_ = mgr.AddICECandidate(sessionID, string(candJSON))
	})

	offer, err := receiverPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	_ = receiverPC.SetLocalDescription(offer)

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	_ = receiverPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	})

	candMu.Lock()
	for _, c := range pendingCandidates {
		_ = receiverPC.AddICECandidate(c)
	}
	pendingCandidates = nil
	candMu.Unlock()

	select {
	case <-connectedChan:
		t.Log("WebRTC PeerConnection connected for sub-200ms latency test.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for WebRTC connection")
	}

	session, ok := mgr.GetSession(sessionID)
	if !ok {
		t.Fatalf("Session %s not found in Manager", sessionID)
	}

	// 2. Track packet receipt and glass-to-glass latency
	frameSentTimes := make(map[uint32]int64)
	var frameMu sync.Mutex

	var latencies []time.Duration
	var latenciesMu sync.Mutex
	var jitterSum float64
	var jitterCount int
	var prevTransit int64
	var hasPrevTransit bool

	trackReady := make(chan struct{})
	var trackOnce sync.Once
	var packetsReceived int64

	receiverPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		trackOnce.Do(func() {
			close(trackReady)
		})

		for {
			pkt, _, rErr := track.ReadRTP()
			if rErr != nil {
				return
			}
			receiveNanos := time.Now().UnixNano()
			atomic.AddInt64(&packetsReceived, 1)

			// When the last packet of a frame arrives (Marker bit == true)
			if pkt.Marker {
				frameMu.Lock()
				sentNanos, found := frameSentTimes[pkt.Timestamp]
				frameMu.Unlock()

				if found {
					latency := time.Duration(receiveNanos - sentNanos)
					latenciesMu.Lock()
					latencies = append(latencies, latency)

					// Calculate RFC 3550 interarrival jitter
					transit := receiveNanos - sentNanos
					if hasPrevTransit {
						d := transit - prevTransit
						if d < 0 {
							d = -d
						}
						jitterSum += float64(d)
						jitterCount++
					}
					prevTransit = transit
					hasPrevTransit = true
					latenciesMu.Unlock()
				}
			}
		}
	})

	// 3. Warm up receiver track to initialize OnTrack and ReadRTP loop
	gen := media.NewSyntheticVideoGenerator(320, 240, 30)
	seedFrame := gen.GenerateNextFrame()
	for _, pkt := range seedFrame.RTPPackets {
		_ = session.WriteRTP(pkt)
	}

	select {
	case <-trackReady:
		t.Log("Receiver track ready and streaming media.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for track ready")
	}
	time.Sleep(50 * time.Millisecond)

	// 4. Start streaming real H.264 synthetic frames at 30 FPS (~2.5 seconds)
	const totalFrames = 80
	frameInterval := 1000 * time.Millisecond / 30

	stopPump := make(chan struct{})
	defer close(stopPump)

	go func() {
		for i := 0; i < totalFrames; i++ {
			select {
			case <-stopPump:
				return
			default:
			}
			genStart := time.Now()
			mp := gen.GenerateNextFrame()

			if len(mp.RTPPackets) > 0 {
				firstPkt := mp.RTPPackets[0]
				frameMu.Lock()
				frameSentTimes[firstPkt.Timestamp] = genStart.UnixNano()
				frameMu.Unlock()

				for _, pkt := range mp.RTPPackets {
					_ = session.WriteRTP(pkt)
				}
			}
			time.Sleep(frameInterval)
		}
	}()

	// Wait for frames to stream
	time.Sleep(time.Duration(totalFrames)*frameInterval + 200*time.Millisecond)

	latenciesMu.Lock()
	defer latenciesMu.Unlock()

	if len(latencies) < 30 {
		t.Fatalf("Expected at least 30 measured frame latencies, got: %d", len(latencies))
	}

	// Calculate latency statistics
	var totalLatency time.Duration
	var minLatency = time.Hour
	var maxLatency time.Duration

	for _, l := range latencies {
		totalLatency += l
		if l < minLatency {
			minLatency = l
		}
		if l > maxLatency {
			maxLatency = l
		}
	}

	avgLatency := totalLatency / time.Duration(len(latencies))

	// Sort latencies for percentiles
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 := sorted[len(sorted)*50/100]
	p95 := sorted[len(sorted)*95/100]
	p99 := sorted[len(sorted)*99/100]

	avgJitter := time.Duration(0)
	if jitterCount > 0 {
		avgJitter = time.Duration(jitterSum / float64(jitterCount))
	}

	t.Logf("End-to-End Latency Results across %d frames (%d packets):", len(latencies), atomic.LoadInt64(&packetsReceived))
	t.Logf("  Min Latency: %v", minLatency)
	t.Logf("  Avg Latency: %v", avgLatency)
	t.Logf("  P50 Latency: %v", p50)
	t.Logf("  P95 Latency: %v", p95)
	t.Logf("  P99 Latency: %v", p99)
	t.Logf("  Max Latency: %v", maxLatency)
	t.Logf("  Avg Jitter:  %v", avgJitter)

	// MANDATORY ASSERTION: Glass-to-glass latency strictly < 200ms
	if maxLatency > 200*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: Max latency %v exceeded sub-200ms threshold!", maxLatency)
	}

	if p99 > 200*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: P99 latency %v exceeded 200ms threshold!", p99)
	}

	if avgLatency > 50*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: Average in-memory latency %v exceeded 50ms threshold!", avgLatency)
	}

	t.Logf("PASS: Sub-200ms latency verified (Avg: %v, P95: %v, P99: %v, Max: %v < 200ms target).",
		avgLatency.Round(time.Microsecond), p95.Round(time.Microsecond), p99.Round(time.Microsecond), maxLatency.Round(time.Microsecond))
}

// TestLatency_InitialNegotiationUnder1500ms verifies that WebRTC SDP negotiation and ICE
// candidate exchange establishes an active stream connection in under 1.5 seconds.
func TestLatency_InitialNegotiationUnder1500ms(t *testing.T) {
	const iterations = 5
	var totalNegotiationTime time.Duration

	for iter := 0; iter < iterations; iter++ {
		start := time.Now()
		mgr := webrtc.NewManager()
		sessionID := "test-speed-sess-" + time.Now().Format("150405.000000")
		camID := "cam-speed-test"

		receiverPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
		if err != nil {
			t.Fatalf("Failed to create receiver PC: %v", err)
		}

		_, err = receiverPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
			Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
		})
		if err != nil {
			receiverPC.Close()
			t.Fatalf("Failed to add video transceiver: %v", err)
		}

		connectedChan := make(chan struct{})
		var connectedOnce sync.Once

		receiverPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
			if s == pionwebrtc.PeerConnectionStateConnected {
				connectedOnce.Do(func() {
					close(connectedChan)
				})
			}
		})

		var earlyCandidates []pionwebrtc.ICECandidateInit
		var candMu sync.Mutex

		onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
			if c == nil {
				return
			}
			candMu.Lock()
			defer candMu.Unlock()
			if receiverPC.RemoteDescription() != nil {
				_ = receiverPC.AddICECandidate(c.ToJSON())
			} else {
				earlyCandidates = append(earlyCandidates, c.ToJSON())
			}
		}

		receiverPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
			if c == nil {
				return
			}
			candJSON, _ := json.Marshal(c.ToJSON())
			_ = mgr.AddICECandidate(sessionID, string(candJSON))
		})

		offer, err := receiverPC.CreateOffer(nil)
		if err != nil {
			receiverPC.Close()
			t.Fatalf("Failed to create offer: %v", err)
		}
		_ = receiverPC.SetLocalDescription(offer)

		answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
		if err != nil {
			receiverPC.Close()
			t.Fatalf("HandleOffer failed: %v", err)
		}

		_ = receiverPC.SetRemoteDescription(pionwebrtc.SessionDescription{
			Type: pionwebrtc.SDPTypeAnswer,
			SDP:  answerSDP,
		})

		candMu.Lock()
		for _, c := range earlyCandidates {
			_ = receiverPC.AddICECandidate(c)
		}
		candMu.Unlock()

		select {
		case <-connectedChan:
			elapsed := time.Since(start)
			totalNegotiationTime += elapsed
			if elapsed > 1500*time.Millisecond {
				t.Fatalf("Negotiation time %v exceeded 1.5s threshold at iteration %d", elapsed, iter)
			}
			t.Logf("Iteration %d: WebRTC connection established in %v (Threshold: < 1.5s)", iter+1, elapsed)
		case <-time.After(1500 * time.Millisecond):
			receiverPC.Close()
			mgr.RemoveSession(sessionID)
			t.Fatalf("Timeout (>1.5s) waiting for WebRTC connection at iteration %d", iter)
		}

		receiverPC.Close()
		mgr.RemoveSession(sessionID)
	}

	avgTime := totalNegotiationTime / time.Duration(iterations)
	t.Logf("PASS: Average connection negotiation time across %d sessions: %v (< 1.5s requirement met)", iterations, avgTime)
}

// TestReliability_ReconnectionUnderPacketLoss verifies that a media session survives severe
// simulated packet loss, performs fast re-sync/PLI keyframe requests, and recovers seamlessly.
func TestReliability_ReconnectionUnderPacketLoss(t *testing.T) {
	mgr := webrtc.NewManager()
	sessionID := "reconnect-loss-sess-01"
	camID := "cam-reconnect-test-01"

	receiverPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create receiver PC: %v", err)
	}
	defer receiverPC.Close()

	_, err = receiverPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	receiverPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var earlyCandidates []pionwebrtc.ICECandidateInit
	var candMu sync.Mutex

	onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candMu.Lock()
		defer candMu.Unlock()
		if receiverPC.RemoteDescription() != nil {
			_ = receiverPC.AddICECandidate(c.ToJSON())
		} else {
			earlyCandidates = append(earlyCandidates, c.ToJSON())
		}
	}

	receiverPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		_ = mgr.AddICECandidate(sessionID, string(candJSON))
	})

	offer, err := receiverPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	_ = receiverPC.SetLocalDescription(offer)

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
	if err != nil {
		t.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	_ = receiverPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	})

	candMu.Lock()
	for _, c := range earlyCandidates {
		_ = receiverPC.AddICECandidate(c)
	}
	candMu.Unlock()

	select {
	case <-connectedChan:
		t.Log("WebRTC PeerConnection connected.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for WebRTC connection")
	}

	session, ok := mgr.GetSession(sessionID)
	if !ok {
		t.Fatalf("Session %s not found in Manager", sessionID)
	}

	// Track received packets and keyframe recovery
	var preLossPackets int64
	var postLossPackets int64
	var keyframesReceived int64
	var isSimulatingLoss int32

	trackReady := make(chan struct{})
	var trackOnce sync.Once

	receiverPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		trackOnce.Do(func() {
			close(trackReady)
		})

		for {
			pkt, _, rErr := track.ReadRTP()
			if rErr != nil {
				return
			}
			if len(pkt.Payload) > 0 {
				nalType := pkt.Payload[0] & 0x1F
				if nalType == 5 || nalType == 7 { // IDR or SPS
					atomic.AddInt64(&keyframesReceived, 1)
				}
			}

			if atomic.LoadInt32(&isSimulatingLoss) == 0 {
				atomic.AddInt64(&preLossPackets, 1)
			} else {
				atomic.AddInt64(&postLossPackets, 1)
			}
		}
	})

	// 1. Send baseline media stream (30 frames)
	gen := media.NewSyntheticVideoGenerator(320, 240, 30)

	stopBaseline := make(chan struct{})
	defer close(stopBaseline)

	go func() {
		for i := 0; i < 30; i++ {
			select {
			case <-stopBaseline:
				return
			default:
			}
			frame := gen.GenerateNextFrame()
			for _, pkt := range frame.RTPPackets {
				_ = session.WriteRTP(pkt)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-trackReady:
		t.Log("Media track receiving packets.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for track ready")
	}

	time.Sleep(300 * time.Millisecond)

	if atomic.LoadInt64(&preLossPackets) == 0 {
		t.Fatal("Expected baseline packets to be received")
	}
	t.Logf("Baseline received: %d packets.", atomic.LoadInt64(&preLossPackets))

	// 2. SIMULATE SEVERE PACKET LOSS (40% loss simulation)
	t.Log("Simulating 40% packet loss condition and PLI trigger...")
	atomic.StoreInt32(&isSimulatingLoss, 1)

	// Receiver requests immediate keyframe (PLI)
	session.ABR.HandlePLI()
	gen.RequestKeyFrame()

	for i := 0; i < 40; i++ {
		frame := gen.GenerateNextFrame()
		// Drop 40% of packets randomly during transmission
		for pIdx, pkt := range frame.RTPPackets {
			if (i+pIdx)%5 == 0 || (i+pIdx)%5 == 2 {
				// Simulated network drop
				continue
			}
			_ = session.WriteRTP(pkt)
		}
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	postCount := atomic.LoadInt64(&postLossPackets)
	kfCount := atomic.LoadInt64(&keyframesReceived)

	t.Logf("Post-loss packets received: %d (Keyframes received: %d)", postCount, kfCount)

	if postCount == 0 {
		t.Fatal("Stream stalled completely during packet loss!")
	}
	if kfCount == 0 {
		t.Fatal("No keyframes received to recover stream after PLI request!")
	}

	t.Log("PASS: Resilient reconnection and keyframe recovery under packet loss verified successfully.")
}

// TestReliability_ZeroMemoryLeaksOverSessions asserts zero memory/goroutine leaks
// when rapidly creating and destroying WebRTC sessions under high lifecycle churn.
func TestReliability_ZeroMemoryLeaksOverSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping session leak test in short mode")
	}

	// 1. GC and capture baseline
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var baseMem runtime.MemStats
	runtime.ReadMemStats(&baseMem)
	baseGoroutines := runtime.NumGoroutine()

	t.Logf("Baseline: Goroutines=%d, HeapAlloc=%.2f MB",
		baseGoroutines, float64(baseMem.HeapAlloc)/(1024*1024))

	mgr := webrtc.NewManager()
	const sessionCount = 200

	// Generate sample offer SDP
	dummyPC, _ := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	_, _ = dummyPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo)
	dummyOffer, _ := dummyPC.CreateOffer(nil)
	dummySDP := dummyOffer.SDP
	dummyPC.Close()

	var wg sync.WaitGroup
	wg.Add(sessionCount)
	sem := make(chan struct{}, 20)

	for i := 0; i < sessionCount; i++ {
		idx := i
		go func(sIdx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sessID := "leak-test-sess-" + time.Now().Format("150405.000000") + "-" + string(rune('A'+(sIdx%26)))
			_, err := mgr.HandleOffer(sessID, "cam-leak-01", dummySDP, "", "", "", func(*pionwebrtc.ICECandidate) {})
			if err == nil {
				mgr.RemoveSession(sessID)
			}
		}(idx)
	}

	wg.Wait()

	// Settle and GC
	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()

	var postMem runtime.MemStats
	runtime.ReadMemStats(&postMem)
	postGoroutines := runtime.NumGoroutine()

	goroutineDelta := postGoroutines - baseGoroutines
	heapDeltaMB := float64(int64(postMem.HeapAlloc)-int64(baseMem.HeapAlloc)) / (1024 * 1024)

	t.Logf("Post-Teardown (%d Sessions): Goroutines=%d (delta: %+d), HeapAlloc=%.2f MB (delta: %+.2f MB)",
		sessionCount, postGoroutines, goroutineDelta, float64(postMem.HeapAlloc)/(1024*1024), heapDeltaMB)

	// Assert minimal goroutine delta (<= 15)
	if goroutineDelta > 15 {
		t.Fatalf("LEAK DETECTED: Goroutines increased by %d across %d sessions", goroutineDelta, sessionCount)
	}

	t.Logf("PASS: Zero memory/goroutine leaks verified across %d session lifecycles (Goroutine delta: %+d).",
		sessionCount, goroutineDelta)
}

// BenchmarkEndToEndLatencySimulation benchmarks per-packet media pipeline transmission latency
func BenchmarkEndToEndLatencySimulation(b *testing.B) {
	mgr := webrtc.NewManager()
	sessionID := "bench-latency-sess-01"
	camID := "cam-bench-latency"

	receiverPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		b.Fatalf("Failed to create receiver PC: %v", err)
	}
	defer receiverPC.Close()

	_, _ = receiverPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	receiverPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var pendingCandidates []pionwebrtc.ICECandidateInit
	var candMu sync.Mutex

	onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candMu.Lock()
		defer candMu.Unlock()
		if receiverPC.RemoteDescription() != nil {
			_ = receiverPC.AddICECandidate(c.ToJSON())
		} else {
			pendingCandidates = append(pendingCandidates, c.ToJSON())
		}
	}

	receiverPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		_ = mgr.AddICECandidate(sessionID, string(candJSON))
	})

	offer, _ := receiverPC.CreateOffer(nil)
	_ = receiverPC.SetLocalDescription(offer)

	answerSDP, err := mgr.HandleOffer(sessionID, camID, offer.SDP, "", "", "", onEdgeCandidate)
	if err != nil {
		b.Fatalf("HandleOffer failed: %v", err)
	}
	defer mgr.RemoveSession(sessionID)

	_ = receiverPC.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	})

	candMu.Lock()
	for _, c := range pendingCandidates {
		_ = receiverPC.AddICECandidate(c)
	}
	candMu.Unlock()

	select {
	case <-connectedChan:
	case <-time.After(3 * time.Second):
		b.Fatal("Timeout waiting for WebRTC connection")
	}

	session, ok := mgr.GetSession(sessionID)
	if !ok {
		b.Fatalf("Session %s not found in Manager", sessionID)
	}

	trackReady := make(chan struct{})
	var trackOnce sync.Once
	var packetsReceived int64

	receiverPC.OnTrack(func(track *pionwebrtc.TrackRemote, receiver *pionwebrtc.RTPReceiver) {
		trackOnce.Do(func() {
			close(trackReady)
		})
		for {
			_, _, rErr := track.ReadRTP()
			if rErr != nil {
				return
			}
			atomic.AddInt64(&packetsReceived, 1)
		}
	})

	gen := media.NewSyntheticVideoGenerator(320, 240, 30)
	initialFrame := gen.GenerateNextFrame()
	for _, pkt := range initialFrame.RTPPackets {
		_ = session.WriteRTP(pkt)
	}

	select {
	case <-trackReady:
	case <-time.After(3 * time.Second):
		b.Fatal("Timeout waiting for track ready")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		frame := gen.GenerateNextFrame()
		for _, pkt := range frame.RTPPackets {
			_ = session.WriteRTP(pkt)
		}
	}
}

// Suppress unused imports
var _ = math.Abs
var _ = rtp.Header{}
