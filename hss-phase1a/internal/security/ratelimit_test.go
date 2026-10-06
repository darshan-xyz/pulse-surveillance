package security

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPRateLimiter_AllowAndReject(t *testing.T) {
	// Create limiter with 2 tokens/sec, burst 3
	limiter := NewIPRateLimiter(rate.Limit(2), 3)
	defer limiter.Close()

	ip := "192.168.1.100"

	// First 3 requests should be allowed (burst)
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ip) {
			t.Fatalf("Request %d should have been allowed within burst", i+1)
		}
	}

	// 4th immediate request should be rejected
	if limiter.Allow(ip) {
		t.Fatal("4th immediate request should have been rejected (exceeded burst)")
	}

	// Different IP should still be allowed
	otherIP := "10.0.0.1"
	if !limiter.Allow(otherIP) {
		t.Fatal("Separate IP should have its own token bucket")
	}

	// Wait for refill (550ms for 1 token at 2/s)
	time.Sleep(550 * time.Millisecond)
	if !limiter.Allow(ip) {
		t.Fatal("Request should be allowed after token bucket refilled")
	}
}

func TestRateLimitMiddleware_HTTP429(t *testing.T) {
	// Limiter with burst 2
	limiter := NewIPRateLimiter(rate.Limit(1), 2)
	defer limiter.Close()

	handler := RateLimitMiddleware(limiter, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Request 1: OK
	req1 := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req1.RemoteAddr = "203.0.113.1:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got: %d", rr1.Code)
	}

	// Request 2: OK (burst 2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req2.RemoteAddr = "203.0.113.1:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected HTTP 200, got: %d", rr2.Code)
	}

	// Request 3: HTTP 429 Too Many Requests
	req3 := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	req3.RemoteAddr = "203.0.113.1:12345"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected HTTP 429 Too Many Requests, got: %d", rr3.Code)
	}

	if retryAfter := rr3.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("Expected Retry-After header on HTTP 429 response")
	}

	var errResp map[string]string
	if err := json.NewDecoder(rr3.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode JSON error response: %v", err)
	}
	if errResp["error"] != "Too Many Requests" {
		t.Fatalf("Unexpected error message: %v", errResp)
	}

	t.Log("PASS: Rate limiting middleware correctly returns HTTP 429 on flood")
}

func TestWSMessageLimiter(t *testing.T) {
	wsLimiter := NewWSMessageLimiter(rate.Limit(10), 5)

	// Burst 5 allowed
	for i := 0; i < 5; i++ {
		if !wsLimiter.Allow() {
			t.Fatalf("WS message %d should have been allowed within burst", i+1)
		}
	}

	// 6th rejected
	if wsLimiter.Allow() {
		t.Fatal("WS message 6 should have been rejected")
	}
}
