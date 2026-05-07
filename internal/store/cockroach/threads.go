package cockroach

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// CreateThread inserts a thread row inside one agent.
func (s *Store) CreateThread(ctx context.Context, req memory.CreateThreadRequest) (memory.Thread, error) {
	if err := req.Validate(); err != nil {
		return memory.Thread{}, err
	}

	metadata, err := marshalJSONMap(req.Metadata)
	if err != nil {
		return memory.Thread{}, err
	}

	var thread memory.Thread
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		ok, err := agentBelongsToTenant(ctx, tx, req.TenantID, req.AgentID)
		if err != nil {
			return err
		}
		if !ok {
			return memory.ErrAgentTenantMismatch
		}

		var (
			externalRef  sql.NullString
			metadataJSON []byte
		)
		if err := tx.QueryRow(ctx,
			`INSERT INTO threads (tenant_id, agent_id, external_ref, metadata)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, tenant_id, agent_id, external_ref, metadata, created_at`,
			req.TenantID, req.AgentID, nullableString(req.ExternalRef), metadata,
		).Scan(&thread.ID, &thread.TenantID, &thread.AgentID, &externalRef, &metadataJSON, &thread.CreatedAt); err != nil {
			return fmt.Errorf("create thread: %w", err)
		}

		if externalRef.Valid {
			thread.ExternalRef = externalRef.String
		}
		thread.Metadata, err = unmarshalJSONMap(metadataJSON)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return memory.Thread{}, err
	}
	return thread, nil
}
