package load

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"hss/internal/cloud/signaling"
	"hss/internal/security"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const (
	SyntheticSDPOffer = "v=0\r\no=- 9876543210 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:mockviewufrag\r\na=ice-pwd:mockviewpassword123456789\r\na=fingerprint:sha-256 FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00:FF:EE:DD:CC:BB:AA:99:88:77:66:55:44:33:22:11:00\r\na=setup:actpass\r\na=mid:0\r\na=recvonly\r\na=rtcp-mux\r\na=rtpmap:96 H264/90000\r\n"

	SyntheticSDPAnswer = "v=0\r\no=- 1234567890 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:mockcamufrag\r\na=ice-pwd:mockcampassword123456789\r\na=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF\r\na=setup:active\r\na=mid:0\r\na=sendonly\r\na=rtcp-mux\r\na=rtpmap:96 H264/90000\r\n"

	SyntheticCandidate1 = `{"candidate":"candidate:1 1 UDP 2130706431 192.168.1.100 50000 typ host","sdpMid":"0","sdpMLineIndex":0}`
	SyntheticCandidate2 = `{"candidate":"candidate:2 1 UDP 1694498815 10.0.0.1 50001 typ srflx raddr 192.168.1.100 rport 50000","sdpMid":"0","sdpMLineIndex":0}`
)

// GenerateUserJWT creates a valid HMAC-SHA256 JWT for a given username.
func GenerateUserJWT(username string) string {
	claims := jwt.MapClaims{
		"sub":      username,
		"username": username,
		"exp":      time.Now().Add(2 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
		"jti":      fmt.Sprintf("jwt-%s-%d", username, time.Now().UnixNano()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(security.GetJWTSecret())
	return tokenString
}

// TargetCameraConfig defines a paired camera and authorized user for multi-tenant load testing.
type TargetCameraConfig struct {
	DeviceID string
	Username string
	Token    string
}

// GetDefaultTargetCameras returns the authorized camera and user pairings for degraded/in-memory test mode.
func GetDefaultTargetCameras() []TargetCameraConfig {
	return []TargetCameraConfig{
		{DeviceID: "test-camera-1", Username: "admin", Token: "mock_user_jwt"},
		{DeviceID: "test-camera-2", Username: "user2", Token: "user2"},
		{DeviceID: "cam-tenant1-01", Username: "tenant1_user", Token: GenerateUserJWT("tenant1_user")},
		{DeviceID: "cam-tenant1-02", Username: "tenant1_viewer", Token: GenerateUserJWT("tenant1_viewer")},
		{DeviceID: "cam-tenant2-01", Username: "tenant2_user", Token: GenerateUserJWT("tenant2_user")},
	}
}

// MockCameraDaemon simulates an edge camera daemon connected to the cloud signaling hub.
type MockCameraDaemon struct {
	DeviceID             string
	ws                   *websocket.Conn
	ip                   string
	metrics              *LoadMetrics
	mu                   sync.Mutex
	closed               bool
	closeChan            chan struct{}
	OffersReceived       int64
	AnswersSent          int64
	CandidatesProcessed  int64
}

// NewMockCameraDaemon creates an initialized mock camera daemon.
func NewMockCameraDaemon(deviceID string, ip string, metrics *LoadMetrics) *MockCameraDaemon {
	return &MockCameraDaemon{
		DeviceID:  deviceID,
		ip:        ip,
		metrics:   metrics,
		closeChan: make(chan struct{}),
	}
}

// Connect dials the signaling WebSocket endpoint as an edge device.
func (c *MockCameraDaemon) Connect(wsURL string, token string) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   2048,
		WriteBufferSize:  2048,
	}

	headers := http.Header{}
	if c.ip != "" {
		headers.Set("X-Forwarded-For", c.ip)
	}

	endpoint := fmt.Sprintf("%s/signaling?token=%s&camera_id=%s", wsURL, token, c.DeviceID)
	start := time.Now()
	conn, resp, err := dialer.Dial(endpoint, headers)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	connectDuration := time.Since(start)

	if err != nil {
		if c.metrics != nil {
			c.metrics.RecordConnect(connectDuration, false, err)
		}
		return fmt.Errorf("camera %s dial failed: %w", c.DeviceID, err)
	}

	c.mu.Lock()
	c.ws = conn
	c.mu.Unlock()

	if c.metrics != nil {
		c.metrics.RecordConnect(connectDuration, true, nil)
	}

	go c.readLoop()
	return nil
}

// readLoop listens for incoming offers from the signaling hub and responds with SDP answers.
func (c *MockCameraDaemon) readLoop() {
	defer func() {
		c.mu.Lock()
		if !c.closed {
			c.closed = true
			if c.ws != nil {
				c.ws.Close()
			}
			close(c.closeChan)
		}
		c.mu.Unlock()
	}()

	for {
		var msg signaling.SignalingMessage
		c.mu.Lock()
		ws := c.ws
		c.mu.Unlock()

		if ws == nil {
			return
		}

		if err := ws.ReadJSON(&msg); err != nil {
			return
		}

		if c.metrics != nil {
			c.metrics.RecordMsgReceived()
		}

		switch msg.Type {
		case "offer":
			atomic.AddInt64(&c.OffersReceived, 1)

			// Respond with synthetic SDP Answer
			answerMsg := signaling.SignalingMessage{
				Type:      "answer",
				SessionID: msg.SessionID,
				DeviceID:  c.DeviceID,
				CameraID:  c.DeviceID,
				SDP:       SyntheticSDPAnswer,
			}

			c.mu.Lock()
			if !c.closed && c.ws != nil {
				_ = c.ws.WriteJSON(answerMsg)
				if c.metrics != nil {
					c.metrics.RecordMsgSent()
				}
				atomic.AddInt64(&c.AnswersSent, 1)
			}
			c.mu.Unlock()

		case "candidate":
			atomic.AddInt64(&c.CandidatesProcessed, 1)
		}
	}
}

// Close disconnects the mock camera daemon cleanly.
func (c *MockCameraDaemon) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	var err error
	if c.ws != nil {
		err = c.ws.Close()
	}
	select {
	case <-c.closeChan:
	default:
		close(c.closeChan)
	}
	return err
}

// MockViewer simulates a mobile or web client connecting to the signaling hub to stream video.
type MockViewer struct {
	ViewerID       string
	DeviceID       string
	SessionID      string
	ws             *websocket.Conn
	ip             string
	metrics        *LoadMetrics
	mu             sync.Mutex
	closed         bool
	closeChan      chan struct{}
	answerChan     chan signaling.SignalingMessage
	candidateChan  chan signaling.SignalingMessage
}

// NewMockViewer creates an initialized mock viewer.
func NewMockViewer(viewerID, deviceID, sessionID, ip string, metrics *LoadMetrics) *MockViewer {
	return &MockViewer{
		ViewerID:      viewerID,
		DeviceID:      deviceID,
		SessionID:     sessionID,
		ip:            ip,
		metrics:       metrics,
		closeChan:     make(chan struct{}),
		answerChan:    make(chan signaling.SignalingMessage, 10),
		candidateChan: make(chan signaling.SignalingMessage, 20),
	}
}

// Connect dials the viewer WebSocket endpoint.
func (v *MockViewer) Connect(wsURL string, token string) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   2048,
		WriteBufferSize:  2048,
	}

	headers := http.Header{}
	if v.ip != "" {
		headers.Set("X-Forwarded-For", v.ip)
	}

	endpoint := fmt.Sprintf("%s/client?token=%s", wsURL, token)
	start := time.Now()
	conn, resp, err := dialer.Dial(endpoint, headers)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	connectDuration := time.Since(start)

	if err != nil {
		if v.metrics != nil {
			v.metrics.RecordConnect(connectDuration, false, err)
		}
		return fmt.Errorf("viewer %s dial failed: %w", v.ViewerID, err)
	}

	v.mu.Lock()
	v.ws = conn
	v.mu.Unlock()

	if v.metrics != nil {
		v.metrics.RecordConnect(connectDuration, true, nil)
	}

	go v.readLoop()
	return nil
}

// readLoop receives messages from the signaling hub and routes them to internal channels.
func (v *MockViewer) readLoop() {
	defer func() {
		v.mu.Lock()
		if !v.closed {
			v.closed = true
			if v.ws != nil {
				v.ws.Close()
			}
			close(v.closeChan)
		}
		v.mu.Unlock()
	}()

	for {
		var msg signaling.SignalingMessage
		v.mu.Lock()
		ws := v.ws
		v.mu.Unlock()

		if ws == nil {
			return
		}

		if err := ws.ReadJSON(&msg); err != nil {
			return
		}

		if v.metrics != nil {
			v.metrics.RecordMsgReceived()
		}

		switch msg.Type {
		case "answer":
			select {
			case v.answerChan <- msg:
			default:
			}
		case "candidate":
			select {
			case v.candidateChan <- msg:
			default:
			}
		}
	}
}

// PerformHandshake sends an SDP offer and waits for the SDP answer.
func (v *MockViewer) PerformHandshake(timeout time.Duration) (time.Duration, error) {
	start := time.Now()

	// 1. Send SDP Offer
	offerMsg := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: v.SessionID,
		DeviceID:  v.DeviceID,
		CameraID:  v.DeviceID,
		SDP:       SyntheticSDPOffer,
	}

	v.mu.Lock()
	if v.closed || v.ws == nil {
		v.mu.Unlock()
		err := fmt.Errorf("viewer connection closed")
		if v.metrics != nil {
			v.metrics.RecordHandshake(time.Since(start), false, err)
		}
		return 0, err
	}

	if err := v.ws.WriteJSON(offerMsg); err != nil {
		v.mu.Unlock()
		if v.metrics != nil {
			v.metrics.RecordHandshake(time.Since(start), false, err)
		}
		return 0, fmt.Errorf("failed to send offer: %w", err)
	}
	if v.metrics != nil {
		v.metrics.RecordMsgSent()
	}
	v.mu.Unlock()

	// 2. Await Answer with timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case answer := <-v.answerChan:
		elapsed := time.Since(start)
		if answer.SDP == "" {
			err := fmt.Errorf("received empty SDP in answer")
			if v.metrics != nil {
				v.metrics.RecordHandshake(elapsed, false, err)
			}
			return elapsed, err
		}

		if v.metrics != nil {
			v.metrics.RecordHandshake(elapsed, true, nil)
		}
		return elapsed, nil

	case <-timer.C:
		elapsed := time.Since(start)
		err := fmt.Errorf("handshake timeout after %v for session %s", timeout, v.SessionID)
		if v.metrics != nil {
			v.metrics.RecordHandshake(elapsed, false, err)
		}
		return elapsed, err

	case <-v.closeChan:
		elapsed := time.Since(start)
		err := fmt.Errorf("viewer closed during handshake")
		if v.metrics != nil {
			v.metrics.RecordHandshake(elapsed, false, err)
		}
		return elapsed, err
	}
}

// Close terminates the mock viewer WebSocket connection.
func (v *MockViewer) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.closed {
		return nil
	}
	v.closed = true
	var err error
	if v.ws != nil {
		err = v.ws.Close()
	}
	select {
	case <-v.closeChan:
	default:
		close(v.closeChan)
	}
	return err
}
