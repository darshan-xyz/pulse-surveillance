package media

import (
	"fmt"
	"hss/internal/camera"
	"log"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/pion/webrtc/v4"
)

type MediaTrack interface{} // Placeholder

type Engine struct {
	clients     map[string]*gortsplib.Client
	tracks      map[string]*webrtc.TrackLocalStaticRTP
	subscribers map[string][]chan *format.H264
}

func NewEngine() *Engine {
	return &Engine{
		clients:     make(map[string]*gortsplib.Client),
		tracks:      make(map[string]*webrtc.TrackLocalStaticRTP),
		subscribers: make(map[string][]chan *format.H264),
	}
}

func (e *Engine) AttachCamera(cam camera.Camera) error {
	log.Printf("Attaching camera %s to media engine...\n", cam.Name())

	c := &gortsplib.Client{}

	u, err := base.ParseURL(cam.MainStreamURL())
	if err != nil {
		return err
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		return err
	}

	desc, _, err := c.Describe(u)
	if err != nil {
		c.Close()
		return err
	}

	// Find H264 track
	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		c.Close()
		return fmt.Errorf("h264 format not found")
	}

	_, err = c.SetupAndPlay(desc.BaseURL)
	if err != nil {
		c.Close()
		return err
	}

	// Save client to map (requires mutex in production)
	e.clients[cam.ID()] = c

	// Create Pion WebRTC video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	if err != nil {
		c.Close()
		delete(e.clients, cam.ID())
		return err
	}
	e.tracks[cam.ID()] = videoTrack

	log.Printf("Camera %s attached successfully and receiving frames.\n", cam.Name())

	// Write raw RTP packets to the WebRTC track and subscribers
	c.OnPacketRTP(medi, forma, func(pkt *format.H264) {
		if writeErr := videoTrack.WriteRTP(pkt.RTP); writeErr != nil {
			log.Printf("Failed to write RTP packet for %s: %v", cam.ID(), writeErr)
		}

		// Broadcast to internal recorders/analytics
		if subs, ok := e.subscribers[cam.ID()]; ok {
			for _, sub := range subs {
				select {
				case sub <- pkt:
				default:
					// Drop if subscriber is busy
				}
			}
		}
	})

	return nil
}

func (e *Engine) DetachCamera(camID string) error {
	if client, ok := e.clients[camID]; ok {
		client.Close()
		delete(e.clients, camID)
		delete(e.tracks, camID)
		
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
	if track, ok := e.tracks[camID]; ok {
		return track, nil
	}
	return nil, fmt.Errorf("camera %s not attached or track not initialized", camID)
}

// Subscribe provides a channel that yields raw H264 RTP packets for recording
func (e *Engine) Subscribe(camID string) (<-chan *format.H264, error) {
	if _, ok := e.clients[camID]; !ok {
		return nil, fmt.Errorf("camera %s not attached", camID)
	}

	ch := make(chan *format.H264, 1000)
	e.subscribers[camID] = append(e.subscribers[camID], ch)
	return ch, nil
}

var DefaultEngine = NewEngine()
