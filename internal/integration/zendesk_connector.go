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

// ZendeskConnector implements Zendesk support integration
type ZendeskConnector struct {
	config      *ZendeskConfig
	credentials *Credentials
	vault       CredentialVault
	rateLimiter RateLimiter
	auditor     AuditLogger
	httpClient  *http.Client
	connected   bool
	mu          sync.RWMutex
}

// ZendeskConfig holds Zendesk-specific configuration
type ZendeskConfig struct {
	Enabled   bool
	Subdomain string // e.g., "yourcompany" for yourcompany.zendesk.com
	Email     string // For basic auth as fallback
	APIToken  string
	OAuth2    *OAuth2Config
}

// NewZendeskConnector creates a new Zendesk connector
func NewZendeskConnector(
	config *ZendeskConfig,
	vault CredentialVault,
	rateLimiter RateLimiter,
	auditor AuditLogger,
) *ZendeskConnector {
	return &ZendeskConnector{
		config:      config,
		vault:       vault,
		rateLimiter: rateLimiter,
		auditor:     auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (z *ZendeskConnector) Name() string      { return "zendesk" }
func (z *ZendeskConnector) Type() ServiceType { return ServiceTypeZendesk }

func (z *ZendeskConnector) Connect(ctx context.Context) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	creds, err := z.vault.Retrieve(ctx, z.Name())
	if err != nil {
		// Try API token as fallback
		if z.config.APIToken != "" {
			creds = &Credentials{
				ServiceType: ServiceTypeZendesk,
				AccessToken: z.config.APIToken,
				Metadata:    map[string]string{"email": z.config.Email},
			}
		} else {
			return fmt.Errorf("failed to retrieve credentials: %w", err)
		}
	}

	z.credentials = creds
	z.connected = true
	return nil
}

func (z *ZendeskConnector) Disconnect() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.connected = false
	return nil
}

func (z *ZendeskConnector) IsConnected() bool {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.connected
}

func (z *ZendeskConnector) GetRateLimits() *RateLimitStatus {
	return z.rateLimiter.GetStatus(z.Name())
}

// GetTicket retrieves a ticket by ID
func (z *ZendeskConnector) GetTicket(ctx context.Context, id int64) (*Ticket, error) {
	endpoint := fmt.Sprintf("/api/v2/tickets/%d.json", id)

	var result struct {
		Ticket *Ticket `json:"ticket"`
	}

	if err := z.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result.Ticket, nil
}

// ListTickets retrieves tickets with pagination
func (z *ZendeskConnector) ListTickets(ctx context.Context, status string, page int) ([]*Ticket, error) {
	endpoint := fmt.Sprintf("/api/v2/tickets.json?status=%s&page=%d", status, page)

	var result struct {
		Tickets []*Ticket `json:"tickets"`
	}

	if err := z.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result.Tickets, nil
}

// CreateTicket creates a new support ticket
func (z *ZendeskConnector) CreateTicket(ctx context.Context, ticket *TicketCreate) (*Ticket, error) {
	endpoint := "/api/v2/tickets.json"

	payload := map[string]interface{}{
		"ticket": ticket,
	}

	var result struct {
		Ticket *Ticket `json:"ticket"`
	}

	if err := z.apiCall(ctx, "POST", endpoint, payload, &result); err != nil {
		return nil, err
	}

	return result.Ticket, nil
}

// UpdateTicket updates an existing ticket
func (z *ZendeskConnector) UpdateTicket(ctx context.Context, id int64, update *TicketUpdate) (*Ticket, error) {
	endpoint := fmt.Sprintf("/api/v2/tickets/%d.json", id)

	payload := map[string]interface{}{
		"ticket": update,
	}

	var result struct {
		Ticket *Ticket `json:"ticket"`
	}

	if err := z.apiCall(ctx, "PUT", endpoint, payload, &result); err != nil {
		return nil, err
	}

	return result.Ticket, nil
}

// SearchTickets searches tickets
func (z *ZendeskConnector) SearchTickets(ctx context.Context, query string) ([]*Ticket, error) {
	endpoint := fmt.Sprintf("/api/v2/search.json?query=type:ticket %s", query)

	var result struct {
		Results []*Ticket `json:"results"`
	}

	if err := z.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// GetUser retrieves a user by ID
func (z *ZendeskConnector) GetUser(ctx context.Context, id int64) (*User, error) {
	endpoint := fmt.Sprintf("/api/v2/users/%d.json", id)

	var result struct {
		User *User `json:"user"`
	}

	if err := z.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result.User, nil
}

func (z *ZendeskConnector) apiCall(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	startTime := time.Now()

	if err := z.rateLimiter.Wait(ctx, z.Name()); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	url := fmt.Sprintf("https://%s.zendesk.com%s", z.config.Subdomain, endpoint)

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

	// Use API token authentication
	if email, ok := z.credentials.Metadata["email"]; ok {
		tokenAuth := fmt.Sprintf("%s/token:%s", email, z.credentials.AccessToken)
		req.SetBasicAuth(tokenAuth, "")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := z.httpClient.Do(req)
	if err != nil {
		z.logAudit(ctx, method, endpoint, 0, time.Since(startTime), false, err.Error())
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		z.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, "HTTP error")
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			z.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, err.Error())
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	z.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), true, "")
	return nil
}

func (z *ZendeskConnector) logAudit(ctx context.Context, method, endpoint string, status int, duration time.Duration, success bool, errorMsg string) {
	if z.auditor == nil {
		return
	}

	entry := &AuditEntry{
		Timestamp:  time.Now(),
		Service:    ServiceTypeZendesk,
		Operation:  method + " " + endpoint,
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: status,
		Duration:   duration,
		Success:    success,
		Error:      errorMsg,
	}

	_ = z.auditor.Log(ctx, entry)
}

// Zendesk data models

type Ticket struct {
	ID          int64    `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // new, open, pending, hold, solved, closed
	Priority    string   `json:"priority"` // low, normal, high, urgent
	Type        string   `json:"type"` // problem, incident, question, task
	RequesterID int64    `json:"requester_id"`
	AssigneeID  int64    `json:"assignee_id,omitempty"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type TicketCreate struct {
	Subject     string                 `json:"subject"`
	Description string                 `json:"description,omitempty"`
	Comment     *Comment               `json:"comment,omitempty"`
	Priority    string                 `json:"priority,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

type TicketUpdate struct {
	Subject  string   `json:"subject,omitempty"`
	Comment  *Comment `json:"comment,omitempty"`
	Status   string   `json:"status,omitempty"`
	Priority string   `json:"priority,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Comment struct {
	Body   string `json:"body"`
	Public bool   `json:"public"`
}

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
	Phone string `json:"phone,omitempty"`
}
