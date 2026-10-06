package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"hss/internal/camera"
	"hss/internal/metrics"
	"hss/internal/security"
	"hss/internal/webrtc"
)

type Server struct {
	server *http.Server
}

func NewServer(port int) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/cameras", camerasHandler)
	mux.HandleFunc("/api/pairing", pairingHandler)
	mux.HandleFunc("/metrics", metrics.PrometheusHandler)
	
	// WebRTC Signaling WebSocket
	mux.HandleFunc("/ws", webrtc.DefaultManager.HandleSignaling)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return &Server{
		server: srv,
	}
}

func (s *Server) Start(ctx context.Context) {
	go func() {
		log.Printf("Starting local API server on %s...\n", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API Server Error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down API server...")
	
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		log.Printf("API Server shutdown error: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "online",
		"device_id": security.DeviceID,
		"version":   "1.0.0",
	})
}

func camerasHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cameras, err := camera.GetCameras()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cameras)
}

func pairingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device_id":    security.DeviceID,
		"public_key":   security.PublicKeyPEM,
		"pairing_code": "1234-5678", // Placeholder
	})
}
