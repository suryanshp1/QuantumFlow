package agent

import (
	"strings"
	"sync"
	"time"

	"github.com/quantumflow/quantumflow/internal/models"
)

// CachedRoute holds a cached routing decision
type CachedRoute struct {
	AgentType  models.AgentType
	Confidence float64
	CachedAt   time.Time
}

// RoutingCache provides TTL-based caching for routing decisions
type RoutingCache struct {
	cache map[string]*CachedRoute
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewRoutingCache creates a new cache with specified TTL
func NewRoutingCache(ttl time.Duration) *RoutingCache {
	c := &RoutingCache{
		cache: make(map[string]*CachedRoute),
		ttl:   ttl,
	}
	// Start background cleanup
	go c.cleanup()
	return c
}

// Get retrieves a cached route if valid
func (c *RoutingCache) Get(query string) (*CachedRoute, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := normalizeQuery(query)
	if route, ok := c.cache[key]; ok {
		if time.Since(route.CachedAt) < c.ttl {
			return route, true
		}
	}
	return nil, false
}

// Set stores a routing decision in cache
func (c *RoutingCache) Set(query string, agentType models.AgentType, confidence float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[normalizeQuery(query)] = &CachedRoute{
		AgentType:  agentType,
		Confidence: confidence,
		CachedAt:   time.Now(),
	}
}

// cleanup removes expired entries periodically
func (c *RoutingCache) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, route := range c.cache {
			if now.Sub(route.CachedAt) > c.ttl {
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
}

// normalizeQuery creates a cache key from query
func normalizeQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}
