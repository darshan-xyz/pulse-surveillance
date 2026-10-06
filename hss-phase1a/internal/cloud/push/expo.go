package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const DefaultExpoPushEndpoint = "https://exp.host/--/api/v2/push/send"

// ExpoPushMessage represents an Expo push notification request payload
type ExpoPushMessage struct {
	To        string                 `json:"to"`
	Title     string                 `json:"title"`
	Body      string                 `json:"body"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Sound     string                 `json:"sound,omitempty"`
	Priority  string                 `json:"priority,omitempty"`
	ChannelID string                 `json:"channelId,omitempty"`
	Badge     int                    `json:"badge,omitempty"`
}

// ExpoPushTicket represents the receipt ticket returned by Expo API
type ExpoPushTicket struct {
	Status  string                 `json:"status"` // "ok" or "error"
	ID      string                 `json:"id,omitempty"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ExpoPushResponse represents the top-level API response from Expo
type ExpoPushResponse struct {
	Data   []ExpoPushTicket       `json:"data"`
	Errors []interface{}          `json:"errors,omitempty"`
}

// Client defines the interface for push notification services
type Client interface {
	SendNotification(ctx context.Context, msg ExpoPushMessage) error
	SendBatch(ctx context.Context, msgs []ExpoPushMessage) ([]ExpoPushTicket, error)
	GetEndpoint() string
}

// ExpoPushService implements Client using Expo's HTTP push API
type ExpoPushService struct {
	client   *http.Client
	endpoint string
	mu       sync.RWMutex
}

// NewExpoPushService creates a new Expo push service with optional custom endpoint
func NewExpoPushService(endpoint ...string) *ExpoPushService {
	url := DefaultExpoPushEndpoint
	if len(endpoint) > 0 && endpoint[0] != "" {
		url = endpoint[0]
	}
	return &ExpoPushService{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		endpoint: url,
	}
}

// DefaultService is the global push service singleton
var (
	defaultServiceMu sync.RWMutex
	DefaultService   Client = NewExpoPushService()
)

// SetDefaultService allows swapping the global push client (e.g. for testing)
func SetDefaultService(svc Client) {
	defaultServiceMu.Lock()
	defer defaultServiceMu.Unlock()
	DefaultService = svc
}

// GetDefaultService safely gets the current default push service
func GetDefaultService() Client {
	defaultServiceMu.RLock()
	defer defaultServiceMu.RUnlock()
	return DefaultService
}

// GetEndpoint returns the configured API endpoint
func (s *ExpoPushService) GetEndpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpoint
}

// SetEndpoint allows modifying endpoint dynamically
func (s *ExpoPushService) SetEndpoint(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpoint = url
}

// SendNotification sends a single push notification via Expo Push API
func (s *ExpoPushService) SendNotification(ctx context.Context, msg ExpoPushMessage) error {
	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("recipient push token is required")
	}

	tickets, err := s.SendBatch(ctx, []ExpoPushMessage{msg})
	if err != nil {
		return err
	}

	if len(tickets) > 0 && tickets[0].Status == "error" {
		errMsg := tickets[0].Message
		if errMsg == "" {
			errMsg = fmt.Sprintf("push ticket error: %+v", tickets[0].Details)
		}
		return fmt.Errorf("expo push delivery error: %s", errMsg)
	}

	return nil
}

// SendBatch sends a batch of push notifications to Expo Push API
func (s *ExpoPushService) SendBatch(ctx context.Context, msgs []ExpoPushMessage) ([]ExpoPushTicket, error) {
	if len(msgs) == 0 {
		return nil, nil
	}

	payload, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal push payload: %w", err)
	}

	s.mu.RLock()
	endpoint := s.endpoint
	client := s.client
	s.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("expo push request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read expo push response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("expo push API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var expoResp ExpoPushResponse
	if err := json.Unmarshal(bodyBytes, &expoResp); err != nil {
		return nil, fmt.Errorf("failed to decode expo push response (%s): %w", string(bodyBytes), err)
	}

	if len(expoResp.Errors) > 0 {
		log.Printf("[ExpoPush] Warning: API returned top-level errors: %+v", expoResp.Errors)
	}

	return expoResp.Data, nil
}
