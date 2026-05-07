package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// CreateTenant inserts a tenant row.
func (s *Store) CreateTenant(ctx context.Context, req memory.CreateTenantRequest) (memory.Tenant, error) {
	if err := req.Validate(); err != nil {
		return memory.Tenant{}, err
	}

	var tenant memory.Tenant
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (slug)
		 VALUES ($1)
		 RETURNING id, slug, created_at`,
		strings.TrimSpace(req.Slug),
	).Scan(&tenant.ID, &tenant.Slug, &tenant.CreatedAt)
	if err != nil {
		return memory.Tenant{}, fmt.Errorf("create tenant: %w", err)
	}
	return tenant, nil
}
