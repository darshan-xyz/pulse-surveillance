package webrtc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
)

func TestABR_QualityThresholdTransitions(t *testing.T) {
	abr := NewABRController("test-sess-1", DefaultBitrate)

	if abr.GetQuality() != QualityHigh {
		t.Fatalf("Expected initial quality %s, got %s", QualityHigh, abr.GetQuality())
	}
	if abr.GetBitrate() != DefaultBitrate {
		t.Fatalf("Expected initial bitrate %f, got %f", DefaultBitrate, abr.GetBitrate())
	}

	var transitions []string
	var transMu sync.Mutex
	abr.SetOnQualityChange(func(oldMode, newMode QualityMode, bitrate float64) {
		transMu.Lock()
		defer transMu.Unlock()
		transitions = append(transitions, string(oldMode)+"->"+string(newMode))
	})

	// Transition to Throttled (400 kbps)
	abr.UpdateBitrate(400_000)
	if abr.GetQuality() != QualityThrottled {
		t.Fatalf("Expected quality %s, got %s", QualityThrottled, abr.GetQuality())
	}

	// Transition to Emergency (150 kbps)
	abr.UpdateBitrate(150_000)
	if abr.GetQuality() != QualityEmergency {
		t.Fatalf("Expected quality %s, got %s", QualityEmergency, abr.GetQuality())
	}

	// Recover to High (2.5 Mbps)
	abr.UpdateBitrate(2_500_000)
	if abr.GetQuality() != QualityHigh {
		t.Fatalf("Expected quality %s, got %s", QualityHigh, abr.GetQuality())
	}

	// Wait briefly for async callbacks
	time.Sleep(50 * time.Millisecond)

	transMu.Lock()
	defer transMu.Unlock()
	if len(transitions) != 3 {
		t.Fatalf("Expected 3 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "high->throttled" || transitions[1] != "throttled->emergency" || transitions[2] != "emergency->high" {
		t.Fatalf("Unexpected transition sequence: %v", transitions)
	}

	stats := abr.GetStats()
	if stats.AdaptationCount != 3 {
		t.Fatalf("Expected adaptation count 3, got %d", stats.AdaptationCount)
	}
}

func TestABR_KeyframePreservation(t *testing.T) {
	abr := NewABRController("test-sess-keyframe", 100_000) // Emergency mode (<200kbps)

	// 1. Single NALU IDR (Type 5)
	idrPkt := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 10, Timestamp: 1000},
		Payload: []byte{0x65, 0x88, 0x84, 0x00}, // 0x65 & 0x1F == 5 (IDR)
	}
	out, forward := abr.ProcessPacket(idrPkt)
	if !forward || out == nil {
		t.Fatal("Expected IDR keyframe to NEVER be dropped in Emergency mode")
	}

	// 2. Single NALU SPS (Type 7)
	spsPkt := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 11, Timestamp: 1000},
		Payload: []byte{0x67, 0x42, 0x00, 0x1f}, // 0x67 & 0x1F == 7 (SPS)
	}
	out, forward = abr.ProcessPacket(spsPkt)
	if !forward || out == nil {
		t.Fatal("Expected SPS packet to NEVER be dropped")
	}

	// 3. Single NALU PPS (Type 8)
	ppsPkt := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 12, Timestamp: 1000},
		Payload: []byte{0x68, 0xce, 0x3c, 0x80}, // 0x68 & 0x1F == 8 (PPS)
	}
	out, forward = abr.ProcessPacket(ppsPkt)
	if !forward || out == nil {
		t.Fatal("Expected PPS packet to NEVER be dropped")
	}

	// 4. STAP-A containing SPS + PPS (Type 24)
	stapPkt := &rtp.Packet{
		Header: rtp.Header{SequenceNumber: 13, Timestamp: 1000},
		Payload: []byte{
			0x78,       // STAP-A (24)
			0x00, 0x04, // size 4
			0x67, 0x42, 0x00, 0x1f, // SPS (7)
			0x00, 0x04, // size 4
			0x68, 0xce, 0x3c, 0x80, // PPS (8)
		},
	}
	out, forward = abr.ProcessPacket(stapPkt)
	if !forward || out == nil {
		t.Fatal("Expected STAP-A keyframe packet to NEVER be dropped")
	}

	// 5. FU-A containing IDR slice (Type 28, inner 5)
	fuaPkt := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 14, Timestamp: 1000},
		Payload: []byte{0x5c, 0x85, 0x01, 0x02}, // 0x5c & 0x1F == 28 (FU-A), 0x85 & 0x1F == 5 (IDR)
	}
	out, forward = abr.ProcessPacket(fuaPkt)
	if !forward || out == nil {
		t.Fatal("Expected FU-A IDR packet to NEVER be dropped")
	}
}

func TestABR_ThrottledFrameDropRatio(t *testing.T) {
	abr := NewABRController("test-sess-drop", 350_000) // Throttled mode: keep 1 out of 3 frames

	if abr.GetQuality() != QualityThrottled {
		t.Fatalf("Expected quality %s, got %s", QualityThrottled, abr.GetQuality())
	}

	totalFrames := 90
	forwardedFrames := 0
	droppedFrames := 0

	for i := 0; i < totalFrames; i++ {
		ts := uint32(i * 3000) // 3000 ticks per frame at 90kHz (33.3ms)
		// P-frame packet (NALU Type 1)
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i + 1), Timestamp: ts},
			Payload: []byte{0x41, 0x9a, 0x01, 0x02}, // 0x41 & 0x1F == 1 (Non-IDR P-frame)
		}

		out, forward := abr.ProcessPacket(pkt)
		if forward && out != nil {
			forwardedFrames++
		} else {
			droppedFrames++
		}
	}

	// In throttled mode: 1 of 3 kept = 30 kept, 60 dropped
	if forwardedFrames != 30 {
		t.Fatalf("Expected 30 forwarded frames, got %d", forwardedFrames)
	}
	if droppedFrames != 60 {
		t.Fatalf("Expected 60 dropped frames, got %d", droppedFrames)
	}

	dropRatio := float64(droppedFrames) / float64(totalFrames)
	if dropRatio < 0.60 || dropRatio > 0.70 {
		t.Fatalf("Expected drop ratio ~66.7%%, got %f", dropRatio)
	}
}

func TestABR_EmergencyFrameDropRatio(t *testing.T) {
	abr := NewABRController("test-sess-emerg", 150_000) // Emergency mode: keep 1 out of 6 frames

	totalFrames := 60
	forwardedFrames := 0
	droppedFrames := 0

	for i := 0; i < totalFrames; i++ {
		ts := uint32(i * 3000)
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i + 1), Timestamp: ts},
			Payload: []byte{0x41, 0x9a, 0x01, 0x02},
		}

		out, forward := abr.ProcessPacket(pkt)
		if forward && out != nil {
			forwardedFrames++
		} else {
			droppedFrames++
		}
	}

	// 1 of 6 kept = 10 kept, 50 dropped
	if forwardedFrames != 10 {
		t.Fatalf("Expected 10 forwarded frames, got %d", forwardedFrames)
	}
	if droppedFrames != 50 {
		t.Fatalf("Expected 50 dropped frames, got %d", droppedFrames)
	}
}

func TestABR_MonotonicSequenceNumbers(t *testing.T) {
	abr := NewABRController("test-sess-seq", 350_000) // Throttled

	var forwardedSeqNums []uint16
	for i := 0; i < 30; i++ {
		pkt := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(100 + i), Timestamp: uint32(i * 3000)},
			Payload: []byte{0x41, 0x01, 0x02},
		}
		if out, ok := abr.ProcessPacket(pkt); ok {
			forwardedSeqNums = append(forwardedSeqNums, out.SequenceNumber)
		}
	}

	if len(forwardedSeqNums) == 0 {
		t.Fatal("No packets forwarded")
	}

	// Check strictly monotonic increments
	for i := 1; i < len(forwardedSeqNums); i++ {
		expected := forwardedSeqNums[i-1] + 1
		if forwardedSeqNums[i] != expected {
			t.Fatalf("Sequence number gap detected: prev=%d, curr=%d (expected %d)",
				forwardedSeqNums[i-1], forwardedSeqNums[i], expected)
		}
	}
}

func TestABR_PLIReset(t *testing.T) {
	abr := NewABRController("test-sess-pli", 350_000)
	abr.HandlePLI()

	stats := abr.GetStats()
	if stats.LastPLITime.IsZero() {
		t.Fatal("Expected LastPLITime to be non-zero after HandlePLI")
	}
}

func TestABR_Concurrency(t *testing.T) {
	abr := NewABRController("test-sess-conc", DefaultBitrate)

	var wg sync.WaitGroup
	var droppedCount int64

	// Writer goroutines
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				pkt := &rtp.Packet{
					Header:  rtp.Header{SequenceNumber: uint16(gid*1000 + i), Timestamp: uint32(i * 3000)},
					Payload: []byte{0x41, 0x00, 0x01},
				}
				if _, ok := abr.ProcessPacket(pkt); !ok {
					atomic.AddInt64(&droppedCount, 1)
				}
			}
		}(g)
	}

	// Bitrate updater goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		rates := []float64{3_000_000, 400_000, 150_000, 2_000_000, 300_000}
		for _, r := range rates {
			abr.UpdateBitrate(r)
			time.Sleep(2 * time.Millisecond)
		}
	}()

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = abr.GetStats()
			_ = abr.GetQuality()
			_ = abr.GetBitrate()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}
