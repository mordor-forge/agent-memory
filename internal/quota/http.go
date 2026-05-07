package quota

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/time/rate"

	"github.com/mordor-forge/agent-memory/internal/auth"
	"github.com/mordor-forge/agent-memory/internal/config"
)

var (
	// ErrThrottled indicates the request exceeded the configured rate limit.
	ErrThrottled = errors.New("quota: throttled")
)

// HTTPRateLimiter is the first in-memory principal-scoped HTTP rate limiter.
type HTTPRateLimiter struct {
	enabled bool
	limit   rate.Limit
	burst   int

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewHTTPRateLimiter builds an HTTP limiter from process config.
func NewHTTPRateLimiter(cfg config.Config) *HTTPRateLimiter {
	if cfg.HTTPRateLimitRPS <= 0 {
		return &HTTPRateLimiter{enabled: false}
	}
	return &HTTPRateLimiter{
		enabled:  true,
		limit:    rate.Limit(cfg.HTTPRateLimitRPS),
		burst:    cfg.HTTPRateLimitBurst,
		limiters: make(map[string]*rate.Limiter),
	}
}

// Enabled reports whether HTTP throttling is active.
func (l *HTTPRateLimiter) Enabled() bool {
	return l != nil && l.enabled
}

// Allow reports whether the principal may proceed right now.
func (l *HTTPRateLimiter) Allow(ctx context.Context, principal auth.Principal) error {
	if l == nil || !l.enabled || principal.Bypass {
		return nil
	}
	key := principal.Subject
	if key == "" {
		key = "anonymous"
	}

	limiter := l.getLimiter(key)
	if !limiter.Allow() {
		return ErrThrottled
	}
	return nil
}

func (l *HTTPRateLimiter) getLimiter(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limiter, ok := l.limiters[key]; ok {
		return limiter
	}
	limiter := rate.NewLimiter(l.limit, l.burst)
	l.limiters[key] = limiter
	return limiter
}
