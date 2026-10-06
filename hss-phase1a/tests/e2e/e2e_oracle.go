package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"time"
)

// ============================================================================
// Synthetic Test Frame Generators
// ============================================================================

// FrameType specifies the visual content of a synthetic test frame.
type FrameType string

const (
	FrameTypeCleanRoom    FrameType = "CLEAN_ROOM"
	FrameTypeFireActive   FrameType = "FIRE_ACTIVE"
	FrameTypeSmokeActive  FrameType = "SMOKE_ACTIVE"
	FrameTypeFireAndSmoke FrameType = "FIRE_AND_SMOKE"
	FrameTypeFaintSmoke   FrameType = "FAINT_SMOKE"
	FrameTypeCorrupted    FrameType = "CORRUPTED"
	FrameTypeEmpty        FrameType = "EMPTY"
)

// GenerateSyntheticFrame creates a mock JPEG/byte frame for testing inference pipelines.
func GenerateSyntheticFrame(frameType FrameType, width, height int) ([]byte, []Detection) {
	if frameType == FrameTypeEmpty {
		return []byte{}, nil
	}
	if frameType == FrameTypeCorrupted {
		corrupt := make([]byte, 256)
		_, _ = rand.Read(corrupt)
		return corrupt, nil
	}

	if width <= 0 {
		width = 640
	}
	if height <= 0 {
		height = 480
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	var expectedDetections []Detection

	switch frameType {
	case FrameTypeCleanRoom:
		// Solid gray background, no fire/smoke
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Set(x, y, color.RGBA{R: 50, G: 50, B: 50, A: 255})
			}
		}

	case FrameTypeFireActive:
		// Draw simulated red/yellow fire hot-spot in center
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if x > width/3 && x < (2*width)/3 && y > height/3 && y < (2*height)/3 {
					img.Set(x, y, color.RGBA{R: 255, G: 69, B: 0, A: 255}) // Fire orange-red
				} else {
					img.Set(x, y, color.RGBA{R: 30, G: 30, B: 30, A: 255})
				}
			}
		}
		expectedDetections = append(expectedDetections, Detection{
			Class:      ClassNameFire,
			ClassID:    ClassIDFire,
			Confidence: 0.88,
			Box:        DetectionBox{0.33, 0.33, 0.34, 0.34},
		})

	case FrameTypeSmokeActive:
		// Draw simulated gray/white smoke haze in top half
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if y < height/2 {
					img.Set(x, y, color.RGBA{R: 180, G: 180, B: 180, A: 200}) // Smoke gray
				} else {
					img.Set(x, y, color.RGBA{R: 40, G: 40, B: 40, A: 255})
				}
			}
		}
		expectedDetections = append(expectedDetections, Detection{
			Class:      ClassNameSmoke,
			ClassID:    ClassIDSmoke,
			Confidence: 0.76,
			Box:        DetectionBox{0.05, 0.05, 0.90, 0.45},
		})

	case FrameTypeFireAndSmoke:
		// Left side smoke, right side fire
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if x < width/2 && y < height/2 {
					img.Set(x, y, color.RGBA{R: 160, G: 160, B: 160, A: 220})
				} else if x >= width/2 && y >= height/3 {
					img.Set(x, y, color.RGBA{R: 255, G: 50, B: 0, A: 255})
				} else {
					img.Set(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
				}
			}
		}
		expectedDetections = append(expectedDetections,
			Detection{
				Class:      ClassNameSmoke,
				ClassID:    ClassIDSmoke,
				Confidence: 0.72,
				Box:        DetectionBox{0.05, 0.05, 0.45, 0.45},
			},
			Detection{
				Class:      ClassNameFire,
				ClassID:    ClassIDFire,
				Confidence: 0.91,
				Box:        DetectionBox{0.50, 0.33, 0.45, 0.60},
			},
		)

	case FrameTypeFaintSmoke:
		// Faint smoke under confidence threshold (< 0.40)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				img.Set(x, y, color.RGBA{R: 60, G: 60, B: 60, A: 255})
			}
		}
		expectedDetections = append(expectedDetections, Detection{
			Class:      ClassNameSmoke,
			ClassID:    ClassIDSmoke,
			Confidence: 0.35, // Below 0.40 threshold -> must be filtered
			Box:        DetectionBox{0.20, 0.20, 0.60, 0.60},
		})
	}

	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes(), expectedDetections
}

// ============================================================================
// Authoritative HUD State Machine Oracle
// ============================================================================

type HUDOracle struct {
	ActiveAlerts   map[string]EmergencyEvent
	DismissedAlerts map[string]bool
	HoldDuration   time.Duration
}

func NewHUDOracle() *HUDOracle {
	return &HUDOracle{
		ActiveAlerts:    make(map[string]EmergencyEvent),
		DismissedAlerts: make(map[string]bool),
		HoldDuration:    3 * time.Second,
	}
}

func (h *HUDOracle) ProcessEvent(evt EmergencyEvent) HUDAlertStatus {
	if evt.Confidence >= DefaultConfidenceThreshold && !h.DismissedAlerts[evt.ID] {
		key := fmt.Sprintf("%s:%s", evt.CameraID, evt.Type)
		h.ActiveAlerts[key] = evt
	}

	return h.GetStatus()
}

func (h *HUDOracle) DismissAlert(eventID string) {
	h.DismissedAlerts[eventID] = true
	for key, evt := range h.ActiveAlerts {
		if evt.ID == eventID {
			delete(h.ActiveAlerts, key)
		}
	}
}

func (h *HUDOracle) ClearExpired(now time.Time) {
	for key, evt := range h.ActiveAlerts {
		if now.Sub(evt.StartedAt) > h.HoldDuration {
			delete(h.ActiveAlerts, key)
		}
	}
}

func (h *HUDOracle) GetStatus() HUDAlertStatus {
	status := HUDAlertStatus{
		BannerVisible:    len(h.ActiveAlerts) > 0,
		ActiveFireBadges:  []string{},
		ActiveSmokeBadges: []string{},
	}

	hasFire := false
	hasSmoke := false
	seenFire := make(map[string]bool)
	seenSmoke := make(map[string]bool)

	for _, evt := range h.ActiveAlerts {
		if evt.Type == EventFireDetected {
			hasFire = true
			if !seenFire[evt.CameraID] {
				status.ActiveFireBadges = append(status.ActiveFireBadges, evt.CameraID)
				seenFire[evt.CameraID] = true
			}
		} else if evt.Type == EventSmokeDetected {
			hasSmoke = true
			if !seenSmoke[evt.CameraID] {
				status.ActiveSmokeBadges = append(status.ActiveSmokeBadges, evt.CameraID)
				seenSmoke[evt.CameraID] = true
			}
		}
		if evt.StartedAt.After(status.LastAlertTime) {
			status.LastAlertTime = evt.StartedAt
		}
	}

	if hasFire {
		status.BannerSeverity = "critical"
		status.BannerMessage = fmt.Sprintf("EMERGENCY: Fire detected on %s!", strings.Join(status.ActiveFireBadges, ", "))
	} else if hasSmoke {
		status.BannerSeverity = "warning"
		status.BannerMessage = fmt.Sprintf("WARNING: Smoke detected on %s!", strings.Join(status.ActiveSmokeBadges, ", "))
	} else {
		status.BannerSeverity = "info"
		status.BannerMessage = "All systems normal"
	}

	return status
}

// ============================================================================
// WebSocket Message Validation Helper
// ============================================================================

func ValidateDetectionPacketSchema(rawJSON []byte) (*DetectionPacket, error) {
	var pkt DetectionPacket
	if err := json.Unmarshal(rawJSON, &pkt); err != nil {
		return nil, fmt.Errorf("invalid json payload: %w", err)
	}

	if pkt.Type != "detection" {
		return nil, fmt.Errorf("expected packet type 'detection', got: %s", pkt.Type)
	}

	if pkt.CameraID == "" {
		return nil, fmt.Errorf("missing camera_id in detection packet")
	}

	if pkt.Timestamp <= 0 {
		return nil, fmt.Errorf("invalid or non-positive timestamp: %d", pkt.Timestamp)
	}

	for i, d := range pkt.Detections {
		if d.Confidence < 0.0 || d.Confidence > 1.0 {
			return nil, fmt.Errorf("detection %d confidence out of bounds [0.0..1.0]: %f", i, d.Confidence)
		}
		if d.ClassID != ClassIDFire && d.ClassID != ClassIDSmoke && d.Class != ClassNameFire && d.Class != ClassNameSmoke {
			return nil, fmt.Errorf("detection %d unknown class: id=%d name=%s", i, d.ClassID, d.Class)
		}
		for j, coord := range d.Box {
			if coord < 0.0 || coord > 1.0 {
				return nil, fmt.Errorf("detection %d coordinate %d out of bounds [0.0..1.0]: %f", i, j, coord)
			}
		}
	}

	return &pkt, nil
}

// GenerateRandomUUID generates a deterministic or pseudo-random UUID for test verification.
func GenerateTestUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// PackRTPVideoHeader simulates an RTP header for RTSP feed tests.
func PackRTPVideoHeader(seq uint16, timestamp uint32, ssrc uint32) []byte {
	hdr := make([]byte, 12)
	hdr[0] = 0x80 // V=2, P=0, X=0, CC=0
	hdr[1] = 96   // M=0, PT=96 (Dynamic H.264)
	binary.BigEndian.PutUint16(hdr[2:4], seq)
	binary.BigEndian.PutUint32(hdr[4:8], timestamp)
	binary.BigEndian.PutUint32(hdr[8:12], ssrc)
	return hdr
}
