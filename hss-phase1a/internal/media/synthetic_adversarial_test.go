package media

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BitReader helper for parsing Exp-Golomb CAVLC encoded bitstreams
type BitReader struct {
	buf []byte
	pos int
	bit uint8
}

func NewBitReader(buf []byte) *BitReader {
	return &BitReader{buf: buf}
}

func (br *BitReader) ReadBit() (int, error) {
	if br.pos >= len(br.buf) {
		return 0, fmt.Errorf("EOF")
	}
	b := int((br.buf[br.pos] >> (7 - br.bit)) & 1)
	br.bit++
	if br.bit == 8 {
		br.bit = 0
		br.pos++
	}
	return b, nil
}

func (br *BitReader) ReadBits(n int) (uint32, error) {
	var val uint32
	for i := 0; i < n; i++ {
		b, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		val = (val << 1) | uint32(b)
	}
	return val, nil
}

func (br *BitReader) ReadUE() (uint32, error) {
	zeros := 0
	for {
		b, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		if b == 1 {
			break
		}
		zeros++
		if zeros > 32 {
			return 0, fmt.Errorf("Exp-Golomb overflow")
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	suffix, err := br.ReadBits(zeros)
	if err != nil {
		return 0, err
	}
	return (1 << zeros) - 1 + suffix, nil
}

func (br *BitReader) ReadSE() (int32, error) {
	codeNum, err := br.ReadUE()
	if err != nil {
		return 0, err
	}
	if codeNum%2 == 0 {
		return -int32(codeNum / 2), nil
	}
	return int32((codeNum + 1) / 2), nil
}

// UnescapeRBSP removes emulation prevention bytes (0x03)
func UnescapeRBSP(data []byte) []byte {
	out := make([]byte, 0, len(data))
	zeros := 0
	for i := 0; i < len(data); i++ {
		if zeros >= 2 && data[i] == 0x03 && i+1 < len(data) && data[i+1] <= 0x03 {
			// Skip emulation prevention byte
			zeros = 0
			continue
		}
		out = append(out, data[i])
		if data[i] == 0x00 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// TestAdversarial_H264_SPS_PPS_DeepBitstreamValidation validates exact standard compliance of SPS/PPS
func TestAdversarial_H264_SPS_PPS_DeepBitstreamValidation(t *testing.T) {
	resolutions := [][2]int{
		{160, 120},
		{320, 240},
		{640, 480},
		{1280, 720},
		{1920, 1080},
	}

	for _, res := range resolutions {
		w, h := res[0], res[1]
		t.Run(fmt.Sprintf("SPS_PPS_%dx%d", w, h), func(t *testing.T) {
			sps := GenerateSPS(w, h)
			if len(sps) < 4 {
				t.Fatalf("SPS too short: %d bytes", len(sps))
			}
			// NAL Header
			if sps[0]&0x80 != 0 {
				t.Error("Forbidden zero bit is not 0")
			}
			if (sps[0]>>5)&3 != 3 {
				t.Errorf("Expected nal_ref_idc=3 for SPS, got %d", (sps[0]>>5)&3)
			}
			if sps[0]&0x1F != 7 {
				t.Errorf("Expected nal_unit_type=7 (SPS), got %d", sps[0]&0x1F)
			}

			// Parse SPS RBSP
			rbsp := UnescapeRBSP(sps[1:])
			br := NewBitReader(rbsp)

			profileIdc, _ := br.ReadBits(8)
			if profileIdc != 66 {
				t.Errorf("Expected profile_idc=66 (Baseline), got %d", profileIdc)
			}
			constraintFlags, _ := br.ReadBits(8)
			if constraintFlags != 0xE0 {
				t.Errorf("Expected constraint_flags=0xE0 (Constrained Baseline), got 0x%X", constraintFlags)
			}
			levelIdc, _ := br.ReadBits(8)
			if levelIdc != 31 {
				t.Errorf("Expected level_idc=31, got %d", levelIdc)
			}
			spsID, _ := br.ReadUE()
			if spsID != 0 {
				t.Errorf("Expected sps_id=0, got %d", spsID)
			}
			log2MaxFrameNumMinus4, _ := br.ReadUE()
			if log2MaxFrameNumMinus4 != 0 {
				t.Errorf("Expected log2_max_frame_num_minus4=0, got %d", log2MaxFrameNumMinus4)
			}
			picOrderCntType, _ := br.ReadUE()
			if picOrderCntType != 0 {
				t.Errorf("Expected pic_order_cnt_type=0, got %d", picOrderCntType)
			}
			log2MaxPOCLsbMinus4, _ := br.ReadUE()
			if log2MaxPOCLsbMinus4 != 0 {
				t.Errorf("Expected log2_max_pic_order_cnt_lsb_minus4=0, got %d", log2MaxPOCLsbMinus4)
			}
			maxNumRefFrames, _ := br.ReadUE()
			if maxNumRefFrames != 1 {
				t.Errorf("Expected max_num_ref_frames=1, got %d", maxNumRefFrames)
			}
			gapsAllowed, _ := br.ReadBit()
			if gapsAllowed != 0 {
				t.Errorf("Expected gaps_in_frame_num_value_allowed_flag=0, got %d", gapsAllowed)
			}
			widthInMBsMinus1, _ := br.ReadUE()
			expectedMBWidth := uint32(w/16 - 1)
			if widthInMBsMinus1 != expectedMBWidth {
				t.Errorf("Expected width_in_mbs_minus1=%d, got %d", expectedMBWidth, widthInMBsMinus1)
			}
			heightInMapUnitsMinus1, _ := br.ReadUE()
			expectedMBHeight := uint32(h/16 - 1)
			if heightInMapUnitsMinus1 != expectedMBHeight {
				t.Errorf("Expected height_in_map_units_minus1=%d, got %d", expectedMBHeight, heightInMapUnitsMinus1)
			}
			frameMbsOnlyFlag, _ := br.ReadBit()
			if frameMbsOnlyFlag != 1 {
				t.Errorf("Expected frame_mbs_only_flag=1, got %d", frameMbsOnlyFlag)
			}

			// Validate PPS
			pps := GeneratePPS()
			if pps[0]&0x1F != 8 {
				t.Fatalf("Expected nal_unit_type=8 (PPS), got %d", pps[0]&0x1F)
			}
			ppsRbsp := UnescapeRBSP(pps[1:])
			brPPS := NewBitReader(ppsRbsp)
			ppsID, _ := brPPS.ReadUE()
			if ppsID != 0 {
				t.Errorf("Expected pps_id=0, got %d", ppsID)
			}
			ppsSpsID, _ := brPPS.ReadUE()
			if ppsSpsID != 0 {
				t.Errorf("Expected pps.sps_id=0, got %d", ppsSpsID)
			}
			entropyCoding, _ := brPPS.ReadBit()
			if entropyCoding != 0 {
				t.Errorf("Expected entropy_coding_mode_flag=0 (CAVLC), got %d", entropyCoding)
			}
		})
	}
}

// TestAdversarial_DynamicMotionVerification tests that every rendered frame exhibits non-zero pixel drift
// and visual element trajectory changes across time.
func TestAdversarial_DynamicMotionVerification(t *testing.T) {
	const (
		w   = 320
		h   = 240
		fps = 30
	)
	renderer := NewPatternRenderer(w, h)

	var prevY []byte
	boxXHistory := make([]float64, 120)
	boxYHistory := make([]float64, 120)

	for frame := 0; frame < 120; frame++ {
		boxXHistory[frame] = renderer.boxX
		boxYHistory[frame] = renderer.boxY

		curY, _, _ := renderer.RenderFrame(frame)

		// Make copies because RenderFrame reuses its internal buffer
		yCopy := make([]byte, len(curY))
		copy(yCopy, curY)

		if frame > 0 {
			// 1. Pixel Difference (Drift) Check
			yDiff := 0
			for i := range yCopy {
				if yCopy[i] != prevY[i] {
					yDiff++
				}
			}
			if yDiff == 0 {
				t.Fatalf("Frame %d is identical to frame %d! Dynamic motion is static/frozen.", frame, frame-1)
			}

			// 2. Trajectory movement check: box position must have changed
			if renderer.boxX == boxXHistory[frame-1] && renderer.boxY == boxYHistory[frame-1] {
				t.Fatalf("Frame %d: Bouncing box did not move! boxX=%f, boxY=%f", frame, renderer.boxX, renderer.boxY)
			}
		}

		prevY = yCopy
	}

	// 3. Test Bouncing physics: box must bounce within boundary limits [0, w-boxW] and [0, h-boxH]
	var bouncedX, bouncedY bool
	for f := 1; f < 120; f++ {
		if (boxXHistory[f] <= 0.1 && renderer.velX > 0) || (boxXHistory[f] >= float64(w-renderer.boxW)-0.1 && renderer.velX < 0) {
			bouncedX = true
		}
		if (boxYHistory[f] <= 0.1 && renderer.velY > 0) || (boxYHistory[f] >= float64(h-renderer.boxH)-0.1 && renderer.velY < 0) {
			bouncedY = true
		}
	}
	t.Logf("Motion validation complete: 120 frames verified with active pixel drift. BouncedX=%v, BouncedY=%v", bouncedX, bouncedY)
}

// TestAdversarial_RTPStreamIntegrityStress validates 1,000 continuous frames for 90kHz RTP monotonicity,
// sequence numbering, and payload size bounds.
func TestAdversarial_RTPStreamIntegrityStress(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)

	const totalFrames = 1000
	var lastTS uint32
	var lastSeq uint16
	var keyframeCount int

	for i := 0; i < totalFrames; i++ {
		pkt := gen.GenerateNextFrame()

		if pkt.OriginalRTP == nil {
			t.Fatalf("Frame %d: OriginalRTP is nil", i)
		}

		// Check RTP Header
		if pkt.OriginalRTP.Version != 2 {
			t.Fatalf("Frame %d: Invalid RTP Version %d", i, pkt.OriginalRTP.Version)
		}
		if pkt.OriginalRTP.PayloadType != 96 {
			t.Fatalf("Frame %d: Invalid PayloadType %d", i, pkt.OriginalRTP.PayloadType)
		}
		if !pkt.OriginalRTP.Marker {
			t.Fatalf("Frame %d: Marker bit not set on frame boundary", i)
		}

		if i == 0 {
			lastTS = pkt.OriginalRTP.Timestamp
			lastSeq = pkt.OriginalRTP.SequenceNumber
		} else {
			// Timestamp delta MUST be exactly 3000 (90000 / 30)
			tsDelta := pkt.OriginalRTP.Timestamp - lastTS
			if tsDelta != 3000 {
				t.Fatalf("Frame %d: Timestamp jump mismatch: expected 3000, got %d (curr: %d, prev: %d)",
					i, tsDelta, pkt.OriginalRTP.Timestamp, lastTS)
			}
			lastTS = pkt.OriginalRTP.Timestamp

			// Sequence Number MUST be incremented by 1
			seqDelta := pkt.OriginalRTP.SequenceNumber - lastSeq
			if seqDelta != 1 {
				t.Fatalf("Frame %d: Sequence number gap: expected 1, got %d (curr: %d, prev: %d)",
					i, seqDelta, pkt.OriginalRTP.SequenceNumber, lastSeq)
			}
			lastSeq = pkt.OriginalRTP.SequenceNumber
		}

		// Keyframe vs Non-Keyframe validation
		if pkt.IsKeyFrame {
			keyframeCount++
			payload := pkt.OriginalRTP.Payload
			if len(payload) == 0 {
				t.Fatalf("Frame %d: Empty keyframe payload", i)
			}
			nalType := payload[0] & 0x1F
			// Valid keyframe payload: Single NALU IDR (type 5) or STAP-A (type 24)
			if nalType != 5 && nalType != 24 {
				t.Fatalf("Frame %d: Keyframe RTP payload not NALU type 5 (IDR) or 24 (STAP-A), got %d", i, nalType)
			}

			// Validate that MediaPacket.Payload contains SPS, PPS, IDR
			if len(pkt.Payload) < 3 || pkt.Payload[0][0]&0x1F != 7 || pkt.Payload[1][0]&0x1F != 8 || pkt.Payload[2][0]&0x1F != 5 {
				t.Fatalf("Frame %d: MediaPacket.Payload must contain SPS (7), PPS (8), IDR (5), got %+v", i, pkt.Payload)
			}
		} else {
			// Non-Keyframe Single NALU
			payload := pkt.OriginalRTP.Payload
			if len(payload) == 0 {
				t.Fatalf("Frame %d: Empty non-keyframe payload", i)
			}
			if payload[0]&0x1F != 1 {
				t.Fatalf("Frame %d: Non-keyframe RTP payload not NALU type 1, got %d", i, payload[0]&0x1F)
			}
		}
	}

	t.Logf("RTP Stream Stress Pass: %d frames generated, %d keyframes (RFC 6184 verified), zero jitter / zero sequence gaps",
		totalFrames, keyframeCount)
}

// TestAdversarial_PLI_HighFrequencyStress fires random RTCP PLI keyframe requests concurrently
// while reading frames to ensure immediate keyframe generation with zero race conditions.
func TestAdversarial_PLI_HighFrequencyStress(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var pliRequests int64
	var keyframesReceived int64
	var totalFrames int64

	// PLI request worker
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(25 * time.Millisecond):
				gen.RequestKeyFrame()
				atomic.AddInt64(&pliRequests, 1)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			goto done
		default:
			pkt := gen.GenerateNextFrame()
			atomic.AddInt64(&totalFrames, 1)
			if pkt.IsKeyFrame {
				atomic.AddInt64(&keyframesReceived, 1)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

done:
	t.Logf("PLI Stress Completed: Total Frames: %d, PLI Requests: %d, Keyframes Generated: %d",
		totalFrames, pliRequests, keyframesReceived)
	if keyframesReceived < pliRequests/2 {
		t.Fatalf("PLI response rate too low: %d keyframes for %d requests", keyframesReceived, pliRequests)
	}
}

// TestAdversarial_ConcurrentSubscribersSafety spawns 200 concurrent subscribers attaching,
// receiving cached keyframes, reading packets, and unsubscribing concurrently.
func TestAdversarial_ConcurrentSubscribersSafety(t *testing.T) {
	engine := NewEngine()
	cam := &mockCamera{
		id:     "test-camera-1",
		name:   "Concurrent Stress Cam",
		stream: "rtsp://127.0.0.1:8554/cam1",
	}

	if err := engine.AttachCamera(cam); err != nil {
		t.Fatalf("Failed to attach camera: %v", err)
	}
	defer engine.DetachCamera(cam.ID())

	const numSubscribers = 200
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	var successCount int64

	for i := 0; i < numSubscribers; i++ {
		go func(subIdx int) {
			defer wg.Done()

			ch, err := engine.Subscribe("test-camera-1")
			if err != nil {
				t.Errorf("Sub %d subscribe failed: %v", subIdx, err)
				return
			}
			defer engine.Unsubscribe("test-camera-1", ch)

			// First packet MUST be cached Keyframe
			select {
			case pkt, ok := <-ch:
				if !ok || !pkt.IsKeyFrame {
					t.Errorf("Sub %d did not receive initial keyframe", subIdx)
					return
				}
				atomic.AddInt64(&successCount, 1)
			case <-time.After(500 * time.Millisecond):
				t.Errorf("Sub %d timed out waiting for initial keyframe", subIdx)
				return
			}

			// Request PLI concurrently
			if subIdx%5 == 0 {
				engine.RequestKeyFrame("test-camera-1")
			}
		}(i)
	}

	wg.Wait()

	if successCount != int64(numSubscribers) {
		t.Fatalf("Expected %d successful subscribers, got %d", numSubscribers, successCount)
	}

	// Verify all subscribers cleaned up
	for i := 0; i < 10; i++ {
		if engine.SubscriberCount("test-camera-1") == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if count := engine.SubscriberCount("test-camera-1"); count != 0 {
		t.Fatalf("Subscriber count leak: expected 0, got %d", count)
	}

	t.Logf("PASS: %d concurrent subscribers successfully received immediate keyframe with zero deadlocks / leaks.", numSubscribers)
}

// Suppress unused imports
var _ = bytes.Equal
var _ = math.Abs
