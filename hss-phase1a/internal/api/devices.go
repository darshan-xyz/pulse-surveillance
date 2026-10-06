package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"hss/internal/cloud/db"
)

type DeviceResponse struct {
	DeviceID string `json:"device_id"`
	Role     string `json:"role"`
}

// HandleDevices returns the list of devices the logged-in user is allowed to view,
// strictly enforcing tenant isolation without leakage.
func HandleDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method Not Allowed"})
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: missing or invalid Authorization header"})
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Verify User JWT using centralized validation with blacklist check
	_, claims, err := ParseAndVerifyToken(r.Context(), tokenString, "")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: invalid, expired, or revoked token"})
		return
	}

	subClaim, hasSub := claims["sub"]
	if !hasSub {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: missing subject in token"})
		return
	}
	username, _ := subClaim.(string)

	var devices []DeviceResponse

	if !db.IsConnected {
		// Mock Multi-Tenant Devices for Dev/Standalone Mode
		switch username {
		case "admin":
			devices = []DeviceResponse{
				{DeviceID: "test-camera-1", Role: "admin"},
				{DeviceID: "webcam-live-01", Role: "admin"},
			}
		case "user2":
			devices = []DeviceResponse{{DeviceID: "test-camera-2", Role: "viewer"}}
		case "tenant1_user":
			devices = []DeviceResponse{
				{DeviceID: "cam-tenant1-01", Role: "admin"},
				{DeviceID: "cam-tenant1-02", Role: "viewer"},
			}
		case "tenant1_viewer":
			devices = []DeviceResponse{
				{DeviceID: "cam-tenant1-01", Role: "viewer"},
				{DeviceID: "cam-tenant1-02", Role: "viewer"},
			}
		case "tenant2_user":
			devices = []DeviceResponse{
				{DeviceID: "cam-tenant2-01", Role: "admin"},
			}
		default:
			devices = make([]DeviceResponse, 0)
		}
	} else {
		// Fetch authorized devices strictly scoped to this user from PostgreSQL
		rows, err := db.DB.Query(`
			SELECT p.device_id, p.role 
			FROM permissions p
			JOIN users u ON p.user_id = u.id
			WHERE u.username = $1
		`, username)

		if err != nil {
			log.Printf("DB error fetching devices for %s: %v", username, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Internal Server Error"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var d DeviceResponse
			if err := rows.Scan(&d.DeviceID, &d.Role); err == nil {
				devices = append(devices, d)
			}
		}
	}

	// Ensure we always return an array, even if empty (never null)
	if devices == nil {
		devices = make([]DeviceResponse, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

