package integration

import (
	"context"
	"time"
)

// Connector represents a connection to an external service
type Connector interface {
	// Name returns the connector identifier
	Name() string

	// Type returns the service type
	Type() ServiceType

	// Connect establishes connection with the service
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect() error

	// IsConnected returns connection status
	IsConnected() bool

	// GetRateLimits returns current rate limit status
	GetRateLimits() *RateLimitStatus
}

// ServiceType defines the type of external service
type ServiceType string

const (
	ServiceTypeGitHub     ServiceType = "github"
	ServiceTypeSlack      ServiceType = "slack"
	ServiceTypeSalesforce ServiceType = "salesforce"
	ServiceTypeZendesk    ServiceType = "zendesk"
)

// OAuth2Config holds OAuth2 configuration
type OAuth2Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	AuthURL      string
	TokenURL     string
}

// Credentials holds service authentication credentials
type Credentials struct {
	ServiceType ServiceType
	AccessToken string
	RefreshToken string
	TokenType   string
	Expiry      time.Time
	Metadata    map[string]string
}

// CredentialVault manages secure credential storage
type CredentialVault interface {
	// Store saves credentials securely
	Store(ctx context.Context, serviceName string, creds *Credentials) error

	// Retrieve gets stored credentials
	Retrieve(ctx context.Context, serviceName string) (*Credentials, error)

	// Delete removes stored credentials
	Delete(ctx context.Context, serviceName string) error

	// List returns all stored service names
	List(ctx context.Context) ([]string, error)
}

// RateLimiter manages API rate limiting
type RateLimiter interface {
	// Allow checks if a request is allowed
	Allow(ctx context.Context, service string) (bool, error)

	// Wait blocks until a request is allowed
	Wait(ctx context.Context, service string) error

	// GetStatus returns current rate limit status
	GetStatus(service string) *RateLimitStatus
}

// RateLimitStatus holds rate limit information
type RateLimitStatus struct {
	Limit     int       // Maximum requests allowed
	Remaining int       // Requests remaining
	Reset     time.Time // When the limit resets
	RetryAfter time.Duration // How long to wait if exceeded
}

// AuditLogger records all API interactions
type AuditLogger interface {
	// Log records an API call
	Log(ctx context.Context, entry *AuditEntry) error

	// Query retrieves audit logs
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)
}

// AuditEntry represents a single audit log entry
type AuditEntry struct {
	ID          string
	Timestamp   time.Time
	Service     ServiceType
	Operation   string
	UserID      string
	RequestID   string
	Method      string
	Endpoint    string
	StatusCode  int
	Duration    time.Duration
	Success     bool
	Error       string
	Metadata    map[string]interface{}
}

// AuditFilter defines criteria for querying audit logs
type AuditFilter struct {
	Service    *ServiceType
	StartTime  *time.Time
	EndTime    *time.Time
	UserID     *string
	Success    *bool
	Limit      int
	Offset     int
}

// Config holds integration configuration
type Config struct {
	// GitHub configuration
	GitHub *GitHubConfig

	// Slack configuration
	Slack *SlackConfig

	// Credential vault settings
	VaultType string // "keyring", "env", "file"
	VaultPath string

	// Rate limiting
	EnableRateLimiting bool
	DefaultRateLimit   int // requests per hour

	// Audit logging
	AuditLogEnabled bool
	AuditLogPath    string
}

// GitHubConfig holds GitHub-specific configuration
type GitHubConfig struct {
	Enabled      bool
	OAuth2       *OAuth2Config
	EnterpriseURL string // For GitHub Enterprise
	DefaultOrg   string
	DefaultRepo  string
}

// SlackConfig holds Slack-specific configuration
type SlackConfig struct {
	Enabled     bool
	OAuth2      *OAuth2Config
	BotToken    string
	SigningSecret string
	DefaultChannel string
}

// DefaultConfig returns default integration configuration
func DefaultConfig() *Config {
	return &Config{
		GitHub: &GitHubConfig{
			Enabled: false,
			OAuth2: &OAuth2Config{
				AuthURL:  "https://github.com/login/oauth/authorize",
				TokenURL: "https://github.com/login/oauth/access_token",
				Scopes:   []string{"repo", "read:user"},
			},
		},
		Slack: &SlackConfig{
			Enabled: false,
			OAuth2: &OAuth2Config{
				AuthURL:  "https://slack.com/oauth/v2/authorize",
				TokenURL: "https://slack.com/api/oauth.v2.access",
				Scopes:   []string{"chat:write", "channels:read"},
			},
		},
		VaultType:          "keyring",
		EnableRateLimiting: true,
		DefaultRateLimit:   5000, // GitHub's default
		AuditLogEnabled:    true,
		AuditLogPath:       "~/.quantumflow/audit.db",
	}
}
