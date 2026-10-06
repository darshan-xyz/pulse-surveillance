package webrtc

import (
	"log"
	"sync"
	"time"

	"github.com/pion/rtp"
)

// QualityMode represents the streaming quality/rate mode of the WebRTC session.
type QualityMode string

const (
	QualityHigh      QualityMode = "high"      // >= 500kbps (Full 30fps, 100% throughput)
	QualityThrottled QualityMode = "throttled" // 200kbps <= Bitrate < 500kbps (10fps, ~66% dropped non-keyframe frames)
	QualityEmergency QualityMode = "emergency" // < 200kbps (5fps or keyframes only, ~83% dropped non-keyframe frames)
)

const (
	DefaultBitrate            = 3_000_000.0 // 3.0 Mbps initial baseline
	BitrateThresholdThrottled = 500_000.0   // 500 kbps threshold
	BitrateThresholdEmergency = 200_000.0   // 200 kbps threshold
)

// ABRStats contains telemetry metrics for congestion and rate adaptation.
type ABRStats struct {
	SessionID       string      `json:"session_id"`
	CurrentBitrate  float64     `json:"current_bitrate"`
	CurrentQuality  QualityMode `json:"current_quality"`
	TotalPacketsIn  uint64      `json:"total_packets_in"`
	TotalPacketsOut uint64      `json:"total_packets_out"`
	DroppedPackets  uint64      `json:"dropped_packets"`
	LastREMBTime    time.Time   `json:"last_remb_time"`
	LastPLITime     time.Time   `json:"last_pli_time"`
	AdaptationCount uint64      `json:"adaptation_count"`
}

// ABRController manages per-session bandwidth estimation, quality mode transitions,
// and intelligent frame dropping / packet pacing for WebRTC streams.
type ABRController struct {
	mu             sync.RWMutex
	sessionID      string
	currentBitrate float64
	currentQuality QualityMode

	// Frame pacing state
	frameCounter  uint32
	lastTimestamp uint32
	lastDropFrame bool
	hasTimestamp  bool

	// Sequence number rewriting for smooth playback without loss alarms
	outSeqNum  uint16
	hasSeqInit bool

	// Telemetry & metrics
	totalIn         uint64
	totalOut        uint64
	dropped         uint64
	lastREMBTime    time.Time
	lastPLITime     time.Time
	adaptationCount uint64

	// Callbacks
	onQualityChange func(oldMode, newMode QualityMode, bitrate float64)
}

// NewABRController creates an ABR controller with an initial target bitrate.
func NewABRController(sessionID string, initialBitrate float64) *ABRController {
	if initialBitrate <= 0 {
		initialBitrate = DefaultBitrate
	}
	quality := determineQuality(initialBitrate)
	return &ABRController{
		sessionID:      sessionID,
		currentBitrate: initialBitrate,
		currentQuality: quality,
		lastREMBTime:   time.Now(),
	}
}

func determineQuality(bitrate float64) QualityMode {
	if bitrate >= BitrateThresholdThrottled {
		return QualityHigh
	} else if bitrate >= BitrateThresholdEmergency {
		return QualityThrottled
	}
	return QualityEmergency
}

// UpdateBitrate updates the estimated maximum bitrate (from RTCP REMB) and triggers
// quality mode transitions when crossing bitrate thresholds.
func (a *ABRController) UpdateBitrate(bitrate float64) {
	if bitrate <= 0 {
		return
	}

	a.mu.Lock()
	oldQuality := a.currentQuality
	newQuality := determineQuality(bitrate)

	a.currentBitrate = bitrate
	a.lastREMBTime = time.Now()

	var cb func(oldMode, newMode QualityMode, bitrate float64)
	if oldQuality != newQuality {
		a.currentQuality = newQuality
		a.adaptationCount++
		log.Printf("[ABR] Session %s transitioned quality %s -> %s (REMB bitrate: %.0f bps)",
			a.sessionID, oldQuality, newQuality, bitrate)

		if a.onQualityChange != nil {
			cb = a.onQualityChange
		}
	}
	a.mu.Unlock()

	if cb != nil {
		cb(oldQuality, newQuality, bitrate)
	}
}

// HandleREMB is an alias for UpdateBitrate.
func (a *ABRController) HandleREMB(bitrate float64) {
	a.UpdateBitrate(bitrate)
}

// HandlePLI records a Picture Loss Indication event and resets drop state to ensure
// the next incoming keyframe is forwarded immediately.
func (a *ABRController) HandlePLI() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastPLITime = time.Now()
	a.lastDropFrame = false
	log.Printf("[ABR] Session %s handled PLI request", a.sessionID)
}

// SetOnQualityChange registers a listener callback for quality transitions.
func (a *ABRController) SetOnQualityChange(cb func(oldMode, newMode QualityMode, bitrate float64)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onQualityChange = cb
}

// GetBitrate returns the current estimated bitrate.
func (a *ABRController) GetBitrate() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentBitrate
}

// GetQuality returns the current streaming quality mode.
func (a *ABRController) GetQuality() QualityMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentQuality
}

// GetStats returns snapshot metrics of the ABR controller.
func (a *ABRController) GetStats() ABRStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return ABRStats{
		SessionID:       a.sessionID,
		CurrentBitrate:  a.currentBitrate,
		CurrentQuality:  a.currentQuality,
		TotalPacketsIn:  a.totalIn,
		TotalPacketsOut: a.totalOut,
		DroppedPackets:  a.dropped,
		LastREMBTime:    a.lastREMBTime,
		LastPLITime:     a.lastPLITime,
		AdaptationCount: a.adaptationCount,
	}
}

// ProcessPacket evaluates whether to forward or drop an RTP packet based on the current
// quality mode and H.264 frame structure. It rewrites sequence numbers monotonically
// for forwarded packets to prevent jitter buffer alarms on client decoders.
func (a *ABRController) ProcessPacket(pkt *rtp.Packet) (*rtp.Packet, bool) {
	if pkt == nil {
		return nil, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.totalIn++

	// Keyframes (SPS, PPS, IDR) are critical and MUST NEVER be dropped
	if isKeyframeRTP(pkt.Payload) {
		a.lastDropFrame = false
		return a.forwardLocked(pkt)
	}

	// Frame grouping by RTP timestamp
	if !a.hasTimestamp || pkt.Timestamp != a.lastTimestamp {
		a.lastTimestamp = pkt.Timestamp
		a.hasTimestamp = true
		a.frameCounter++

		// Frame drop decision based on quality mode
		switch a.currentQuality {
		case QualityHigh:
			a.lastDropFrame = false // Keep 100% of frames (30fps)
		case QualityThrottled:
			// Target ~10fps: Keep 1 out of every 3 non-key frames (drop ~66.7%)
			a.lastDropFrame = (a.frameCounter % 3 != 0)
		case QualityEmergency:
			// Target ~5fps: Keep 1 out of every 6 non-key frames (drop ~83.3%)
			a.lastDropFrame = (a.frameCounter % 6 != 0)
		default:
			a.lastDropFrame = false
		}
	}

	if a.lastDropFrame {
		a.dropped++
		return nil, false
	}

	return a.forwardLocked(pkt)
}

func (a *ABRController) forwardLocked(pkt *rtp.Packet) (*rtp.Packet, bool) {
	a.totalOut++

	// Clone packet header & update sequence number monotonically
	out := *pkt
	if !a.hasSeqInit {
		a.outSeqNum = pkt.SequenceNumber
		a.hasSeqInit = true
	} else {
		a.outSeqNum++
	}
	out.Header.SequenceNumber = a.outSeqNum

	return &out, true
}

// isKeyframeRTP checks if the H.264 RTP payload contains SPS (7), PPS (8), or IDR (5) NALUs.
func isKeyframeRTP(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}

	nalType := payload[0] & 0x1F

	switch nalType {
	case 5, 7, 8: // IDR, SPS, PPS
		return true
	case 24: // STAP-A (Single-Time Aggregation Packet)
		offset := 1
		for offset+2 <= len(payload) {
			size := int(payload[offset])<<8 | int(payload[offset+1])
			offset += 2
			if offset+size > len(payload) {
				break
			}
			if size > 0 {
				innerType := payload[offset] & 0x1F
				if innerType == 5 || innerType == 7 || innerType == 8 {
					return true
				}
			}
			offset += size
		}
		return false
	case 28, 29: // FU-A, FU-B (Fragmentation Units)
		if len(payload) < 2 {
			return false
		}
		innerType := payload[1] & 0x1F
		return innerType == 5 || innerType == 7 || innerType == 8
	default:
		return false
	}
}
