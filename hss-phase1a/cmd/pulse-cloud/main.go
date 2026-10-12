package main

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"hss/internal/api"
	"hss/internal/cloud/cache"
	"hss/internal/cloud/db"
	"hss/internal/cloud/signaling"
	"hss/internal/security"
)

func main() {
	// Configure structured JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting Pulse Cloud Backend...")

	// 1. Initialize PostgreSQL
	dbDSN := os.Getenv("DATABASE_URL")
	if dbDSN == "" {
		dbDSN = "postgres://postgres:password@localhost:5432/pulse?sslmode=disable&connect_timeout=2"
		slog.Warn("DATABASE_URL not set, using default dev string")
	}
	
	if err := db.InitDB(dbDSN); err != nil {
		slog.Warn("Failed to connect to DB, continuing in degraded mode for testing", "error", err)
	}

	// 2. Initialize Valkey (Redis)
	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if err := cache.InitValkey(redisAddr); err != nil {
		slog.Warn("Failed to connect to Valkey, continuing in degraded mode for testing", "error", err)
	}

	// 3. Register HTTP Routes with Rate Limiting and Security Controls
	loginHandler := security.RateLimitMiddleware(security.DefaultLoginLimiter, api.HandleLogin)
	http.HandleFunc("/api/login", loginHandler)
	http.HandleFunc("/api/auth/login", loginHandler)
	http.HandleFunc("/api/auth/refresh", api.HandleRefresh)
	http.HandleFunc("/api/auth/revoke", api.HandleRevoke)
	http.HandleFunc("/api/turn/credentials", api.HandleTurnCredentials)
	http.HandleFunc("/api/devices", api.HandleDevices)
	http.HandleFunc("/api/push/register", api.HandlePushRegister)
	http.HandleFunc("/api/events", api.HandleEvents)
	http.HandleFunc("/api/audit", api.HandleAudit)

	wsLimiter := security.NewIPRateLimiter(security.DefaultWSRateLimit, security.DefaultWSBurst)
	http.HandleFunc("/signaling", security.RateLimitMiddleware(wsLimiter, signaling.HandleDeviceConnection))
	http.HandleFunc("/client", security.RateLimitMiddleware(wsLimiter, signaling.HandleViewerConnection))

	// 4. Reverse Proxy for YOLO AI Detection Endpoints with Full CORS Support
	aiTarget, err := url.Parse("http://127.0.0.1:8000")
	if err == nil {
		aiProxy := httputil.NewSingleHostReverseProxy(aiTarget)
		corsHandler := func(targetPath string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
				r.Host = aiTarget.Host
				r.URL.Path = targetPath
				aiProxy.ServeHTTP(w, r)
			}
		}
		http.HandleFunc("/api/detect", corsHandler("/detect"))
		http.HandleFunc("/api/detect_raw", corsHandler("/detect_raw"))
	}

	// Pairing request / verification
	handlePair := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write([]byte(`{"status": "ok", "pairing_code": "849201"}`))
	}
	http.HandleFunc("/api/pair", handlePair)
	http.HandleFunc("/v1/device/pair/request", handlePair)

	// Prometheus Metrics Endpoint (Stub for scraping)
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`# HELP pulse_active_websockets The total number of active websocket connections
# TYPE pulse_active_websockets gauge
pulse_active_websockets 0
`))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	caFile := os.Getenv("TLS_CA_FILE")
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	enableMTLS := os.Getenv("ENABLE_MTLS") == "true" || (caFile != "" && certFile != "" && keyFile != "")

	if enableMTLS && caFile != "" && certFile != "" && keyFile != "" {
		caPEM, certPEM, keyPEM, err := security.LoadMTLSCredentialsFromFiles(caFile, certFile, keyFile)
		if err != nil {
			slog.Error("Failed to load mTLS files", "error", err)
			os.Exit(1)
		}
		tlsConfig, err := security.NewServerMTLSConfig(caPEM, certPEM, keyPEM)
		if err != nil {
			slog.Error("Failed to create server mTLS config", "error", err)
			os.Exit(1)
		}

		server := &http.Server{
			Addr:      ":" + port,
			TLSConfig: tlsConfig,
		}
		slog.Info("Pulse Cloud Node listening with mTLS", "port", port)
		if err := server.ListenAndServeTLS("", ""); err != nil {
			slog.Error("mTLS Server failed", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Pulse Cloud Node listening", "port", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}
}
