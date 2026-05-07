package cockroach

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// ExportTenant returns a tenant-scoped snapshot of the stored data and operator state.
func (s *Store) ExportTenant(ctx context.Context, tenantID uuid.UUID) (memory.TenantExport, error) {
	if tenantID == uuid.Nil {
		return memory.TenantExport{}, fmt.Errorf("export tenant: tenant_id must not be nil")
	}

	tenant, err := getTenantByID(ctx, s.pool, tenantID)
	if err != nil {
		return memory.TenantExport{}, err
	}

	agents, err := listAgentsByTenant(ctx, s.pool, tenantID)
	if err != nil {
		return memory.TenantExport{}, err
	}
	threads, err := listThreadsByTenant(ctx, s.pool, tenantID)
	if err != nil {
		return memory.TenantExport{}, err
	}
	episodes, err := s.ListEpisodesAfter(ctx, tenantID, nil, 1_000_000)
	if err != nil {
		return memory.TenantExport{}, err
	}
	memories, err := s.QueryMemories(ctx, memory.QueryMemoriesRequest{TenantID: tenantID, Limit: 1_000_000})
	if err != nil {
		return memory.TenantExport{}, err
	}
	checkpoints, err := s.ListProjectionCheckpoints(ctx, memory.ListProjectionCheckpointsRequest{TenantID: tenantID, Limit: 1_000_000})
	if err != nil {
		return memory.TenantExport{}, err
	}
	runs, err := s.ListConsolidationRuns(ctx, memory.ListConsolidationRunsRequest{TenantID: tenantID, Limit: 1_000_000})
	if err != nil {
		return memory.TenantExport{}, err
	}
	leases, err := s.listWorkerLeasesForTenant(ctx, tenantID)
	if err != nil {
		return memory.TenantExport{}, err
	}

	return memory.TenantExport{
		Tenant:                tenant,
		Agents:                agents,
		Threads:               threads,
		Episodes:              episodes,
		Memories:              memories,
		ProjectionCheckpoints: checkpoints,
		ConsolidationRuns:     runs,
		WorkerLeases:          leases,
	}, nil
}

// DeleteTenant deletes tenant-scoped data in a safe explicit order.
func (s *Store) DeleteTenant(ctx context.Context, tenantID uuid.UUID) (memory.TenantDeletionResult, error) {
	if tenantID == uuid.Nil {
		return memory.TenantDeletionResult{}, fmt.Errorf("delete tenant: tenant_id must not be nil")
	}

	result := memory.TenantDeletionResult{TenantID: tenantID, Deleted: true}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := getTenantByID(ctx, tx, tenantID); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `DELETE FROM projection_checkpoints WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete projection checkpoints: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM worker_leases WHERE lease_name LIKE $1`, "%:"+tenantID.String()+":%"); err != nil {
			return fmt.Errorf("delete worker leases: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memory_provenance WHERE memory_id IN (SELECT id FROM memories WHERE tenant_id = $1)`, tenantID); err != nil {
			return fmt.Errorf("delete memory provenance: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM consolidation_runs WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete consolidation runs: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memories WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete memories: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM episodes WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete episodes: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM threads WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete threads: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM agents WHERE tenant_id = $1`, tenantID); err != nil {
			return fmt.Errorf("delete agents: %w", err)
		}
		tag, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		if err != nil {
			return fmt.Errorf("delete tenant: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return memory.ErrTenantNotFound
		}
		return nil
	})
	if err != nil {
		return memory.TenantDeletionResult{}, err
	}
	return result, nil
}

func getTenantByID(ctx context.Context, q queryRower, tenantID uuid.UUID) (memory.Tenant, error) {
	var tenant memory.Tenant
	err := q.QueryRow(ctx, `SELECT id, slug, created_at FROM tenants WHERE id = $1`, tenantID).Scan(&tenant.ID, &tenant.Slug, &tenant.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return memory.Tenant{}, memory.ErrTenantNotFound
		}
		return memory.Tenant{}, fmt.Errorf("get tenant by id: %w", err)
	}
	return tenant, nil
}

func listAgentsByTenant(ctx context.Context, pool queryer, tenantID uuid.UUID) ([]memory.Agent, error) {
	rows, err := pool.Query(ctx, `SELECT id, tenant_id, external_ref, name, metadata, created_at FROM agents WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list agents by tenant: %w", err)
	}
	defer rows.Close()

	var result []memory.Agent
	for rows.Next() {
		var (
			item         memory.Agent
			externalRef  sql.NullString
			metadataJSON []byte
		)
		if err := rows.Scan(&item.ID, &item.TenantID, &externalRef, &item.Name, &metadataJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		if externalRef.Valid {
			item.ExternalRef = externalRef.String
		}
		metadata, err := unmarshalJSONMap(metadataJSON)
		if err != nil {
			return nil, err
		}
		item.Metadata = metadata
		result = append(result, item)
	}
	return result, rows.Err()
}

func listThreadsByTenant(ctx context.Context, pool queryer, tenantID uuid.UUID) ([]memory.Thread, error) {
	rows, err := pool.Query(ctx, `SELECT id, tenant_id, agent_id, external_ref, metadata, created_at FROM threads WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list threads by tenant: %w", err)
	}
	defer rows.Close()

	var result []memory.Thread
	for rows.Next() {
		var (
			item         memory.Thread
			externalRef  sql.NullString
			metadataJSON []byte
		)
		if err := rows.Scan(&item.ID, &item.TenantID, &item.AgentID, &externalRef, &metadataJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		if externalRef.Valid {
			item.ExternalRef = externalRef.String
		}
		metadata, err := unmarshalJSONMap(metadataJSON)
		if err != nil {
			return nil, err
		}
		item.Metadata = metadata
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) listWorkerLeasesForTenant(ctx context.Context, tenantID uuid.UUID) ([]memory.WorkerLease, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT lease_name, holder_id, expires_at, renewed_at, metadata
		 FROM worker_leases
		 WHERE lease_name LIKE $1
		 ORDER BY renewed_at DESC, lease_name`,
		"%:"+tenantID.String()+":%",
	)
	if err != nil {
		return nil, fmt.Errorf("list tenant worker leases: %w", err)
	}
	defer rows.Close()

	var result []memory.WorkerLease
	for rows.Next() {
		var (
			item         memory.WorkerLease
			metadataJSON []byte
		)
		if err := rows.Scan(&item.LeaseName, &item.HolderID, &item.ExpiresAt, &item.RenewedAt, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan tenant worker lease: %w", err)
		}
		metadata, err := unmarshalJSONMap(metadataJSON)
		if err != nil {
			return nil, err
		}
		item.Metadata = metadata
		result = append(result, item)
	}
	return result, rows.Err()
}
