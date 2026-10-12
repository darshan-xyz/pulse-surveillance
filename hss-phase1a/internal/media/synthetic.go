package media

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
)

// BitWriter helper for H.264 bitstream and Exp-Golomb CAVLC encoding
type BitWriter struct {
	buf []byte
	cur byte
	bit uint8
}

var bitWriterPool = sync.Pool{
	New: func() any {
		return &BitWriter{
			buf: make([]byte, 0, 131072),
		}
	},
}

func GetBitWriter() *BitWriter {
	bw := bitWriterPool.Get().(*BitWriter)
	bw.buf = bw.buf[:0]
	bw.cur = 0
	bw.bit = 0
	return bw
}

func PutBitWriter(bw *BitWriter) {
	if cap(bw.buf) <= 262144 {
		bitWriterPool.Put(bw)
	}
}

func NewBitWriter() *BitWriter {
	return GetBitWriter()
}

func (bw *BitWriter) WriteBit(b int) {
	if b != 0 {
		bw.cur |= (1 << (7 - bw.bit))
	}
	bw.bit++
	if bw.bit == 8 {
		bw.buf = append(bw.buf, bw.cur)
		bw.cur = 0
		bw.bit = 0
	}
}

func (bw *BitWriter) WriteBits(val uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		bw.WriteBit(int((val >> i) & 1))
	}
}

// WriteUE writes an unsigned integer using Exp-Golomb coding
func (bw *BitWriter) WriteUE(val uint32) {
	v := val + 1
	var bits int
	for temp := v; temp > 0; temp >>= 1 {
		bits++
	}
	for i := 0; i < bits-1; i++ {
		bw.WriteBit(0)
	}
	bw.WriteBits(v, bits)
}

// WriteSE writes a signed integer using Exp-Golomb coding
func (bw *BitWriter) WriteSE(val int32) {
	if val <= 0 {
		bw.WriteUE(uint32(-2 * val))
	} else {
		bw.WriteUE(uint32(2*val - 1))
	}
}

// ByteAlign pads current byte with zero bits if not already aligned
func (bw *BitWriter) ByteAlign() {
	if bw.bit > 0 {
		bw.buf = append(bw.buf, bw.cur)
		bw.cur = 0
		bw.bit = 0
	}
}

// RBSPTrailingBits writes rbsp_stop_one_bit (1) and pads to byte boundary
func (bw *BitWriter) RBSPTrailingBits() {
	bw.WriteBit(1)
	for bw.bit != 0 {
		bw.WriteBit(0)
	}
}

// WriteRawBytes writes raw byte slice directly after byte alignment
func (bw *BitWriter) WriteRawBytes(data []byte) {
	bw.ByteAlign()
	bw.buf = append(bw.buf, data...)
}

func (bw *BitWriter) Bytes() []byte {
	bw.ByteAlign()
	return bw.buf
}

var rbspBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 262144)
		return &b
	},
}

func GetRBSPBuffer() *[]byte {
	bufPtr := rbspBufferPool.Get().(*[]byte)
	*bufPtr = (*bufPtr)[:0]
	return bufPtr
}

func PutRBSPBuffer(bufPtr *[]byte) {
	if bufPtr != nil && cap(*bufPtr) <= 524288 {
		*bufPtr = (*bufPtr)[:0]
		rbspBufferPool.Put(bufPtr)
	}
}

// EscapeRBSP inserts 0x03 emulation prevention bytes where required
func EscapeRBSP(rbsp []byte) []byte {
	out := make([]byte, 0, len(rbsp)+len(rbsp)/16)
	return EscapeRBSPAppend(out, rbsp)
}

// EscapeRBSPAppend appends escaped rbsp bytes to dst and returns the resulting slice
func EscapeRBSPAppend(dst []byte, rbsp []byte) []byte {
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 3 {
			dst = append(dst, 0x03)
			zeros = 0
		}
		dst = append(dst, b)
		if b == 0x00 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return dst
}

// GenerateSPS produces a valid H.264 Constrained Baseline Profile SPS NALU
func GenerateSPS(width, height int) []byte {
	bw := NewBitWriter()
	defer PutBitWriter(bw)
	// profile_idc = 66 (Baseline)
	bw.WriteBits(66, 8)
	// constraint_set0..5_flags + reserved_zero_2bits = 1110 0000 (0xE0: Constrained Baseline)
	bw.WriteBits(0xE0, 8)
	// level_idc = 31 (Level 3.1)
	bw.WriteBits(31, 8)
	// seq_parameter_set_id = 0
	bw.WriteUE(0)
	// log2_max_frame_num_minus4 = 0 (frame_num is 4 bits: 0..15)
	bw.WriteUE(0)
	// pic_order_cnt_type = 0
	bw.WriteUE(0)
	// log2_max_pic_order_cnt_lsb_minus4 = 0 (poc is 4 bits)
	bw.WriteUE(0)
	// max_num_ref_frames = 1
	bw.WriteUE(1)
	// gaps_in_frame_num_value_allowed_flag = 0
	bw.WriteBit(0)
	// pic_width_in_mbs_minus1 = width/16 - 1
	bw.WriteUE(uint32(width/16 - 1))
	// pic_height_in_map_units_minus1 = height/16 - 1
	bw.WriteUE(uint32(height/16 - 1))
	// frame_mbs_only_flag = 1
	bw.WriteBit(1)
	// direct_8x8_inference_flag = 1
	bw.WriteBit(1)
	// frame_cropping_flag = 0
	bw.WriteBit(0)
	// vui_parameters_present_flag = 0
	bw.WriteBit(0)
	// rbsp_trailing_bits
	bw.RBSPTrailingBits()

	bufPtr := GetRBSPBuffer()
	defer PutRBSPBuffer(bufPtr)
	escaped := EscapeRBSPAppend((*bufPtr)[:0], bw.Bytes())

	nalu := make([]byte, 1+len(escaped))
	// NAL header: forbidden_zero_bit=0, nal_ref_idc=3, nal_unit_type=7 (SPS) -> 0x67
	nalu[0] = 0x67
	copy(nalu[1:], escaped)
	return nalu
}

// GeneratePPS produces a valid H.264 PPS NALU
func GeneratePPS() []byte {
	bw := NewBitWriter()
	defer PutBitWriter(bw)
	// pic_parameter_set_id = 0
	bw.WriteUE(0)
	// seq_parameter_set_id = 0
	bw.WriteUE(0)
	// entropy_coding_mode_flag = 0 (CAVLC)
	bw.WriteBit(0)
	// bottom_field_pic_order_in_frame_present_flag = 0
	bw.WriteBit(0)
	// num_slice_groups_minus1 = 0
	bw.WriteUE(0)
	// num_ref_idx_l0_default_active_minus1 = 0
	bw.WriteUE(0)
	// num_ref_idx_l1_default_active_minus1 = 0
	bw.WriteUE(0)
	// weighted_pred_flag = 0
	bw.WriteBit(0)
	// weighted_bipred_idc = 0
	bw.WriteBits(0, 2)
	// pic_init_qp_minus26 = 0
	bw.WriteSE(0)
	// pic_init_qs_minus26 = 0
	bw.WriteSE(0)
	// chroma_qp_index_offset = 0
	bw.WriteSE(0)
	// deblocking_filter_control_present_flag = 1
	bw.WriteBit(1)
	// constrained_intra_pred_flag = 0
	bw.WriteBit(0)
	// redundant_pic_cnt_present_flag = 0
	bw.WriteBit(0)
	// rbsp_trailing_bits
	bw.RBSPTrailingBits()

	bufPtr := GetRBSPBuffer()
	defer PutRBSPBuffer(bufPtr)
	escaped := EscapeRBSPAppend((*bufPtr)[:0], bw.Bytes())

	nalu := make([]byte, 1+len(escaped))
	// NAL header: forbidden_zero_bit=0, nal_ref_idc=3, nal_unit_type=8 (PPS) -> 0x68
	nalu[0] = 0x68
	copy(nalu[1:], escaped)
	return nalu
}

// GenerateIDRSlice produces an IDR slice (Keyframe, NALU type 5)
func GenerateIDRSlice(width, height int, frameNum uint32, yPlane, uPlane, vPlane []byte) []byte {
	bw := NewBitWriter()
	defer PutBitWriter(bw)
	// first_mb_in_slice = 0
	bw.WriteUE(0)
	// slice_type = 7 (I slice) -> ue(7)
	bw.WriteUE(7)
	// pic_parameter_set_id = 0
	bw.WriteUE(0)
	// frame_num = frameNum % 16 (4 bits)
	bw.WriteBits(frameNum%16, 4)
	// idr_pic_id = 0 -> ue(0)
	bw.WriteUE(0)
	// pic_order_cnt_lsb = (frameNum * 2) % 16 (4 bits)
	bw.WriteBits((frameNum*2)%16, 4)
	// no_output_of_prior_pics_flag = 0
	bw.WriteBit(0)
	// long_term_reference_flag = 0
	bw.WriteBit(0)
	// slice_qp_delta = 0
	bw.WriteSE(0)
	// disable_deblocking_filter_idc = 1 (disable)
	bw.WriteUE(1)

	mbCols := width / 16
	mbRows := height / 16

	for mbY := 0; mbY < mbRows; mbY++ {
		for mbX := 0; mbX < mbCols; mbX++ {
			// mb_type = 25 (I_PCM mode in I slice)
			bw.WriteUE(25)
			bw.ByteAlign()

			// 256 bytes Y samples (16x16)
			startY := (mbY * 16) * width + (mbX * 16)
			for row := 0; row < 16; row++ {
				rowOffset := startY + row*width
				bw.WriteRawBytes(yPlane[rowOffset : rowOffset+16])
			}

			// 64 bytes U (Cb) samples (8x8)
			uvWidth := width / 2
			startUV := (mbY * 8) * uvWidth + (mbX * 8)
			for row := 0; row < 8; row++ {
				rowOffset := startUV + row*uvWidth
				bw.WriteRawBytes(uPlane[rowOffset : rowOffset+8])
			}

			// 64 bytes V (Cr) samples (8x8)
			for row := 0; row < 8; row++ {
				rowOffset := startUV + row*uvWidth
				bw.WriteRawBytes(vPlane[rowOffset : rowOffset+8])
			}
		}
	}

	bw.RBSPTrailingBits()
	bufPtr := GetRBSPBuffer()
	defer PutRBSPBuffer(bufPtr)
	escaped := EscapeRBSPAppend((*bufPtr)[:0], bw.Bytes())

	nalu := make([]byte, 1+len(escaped))
	// NAL header: forbidden_zero_bit=0, nal_ref_idc=3, nal_unit_type=5 (IDR) -> 0x65
	nalu[0] = 0x65
	copy(nalu[1:], escaped)
	return nalu
}

// GenerateNonIDRSlice produces a non-IDR slice (NALU type 1)
func GenerateNonIDRSlice(width, height int, frameNum uint32, yPlane, uPlane, vPlane []byte) []byte {
	bw := NewBitWriter()
	defer PutBitWriter(bw)
	// first_mb_in_slice = 0
	bw.WriteUE(0)
	// slice_type = 7 (I slice in non-IDR) -> ue(7)
	bw.WriteUE(7)
	// pic_parameter_set_id = 0
	bw.WriteUE(0)
	// frame_num = frameNum % 16 (4 bits)
	bw.WriteBits(frameNum%16, 4)
	// pic_order_cnt_lsb = (frameNum * 2) % 16 (4 bits)
	bw.WriteBits((frameNum*2)%16, 4)
	// adaptive_ref_pic_marking_mode_flag = 0
	bw.WriteBit(0)
	// slice_qp_delta = 0
	bw.WriteSE(0)
	// disable_deblocking_filter_idc = 1
	bw.WriteUE(1)

	mbCols := width / 16
	mbRows := height / 16

	for mbY := 0; mbY < mbRows; mbY++ {
		for mbX := 0; mbX < mbCols; mbX++ {
			bw.WriteUE(25) // I_PCM mode
			bw.ByteAlign()

			startY := (mbY * 16) * width + (mbX * 16)
			for row := 0; row < 16; row++ {
				rowOffset := startY + row*width
				bw.WriteRawBytes(yPlane[rowOffset : rowOffset+16])
			}

			uvWidth := width / 2
			startUV := (mbY * 8) * uvWidth + (mbX * 8)
			for row := 0; row < 8; row++ {
				rowOffset := startUV + row*uvWidth
				bw.WriteRawBytes(uPlane[rowOffset : rowOffset+8])
			}

			for row := 0; row < 8; row++ {
				rowOffset := startUV + row*uvWidth
				bw.WriteRawBytes(vPlane[rowOffset : rowOffset+8])
			}
		}
	}

	bw.RBSPTrailingBits()
	bufPtr := GetRBSPBuffer()
	defer PutRBSPBuffer(bufPtr)
	escaped := EscapeRBSPAppend((*bufPtr)[:0], bw.Bytes())

	nalu := make([]byte, 1+len(escaped))
	// NAL header: forbidden_zero_bit=0, nal_ref_idc=0, nal_unit_type=1 (Non-IDR non-reference slice) -> 0x01
	nalu[0] = 0x01
	copy(nalu[1:], escaped)
	return nalu
}

// PatternRenderer renders an animated moving test pattern into YUV420 canvas
type PatternRenderer struct {
	width   int
	height  int
	boxX    float64
	boxY    float64
	velX    float64
	velY    float64
	boxW    int
	boxH    int
	yPlane  []byte
	uPlane  []byte
	vPlane  []byte
	colorIdx int
}

func NewPatternRenderer(width, height int) *PatternRenderer {
	ySize := width * height
	uvSize := (width / 2) * (height / 2)
	return &PatternRenderer{
		width:   width,
		height:  height,
		boxX:    20,
		boxY:    20,
		velX:    3.5,
		velY:    2.5,
		boxW:    48,
		boxH:    32,
		yPlane:  make([]byte, ySize),
		uPlane:  make([]byte, uvSize),
		vPlane:  make([]byte, uvSize),
	}
}

// RenderFrame generates a new frame with updated moving elements
func (pr *PatternRenderer) RenderFrame(frameIdx int) ([]byte, []byte, []byte) {
	w := pr.width
	h := pr.height
	uvW := w / 2

	// 1. Render Base SMPTE-Style Color Bars (8 vertical bars across width)
	colorsYUV := [][3]byte{
		{235, 128, 128}, // White
		{210, 16, 146},  // Yellow
		{170, 166, 16},  // Cyan
		{145, 54, 34},   // Green
		{106, 202, 222}, // Magenta
		{81, 90, 240},   // Red
		{41, 240, 110},  // Blue
		{24, 128, 128},  // Dark Gray / Black
	}

	barWidth := w / len(colorsYUV)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			barIdx := x / barWidth
			if barIdx >= len(colorsYUV) {
				barIdx = len(colorsYUV) - 1
			}
			yVal := colorsYUV[barIdx][0]
			uVal := colorsYUV[barIdx][1]
			vVal := colorsYUV[barIdx][2]

			// Subtle coordinate grid overlay every 32 pixels
			if x%32 == 0 || y%32 == 0 {
				if yVal > 40 {
					yVal -= 30
				} else {
					yVal += 40
				}
			}

			pr.yPlane[y*w+x] = yVal
			if y%2 == 0 && x%2 == 0 {
				pr.uPlane[(y/2)*uvW+(x/2)] = uVal
				pr.vPlane[(y/2)*uvW+(x/2)] = vVal
			}
		}
	}

	// 2. Animate Bouncing Box Physics
	pr.boxX += pr.velX
	pr.boxY += pr.velY

	if pr.boxX <= 0 {
		pr.boxX = 0
		pr.velX = -pr.velX
		pr.colorIdx++
	} else if pr.boxX+float64(pr.boxW) >= float64(w) {
		pr.boxX = float64(w - pr.boxW)
		pr.velX = -pr.velX
		pr.colorIdx++
	}

	if pr.boxY <= 0 {
		pr.boxY = 0
		pr.velY = -pr.velY
		pr.colorIdx++
	} else if pr.boxY+float64(pr.boxH) >= float64(h) {
		pr.boxY = float64(h - pr.boxH)
		pr.velY = -pr.velY
		pr.colorIdx++
	}

	bx := int(pr.boxX)
	by := int(pr.boxY)

	// Box Fill Colors (Cycling bright neon colors)
	boxColors := [][3]byte{
		{210, 54, 34},   // Neon Green/Yellow
		{170, 16, 240},  // Electric Purple
		{235, 128, 128}, // Bright White
		{180, 240, 16},  // Deep Cyan
		{220, 128, 200}, // Bright Orange/Red
	}
	curColor := boxColors[pr.colorIdx%len(boxColors)]

	// Draw Bouncing Box with distinct dark border
	for y := by; y < by+pr.boxH && y < h; y++ {
		for x := bx; x < bx+pr.boxW && x < w; x++ {
			isBorder := (x == bx || x == bx+pr.boxW-1 || y == by || y == by+pr.boxH-1)
			isCrosshair := (x == bx+pr.boxW/2 || y == by+pr.boxH/2)

			var yVal, uVal, vVal byte
			if isBorder {
				yVal, uVal, vVal = 16, 128, 128 // Solid Black Border
			} else if isCrosshair {
				yVal, uVal, vVal = 235, 128, 128 // White Crosshair
			} else {
				yVal, uVal, vVal = curColor[0], curColor[1], curColor[2]
			}

			pr.yPlane[y*w+x] = yVal
			if y%2 == 0 && x%2 == 0 {
				pr.uPlane[(y/2)*uvW+(x/2)] = uVal
				pr.vPlane[(y/2)*uvW+(x/2)] = vVal
			}
		}
	}

	// 3. Animated Running Top Ticker Bar
	tickerX := (frameIdx * 6) % w
	for y := 4; y < 12 && y < h; y++ {
		for x := tickerX; x < tickerX+20 && x < w; x++ {
			pr.yPlane[y*w+x] = 235
			if y%2 == 0 && x%2 == 0 {
				pr.uPlane[(y/2)*uvW+(x/2)] = 128
				pr.vPlane[(y/2)*uvW+(x/2)] = 128
			}
		}
	}

	// 4. Animated Pulsing LIVE Motion Indicator in Bottom Corner
	pulse := (frameIdx / 15) % 2
	var pulseY, pulseU, pulseV byte
	if pulse == 0 {
		pulseY, pulseU, pulseV = 81, 90, 240 // Bright Red
	} else {
		pulseY, pulseU, pulseV = 235, 128, 128 // White Flash
	}

	cornerX := w - 28
	cornerY := h - 28
	for y := cornerY; y < cornerY+20 && y < h; y++ {
		for x := cornerX; x < cornerX+20 && x < w; x++ {
			pr.yPlane[y*w+x] = pulseY
			if y%2 == 0 && x%2 == 0 {
				pr.uPlane[(y/2)*uvW+(x/2)] = pulseU
				pr.vPlane[(y/2)*uvW+(x/2)] = pulseV
			}
		}
	}

	// 5. Rotating Radar / Sweep Line at Center
	centerX := float64(w / 2)
	centerY := float64(h / 2)
	angle := float64(frameIdx%60) * (2 * math.Pi / 60.0)
	for r := 0.0; r < 30.0; r += 1.0 {
		px := int(centerX + r*math.Cos(angle))
		py := int(centerY + r*math.Sin(angle))
		if px >= 0 && px < w && py >= 0 && py < h {
			pr.yPlane[py*w+px] = 235
			if py%2 == 0 && px%2 == 0 {
				pr.uPlane[(py/2)*uvW+(px/2)] = 16
				pr.vPlane[(py/2)*uvW+(px/2)] = 240
			}
		}
	}

	return pr.yPlane, pr.uPlane, pr.vPlane
}

// SyntheticVideoGenerator generates valid Constrained Baseline H.264 frames in pure Go
type SyntheticVideoGenerator struct {
	width         int
	height        int
	fps           int
	gopSize       int
	renderer      *PatternRenderer
	sps           []byte
	pps           []byte
	frameCounter  uint32
	seqNumber     uint16
	fuSeqNumber   uint16
	rtpTimestamp  uint32
	ssrc          uint32
	forceKeyframe atomic.Bool
	mu            sync.Mutex
}

func NewSyntheticVideoGenerator(width, height, fps int) *SyntheticVideoGenerator {
	if width <= 0 || width%16 != 0 {
		width = 320
	}
	if height <= 0 || height%16 != 0 {
		height = 240
	}
	if fps <= 0 {
		fps = 30
	}

	sps := GenerateSPS(width, height)
	pps := GeneratePPS()

	return &SyntheticVideoGenerator{
		width:        width,
		height:       height,
		fps:          fps,
		gopSize:      fps, // 1-second GOP (e.g. 30 frames)
		renderer:     NewPatternRenderer(width, height),
		sps:          sps,
		pps:          pps,
		ssrc:         0x98765432,
		rtpTimestamp: 1000,
	}
}

// RequestKeyFrame forces the very next generated frame to be an IDR keyframe with SPS/PPS
func (g *SyntheticVideoGenerator) RequestKeyFrame() {
	g.forceKeyframe.Store(true)
}

// GetSPS returns the cached SPS NALU
func (g *SyntheticVideoGenerator) GetSPS() []byte {
	return g.sps
}

// GetCurrentBoxNormalized returns the normalized bounding box [x, y, w, h] of the bouncing test pattern.
func (pr *PatternRenderer) GetCurrentBoxNormalized() []float64 {
	w := float64(pr.width)
	h := float64(pr.height)
	if w <= 0 {
		w = 320
	}
	if h <= 0 {
		h = 240
	}
	x := pr.boxX / w
	y := pr.boxY / h
	bw := float64(pr.boxW) / w
	bh := float64(pr.boxH) / h

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+bw > 1.0 {
		bw = 1.0 - x
	}
	if y+bh > 1.0 {
		bh = 1.0 - y
	}

	return []float64{x, y, bw, bh}
}

// GetCurrentBoxNormalized returns the normalized bouncing box from the underlying renderer.
func (g *SyntheticVideoGenerator) GetCurrentBoxNormalized() []float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.renderer != nil {
		return g.renderer.GetCurrentBoxNormalized()
	}
	return []float64{0.45, 0.50, 0.20, 0.30}
}

// GetPPS returns the cached PPS NALU
func (g *SyntheticVideoGenerator) GetPPS() []byte {
	return g.pps
}

// GenerateNextFrame generates the next video frame MediaPacket
func (g *SyntheticVideoGenerator) GenerateNextFrame() MediaPacket {
	g.mu.Lock()
	defer g.mu.Unlock()

	idx := g.frameCounter
	g.frameCounter++

	// Determine if this frame is a Keyframe
	isKey := (idx%uint32(g.gopSize) == 0) || g.forceKeyframe.Swap(false)

	// Render dynamic pattern
	y, u, v := g.renderer.RenderFrame(int(idx))

	var nalus [][]byte
	var sliceNALU []byte

	if isKey {
		sliceNALU = GenerateIDRSlice(g.width, g.height, idx, y, u, v)
		// Bundle SPS, PPS, and IDR Slice
		nalus = [][]byte{g.sps, g.pps, sliceNALU}
	} else {
		sliceNALU = GenerateNonIDRSlice(g.width, g.height, idx, y, u, v)
		nalus = [][]byte{sliceNALU}
	}

	// 90 kHz clock rate: increment by 90000 / fps per frame (3000 ticks at 30 fps)
	tsIncrement := uint32(90000 / g.fps)
	currentTS := g.rtpTimestamp
	g.rtpTimestamp += tsIncrement

	g.seqNumber++
	currentSeq := g.seqNumber

	// Construct primary OriginalRTP packet for tests and frame-level consumers
	origRTP := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: currentSeq,
			Timestamp:      currentTS,
			SSRC:           g.ssrc,
			Marker:         true, // Frame boundary marker
		},
		Payload: sliceNALU,
	}

	// Construct MTU-safe RFC 6184 RTP packets with FU-A fragmentation for WebRTC
	rtpPackets := PacketizeNALUs(nalus, 1200, g.ssrc, currentTS, &g.fuSeqNumber)

	pts := time.Duration(idx) * (time.Second / time.Duration(g.fps))

	return MediaPacket{
		Payload:     nalus,
		OriginalRTP: origRTP,
		RTPPackets:  rtpPackets,
		PTS:         pts,
		DTS:         pts,
		IsKeyFrame:  isKey,
	}
}

// PacketizeNALUs breaks H.264 NALUs into RFC 6184 MTU-compliant RTP packets (with FU-A fragmentation)
func PacketizeNALUs(nalus [][]byte, mtu int, ssrc uint32, timestamp uint32, seq *uint16) []*rtp.Packet {
	estPackets := 0
	maxChunk := mtu - 2
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		if len(nalu) <= mtu {
			estPackets++
		} else {
			rawLen := len(nalu) - 1
			estPackets += (rawLen + maxChunk - 1) / maxChunk
		}
	}

	packets := make([]*rtp.Packet, 0, estPackets)
	for i, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		isLastNALU := (i == len(nalus)-1)

		if len(nalu) <= mtu {
			*seq++
			packets = append(packets, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    96,
					SequenceNumber: *seq,
					Timestamp:      timestamp,
					SSRC:           ssrc,
					Marker:         isLastNALU,
				},
				Payload: nalu,
			})
			continue
		}

		// FU-A fragmentation (RFC 6184 Section 5.8)
		nalType := nalu[0] & 0x1F
		nri := nalu[0] & 0x60
		indicator := nri | 28 // 28 = FU-A

		raw := nalu[1:]
		totalChunks := (len(raw) + maxChunk - 1) / maxChunk
		chunkIdx := 0

		for len(raw) > 0 {
			chunkSize := len(raw)
			if chunkSize > maxChunk {
				chunkSize = maxChunk
			}
			isStartChunk := (chunkIdx == 0)
			isLastChunk := (chunkIdx == totalChunks-1)

			var fuHeader byte = nalType
			if isStartChunk {
				fuHeader |= 0x80 // S bit (Start)
			} else if isLastChunk {
				fuHeader |= 0x40 // E bit (End)
			}

			payload := make([]byte, 2+chunkSize)
			payload[0] = indicator
			payload[1] = fuHeader
			copy(payload[2:], raw[:chunkSize])
			raw = raw[chunkSize:]
			chunkIdx++

			*seq++
			packets = append(packets, &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    96,
					SequenceNumber: *seq,
					Timestamp:      timestamp,
					SSRC:           ssrc,
					Marker:         isLastNALU && isLastChunk,
				},
				Payload: payload,
			})
		}
	}
	return packets
}

// RunLoop runs the generator at the specified FPS and outputs MediaPackets to onPacket
func (g *SyntheticVideoGenerator) RunLoop(ctx context.Context, onPacket func(MediaPacket)) {
	frameDuration := time.Second / time.Duration(g.fps)
	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pkt := g.GenerateNextFrame()
			onPacket(pkt)
		}
	}
}
