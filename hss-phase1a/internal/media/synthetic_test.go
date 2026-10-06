package media

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	mp4 "github.com/yapingcat/gomedia/go-mp4"
)

type memSeeker struct {
	buf []byte
	pos int64
}

func newMemSeeker() *memSeeker {
	return &memSeeker{buf: make([]byte, 0, 64*1024)}
}

func (m *memSeeker) Write(p []byte) (n int, err error) {
	minCap := int(m.pos) + len(p)
	if minCap > len(m.buf) {
		newBuf := make([]byte, minCap)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[m.pos:], p)
	m.pos += int64(len(p))
	return len(p), nil
}

func (m *memSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = m.pos + offset
	case io.SeekEnd:
		newPos = int64(len(m.buf)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if newPos < 0 {
		return 0, errors.New("negative position")
	}
	m.pos = newPos
	return newPos, nil
}

func (m *memSeeker) Bytes() []byte {
	return m.buf
}

func TestSyntheticVideoGenerator_NALUStructure(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)
	if gen == nil {
		t.Fatal("Failed to create SyntheticVideoGenerator")
	}

	// 1. Check SPS
	sps := gen.GetSPS()
	if len(sps) == 0 {
		t.Fatal("SPS is empty")
	}
	if sps[0]&0x1F != 7 {
		t.Fatalf("Expected SPS NALU type 7, got: %d", sps[0]&0x1F)
	}

	// 2. Check PPS
	pps := gen.GetPPS()
	if len(pps) == 0 {
		t.Fatal("PPS is empty")
	}
	if pps[0]&0x1F != 8 {
		t.Fatalf("Expected PPS NALU type 8, got: %d", pps[0]&0x1F)
	}

	// 3. Generate Keyframe (Frame 0)
	kf := gen.GenerateNextFrame()
	if !kf.IsKeyFrame {
		t.Fatal("Expected Frame 0 to be a KeyFrame")
	}
	if len(kf.Payload) < 3 {
		t.Fatalf("Expected at least 3 NALUs in KeyFrame payload, got: %d", len(kf.Payload))
	}
	if kf.Payload[0][0]&0x1F != 7 {
		t.Fatalf("Expected NALU 0 to be SPS (type 7), got: %d", kf.Payload[0][0]&0x1F)
	}
	if kf.Payload[1][0]&0x1F != 8 {
		t.Fatalf("Expected NALU 1 to be PPS (type 8), got: %d", kf.Payload[1][0]&0x1F)
	}
	if kf.Payload[2][0]&0x1F != 5 {
		t.Fatalf("Expected NALU 2 to be IDR (type 5), got: %d", kf.Payload[2][0]&0x1F)
	}

	// Check RTP packet
	if kf.OriginalRTP == nil {
		t.Fatal("Expected OriginalRTP to be non-nil")
	}
	if !kf.OriginalRTP.Marker {
		t.Fatal("Expected RTP Marker bit to be set on frame")
	}
	if kf.OriginalRTP.PayloadType != 96 {
		t.Fatalf("Expected RTP PayloadType 96, got: %d", kf.OriginalRTP.PayloadType)
	}

	// 4. Generate Non-Keyframe (Frame 1)
	nonKf := gen.GenerateNextFrame()
	if nonKf.IsKeyFrame {
		t.Fatal("Expected Frame 1 to be a Non-KeyFrame")
	}
	if len(nonKf.Payload) != 1 {
		t.Fatalf("Expected 1 NALU in Non-KeyFrame payload, got: %d", len(nonKf.Payload))
	}
	if nonKf.Payload[0][0]&0x1F != 1 {
		t.Fatalf("Expected NALU 0 to be Non-IDR Slice (type 1), got: %d", nonKf.Payload[0][0]&0x1F)
	}

	// 5. Verify Timestamp delta is 3000 at 30 fps
	tsDelta := nonKf.OriginalRTP.Timestamp - kf.OriginalRTP.Timestamp
	if tsDelta != 3000 {
		t.Fatalf("Expected 90 kHz timestamp delta 3000, got: %d", tsDelta)
	}

	// 6. Verify Sequence Number monotonicity
	seqDelta := nonKf.OriginalRTP.SequenceNumber - kf.OriginalRTP.SequenceNumber
	if seqDelta != 1 {
		t.Fatalf("Expected sequence number increment of 1, got: %d", seqDelta)
	}
}

func TestSyntheticVideoGenerator_PLIImmediateKeyframe(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)

	// Consume frame 0 (Keyframe)
	f0 := gen.GenerateNextFrame()
	if !f0.IsKeyFrame {
		t.Fatal("Expected Frame 0 to be KeyFrame")
	}

	// Frame 1 is Non-Keyframe
	f1 := gen.GenerateNextFrame()
	if f1.IsKeyFrame {
		t.Fatal("Expected Frame 1 to be Non-KeyFrame")
	}

	// Request KeyFrame (simulating RTCP PLI)
	gen.RequestKeyFrame()

	// Frame 2 MUST be an immediate Keyframe!
	f2 := gen.GenerateNextFrame()
	if !f2.IsKeyFrame {
		t.Fatal("Expected Frame 2 to be KeyFrame following RequestKeyFrame (PLI)")
	}
	if len(f2.Payload) < 3 || f2.Payload[2][0]&0x1F != 5 {
		t.Fatalf("Expected Frame 2 payload to contain IDR slice, got: %+v", f2.Payload)
	}

	// Frame 3 should resume normal non-keyframe
	f3 := gen.GenerateNextFrame()
	if f3.IsKeyFrame {
		t.Fatal("Expected Frame 3 to be Non-KeyFrame")
	}
}

func TestSyntheticVideoGenerator_MP4MuxingCompatibility(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)

	// Mux 60 frames (2 seconds) to in-memory MP4 buffer using gomedia
	seeker := newMemSeeker()
	muxer, err := mp4.CreateMp4Muxer(seeker)
	if err != nil {
		t.Fatalf("Failed to create MP4 muxer: %v", err)
	}

	videoTrack := muxer.AddVideoTrack(mp4.MP4_CODEC_H264)

	var firstPTS time.Duration
	var initPTS bool

	for i := 0; i < 60; i++ {
		pkt := gen.GenerateNextFrame()
		if !initPTS {
			if !pkt.IsKeyFrame {
				continue
			}
			firstPTS = pkt.PTS
			initPTS = true
		}

		samplePTS := uint64((pkt.PTS - firstPTS).Milliseconds())
		sampleDTS := uint64((pkt.DTS - firstPTS).Milliseconds())

		// Reconstruct Annex B stream
		var annexB []byte
		for _, nalu := range pkt.Payload {
			annexB = append(annexB, []byte{0x00, 0x00, 0x00, 0x01}...)
			annexB = append(annexB, nalu...)
		}

		err = muxer.Write(videoTrack, annexB, samplePTS, sampleDTS)
		if err != nil {
			t.Fatalf("Failed to write frame %d to MP4 muxer: %v", i, err)
		}
	}

	if err := muxer.WriteTrailer(); err != nil {
		t.Fatalf("Failed to write MP4 trailer: %v", err)
	}

	if len(seeker.Bytes()) == 0 {
		t.Fatal("Generated MP4 file is empty")
	}

	t.Logf("Successfully muxed 60 synthetic H.264 frames into MP4 container (%d bytes)", len(seeker.Bytes()))
}

func TestSyntheticVideoGenerator_RunLoop(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var count int
	gen.RunLoop(ctx, func(pkt MediaPacket) {
		count++
		if len(pkt.Payload) == 0 {
			t.Error("Received empty payload in RunLoop")
		}
	})

	if count < 3 {
		t.Fatalf("Expected at least 3 frames in 200ms at 30fps, got: %d", count)
	}
	t.Logf("RunLoop generated %d frames in 200ms", count)
}

func TestSyntheticVideoGenerator_FUMonotonicity(t *testing.T) {
	gen := NewSyntheticVideoGenerator(320, 240, 30)

	var lastSeq uint16
	var packetCount int

	for f := 0; f < 100; f++ {
		frame := gen.GenerateNextFrame()
		if len(frame.RTPPackets) == 0 {
			t.Fatalf("Frame %d produced 0 RTPPackets", f)
		}

		for pIdx, pkt := range frame.RTPPackets {
			packetCount++
			if packetCount == 1 {
				lastSeq = pkt.SequenceNumber
				continue
			}

			expectedSeq := lastSeq + 1
			if pkt.SequenceNumber != expectedSeq {
				t.Fatalf("Sequence gap or desync at frame %d, packet %d: expected %d, got %d (lastSeq: %d)",
					f, pIdx, expectedSeq, pkt.SequenceNumber, lastSeq)
			}
			lastSeq = pkt.SequenceNumber
		}
	}

	t.Logf("PASS: Verified %d RTP packets across 100 frames with strictly monotonic sequence numbers", packetCount)
}

func TestMemoryRecycling_RBSPBufferPool(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for iter := 0; iter < 100; iter++ {
				buf := GetRBSPBuffer()
				if buf == nil {
					t.Errorf("Worker %d got nil RBSP buffer", workerID)
					return
				}
				if len(*buf) != 0 {
					t.Errorf("Worker %d got non-empty RBSP buffer of len %d", workerID, len(*buf))
					return
				}
				raw := []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x02, 0x00, 0x00, 0x03}
				escaped := EscapeRBSPAppend((*buf)[:0], raw)
				if len(escaped) == 0 {
					t.Errorf("Worker %d failed to escape RBSP bytes", workerID)
				}
				*buf = escaped
				PutRBSPBuffer(buf)
			}
		}(i)
	}
	wg.Wait()
}

