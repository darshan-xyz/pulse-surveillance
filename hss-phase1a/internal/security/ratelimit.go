package security

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Default limits
const (
	DefaultLoginRateLimit = 10.0 // 10 requests per second
	DefaultLoginBurst     = 15   // burst up to 15 requests
	DefaultWSRateLimit    = 60.0 // 60 messages per second
	DefaultWSBurst        = 100  // burst up to 100 messages
	MaxWSPayloadSize      = 64 * 1024 // 64 KB strict read limit
)

// clientLimiter holds a rate limiter and its last accessed timestamp
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// IPRateLimiter manages per-client token-bucket rate limiters
type IPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*clientLimiter
	rate     rate.Limit
	burst    int
	stopChan chan struct{}
}

// NewIPRateLimiter creates a new rate limiter with the specified rate and burst
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	limiter := &IPRateLimiter{
		limiters: make(map[string]*clientLimiter),
		rate:     r,
		burst:    b,
		stopChan: make(chan struct{}),
	}

	// Clean up stale limiters periodically
	go limiter.cleanupLoop(5 * time.Minute)

	return limiter
}

func (i *IPRateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.mu.Lock()
			now := time.Now()
			for ip, client := range i.limiters {
				if now.Sub(client.lastSeen) > 15*time.Minute {
					delete(i.limiters, ip)
				}
			}
			i.mu.Unlock()
		case <-i.stopChan:
			return
		}
	}
}

// Close stops the background cleanup worker
func (i *IPRateLimiter) Close() {
	select {
	case <-i.stopChan:
	default:
		close(i.stopChan)
	}
}

// GetLimiter returns the rate limiter for a specific key, creating one if needed
func (i *IPRateLimiter) GetLimiter(key string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	client, exists := i.limiters[key]
	if !exists {
		client = &clientLimiter{
			limiter:  rate.NewLimiter(i.rate, i.burst),
			lastSeen: time.Now(),
		}
		i.limiters[key] = client
		return client.limiter
	}

	client.lastSeen = time.Now()
	return client.limiter
}

// Allow reports whether an event may happen now for the given key
func (i *IPRateLimiter) Allow(key string) bool {
	return i.GetLimiter(key).Allow()
}

// ExtractIP extracts the client IP address from an http.Request
func ExtractIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	// Fallback to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimitMiddleware wraps an http.HandlerFunc with rate limiting logic
func RateLimitMiddleware(limiter *IPRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		if limiter != nil {
			ip := ExtractIP(r)
			if !limiter.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "Too Many Requests",
					"message": "Rate limit exceeded. Please retry later.",
				})
				return
			}
		}

		next(w, r)
	}
}

// WSMessageLimiter provides rate limiting for individual WebSocket connections
type WSMessageLimiter struct {
	limiter *rate.Limiter
}

// NewWSMessageLimiter creates a rate limiter for a WebSocket connection
func NewWSMessageLimiter(r rate.Limit, b int) *WSMessageLimiter {
	if r <= 0 {
		r = rate.Limit(DefaultWSRateLimit)
	}
	if b <= 0 {
		b = DefaultWSBurst
	}
	return &WSMessageLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

// Allow reports whether a WebSocket message may be processed
func (w *WSMessageLimiter) Allow() bool {
	return w.limiter.Allow()
}

// Global default limiters
var (
	DefaultLoginLimiter = NewIPRateLimiter(rate.Limit(DefaultLoginRateLimit), DefaultLoginBurst)
)
