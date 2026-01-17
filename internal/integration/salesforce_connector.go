package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SalesforceConnector implements Salesforce CRM integration
type SalesforceConnector struct {
	config       *SalesforceConfig
	credentials  *Credentials
	vault        CredentialVault
	rateLimiter  RateLimiter
	auditor      AuditLogger
	httpClient   *http.Client
	instanceURL  string
	connected    bool
	mu           sync.RWMutex
}

// SalesforceConfig holds Salesforce-specific configuration
type SalesforceConfig struct {
	Enabled      bool
	OAuth2       *OAuth2Config
	InstanceURL  string // e.g., https://yourinstance.salesforce.com
	APIVersion   string // e.g., "v59.0"
	IsSandbox    bool
}

// NewSalesforceConnector creates a new Salesforce connector
func NewSalesforceConnector(
	config *SalesforceConfig,
	vault CredentialVault,
	rateLimiter RateLimiter,
	auditor AuditLogger,
) *SalesforceConnector {
	if config.APIVersion == "" {
		config.APIVersion = "v59.0"
	}

	return &SalesforceConnector{
		config:      config,
		vault:       vault,
		rateLimiter: rateLimiter,
		auditor:     auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *SalesforceConnector) Name() string      { return "salesforce" }
func (s *SalesforceConnector) Type() ServiceType { return ServiceTypeSalesforce }

func (s *SalesforceConnector) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	creds, err := s.vault.Retrieve(ctx, s.Name())
	if err != nil {
		return fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	s.credentials = creds
	s.instanceURL = s.config.InstanceURL
	s.connected = true

	return nil
}

func (s *SalesforceConnector) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = false
	return nil
}

func (s *SalesforceConnector) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

func (s *SalesforceConnector) GetRateLimits() *RateLimitStatus {
	return s.rateLimiter.GetStatus(s.Name())
}

// Query executes a SOQL query
func (s *SalesforceConnector) Query(ctx context.Context, soql string) (*QueryResult, error) {
	endpoint := fmt.Sprintf("/services/data/%s/query?q=%s", s.config.APIVersion, url.QueryEscape(soql))

	var result QueryResult
	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetObject retrieves a Salesforce object by ID
func (s *SalesforceConnector) GetObject(ctx context.Context, objectType, id string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/services/data/%s/sobjects/%s/%s", s.config.APIVersion, objectType, id)

	var result map[string]interface{}
	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateObject creates a new Salesforce object
func (s *SalesforceConnector) CreateObject(ctx context.Context, objectType string, data map[string]interface{}) (string, error) {
	endpoint := fmt.Sprintf("/services/data/%s/sobjects/%s", s.config.APIVersion, objectType)

	var result struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := s.apiCall(ctx, "POST", endpoint, data, &result); err != nil {
		return "", err
	}

	if !result.Success {
		if len(result.Errors) > 0 {
			return "", fmt.Errorf("salesforce error: %s", result.Errors[0].Message)
		}
		return "", fmt.Errorf("unknown salesforce error")
	}

	return result.ID, nil
}

// UpdateObject updates an existing Salesforce object
func (s *SalesforceConnector) UpdateObject(ctx context.Context, objectType, id string, data map[string]interface{}) error {
	endpoint := fmt.Sprintf("/services/data/%s/sobjects/%s/%s", s.config.APIVersion, objectType, id)
	return s.apiCall(ctx, "PATCH", endpoint, data, nil)
}

// DescribeObject retrieves object metadata/schema
func (s *SalesforceConnector) DescribeObject(ctx context.Context, objectType string) (*ObjectMetadata, error) {
	endpoint := fmt.Sprintf("/services/data/%s/sobjects/%s/describe", s.config.APIVersion, objectType)

	var result ObjectMetadata
	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SearchRecords performs a SOSL search
func (s *SalesforceConnector) SearchRecords(ctx context.Context, searchString string) ([]map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/services/data/%s/search?q=%s", s.config.APIVersion, url.QueryEscape(searchString))

	var result []map[string]interface{}
	if err := s.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *SalesforceConnector) apiCall(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	startTime := time.Now()

	if err := s.rateLimiter.Wait(ctx, s.Name()); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	url := s.instanceURL + endpoint

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

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.credentials.AccessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logAudit(ctx, method, endpoint, 0, time.Since(startTime), false, err.Error())
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		s.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, "HTTP error")
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			s.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, err.Error())
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	s.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), true, "")
	return nil
}

func (s *SalesforceConnector) logAudit(ctx context.Context, method, endpoint string, status int, duration time.Duration, success bool, errorMsg string) {
	if s.auditor == nil {
		return
	}

	entry := &AuditEntry{
		Timestamp:  time.Now(),
		Service:    ServiceTypeSalesforce,
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

// Salesforce data models

type QueryResult struct {
	TotalSize      int                      `json:"totalSize"`
	Done           bool                     `json:"done"`
	Records        []map[string]interface{} `json:"records"`
	NextRecordsURL string                   `json:"nextRecordsUrl,omitempty"`
}

type ObjectMetadata struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Fields     []Field  `json:"fields"`
	Queryable  bool     `json:"queryable"`
	Searchable bool     `json:"searchable"`
	Updateable bool     `json:"updateable"`
}

type Field struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Length       int    `json:"length"`
	Updateable   bool   `json:"updateable"`
	Createable   bool   `json:"createable"`
	Nillable     bool   `json:"nillable"`
	Unique       bool   `json:"unique"`
}
