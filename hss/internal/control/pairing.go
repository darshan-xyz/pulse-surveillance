package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"hss/internal/config"
	"hss/internal/security"
)

// StartPairingRoutine checks if the device is paired; if not, requests a pairing code.
func StartPairingRoutine(ctx context.Context) {
	// In a real system, we'd check if we already have a pairing token saved.
	// For MVP, we simply request a code on boot and print it.
	
	// Assuming the API is running on config.AppConfig.Control.Endpoint but using http instead of ws
	apiURL := "http://localhost:9090/v1/device/pair/request"
	
	reqBody, _ := json.Marshal(map[string]string{
		"device_id": security.DeviceID,
	})

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		log.Println("Requesting Pairing Code from Central API...")
		
		resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			log.Printf("Failed to request pairing code: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var res struct {
					PairingCode string `json:"pairing_code"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
					// Use specific log format that stands out for the user
					fmt.Println("\n===========================================================")
					fmt.Printf(" ACTION REQUIRED: Device is Unpaired\n")
					fmt.Printf(" Please enter this 6-digit code in your Web Dashboard: %s\n", res.PairingCode)
					fmt.Println("===========================================================\n")
				}
			} else {
				body, _ := io.ReadAll(resp.Body)
				log.Printf("Failed to request pairing code. Status: %d, Body: %s\n", resp.StatusCode, string(body))
			}
		}

		select {
		case <-ctx.Done():
			log.Println("Pairing routine shutting down.")
			return
		case <-ticker.C:
			// Loop and request a new code every 60 seconds until paired
			// In MVP, we just loop for demo purposes.
		}
	}
}
