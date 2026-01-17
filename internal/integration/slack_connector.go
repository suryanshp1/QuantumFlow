package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SlackConnector implements Slack API integration
type SlackConnector struct {
	config      *SlackConfig
	credentials *Credentials
	vault       CredentialVault
	rateLimiter RateLimiter
	auditor     AuditLogger
	httpClient  *http.Client
	connected   bool
	mu          sync.RWMutex
}

// NewSlackConnector creates a new Slack connector
func NewSlackConnector(
	config *SlackConfig,
	vault CredentialVault,
	rateLimiter RateLimiter,
	auditor AuditLogger,
) *SlackConnector {
	return &SlackConnector{
		config:      config,
		vault:       vault,
		rateLimiter: rateLimiter,
		auditor:     auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *SlackConnector) Name() string          { return "slack" }
func (s *SlackConnector) Type() ServiceType     { return ServiceTypeSlack }

func (s *SlackConnector) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds, err := s.vault.Retrieve(ctx, s.Name())
	if err != nil {
		// Try bot token as fallback
		if s.config.BotToken != "" {
			creds = &Credentials{
				ServiceType: ServiceTypeSlack,
				AccessToken: s.config.BotToken,
				TokenType:   "bot",
			}
		} else {
			return fmt.Errorf("failed to retrieve credentials: %w", err)
		}
	}

	s.credentials = creds
	s.connected = true
	return nil
}

func (s *SlackConnector) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	return nil
}

func (s *SlackConnector) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

func (s *SlackConnector) GetRateLimits() *RateLimitStatus {
	return s.rateLimiter.GetStatus(s.Name())
}

// PostMessage posts a message to a channel
func (s *SlackConnector) PostMessage(ctx context.Context, channel, text string) (*Message, error) {
	payload := map[string]interface{}{
		"channel": channel,
		"text":    text,
	}

	var result struct {
		OK      bool    `json:"ok"`
		Message Message `json:"message"`
		Error   string  `json:"error,omitempty"`
	}

	if err := s.apiCall(ctx, "POST", "/chat.postMessage", payload, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return &result.Message, nil
}

// GetChannelHistory retrieves channel message history
func (s *SlackConnector) GetChannelHistory(ctx context.Context, channel string, limit int) ([]*Message, error) {
	endpoint := fmt.Sprintf("/conversations.history?channel=%s&limit=%d", channel, limit)

	var result struct {
		OK       bool       `json:"ok"`
		Messages []*Message `json:"messages"`
		Error    string     `json:"error,omitempty"`
	}

	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.Messages, nil
}

// SearchMessages searches for messages containing query
func (s *SlackConnector) SearchMessages(ctx context.Context, query string) ([]*Message, error) {
	endpoint := fmt.Sprintf("/search.messages?query=%s", query)

	var result struct {
		OK       bool `json:"ok"`
		Messages struct {
			Matches []*Message `json:"matches"`
		} `json:"messages"`
		Error string `json:"error,omitempty"`
	}

	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.Messages.Matches, nil
}

// ListChannels lists all channels
func (s *SlackConnector) ListChannels(ctx context.Context) ([]*Channel, error) {
	var result struct {
		OK       bool       `json:"ok"`
		Channels []*Channel `json:"channels"`
		Error    string     `json:"error,omitempty"`
	}

	if err := s.apiCall(ctx, "GET", "/conversations.list", nil, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.Channels, nil
}

// apiCall makes an authenticated API call to Slack
func (s *SlackConnector) apiCall(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	startTime := time.Now()

	// Check rate limit
	if err := s.rateLimiter.Wait(ctx, s.Name()); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	url := "https://slack.com/api" + endpoint

	var req *http.Request
	var err error

	if body != nil {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal body: %w", err)
		}
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(bodyJSON)))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}

	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.credentials.AccessToken))
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logAudit(ctx, method, endpoint, 0, time.Since(startTime), false, err.Error())
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			s.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, err.Error())
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	s.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), success, "")

	return nil
}

func (s *SlackConnector) logAudit(ctx context.Context, method, endpoint string, status int, duration time.Duration, success bool, errorMsg string) {
	if s.auditor == nil {
		return
	}

	entry := &AuditEntry{
		Timestamp:  time.Now(),
		Service:    ServiceTypeSlack,
		Operation:  method + " " + endpoint,
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: status,
		Duration:   duration,
		Success:    success,
		Error:      errorMsg,
	}

	_ = s.auditor.Log(ctx, entry)
}

// Slack data models

type Message struct {
	Type      string `json:"type"`
	User      string `json:"user"`
	Text      string `json:"text"`
	Timestamp string `json:"ts"`
	Channel   string `json:"channel,omitempty"`
}

type Channel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsMember bool   `json:"is_member"`
	IsPrivate bool  `json:"is_private"`
}
