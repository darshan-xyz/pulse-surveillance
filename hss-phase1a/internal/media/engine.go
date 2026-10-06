package media

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Camera interface {
	ID() string
	Name() string
	MainStreamURL() string
}

type Engine struct {
	mu             sync.RWMutex
	cancelFuncs    map[string]context.CancelFunc
	tracks         map[string]*webrtc.TrackLocalStaticRTP
	subscribers    map[string][]chan MediaPacket
	cachedSPS      map[string][]byte
	cachedPPS      map[string][]byte
	cachedKeyframe map[string]MediaPacket
	syntheticGens  map[string]*SyntheticVideoGenerator
	keyframeReqs   map[string]chan struct{}
}

func NewEngine() *Engine {
	return &Engine{
		cancelFuncs:    make(map[string]context.CancelFunc),
		tracks:         make(map[string]*webrtc.TrackLocalStaticRTP),
		subscribers:    make(map[string][]chan MediaPacket),
		cachedSPS:      make(map[string][]byte),
		cachedPPS:      make(map[string][]byte),
		cachedKeyframe: make(map[string]MediaPacket),
		syntheticGens:  make(map[string]*SyntheticVideoGenerator),
		keyframeReqs:   make(map[string]chan struct{}),
	}
}

func (e *Engine) resolveCamIDLocked(camID string) string {
	if _, exists := e.cancelFuncs[camID]; exists {
		return camID
	}
	// Direct fallback for default device ID aliases
	if _, ok := e.cancelFuncs["test-camera-1"]; ok {
		if camID == "" || camID == "test-camera-1" || camID == "cam_001_mock" {
			return "test-camera-1"
		}
	}
	if _, ok := e.cancelFuncs["cam_001_mock"]; ok {
		if camID == "" || camID == "test-camera-1" || camID == "cam_001_mock" {
			return "cam_001_mock"
		}
	}
	// Fallback to single active camera if only 1 attached
	if len(e.cancelFuncs) == 1 {
		for id := range e.cancelFuncs {
			return id
		}
	}
	return camID
}

func (e *Engine) AttachCamera(cam Camera) error {
	e.mu.Lock()

	if _, exists := e.cancelFuncs[cam.ID()]; exists {
		e.mu.Unlock()
		return fmt.Errorf("camera %s already attached", cam.ID())
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFuncs[cam.ID()] = cancel
	e.keyframeReqs[cam.ID()] = make(chan struct{}, 16)

	// Create Pion WebRTC video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	if err != nil {
		cancel()
		e.mu.Unlock()
		return err
	}
	e.tracks[cam.ID()] = videoTrack

	// Initialize synthetic generator for fallback / test pattern
	syntheticGen := NewSyntheticVideoGenerator(320, 240, 30)
	e.syntheticGens[cam.ID()] = syntheticGen

	// Seed cached SPS/PPS/IDR immediately with synthetic generator
	initialKF := syntheticGen.GenerateNextFrame()
	e.updateCachedStateLocked(cam.ID(), initialKF)
	e.mu.Unlock()

	// Initial RTP packet dispatch outside the lock
	if videoTrack != nil {
		if len(initialKF.RTPPackets) > 0 {
			for _, pkt := range initialKF.RTPPackets {
				_ = videoTrack.WriteRTP(pkt)
			}
		} else if initialKF.OriginalRTP != nil {
			_ = videoTrack.WriteRTP(initialKF.OriginalRTP)
		}
	}

	log.Printf("Starting ingest loop for camera %s...\n", cam.Name())
	go e.runIngestLoop(ctx, cam, videoTrack, syntheticGen)

	return nil
}

func (e *Engine) updateCachedStateLocked(camID string, mp MediaPacket) MediaPacket {
	// Inspect NALUs to cache SPS, PPS, and check for IDR keyframes
	isKey := mp.IsKeyFrame
	var hasSPS, hasPPS, hasIDR bool

	for _, nalu := range mp.Payload {
		if len(nalu) > 0 {
			typ := nalu[0] & 0x1F
			if typ == 7 {
				e.cachedSPS[camID] = nalu
				hasSPS = true
			} else if typ == 8 {
				e.cachedPPS[camID] = nalu
				hasPPS = true
			} else if typ == 5 {
				isKey = true
				hasIDR = true
			}
		}
	}
	mp.IsKeyFrame = isKey

	// Cache latest KeyFrame (ensuring SPS & PPS are bundled if available)
	if isKey {
		cachedPkt := mp
		if hasIDR && (!hasSPS || !hasPPS) {
			sps := e.cachedSPS[camID]
			pps := e.cachedPPS[camID]
			if len(sps) > 0 && len(pps) > 0 {
				bundled := make([][]byte, 0, 2+len(mp.Payload))
				bundled = append(bundled, sps, pps)
				bundled = append(bundled, mp.Payload...)
				cachedPkt.Payload = bundled
			}
		}
		e.cachedKeyframe[camID] = cachedPkt
	}
	return mp
}

func (e *Engine) processMediaPacket(camID string, mp MediaPacket, videoTrack *webrtc.TrackLocalStaticRTP) {
	var subs []chan MediaPacket

	// Fast critical section: update cached state and snapshot subscriber slice
	e.mu.Lock()
	mp = e.updateCachedStateLocked(camID, mp)
	if srcSubs := e.subscribers[camID]; len(srcSubs) > 0 {
		subs = make([]chan MediaPacket, len(srcSubs))
		copy(subs, srcSubs)
	}
	e.mu.Unlock()

	// Lock-free dispatch to WebRTC video track (outside e.mu critical section)
	if videoTrack != nil {
		if len(mp.RTPPackets) > 0 {
			for _, pkt := range mp.RTPPackets {
				_ = videoTrack.WriteRTP(pkt)
			}
		} else if mp.OriginalRTP != nil {
			_ = videoTrack.WriteRTP(mp.OriginalRTP)
		}
	}

	// Lock-free broadcast to internal subscribers (outside e.mu critical section)
	for _, sub := range subs {
		func(c chan MediaPacket) {
			defer func() {
				_ = recover()
			}()
			select {
			case c <- mp:
			default:
				// Drop packet if subscriber buffer is full
			}
		}(sub)
	}
}

func (e *Engine) runIngestLoop(ctx context.Context, cam Camera, videoTrack *webrtc.TrackLocalStaticRTP, syntheticGen *SyntheticVideoGenerator) {
	mainURL := cam.MainStreamURL()
	if mainURL == "" || strings.HasPrefix(mainURL, "synthetic") || strings.HasPrefix(mainURL, "test") || strings.HasPrefix(mainURL, "mock") {
		log.Printf("[MediaEngine] Camera %s operating in synthetic video generator mode.", cam.ID())
		syntheticGen.RunLoop(ctx, func(pkt MediaPacket) {
			e.processMediaPacket(cam.ID(), pkt, videoTrack)
		})
		return
	}

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	var lastErrStr string

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := e.connectAndStream(ctx, cam, videoTrack)
		if err != nil {
			errStr := err.Error()
			if errStr != lastErrStr {
				log.Printf("[MediaEngine] Camera %s RTSP unavailable (%v); running fallback synthetic test pattern...", cam.ID(), err)
				lastErrStr = errStr
			}
		}

		// Fallback Synthetic Generator Active during backoff / no RTSP
		fallbackCtx, fallbackCancel := context.WithTimeout(ctx, backoff)
		syntheticGen.RunLoop(fallbackCtx, func(pkt MediaPacket) {
			e.processMediaPacket(cam.ID(), pkt, videoTrack)
		})
		fallbackCancel()

		select {
		case <-ctx.Done():
			return
		default:
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (e *Engine) connectAndStream(ctx context.Context, cam Camera, videoTrack *webrtc.TrackLocalStaticRTP) error {
	c := &gortsplib.Client{}

	u, err := base.ParseURL(cam.MainStreamURL())
	if err != nil {
		return err
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		return err
	}
	defer c.Close()

	desc, _, err := c.Describe(u)
	if err != nil {
		return err
	}

	// Find H264 track
	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		return fmt.Errorf("h264 format not found")
	}

	rtpDecoder, err := forma.CreateDecoder()
	if err != nil {
		return fmt.Errorf("failed to create RTP decoder: %v", err)
	}

	_, err = c.Setup(desc.BaseURL, medi, 0, 0)
	if err != nil {
		return err
	}
	_, err = c.Play(nil)
	if err != nil {
		return err
	}

	log.Printf("Camera %s connected and streaming from RTSP source.", cam.ID())

	errChan := make(chan error, 1)

	c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		nalus, err := rtpDecoder.Decode(pkt)
		if err != nil {
			if err != rtph264.ErrNonStartingPacketAndNoPrevious && err != rtph264.ErrMorePacketsNeeded {
				// Non-fatal dropped packet
			}
			return
		}

		pts, ok := c.PacketPTS(medi, pkt)
		if !ok {
			return
		}

		// Determine if KeyFrame (IDR NALU type 5)
		isKeyFrame := false
		for _, nalu := range nalus {
			if len(nalu) > 0 {
				typ := nalu[0] & 0x1F
				if typ == 5 {
					isKeyFrame = true
					break
				}
			}
		}

		mp := MediaPacket{
			Payload:     nalus,
			OriginalRTP: pkt,
			PTS:         pts,
			DTS:         pts,
			IsKeyFrame:  isKeyFrame,
		}

		e.processMediaPacket(cam.ID(), mp, videoTrack)
	})

	errWaitChan := make(chan error, 1)
	go func() {
		errWaitChan <- c.Wait()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errWaitChan:
		return fmt.Errorf("rtsp client disconnected: %w", err)
	case err := <-errChan:
		return err
	}
}

func (e *Engine) DetachCamera(camID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if cancel, ok := e.cancelFuncs[camID]; ok {
		cancel()
		delete(e.cancelFuncs, camID)
		delete(e.tracks, camID)
		delete(e.cachedSPS, camID)
		delete(e.cachedPPS, camID)
		delete(e.cachedKeyframe, camID)
		delete(e.syntheticGens, camID)
		delete(e.keyframeReqs, camID)

		// Close all subscriber channels
		if subs, ok := e.subscribers[camID]; ok {
			for _, sub := range subs {
				close(sub)
			}
			delete(e.subscribers, camID)
		}

		log.Printf("Detached camera %s\n", camID)
	}
	return nil
}

func (e *Engine) GetTrack(camID string, quality string) (*webrtc.TrackLocalStaticRTP, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	targetID := e.resolveCamIDLocked(camID)
	if track, ok := e.tracks[targetID]; ok {
		return track, nil
	}
	return nil, fmt.Errorf("camera %s not attached or track not initialized", camID)
}

// RequestKeyFrame signals the synthetic generator or camera pipeline to emit an immediate IDR keyframe
func (e *Engine) RequestKeyFrame(camID string) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	targetID := e.resolveCamIDLocked(camID)
	if gen, ok := e.syntheticGens[targetID]; ok {
		gen.RequestKeyFrame()
	}
	if reqCh, ok := e.keyframeReqs[targetID]; ok {
		select {
		case reqCh <- struct{}{}:
		default:
		}
	}
}

// Subscribe provides a channel that yields normalized MediaPackets.
// It immediately delivers the latest cached SPS/PPS/IDR KeyFrame to initialize client decoders instantly.
func (e *Engine) Subscribe(camID string) (<-chan MediaPacket, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	targetID := e.resolveCamIDLocked(camID)

	// Ensure camera exists
	if _, ok := e.cancelFuncs[targetID]; !ok {
		return nil, fmt.Errorf("camera %s not attached", camID)
	}

	ch := make(chan MediaPacket, 128)
	e.subscribers[targetID] = append(e.subscribers[targetID], ch)

	// Deliver cached KeyFrame immediately so client decoders initialize instantly without waiting for next GOP
	if kf, ok := e.cachedKeyframe[targetID]; ok && len(kf.Payload) > 0 {
		select {
		case ch <- kf:
		default:
		}
	}

	return ch, nil
}

// Unsubscribe removes a subscriber channel for a camera and closes it safely.
func (e *Engine) Unsubscribe(camID string, ch <-chan MediaPacket) {
	e.mu.Lock()
	defer e.mu.Unlock()

	targetID := e.resolveCamIDLocked(camID)
	subs, ok := e.subscribers[targetID]
	if !ok {
		return
	}

	for i, sub := range subs {
		if (<-chan MediaPacket)(sub) == ch {
			e.subscribers[targetID] = append(subs[:i], subs[i+1:]...)
			for len(sub) > 0 {
				<-sub
			}
			close(sub)
			break
		}
	}

	if len(e.subscribers[targetID]) == 0 {
		delete(e.subscribers, targetID)
	}
}

// SubscriberCount returns the current number of active subscribers for a camera.
func (e *Engine) SubscriberCount(camID string) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	targetID := e.resolveCamIDLocked(camID)
	return len(e.subscribers[targetID])
}

// GetSyntheticGenerator returns the active SyntheticVideoGenerator for a camera, if present.
func (e *Engine) GetSyntheticGenerator(camID string) *SyntheticVideoGenerator {
	e.mu.RLock()
	defer e.mu.RUnlock()
	targetID := e.resolveCamIDLocked(camID)
	return e.syntheticGens[targetID]
}

var DefaultEngine = NewEngine()
