package media

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"

	"github.com/pion/webrtc/v4"
)

// generateTestRBSPData generates synthetic raw byte sequences with varying frequencies
// of emulation prevention triggers (0x00 0x00 0x00..0x03).
func generateTestRBSPData(size int) []byte {
	rng := rand.New(rand.NewSource(1337))
	data := make([]byte, size)
	for i := 0; i < size; i++ {
		// Periodically inject 0x00 0x00 0x00..0x03 sequences to stress RBSP escaping
		if i%16 == 0 && i+3 < size {
			data[i] = 0x00
			data[i+1] = 0x00
			data[i+2] = byte(rng.Intn(4)) // 0, 1, 2, or 3
			i += 2
		} else {
			data[i] = byte(rng.Intn(256))
		}
	}
	return data
}

// BenchmarkEscapeRBSP measures heap allocation and throughput of standard EscapeRBSP
func BenchmarkEscapeRBSP(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"16KB", 16 * 1024},
		{"64KB", 64 * 1024},
		{"256KB", 256 * 1024},
	}

	for _, tc := range sizes {
		b.Run(tc.name, func(b *testing.B) {
			raw := generateTestRBSPData(tc.size)
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				escaped := EscapeRBSP(raw)
				if len(escaped) < len(raw) {
					b.Fatalf("Escaped length %d cannot be less than raw %d", len(escaped), len(raw))
				}
			}
		})
	}
}

// BenchmarkEscapeRBSPAppend measures throughput and zero-allocation efficiency when recycling buffers from sync.Pool
func BenchmarkEscapeRBSPAppend(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"16KB", 16 * 1024},
		{"64KB", 64 * 1024},
		{"256KB", 256 * 1024},
	}

	for _, tc := range sizes {
		b.Run(tc.name, func(b *testing.B) {
			raw := generateTestRBSPData(tc.size)
			b.SetBytes(int64(tc.size))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				bufPtr := GetRBSPBuffer()
				escaped := EscapeRBSPAppend((*bufPtr)[:0], raw)
				if len(escaped) < len(raw) {
					PutRBSPBuffer(bufPtr)
					b.Fatalf("Escaped length %d cannot be less than raw %d", len(escaped), len(raw))
				}
				*bufPtr = escaped
				PutRBSPBuffer(bufPtr)
			}
		})
	}
}

// BenchmarkPacketizeNALUs benchmarks RFC 6184 NALU slicing and FU-A fragmentation across MTU boundaries
func BenchmarkPacketizeNALUs(b *testing.B) {
	sps := GenerateSPS(320, 240)
	pps := GeneratePPS()
	renderer := NewPatternRenderer(320, 240)
	y, u, v := renderer.RenderFrame(0)
	idrSlice := GenerateIDRSlice(320, 240, 0, y, u, v)

	scenarios := []struct {
		name  string
		nalus [][]byte
		mtu   int
	}{
		{"SingleSmallNALU_1200MTU", [][]byte{sps}, 1200},
		{"SPS_PPS_Bundle_1200MTU", [][]byte{sps, pps}, 1200},
		{"LargeIDRSlice_FU_A_1200MTU", [][]byte{idrSlice}, 1200},
		{"FullKeyframe_SPS_PPS_IDR_1200MTU", [][]byte{sps, pps, idrSlice}, 1200},
		{"FullKeyframe_SPS_PPS_IDR_1400MTU", [][]byte{sps, pps, idrSlice}, 1400},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			var totalBytes int64
			for _, n := range sc.nalus {
				totalBytes += int64(len(n))
			}
			b.SetBytes(totalBytes)
			b.ReportAllocs()
			b.ResetTimer()

			var seq uint16
			for i := 0; i < b.N; i++ {
				pkts := PacketizeNALUs(sc.nalus, sc.mtu, 0x12345678, 90000, &seq)
				if len(pkts) == 0 {
					b.Fatal("Expected non-empty RTP packets from PacketizeNALUs")
				}
			}
		})
	}
}

// BenchmarkSyntheticVideoGenerator_GenerateNextFrame measures frame generation rate,
// color-pattern rendering, H.264 bitstream encoding, and RTP packetization speed.
func BenchmarkSyntheticVideoGenerator_GenerateNextFrame(b *testing.B) {
	resolutions := []struct {
		name   string
		width  int
		height int
		fps    int
	}{
		{"QVGA_320x240_30fps", 320, 240, 30},
		{"VGA_640x480_30fps", 640, 480, 30},
	}

	for _, res := range resolutions {
		b.Run(res.name, func(b *testing.B) {
			gen := NewSyntheticVideoGenerator(res.width, res.height, res.fps)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				frame := gen.GenerateNextFrame()
				if len(frame.RTPPackets) == 0 {
					b.Fatal("GenerateNextFrame produced 0 RTP packets")
				}
			}
		})
	}
}

// mockBenchCamera implements Camera interface for benchmarks
type mockBenchCamera struct {
	id   string
	name string
	url  string
}

func (m *mockBenchCamera) ID() string            { return m.id }
func (m *mockBenchCamera) Name() string          { return m.name }
func (m *mockBenchCamera) MainStreamURL() string { return m.url }

// BenchmarkLockFreeMediaDispatch measures multi-subscriber broadcast throughput
// under concurrent load without lock contention.
func BenchmarkLockFreeMediaDispatch(b *testing.B) {
	subscriberCounts := []int{10, 50, 100, 250}

	for _, subCount := range subscriberCounts {
		b.Run(fmt.Sprintf("%d_Subscribers", subCount), func(b *testing.B) {
			engine := NewEngine()
			cam := &mockBenchCamera{
				id:   fmt.Sprintf("bench-cam-%d", subCount),
				name: "Bench Camera",
				url:  "rtsp://127.0.0.1:8554/live",
			}

			// Pre-initialize camera state
			engine.mu.Lock()
			engine.cancelFuncs[cam.ID()] = func() {}
			videoTrack, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
			engine.tracks[cam.ID()] = videoTrack
			gen := NewSyntheticVideoGenerator(320, 240, 30)
			engine.syntheticGens[cam.ID()] = gen
			engine.mu.Unlock()

			// Create subscriber channels and background draining goroutines
			stopConsumers := make(chan struct{})
			var drainCount int64
			subs := make([]chan MediaPacket, subCount)

			engine.mu.Lock()
			for s := 0; s < subCount; s++ {
				ch := make(chan MediaPacket, 256)
				subs[s] = ch
				engine.subscribers[cam.ID()] = append(engine.subscribers[cam.ID()], ch)

				go func(c chan MediaPacket) {
					for {
						select {
						case <-stopConsumers:
							return
						case _, ok := <-c:
							if !ok {
								return
							}
							atomic.AddInt64(&drainCount, 1)
						}
					}
				}(ch)
			}
			engine.mu.Unlock()

			// Prepare test media packets
			keyPkt := gen.GenerateNextFrame()
			nonKeyPkt := gen.GenerateNextFrame()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				var pkt MediaPacket
				if i%30 == 0 {
					pkt = keyPkt
				} else {
					pkt = nonKeyPkt
				}
				engine.processMediaPacket(cam.ID(), pkt, videoTrack)
			}

			b.StopTimer()
			close(stopConsumers)
			engine.DetachCamera(cam.ID())
		})
	}
}

// BenchmarkPatternRenderer measures raw YUV420 canvas rendering performance per frame
func BenchmarkPatternRenderer(b *testing.B) {
	renderer := NewPatternRenderer(320, 240)
	b.SetBytes(int64(320 * 240 * 3 / 2))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		y, u, v := renderer.RenderFrame(i)
		if len(y) == 0 || len(u) == 0 || len(v) == 0 {
			b.Fatal("Empty YUV plane generated")
		}
	}
}

// BenchmarkBitWriter measures throughput of Exp-Golomb and bit-level bitstream packing
func BenchmarkBitWriter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		bw := GetBitWriter()
		for j := 0; j < 500; j++ {
			bw.WriteUE(uint32(j))
			bw.WriteSE(int32(j - 250))
			bw.WriteBits(uint32(j&0xFF), 8)
		}
		bw.RBSPTrailingBits()
		out := bw.Bytes()
		if len(out) == 0 {
			b.Fatal("BitWriter generated empty bytes")
		}
		PutBitWriter(bw)
	}
}
