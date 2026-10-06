package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"hss/internal/cloud/db"
)

// PushRegisterRequest represents the incoming push token registration payload
type PushRegisterRequest struct {
	PushToken string `json:"push_token"`
	Platform  string `json:"platform"`
}

// PushRegisterResponse represents the successful response
type PushRegisterResponse struct {
	Status string `json:"status"`
}

// HandlePushRegister handles POST /api/push/register
// Authenticates user via JWT and associates the device push token
func HandlePushRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
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

	_, claims, err := ParseAndVerifyToken(r.Context(), tokenString, "")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: invalid, expired, or revoked token"})
		return
	}

	var username string
	if subClaim, hasSub := claims["sub"]; hasSub {
		username, _ = subClaim.(string)
	} else if uClaim, hasUser := claims["username"]; hasUser {
		username, _ = uClaim.(string)
	}

	if username == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: missing subject in token"})
		return
	}

	var req PushRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bad Request: invalid JSON payload"})
		return
	}

	if strings.TrimSpace(req.PushToken) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bad Request: push_token is required"})
		return
	}

	if req.Platform == "" {
		req.Platform = "generic"
	}

	if err := db.RegisterPushToken(username, req.PushToken, req.Platform); err != nil {
		log.Printf("Failed to register push token for %s: %v", username, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Internal Server Error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(PushRegisterResponse{
		Status: "registered",
	})
}
