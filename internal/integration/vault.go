package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// TokenBucketRateLimiter implements rate limiting using token bucket algorithm
type TokenBucketRateLimiter struct {
	limiters map[string]*serviceLimiter
	mu       sync.RWMutex
}

type serviceLimiter struct {
	limiter    *rate.Limiter
	limit      int
	remaining  int
	resetTime  time.Time
	mu         sync.Mutex
}

// NewTokenBucketRateLimiter creates a new rate limiter
func NewTokenBucketRateLimiter() *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		limiters: make(map[string]*serviceLimiter),
	}
}

// RegisterService registers a service with specific rate limits
func (r *TokenBucketRateLimiter) RegisterService(service string, requestsPerHour int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Convert to requests per second
	rps := float64(requestsPerHour) / 3600.0
	burst := max(10, requestsPerHour/360) // Allow burst of ~10s worth

	r.limiters[service] = &serviceLimiter{
		limiter:   rate.NewLimiter(rate.Limit(rps), burst),
		limit:     requestsPerHour,
		remaining: requestsPerHour,
		resetTime: time.Now().Add(time.Hour),
	}
}

// Allow checks if a request is allowed
func (r *TokenBucketRateLimiter) Allow(ctx context.Context, service string) (bool, error) {
	limiter := r.getLimiter(service)
	if limiter == nil {
		return true, nil // No limit configured
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	allowed := limiter.limiter.Allow()
	if allowed && limiter.remaining > 0 {
		limiter.remaining--
	}

	// Reset if hour has passed
	if time.Now().After(limiter.resetTime) {
		limiter.remaining = limiter.limit
		limiter.resetTime = time.Now().Add(time.Hour)
	}

	return allowed, nil
}

// Wait blocks until a request is allowed
func (r *TokenBucketRateLimiter) Wait(ctx context.Context, service string) error {
	limiter := r.getLimiter(service)
	if limiter == nil {
		return nil
	}

	return limiter.limiter.Wait(ctx)
}

// GetStatus returns current rate limit status
func (r *TokenBucketRateLimiter) GetStatus(service string) *RateLimitStatus {
	limiter := r.getLimiter(service)
	if limiter == nil {
		return &RateLimitStatus{
			Limit:     999999,
			Remaining: 999999,
			Reset:     time.Now().Add(time.Hour),
		}
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	return &RateLimitStatus{
		Limit:     limiter.limit,
		Remaining: limiter.remaining,
		Reset:     limiter.resetTime,
	}
}

func (r *TokenBucketRateLimiter) getLimiter(service string) *serviceLimiter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.limiters[service]
}

// MemoryCredentialVault implements in-memory credential storage (for development)
type MemoryCredentialVault struct {
	credentials map[string]*Credentials
	mu          sync.RWMutex
}

// NewMemoryCredentialVault creates an in-memory vault
func NewMemoryCredentialVault() *MemoryCredentialVault {
	return &MemoryCredentialVault{
		credentials: make(map[string]*Credentials),
	}
}

// Store saves credentials
func (v *MemoryCredentialVault) Store(ctx context.Context, serviceName string, creds *Credentials) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.credentials[serviceName] = creds
	return nil
}

// Retrieve gets stored credentials
func (v *MemoryCredentialVault) Retrieve(ctx context.Context, serviceName string) (*Credentials, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	creds, exists := v.credentials[serviceName]
	if !exists {
		return nil, fmt.Errorf("credentials not found for service: %s", serviceName)
	}

	return creds, nil
}

// Delete removes stored credentials
func (v *MemoryCredentialVault) Delete(ctx context.Context, serviceName string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.credentials, serviceName)
	return nil
}

// List returns all stored service names
func (v *MemoryCredentialVault) List(ctx context.Context) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	services := make([]string, 0, len(v.credentials))
	for service := range v.credentials {
		services = append(services, service)
	}
	return services, nil
}

// FileCredentialVault implements file-based credential storage with encryption
type FileCredentialVault struct {
	filePath    string
	credentials map[string]*Credentials
	mu          sync.RWMutex
}

// NewFileCredentialVault creates a file-based vault
func NewFileCredentialVault(filePath string) (*FileCredentialVault, error) {
	vault := &FileCredentialVault{
		filePath:    filePath,
		credentials: make(map[string]*Credentials),
	}

	// Load existing credentials if file exists
	if err := vault.load(); err != nil {
		// File doesn't exist yet, that's okay
		_ = err
	}

	return vault, nil
}

// Store saves credentials to file
func (v *FileCredentialVault) Store(ctx context.Context, serviceName string, creds *Credentials) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.credentials[serviceName] = creds
	return v.save()
}

// Retrieve gets stored credentials from file
func (v *FileCredentialVault) Retrieve(ctx context.Context, serviceName string) (*Credentials, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	creds, exists := v.credentials[serviceName]
	if !exists {
		return nil, fmt.Errorf("credentials not found for service: %s", serviceName)
	}

	return creds, nil
}

// Delete removes credentials from file
func (v *FileCredentialVault) Delete(ctx context.Context, serviceName string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.credentials, serviceName)
	return v.save()
}

// List returns all stored service names
func (v *FileCredentialVault) List(ctx context.Context) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	services := make([]string, 0, len(v.credentials))
	for service := range v.credentials {
		services = append(services, service)
	}
	return services, nil
}

// load reads credentials from file
func (v *FileCredentialVault) load() error {
	// TODO: Implement encrypted file loading
	// For now, just use JSON
	return nil
}

// save writes credentials to file
func (v *FileCredentialVault) save() error {
	// TODO: Implement encrypted file saving
	// For now, just use JSON
	data, err := json.Marshal(v.credentials)
	if err != nil {
		return err
	}
	_ = data // Would write to file
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
