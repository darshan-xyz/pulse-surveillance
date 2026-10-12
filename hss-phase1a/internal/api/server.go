package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"hss/internal/camera"
	"hss/internal/events"
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
	mux.HandleFunc("/api/events", HandleEvents)
	mux.HandleFunc("/api/audit", HandleAudit)
	mux.HandleFunc("/metrics", metrics.PrometheusHandler)
	
	// Serve static files for Browser Test Client
	fs := http.FileServer(http.Dir("web"))
	mux.Handle("/test/", http.StripPrefix("/test/", fs))
	
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
			log.Printf("Warning: API Server could not bind %s: %v. Running in embedded mode.", s.server.Addr, err)
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

// HandleEvents returns recent life-safety and surveillance events.
func HandleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var list []events.Event
	if events.DefaultEngine != nil {
		list = events.DefaultEngine.ListRecent(100)
	}
	if list == nil {
		list = []events.Event{}
	}
	_ = json.NewEncoder(w).Encode(list)
}

// HandleAudit returns the signed cryptographic audit log and its chain integrity status.
func HandleAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	valid := true
	var entries []security.AuditEntry
	if security.DefaultAuditLogger != nil {
		v, _ := security.DefaultAuditLogger.VerifyChain()
		valid = v
		entries = security.DefaultAuditLogger.ListRecent(100)
	}
	if entries == nil {
		entries = []security.AuditEntry{}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"chain_valid": valid,
		"count":       len(entries),
		"entries":     entries,
	})
}
