package media

import (
	"time"

	"github.com/pion/rtp"
)

// MediaPacket represents a normalized, decoded video frame.
// By unpacking RTP packets at the edge boundary, downstream consumers
// (like the MP4 Recorder or AI Analytics) don't have to deal with
// RTP depacketization, sequence numbers, or protocol-specific headers.
type MediaPacket struct {
	// Payload contains the actual video access units (e.g., H.264 NALUs)
	Payload [][]byte

	// OriginalRTP contains the raw RTP packet, necessary for forwarding
	// to WebRTC peer connections without re-packetizing.
	OriginalRTP *rtp.Packet

	// RTPPackets contains MTU-safe RFC 6184 RTP packets (including FU-A fragments and SPS/PPS)
	RTPPackets []*rtp.Packet

	// PTS is the Presentation Timestamp
	PTS time.Duration

	// DTS is the Decode Timestamp (often the same as PTS in baseline H264 without B-frames)
	DTS time.Duration

	// IsKeyFrame indicates if this packet contains an IDR (Instantaneous Decoding Refresh) NALU
	IsKeyFrame bool
}
