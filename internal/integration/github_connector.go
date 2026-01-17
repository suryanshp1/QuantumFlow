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

// GitHubConnector implements GitHub API integration
type GitHubConnector struct {
	config      *GitHubConfig
	credentials *Credentials
	vault       CredentialVault
	rateLimiter RateLimiter
	auditor     AuditLogger
	httpClient  *http.Client
	connected   bool
	mu          sync.RWMutex
}

// NewGitHubConnector creates a new GitHub connector
func NewGitHubConnector(
	config *GitHubConfig,
	vault CredentialVault,
	rateLimiter RateLimiter,
	auditor AuditLogger,
) *GitHubConnector {
	return &GitHubConnector{
		config:      config,
		vault:       vault,
		rateLimiter: rateLimiter,
		auditor:     auditor,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the connector identifier
func (g *GitHubConnector) Name() string {
	return "github"
}

// Type returns the service type
func (g *GitHubConnector) Type() ServiceType {
	return ServiceTypeGitHub
}

// Connect establishes connection with GitHub
func (g *GitHubConnector) Connect(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Retrieve credentials from vault
	creds, err := g.vault.Retrieve(ctx, g.Name())
	if err != nil {
		return fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	g.credentials = creds
	g.connected = true

	return nil
}

// Disconnect closes the connection
func (g *GitHubConnector) Disconnect() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.connected = false
	g.credentials = nil
	return nil
}

// IsConnected returns connection status
func (g *GitHubConnector) IsConnected() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.connected
}

// GetRateLimits returns current rate limit status
func (g *GitHubConnector) GetRateLimits() *RateLimitStatus {
	return g.rateLimiter.GetStatus(g.Name())
}

// GetRepository fetches repository information
func (g *GitHubConnector) GetRepository(ctx context.Context, owner, repo string) (*Repository, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s", owner, repo)
	
	var result Repository
	if err := g.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListPullRequests lists pull requests for a repository
func (g *GitHubConnector) ListPullRequests(ctx context.Context, owner, repo string, state string) ([]*PullRequest, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=%s", owner, repo, state)
	
	var result []*PullRequest
	if err := g.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CreatePullRequest creates a new pull request
func (g *GitHubConnector) CreatePullRequest(ctx context.Context, owner, repo string, pr *PullRequestCreate) (*PullRequest, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	
	var result PullRequest
	if err := g.apiCall(ctx, "POST", endpoint, pr, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetCommits retrieves recent commits
func (g *GitHubConnector) GetCommits(ctx context.Context, owner, repo string, since time.Time) ([]*Commit, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?since=%s", owner, repo, since.Format(time.RFC3339))
	
	var result []*Commit
	if err := g.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// SearchCode searches code across repositories
func (g *GitHubConnector) SearchCode(ctx context.Context, query string) (*SearchResults, error) {
	endpoint := fmt.Sprintf("/search/code?q=%s", query)
	
	var result SearchResults
	if err := g.apiCall(ctx, "GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// apiCall makes an authenticated API call to GitHub
func (g *GitHubConnector) apiCall(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	startTime := time.Now()

	// Check rate limit
	if err := g.rateLimiter.Wait(ctx, g.Name()); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Build URL
	baseURL := "https://api.github.com"
	if g.config.EnterpriseURL != "" {
		baseURL = g.config.EnterpriseURL
	}
	url := baseURL + endpoint

	// Create request
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
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.credentials.AccessToken))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Make request
	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.logAudit(ctx, method, endpoint, 0, time.Since(startTime), false, err.Error())
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		g.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, "HTTP error")
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			g.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), false, err.Error())
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	g.logAudit(ctx, method, endpoint, resp.StatusCode, time.Since(startTime), true, "")
	return nil
}

// logAudit records an audit log entry
func (g *GitHubConnector) logAudit(ctx context.Context, method, endpoint string, status int, duration time.Duration, success bool, errorMsg string) {
	if g.auditor == nil {
		return
	}

	entry := &AuditEntry{
		Timestamp:  time.Now(),
		Service:    ServiceTypeGitHub,
		Operation:  method + " " + endpoint,
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: status,
		Duration:   duration,
		Success:    success,
		Error:      errorMsg,
	}

	_ = g.auditor.Log(ctx, entry) // Ignore audit logging errors
}

// GitHub data models

type Repository struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
	CloneURL    string `json:"clone_url"`
}

type PullRequest struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	User      *User  `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PullRequestCreate struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body,omitempty"`
}

type Commit struct {
	SHA    string        `json:"sha"`
	Commit *CommitDetail `json:"commit"`
	Author *User         `json:"author"`
}

type CommitDetail struct {
	Message string `json:"message"`
	Author  *GitUser `json:"author"`
}

type GitUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type SearchResults struct {
	TotalCount int           `json:"total_count"`
	Items      []SearchItem  `json:"items"`
}

type SearchItem struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	Repository Repository `json:"repository"`
}
