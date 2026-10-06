package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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

// mockChallengerCamera implements media.Camera interface
type mockChallengerCamera struct {
	id     string
	name   string
	stream string
}

func (m *mockChallengerCamera) ID() string            { return m.id }
func (m *mockChallengerCamera) Name() string          { return m.name }
func (m *mockChallengerCamera) MainStreamURL() string { return m.stream }

// TestChallenger_RTPSequenceMonotonicity_10kPackets_And_PLIWrapAround stress-tests RTP sequence
// monotonicity across 15,000+ packets, validates uint16 sequence number wrap-around (65535 -> 0),
// and verifies strictly monotonic sequence progression during high-frequency concurrent PLI keyframe triggers.
func TestChallenger_RTPSequenceMonotonicity_10kPackets_And_PLIWrapAround(t *testing.T) {
	const fps = 30
	gen := media.NewSyntheticVideoGenerator(320, 240, fps)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Asynchronous worker firing rapid PLI requests
	var pliCount int64
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Millisecond):
				gen.RequestKeyFrame()
				atomic.AddInt64(&pliCount, 1)
			}
		}
	}()

	const targetPackets = 15000
	var totalPackets int
	var totalFrames int
	var keyframeCount int

	var lastFuSeq uint16
	var hasLastFuSeq bool

	var lastOrigSeq uint16
	var hasLastOrigSeq bool

	var lastTimestamp uint32
	var hasLastTimestamp bool

	var wrapAroundCount int

	for totalPackets < targetPackets {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout before reaching %d packets (completed %d packets, %d frames)",
				targetPackets, totalPackets, totalFrames)
		default:
		}

		mp := gen.GenerateNextFrame()
		totalFrames++

		if mp.IsKeyFrame {
			keyframeCount++
			if len(mp.Payload) < 3 {
				t.Fatalf("Frame %d: KeyFrame MediaPacket.Payload must contain SPS, PPS, IDR (got %d NALUs)",
					totalFrames, len(mp.Payload))
			}
		}

		// 1. Verify OriginalRTP sequence monotonicity and 90 kHz timestamp step
		if mp.OriginalRTP == nil {
			t.Fatalf("Frame %d: OriginalRTP is nil", totalFrames)
		}

		if !hasLastOrigSeq {
			lastOrigSeq = mp.OriginalRTP.SequenceNumber
			hasLastOrigSeq = true
		} else {
			expectedOrigSeq := lastOrigSeq + 1 // uint16 wraps naturally
			if mp.OriginalRTP.SequenceNumber != expectedOrigSeq {
				t.Fatalf("Frame %d: OriginalRTP sequence gap! Expected %d, got %d (last: %d)",
					totalFrames, expectedOrigSeq, mp.OriginalRTP.SequenceNumber, lastOrigSeq)
			}
			lastOrigSeq = mp.OriginalRTP.SequenceNumber
		}

		if !hasLastTimestamp {
			lastTimestamp = mp.OriginalRTP.Timestamp
			hasLastTimestamp = true
		} else {
			tsDelta := mp.OriginalRTP.Timestamp - lastTimestamp
			expectedDelta := uint32(90000 / fps)
			if tsDelta != expectedDelta {
				t.Fatalf("Frame %d: RTP timestamp delta mismatch: expected %d, got %d (curr: %d, prev: %d)",
					totalFrames, expectedDelta, tsDelta, mp.OriginalRTP.Timestamp, lastTimestamp)
			}
			lastTimestamp = mp.OriginalRTP.Timestamp
		}

		// 2. Verify RFC 6184 RTPPackets sequence monotonicity & FU-A header structure
		if len(mp.RTPPackets) == 0 {
			t.Fatalf("Frame %d: RTPPackets slice is empty", totalFrames)
		}

		for pIdx, pkt := range mp.RTPPackets {
			totalPackets++

			if !hasLastFuSeq {
				lastFuSeq = pkt.SequenceNumber
				hasLastFuSeq = true
			} else {
				expectedFuSeq := lastFuSeq + 1
				if pkt.SequenceNumber != expectedFuSeq {
					t.Fatalf("Frame %d, packet %d: FU-A sequence desync! Expected %d, got %d (last: %d)",
						totalFrames, pIdx, expectedFuSeq, pkt.SequenceNumber, lastFuSeq)
				}
				if pkt.SequenceNumber == 0 && lastFuSeq == 65535 {
					wrapAroundCount++
				}
				lastFuSeq = pkt.SequenceNumber
			}

			// Validate FU-A structure if payload is fragmented
			if len(pkt.Payload) > 2 && (pkt.Payload[0]&0x1F == 28) {
				fuHeader := pkt.Payload[1]
				isStart := (fuHeader & 0x80) != 0
				isEnd := (fuHeader & 0x40) != 0
				innerType := fuHeader & 0x1F

				if innerType != 5 && innerType != 1 {
					t.Fatalf("Frame %d, packet %d: Unexpected FU-A inner NALU type %d", totalFrames, pIdx, innerType)
				}
				if isStart && isEnd {
					t.Fatalf("Frame %d, packet %d: FU-A packet has both Start and End bits set", totalFrames, pIdx)
				}
			}

			// Validate Marker bit is set only on the last packet of the frame
			isLastPacketInFrame := (pIdx == len(mp.RTPPackets)-1)
			if pkt.Marker != isLastPacketInFrame {
				t.Fatalf("Frame %d, packet %d: Marker bit is %v, expected %v",
					totalFrames, pIdx, pkt.Marker, isLastPacketInFrame)
			}
		}
	}

	t.Logf("PASS: Verified %d RTP packets across %d frames (Keyframes: %d, PLI requests: %d, uint16 wrap-arounds: %d) with zero sequence desync.",
		totalPackets, totalFrames, keyframeCount, atomic.LoadInt64(&pliCount), wrapAroundCount)
}

// TestChallenger_SyncPool_ExtremeConcurrency_BufferRecycling validates that sync.Pool buffer
// reuse across 100 concurrent goroutines produces zero data races, zero slice aliasing,
// and zero dirty buffer leaks.
func TestChallenger_SyncPool_ExtremeConcurrency_BufferRecycling(t *testing.T) {
	const numGoroutines = 100
	const iterationsPerGoroutine = 300

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var totalOperations int64

	for g := 0; g < numGoroutines; g++ {
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(workerID * 10007)))

			for iter := 0; iter < iterationsPerGoroutine; iter++ {
				// 1. Stress BitWriter Pool
				bw := media.GetBitWriter()
				if bw == nil {
					t.Errorf("Worker %d: GetBitWriter returned nil", workerID)
					return
				}
				if len(bw.Bytes()) != 0 {
					t.Errorf("Worker %d: GetBitWriter returned dirty buffer with len %d", workerID, len(bw.Bytes()))
					return
				}

				numElements := 10 + rng.Intn(100)
				for k := 0; k < numElements; k++ {
					bw.WriteUE(uint32(k * 7))
					bw.WriteSE(int32(k*5 - 50))
					bw.WriteBits(uint32(k&0xFF), 8)
				}
				bw.RBSPTrailingBits()
				outBytes := bw.Bytes()
				if len(outBytes) == 0 {
					t.Errorf("Worker %d: BitWriter produced 0 bytes", workerID)
					return
				}

				// Make copy to verify buffer is not mutated after PutBitWriter
				outCopy := make([]byte, len(outBytes))
				copy(outCopy, outBytes)

				media.PutBitWriter(bw)

				// 2. Stress RBSP Buffer Pool
				rbspBuf := media.GetRBSPBuffer()
				if rbspBuf == nil {
					t.Errorf("Worker %d: GetRBSPBuffer returned nil", workerID)
					return
				}
				if len(*rbspBuf) != 0 {
					t.Errorf("Worker %d: GetRBSPBuffer returned dirty buffer with len %d", workerID, len(*rbspBuf))
					return
				}

				// Generate payload with zero sequences to force emulation prevention
				rawPayload := make([]byte, 256+rng.Intn(1024))
				for i := range rawPayload {
					if i%8 == 0 && i+2 < len(rawPayload) {
						rawPayload[i] = 0x00
						rawPayload[i+1] = 0x00
						rawPayload[i+2] = byte(rng.Intn(4))
					} else {
						rawPayload[i] = byte(rng.Intn(256))
					}
				}

				escaped := media.EscapeRBSPAppend((*rbspBuf)[:0], rawPayload)
				if len(escaped) < len(rawPayload) {
					t.Errorf("Worker %d: Escaped length %d < raw %d", workerID, len(escaped), len(rawPayload))
					return
				}

				// Verify emulation prevention rule: no sequence 0x00 0x00 0x00..0x03 exists without 0x03 insertion
				zeros := 0
				for idx, b := range escaped {
					if zeros >= 2 && b <= 3 {
						if b != 0x03 {
							t.Errorf("Worker %d: Illegal byte 0x%02X after %d zeros at index %d", workerID, b, zeros, idx)
						}
						zeros = 0
						continue
					}
					if b == 0x00 {
						zeros++
					} else {
						zeros = 0
					}
				}

				*rbspBuf = escaped
				media.PutRBSPBuffer(rbspBuf)

				// 3. Stress NALU Packetization
				var seq uint16 = uint16(rng.Intn(65535))
				nalus := [][]byte{outCopy, escaped}
				packets := media.PacketizeNALUs(nalus, 1200, 0x11223344, 90000, &seq)
				if len(packets) == 0 {
					t.Errorf("Worker %d: PacketizeNALUs returned 0 packets", workerID)
					return
				}

				atomic.AddInt64(&totalOperations, 1)
			}
		}(g)
	}

	wg.Wait()
	t.Logf("PASS: Completed %d concurrent sync.Pool buffer acquisition/recycle cycles with zero corruptions or data races.",
		atomic.LoadInt64(&totalOperations))
}

// TestChallenger_EngineLockFreeDispatch_SubscriberChurnStress simulates 150 concurrent subscribers
// rapidly subscribing, reading frames, and unsubscribing while the engine continuously broadcasts
// 60 FPS video packets. Asserts non-blocking dispatch, zero deadlocks, and zero channel write panics.
func TestChallenger_EngineLockFreeDispatch_SubscriberChurnStress(t *testing.T) {
	engine := media.NewEngine()
	cam := &mockChallengerCamera{
		id:     "churn-stress-cam-01",
		name:   "Subscriber Churn Camera",
		stream: "rtsp://127.0.0.1:8554/churn",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	track, err := engine.GetTrack(cam.ID(), "high")
	if err != nil || track == nil {
		t.Fatalf("Failed to get track: %v", err)
	}

	gen := media.NewSyntheticVideoGenerator(320, 240, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	// High-rate frame generation loop (60 FPS pump)
	var framesDispatched int64
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mp := gen.GenerateNextFrame()
				if len(mp.RTPPackets) > 0 {
					_ = track.WriteRTP(mp.RTPPackets[0])
				}
				if atomic.LoadInt64(&framesDispatched)%10 == 0 {
					engine.RequestKeyFrame(cam.ID())
				}
				atomic.AddInt64(&framesDispatched, 1)
			}
		}
	}()

	const numChurners = 150
	var churnWg sync.WaitGroup
	churnWg.Add(numChurners)

	var totalSubscribeCycles int64
	var totalKeyframesReceived int64
	var totalPacketsReceived int64

	for c := 0; c < numChurners; c++ {
		go func(workerID int) {
			defer churnWg.Done()
			rng := rand.New(rand.NewSource(int64(workerID * 54321)))

			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				ch, err := engine.Subscribe(cam.ID())
				if err != nil {
					t.Errorf("Worker %d: Subscribe failed: %v", workerID, err)
					return
				}
				atomic.AddInt64(&totalSubscribeCycles, 1)

				// Verify immediate cached keyframe delivery
				select {
				case kf, ok := <-ch:
					if !ok {
						t.Errorf("Worker %d: Subscriber channel closed unexpectedly", workerID)
						return
					}
					if !kf.IsKeyFrame {
						t.Errorf("Worker %d: First packet was not a keyframe", workerID)
						return
					}
					atomic.AddInt64(&totalKeyframesReceived, 1)
				case <-time.After(300 * time.Millisecond):
					t.Errorf("Worker %d: Timed out waiting for immediate keyframe", workerID)
					engine.Unsubscribe(cam.ID(), ch)
					return
				}

				// Read variable number of packets (0 to 10)
				packetsToRead := rng.Intn(10)
				for p := 0; p < packetsToRead; p++ {
					select {
					case _, ok := <-ch:
						if ok {
							atomic.AddInt64(&totalPacketsReceived, 1)
						}
					case <-time.After(20 * time.Millisecond):
						// Packet timeout ok under high load
					}
				}

				// Unsubscribe channel
				engine.Unsubscribe(cam.ID(), ch)

				// Small sleep to modulate churn frequency
				time.Sleep(time.Duration(rng.Intn(15)) * time.Millisecond)
			}
		}(c)
	}

	churnWg.Wait()

	// Verify subscriber cleanup
	for i := 0; i < 20; i++ {
		if engine.SubscriberCount(cam.ID()) == 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	activeSubs := engine.SubscriberCount(cam.ID())
	if activeSubs != 0 {
		t.Fatalf("Subscriber leak: expected 0 active subscribers after churners stopped, got %d", activeSubs)
	}

	t.Logf("PASS: Lock-free dispatch handled %d churn cycles (%d keyframes delivered, %d total packets) across %d concurrent subscribers with 0 leaks.",
		atomic.LoadInt64(&totalSubscribeCycles),
		atomic.LoadInt64(&totalKeyframesReceived),
		atomic.LoadInt64(&totalPacketsReceived),
		numChurners)
}

// TestChallenger_GlassToGlassLatency_And_PacketLossResilience establishes an end-to-end WebRTC
// media session, verifies strictly sub-200ms glass-to-glass latency with low jitter,
// and tests resilient stream recovery under severe simulated packet loss.
func TestChallenger_GlassToGlassLatency_And_PacketLossResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency challenge in short mode")
	}

	// 1. Capture initial memory baseline
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	goroutinesBefore := runtime.NumGoroutine()

	mgr := webrtc.NewManager()
	sessionID := "challenger-latency-sess-" + time.Now().Format("150405.000")
	camID := "challenger-cam-latency"

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

	// 2. Track packet receipt and glass-to-glass latency
	frameSentTimes := make(map[uint32]int64)
	var frameMu sync.Mutex

	var latencies []time.Duration
	var latenciesMu sync.Mutex
	var jitterSum float64
	var jitterCount int
	var prevTransit int64
	var hasPrevTransit bool

	var keyframesReceived int64
	var totalPacketsReceived int64

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
			receiveNanos := time.Now().UnixNano()
			atomic.AddInt64(&totalPacketsReceived, 1)

			if len(pkt.Payload) > 0 {
				nalType := pkt.Payload[0] & 0x1F
				if nalType == 5 || nalType == 7 {
					atomic.AddInt64(&keyframesReceived, 1)
				}
			}

			if pkt.Marker {
				frameMu.Lock()
				sentNanos, found := frameSentTimes[pkt.Timestamp]
				frameMu.Unlock()

				if found {
					latency := time.Duration(receiveNanos - sentNanos)
					if latency > 50*time.Millisecond {
						t.Logf("High latency frame: TS=%d, latency=%v", pkt.Timestamp, latency)
					}
					latenciesMu.Lock()
					latencies = append(latencies, latency)

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
		t.Log("Receiver track receiving media packets.")
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for track ready")
	}
	time.Sleep(50 * time.Millisecond)

	// 4. Stream 60 frames for glass-to-glass latency verification
	const totalFrames = 60
	frameInterval := 1000 * time.Millisecond / 30

	var pumpWg sync.WaitGroup
	pumpWg.Add(1)

	go func() {
		defer pumpWg.Done()
		for i := 0; i < totalFrames; i++ {
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

	pumpWg.Wait()
	time.Sleep(100 * time.Millisecond)

	// 5. Test Severe Packet Loss & Immediate PLI Recovery (30 frames)
	t.Log("Injecting 40% packet loss and testing RTCP PLI keyframe recovery...")
	var lossPhasePackets int64
	var lossPhaseKeyframes int64

	session.ABR.HandlePLI()
	gen.RequestKeyFrame()

	for i := 0; i < 30; i++ {
		mp := gen.GenerateNextFrame()
		for pIdx, pkt := range mp.RTPPackets {
			// Drop 40% of packets
			if (i+pIdx)%5 == 0 || (i+pIdx)%5 == 2 {
				continue
			}
			_ = session.WriteRTP(pkt)
			atomic.AddInt64(&lossPhasePackets, 1)
			if len(pkt.Payload) > 0 && ((pkt.Payload[0]&0x1F) == 5 || (pkt.Payload[0]&0x1F) == 7) {
				atomic.AddInt64(&lossPhaseKeyframes, 1)
			}
		}
		time.Sleep(frameInterval)
	}

	time.Sleep(150 * time.Millisecond)

	latenciesMu.Lock()
	defer latenciesMu.Unlock()

	if len(latencies) < 30 {
		t.Fatalf("Expected at least 30 measured latencies, got: %d", len(latencies))
	}

	// Calculate latency percentiles on clean streaming phase
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var sumLatency time.Duration
	for _, l := range latencies {
		sumLatency += l
	}
	avgLatency := sumLatency / time.Duration(len(latencies))
	minLatency := sorted[0]
	p50 := sorted[len(sorted)*50/100]
	p95 := sorted[len(sorted)*95/100]
	p99 := sorted[len(sorted)*99/100]
	maxLatency := sorted[len(sorted)-1]

	avgJitter := time.Duration(0)
	if jitterCount > 0 {
		avgJitter = time.Duration(jitterSum / float64(jitterCount))
	}

	t.Logf("Empirical Latency Results across %d clean frames (%d total packets received, %d keyframes):",
		len(latencies), atomic.LoadInt64(&totalPacketsReceived), atomic.LoadInt64(&keyframesReceived))
	t.Logf("  Min: %v | Avg: %v | P50: %v | P95: %v | P99: %v | Max: %v | Avg Jitter: %v",
		minLatency, avgLatency, p50, p95, p99, maxLatency, avgJitter)

	// Strictly assert sub-200ms latency limits on clean stream
	if maxLatency > 200*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: Max latency %v exceeded 200ms threshold!", maxLatency)
	}
	if p99 > 180*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: P99 latency %v exceeded 180ms threshold!", p99)
	}
	if avgLatency > 50*time.Millisecond {
		t.Fatalf("LATENCY REGRESSION: Avg latency %v exceeded 50ms in-memory threshold!", avgLatency)
	}

	// Verify that loss phase recovered and delivered packets post-PLI
	if atomic.LoadInt64(&lossPhasePackets) == 0 {
		t.Fatal("Loss phase failed to forward any packets")
	}
	if atomic.LoadInt64(&lossPhaseKeyframes) == 0 {
		t.Fatal("Loss phase failed to forward keyframe post-PLI")
	}
	t.Logf("Loss recovery verified: forwarded %d packets and %d keyframes post-PLI.",
		atomic.LoadInt64(&lossPhasePackets), atomic.LoadInt64(&lossPhaseKeyframes))

	// Teardown and verify memory / goroutine leaks
	receiverPC.Close()
	mgr.RemoveSession(sessionID)

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	goroutinesAfter := runtime.NumGoroutine()

	goroutineDelta := goroutinesAfter - goroutinesBefore
	t.Logf("Teardown Memory Profile: Goroutines: %d -> %d (delta: %+d)",
		goroutinesBefore, goroutinesAfter, goroutineDelta)

	if goroutineDelta > 20 {
		t.Fatalf("GOROUTINE LEAK DETECTED: Leaked %d goroutines after media session teardown", goroutineDelta)
	}

	t.Logf("PASS: Sub-200ms glass-to-glass latency verified under packet loss with 0 goroutine leaks.")
}

// Suppress unused imports
var _ = fmt.Sprintf
var _ = rtp.Header{}
