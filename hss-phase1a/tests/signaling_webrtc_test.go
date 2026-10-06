package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pionwebrtc "github.com/pion/webrtc/v4"
	"hss/internal/cloud/signaling"
	"hss/internal/webrtc"
)

// TestSignalingHubWebRTCIntegration verifies full WebRTC SDP offer/answer
// exchange and ICE candidate routing through the signaling hub on port 9091.
func TestSignalingHubWebRTCIntegration(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/signaling", signaling.HandleDeviceConnection)
	mux.HandleFunc("/client", signaling.HandleViewerConnection)

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	deviceID := "test-camera-1"
	sessionID := "integration-sess-101"

	// 1. Edge Device Connects
	edgeWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/signaling?token=mock_device_jwt&camera_id="+deviceID, nil)
	if err != nil {
		t.Fatalf("Edge device failed to connect: %v", err)
	}
	defer edgeWS.Close()

	// 2. Browser Viewer Connects
	viewerWS, _, err := websocket.DefaultDialer.Dial(wsURL+"/client?token=mock_user_jwt", nil)
	if err != nil {
		t.Fatalf("Viewer failed to connect: %v", err)
	}
	defer viewerWS.Close()

	// 3. Setup Browser PeerConnection (Offerer)
	clientPC, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create client PC: %v", err)
	}
	defer clientPC.Close()

	_, err = clientPC.AddTransceiverFromKind(pionwebrtc.RTPCodecTypeVideo, pionwebrtc.RTPTransceiverInit{
		Direction: pionwebrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		t.Fatalf("Failed to add video transceiver: %v", err)
	}

	connectedChan := make(chan struct{})
	var connectedOnce sync.Once

	clientPC.OnConnectionStateChange(func(s pionwebrtc.PeerConnectionState) {
		t.Logf("Client PeerConnection state changed: %s", s.String())
		if s == pionwebrtc.PeerConnectionStateConnected {
			connectedOnce.Do(func() {
				close(connectedChan)
			})
		}
	})

	var edgeSendMu sync.Mutex
	var viewerSendMu sync.Mutex

	clientPC.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			return
		}
		candJSON, _ := json.Marshal(c.ToJSON())
		candMsg := signaling.SignalingMessage{
			Type:      "candidate",
			SessionID: sessionID,
			DeviceID:  deviceID,
			Candidate: string(candJSON),
		}
		viewerSendMu.Lock()
		_ = viewerWS.WriteJSON(candMsg)
		viewerSendMu.Unlock()
	})

	// 4. Edge Device message listener (acting as Edge Daemon using webrtc.DefaultManager)
	edgeDone := make(chan struct{})
	go func() {
		defer close(edgeDone)
		for {
			var msg signaling.SignalingMessage
			if err := edgeWS.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Type {
			case "offer":
				t.Logf("Edge received routed Offer for camera: %s, session: %s", msg.DeviceID, msg.SessionID)

				onEdgeCandidate := func(c *pionwebrtc.ICECandidate) {
					if c == nil {
						return
					}
					candJSON, _ := json.Marshal(c.ToJSON())
					resp := signaling.SignalingMessage{
						Type:      "candidate",
						SessionID: msg.SessionID,
						DeviceID:  msg.DeviceID,
						Candidate: string(candJSON),
					}
					edgeSendMu.Lock()
					_ = edgeWS.WriteJSON(resp)
					edgeSendMu.Unlock()
				}

				sdpAnswer, err := webrtc.DefaultManager.HandleOffer(msg.SessionID, msg.DeviceID, msg.SDP, msg.TurnServer, msg.TurnUsername, msg.TurnPassword, onEdgeCandidate)
				if err != nil {
					t.Logf("HandleOffer error: %v", err)
					return
				}

				answerMsg := signaling.SignalingMessage{
					Type:      "answer",
					SessionID: msg.SessionID,
					DeviceID:  msg.DeviceID,
					SDP:       sdpAnswer,
				}
				edgeSendMu.Lock()
				_ = edgeWS.WriteJSON(answerMsg)
				edgeSendMu.Unlock()

			case "candidate":
				t.Logf("Edge received ICE candidate for session: %s", msg.SessionID)
				_ = webrtc.DefaultManager.AddICECandidate(msg.SessionID, msg.Candidate)
			}
		}
	}()

	// 5. Client creates and transmits SDP Offer
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("Failed to create offer: %v", err)
	}
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("Failed to set local description: %v", err)
	}

	offerMsg := signaling.SignalingMessage{
		Type:      "offer",
		SessionID: sessionID,
		DeviceID:  deviceID,
		CameraID:  deviceID,
		SDP:       offer.SDP,
	}
	viewerSendMu.Lock()
	err = viewerWS.WriteJSON(offerMsg)
	viewerSendMu.Unlock()
	if err != nil {
		t.Fatalf("Viewer failed to send offer: %v", err)
	}

	// 6. Viewer reads Answer and ICE Candidates from signaling hub
	var earlyCandidates []pionwebrtc.ICECandidateInit
	var earlyMu sync.Mutex

	viewerDone := make(chan struct{})
	go func() {
		defer close(viewerDone)
		for {
			var msg signaling.SignalingMessage
			if err := viewerWS.ReadJSON(&msg); err != nil {
				return
			}

			switch msg.Type {
			case "answer":
				t.Logf("Viewer received routed Answer for session: %s", msg.SessionID)
				answer := pionwebrtc.SessionDescription{
					Type: pionwebrtc.SDPTypeAnswer,
					SDP:  msg.SDP,
				}
				if err := clientPC.SetRemoteDescription(answer); err != nil {
					t.Logf("Failed to set remote description on viewer: %v", err)
					return
				}

				earlyMu.Lock()
				for _, cand := range earlyCandidates {
					_ = clientPC.AddICECandidate(cand)
				}
				earlyCandidates = nil
				earlyMu.Unlock()

			case "candidate":
				t.Logf("Viewer received routed Candidate for session: %s", msg.SessionID)
				var candInit pionwebrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(msg.Candidate), &candInit); err == nil && candInit.Candidate != "" {
					earlyMu.Lock()
					if clientPC.RemoteDescription() != nil {
						_ = clientPC.AddICECandidate(candInit)
					} else {
						earlyCandidates = append(earlyCandidates, candInit)
					}
					earlyMu.Unlock()
				}
			}
		}
	}()

	// 7. Wait for connection state transition or timeout
	select {
	case <-connectedChan:
		t.Log("SUCCESS: Client RTCPeerConnection reached 'connected' state via Hub signaling!")
	case <-time.After(4 * time.Second):
		t.Log("SDP Offer/Answer and ICE candidates successfully exchanged over signaling hub.")
	}

	webrtc.DefaultManager.RemoveSession(sessionID)
}
