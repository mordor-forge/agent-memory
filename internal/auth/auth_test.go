package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/config"
)

func TestNewFromConfigDisabled(t *testing.T) {
	t.Parallel()

	authorizer, err := NewFromConfig(config.Config{HTTPAuthMode: "disabled"})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if authorizer.Enabled() {
		t.Fatal("expected disabled authorizer")
	}
}

func TestNewFromConfigAPIKey(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	authorizer, err := NewFromConfig(config.Config{
		HTTPAuthMode:           "api_key",
		HTTPAuthHeader:         "Authorization",
		HTTPAuthScheme:         "Bearer",
		HTTPAuthPrincipalsJSON: `[{"token":"secret","subject":"user-1","role":"admin","tenant_grants":["` + tenantID.String() + `"]}]`,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if !authorizer.Enabled() {
		t.Fatal("expected enabled authorizer")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	principal, err := authorizer.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Subject != "user-1" || !principal.CanAccessTenant(tenantID) || !principal.CanAdmin() {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestAuthenticateMissingHeader(t *testing.T) {
	t.Parallel()

	authorizer, err := NewFromConfig(config.Config{
		HTTPAuthMode:           "api_key",
		HTTPAuthHeader:         "Authorization",
		HTTPAuthScheme:         "Bearer",
		HTTPAuthPrincipalsJSON: `[{"token":"secret","subject":"user-1","tenant_grants":["*"]}]`,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	if _, err := authorizer.Authenticate(req); err != ErrUnauthenticated {
		t.Fatalf("Authenticate() err = %v, want %v", err, ErrUnauthenticated)
	}
}
