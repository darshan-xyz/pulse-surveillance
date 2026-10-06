package main

import (
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"hss/internal/cloud/db"
)

var (
	cloudWSURL    = flag.String("url", "ws://localhost:9091", "Cloud WebSocket URL")
	dbURL         = flag.String("db", "postgres://pulse_test:test_password@localhost:5433/pulse_test_db?sslmode=disable", "Postgres URL")
	numEdges      = flag.Int("edges", 10, "Number of mock edge daemons")
	numViewers    = flag.Int("viewers", 1000, "Number of concurrent mobile viewers")
	
	successCount int32
	failCount    int32
	latencies    []time.Duration
	latenciesMu  sync.Mutex
)

func main() {
	flag.Parse()
	log.Printf("Pulse Load Tester: %d Edges, %d Viewers", *numEdges, *numViewers)

	// 1. Setup DB
	if err := db.InitDB(*dbURL); err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}

	edgeTokens, deviceIDs := seedDatabase(*numEdges, *numViewers)

	// 2. Start Mock Edges
	var wgEdges sync.WaitGroup
	for i := 0; i < *numEdges; i++ {
		wgEdges.Add(1)
		go startMockEdge(edgeTokens[i], &wgEdges)
	}

	// Wait briefly for edges to connect and subscribe to Valkey
	time.Sleep(2 * time.Second)

	// 3. Bombard with Viewers
	log.Printf("Initiating orbital bombardment with %d concurrent WebRTC offers...", *numViewers)
	
	start := time.Now()
	var wgViewers sync.WaitGroup
	for i := 0; i < *numViewers; i++ {
		wgViewers.Add(1)
		targetDevice := deviceIDs[i%*numEdges] // Distribute load across edges
		viewerToken := fmt.Sprintf("stress_user_%d", i)
		go startMockViewer(viewerToken, targetDevice, &wgViewers)
	}

	wgViewers.Wait()
	duration := time.Since(start)

	// 4. Report Metrics
	log.Println("======================================")
	log.Println("Stress Test Complete!")
	log.Printf("Total Time: %v", duration)
	log.Printf("Successful Negotiations: %d", successCount)
	log.Printf("Failed Negotiations: %d", failCount)

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		
		p50 := latencies[len(latencies)*50/100]
		p95 := latencies[len(latencies)*95/100]
		p99 := latencies[len(latencies)*99/100]
		
		log.Printf("Average RTT: %v", totalLatency(latencies)/time.Duration(len(latencies)))
		log.Printf("P50 RTT: %v", p50)
		log.Printf("P95 RTT: %v", p95)
		log.Printf("P99 RTT: %v", p99)
	}
	log.Println("======================================")
}

func seedDatabase(numEdges, numViewers int) ([]string, []string) {
	log.Println("Seeding Database...")
	db.DB.Exec("TRUNCATE users, devices, permissions CASCADE")

	edgeTokens := make([]string, numEdges)
	deviceIDs := make([]string, numEdges)

	for i := 0; i < numEdges; i++ {
		deviceID := fmt.Sprintf("stress_cam_%d", i)
		deviceIDs[i] = deviceID

		pubKey, privKey, _ := ed25519.GenerateKey(nil)
		pubKeyPEM := fmt.Sprintf("-----BEGIN PUBLIC KEY-----\n%x\n-----END PUBLIC KEY-----", pubKey)

		db.DB.Exec("INSERT INTO devices (device_id, public_key) VALUES ($1, $2)", deviceID, pubKeyPEM)

		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
			"sub": deviceID,
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		edgeTokens[i], _ = token.SignedString(privKey)
	}

	// Create users and permissions
	for i := 0; i < numViewers; i++ {
		username := fmt.Sprintf("stress_user_%d", i)
		db.DB.Exec("INSERT INTO users (id, username, password_hash) VALUES ($1, $2, 'hash')", i+1, username)
		
		targetDevice := deviceIDs[i%numEdges]
		db.DB.Exec("INSERT INTO permissions (user_id, device_id, role) VALUES ($1, $2, 'owner')", i+1, targetDevice)
	}
	
	log.Println("Database Seeded!")
	return edgeTokens, deviceIDs
}

func startMockEdge(jwtToken string, wg *sync.WaitGroup) {
	defer wg.Done()

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+jwtToken)
	conn, _, err := websocket.DefaultDialer.Dial(*cloudWSURL+"/signaling", headers)
	if err != nil {
		log.Printf("Edge Connection Failed: %v", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var payload map[string]interface{}
		json.Unmarshal(msg, &payload)

		// Auto-reply to offers
		if payload["type"] == "offer" {
			answer := map[string]string{
				"type":       "answer",
				"session_id": payload["session_id"].(string),
				"sdp":        `{"type":"answer","sdp":"mock_answer_data"}`,
			}
			conn.WriteJSON(answer)
		}
	}
}

func startMockViewer(userToken, targetDevice string, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, _, err := websocket.DefaultDialer.Dial(*cloudWSURL+"/client?token="+userToken, nil)
	if err != nil {
		atomic.AddInt32(&failCount, 1)
		return
	}
	defer conn.Close()

	sessionID := fmt.Sprintf("sess_%s", userToken)
	offer := map[string]string{
		"type":       "offer",
		"device_id":  targetDevice,
		"session_id": sessionID,
		"sdp":        `{"type":"offer","sdp":"mock_offer_data"}`,
	}

	start := time.Now()
	conn.WriteJSON(offer)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg map[string]interface{}
	if err := conn.ReadJSON(&msg); err == nil && msg["type"] == "answer" {
		rtt := time.Since(start)
		atomic.AddInt32(&successCount, 1)
		
		latenciesMu.Lock()
		latencies = append(latencies, rtt)
		latenciesMu.Unlock()
	} else {
		atomic.AddInt32(&failCount, 1)
	}
}

func totalLatency(lats []time.Duration) time.Duration {
	var total time.Duration
	for _, l := range lats {
		total += l
	}
	return total
}
