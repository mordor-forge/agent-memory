package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/config"
)

var (
	// ErrUnauthenticated indicates that the request did not present valid credentials.
	ErrUnauthenticated = errors.New("auth: unauthenticated")
	// ErrUnauthorized indicates that the authenticated principal is not allowed to perform the action.
	ErrUnauthorized = errors.New("auth: unauthorized")
)

type contextKey string

const principalContextKey contextKey = "agent-memory-principal"

// Principal is the authenticated identity attached to a request.
type Principal struct {
	Subject      string
	Role         string
	TenantGrants map[uuid.UUID]struct{}
	Wildcard     bool
	Bypass       bool
}

// CanAccessTenant reports whether the principal can operate on the supplied tenant.
func (p Principal) CanAccessTenant(tenantID uuid.UUID) bool {
	if p.Wildcard {
		return true
	}
	_, ok := p.TenantGrants[tenantID]
	return ok
}

// CanAdmin reports whether the principal has admin-like capabilities.
func (p Principal) CanAdmin() bool {
	return p.Wildcard || p.Role == "admin"
}

// WithPrincipal stores the principal in a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

// PrincipalFromContext fetches a principal from context if present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}

// Authorizer authenticates HTTP requests and produces principals.
type Authorizer struct {
	mode       string
	header     string
	scheme     string
	principals map[string]Principal
}

type principalConfig struct {
	Token        string   `json:"token"`
	Subject      string   `json:"subject"`
	Role         string   `json:"role,omitempty"`
	TenantGrants []string `json:"tenant_grants"`
}

// NewFromConfig builds the initial HTTP authorizer from process config.
func NewFromConfig(cfg config.Config) (*Authorizer, error) {
	if cfg.HTTPAuthMode == "disabled" {
		return &Authorizer{
			mode: "disabled",
		}, nil
	}

	var raw []principalConfig
	if err := json.Unmarshal([]byte(cfg.HTTPAuthPrincipalsJSON), &raw); err != nil {
		return nil, fmt.Errorf("parse MEMORY_HTTP_AUTH_PRINCIPALS_JSON: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("at least one HTTP auth principal must be configured")
	}

	principals := make(map[string]Principal, len(raw))
	for _, item := range raw {
		if strings.TrimSpace(item.Token) == "" {
			return nil, errors.New("http auth principal token must not be empty")
		}
		if strings.TrimSpace(item.Subject) == "" {
			return nil, errors.New("http auth principal subject must not be empty")
		}
		principal := Principal{
			Subject:      strings.TrimSpace(item.Subject),
			Role:         strings.TrimSpace(item.Role),
			TenantGrants: make(map[uuid.UUID]struct{}),
		}
		for _, grant := range item.TenantGrants {
			grant = strings.TrimSpace(grant)
			if grant == "" {
				continue
			}
			if grant == "*" {
				principal.Wildcard = true
				continue
			}
			tenantID, err := uuid.Parse(grant)
			if err != nil {
				return nil, fmt.Errorf("parse tenant grant %q: %w", grant, err)
			}
			principal.TenantGrants[tenantID] = struct{}{}
		}
		principals[item.Token] = principal
	}

	return &Authorizer{
		mode:       "api_key",
		header:     cfg.HTTPAuthHeader,
		scheme:     cfg.HTTPAuthScheme,
		principals: principals,
	}, nil
}

// Enabled reports whether auth is enforced.
func (a *Authorizer) Enabled() bool {
	return a != nil && a.mode != "disabled"
}

// Authenticate extracts a principal from the request or returns a dev bypass principal.
func (a *Authorizer) Authenticate(r *http.Request) (Principal, error) {
	if a == nil || a.mode == "disabled" {
		return Principal{
			Subject:  "local-dev-bypass",
			Role:     "admin",
			Wildcard: true,
			Bypass:   true,
		}, nil
	}

	raw := strings.TrimSpace(r.Header.Get(a.header))
	if raw == "" {
		return Principal{}, ErrUnauthenticated
	}
	prefix := a.scheme + " "
	if !strings.HasPrefix(raw, prefix) {
		return Principal{}, ErrUnauthenticated
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	principal, ok := a.principals[token]
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	return principal, nil
}
