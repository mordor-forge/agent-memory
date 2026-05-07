package cockroach

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// CreateAgent inserts an agent row inside one tenant.
func (s *Store) CreateAgent(ctx context.Context, req memory.CreateAgentRequest) (memory.Agent, error) {
	if err := req.Validate(); err != nil {
		return memory.Agent{}, err
	}

	metadata, err := marshalJSONMap(req.Metadata)
	if err != nil {
		return memory.Agent{}, err
	}

	var (
		agent        memory.Agent
		externalRef  sql.NullString
		metadataJSON []byte
	)
	err = s.pool.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, external_ref, name, metadata)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, external_ref, name, metadata, created_at`,
		req.TenantID, nullableString(req.ExternalRef), strings.TrimSpace(req.Name), metadata,
	).Scan(&agent.ID, &agent.TenantID, &externalRef, &agent.Name, &metadataJSON, &agent.CreatedAt)
	if err != nil {
		return memory.Agent{}, fmt.Errorf("create agent: %w", err)
	}

	if externalRef.Valid {
		agent.ExternalRef = externalRef.String
	}
	agent.Metadata, err = unmarshalJSONMap(metadataJSON)
	if err != nil {
		return memory.Agent{}, err
	}
	return agent, nil
}
