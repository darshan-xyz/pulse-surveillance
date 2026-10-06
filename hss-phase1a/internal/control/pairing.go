package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"hss/internal/config"
	"hss/internal/security"
)

// StartPairingRoutine checks if the device is paired; if not, requests a pairing code.
func StartPairingRoutine(ctx context.Context) {
	apiURL := "http://localhost:9091/v1/device/pair/request"
	if config.AppConfig != nil && config.AppConfig.Control.Endpoint != "" {
		ep := config.AppConfig.Control.Endpoint
		ep = strings.Replace(ep, "ws://", "http://", 1)
		ep = strings.Replace(ep, "wss://", "https://", 1)
		if idx := strings.Index(ep, "/signaling"); idx != -1 {
			ep = ep[:idx]
		}
		apiURL = ep + "/v1/device/pair/request"
	}

	reqBody, _ := json.Marshal(map[string]string{
		"device_id": security.DeviceID,
	})

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial request
	requestPairingCode(apiURL, reqBody)

	for {
		select {
		case <-ctx.Done():
			log.Println("Pairing routine shutting down.")
			return
		case <-ticker.C:
			requestPairingCode(apiURL, reqBody)
		}
	}
}

func requestPairingCode(apiURL string, reqBody []byte) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		log.Printf("[HSS-Control] Central API not reachable yet at %s (%v); will retry.", apiURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var res struct {
			PairingCode string `json:"pairing_code"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.PairingCode != "" {
			fmt.Println("\n===========================================================")
			fmt.Printf(" ACTION REQUIRED: Device is Unpaired\n")
			fmt.Printf(" Please enter this 6-digit code in your Web Dashboard: %s\n", res.PairingCode)
			fmt.Println("===========================================================")
		}
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[HSS-Control] Pairing code request status %d: %s\n", resp.StatusCode, string(body))
	}
}
