package cockroach

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// GetMemoryProvenance returns source episodes and run metadata for one memory row.
func (s *Store) GetMemoryProvenance(ctx context.Context, memoryID uuid.UUID) (memory.MemoryProvenance, error) {
	if memoryID == uuid.Nil {
		return memory.MemoryProvenance{}, fmt.Errorf("get memory provenance: memory_id must not be nil")
	}

	var (
		result         memory.MemoryProvenance
		projectionName sql.NullString
		hasEmbedding   bool
	)
	err := s.pool.QueryRow(ctx,
		`SELECT m.id, m.tenant_id, m.attributes->>'projection' AS projection_name,
		        EXISTS (SELECT 1 FROM memory_embeddings me WHERE me.memory_id = m.id) AS has_embedding
		 FROM memories m
		 WHERE m.id = $1`,
		memoryID,
	).Scan(&result.MemoryID, &result.TenantID, &projectionName, &hasEmbedding)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return memory.MemoryProvenance{}, memory.ErrMemoryNotFound
		}
		return memory.MemoryProvenance{}, fmt.Errorf("get memory provenance header: %w", err)
	}
	if projectionName.Valid {
		result.ProjectionName = projectionName.String
	}
	result.HasEmbedding = hasEmbedding

	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.kind, e.content, e.created_at, mp.role, mp.consolidation_run_id
		 FROM memory_provenance mp
		 JOIN episodes e ON e.id = mp.episode_id
		 WHERE mp.memory_id = $1
		 ORDER BY e.created_at, e.id`,
		memoryID,
	)
	if err != nil {
		return memory.MemoryProvenance{}, fmt.Errorf("get memory provenance sources: %w", err)
	}
	defer rows.Close()

	sources := make([]memory.MemorySourceProvenance, 0)
	for rows.Next() {
		var source memory.MemorySourceProvenance
		if err := rows.Scan(
			&source.EpisodeID,
			&source.EpisodeKind,
			&source.EpisodeContent,
			&source.EpisodeCreatedAt,
			&source.Role,
			&source.ConsolidationRunID,
		); err != nil {
			return memory.MemoryProvenance{}, fmt.Errorf("scan memory provenance source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return memory.MemoryProvenance{}, fmt.Errorf("memory provenance rows: %w", err)
	}
	result.Sources = sources
	return result, nil
}

// ListProjectionCheckpoints returns projection checkpoint rows for one tenant.
func (s *Store) ListProjectionCheckpoints(ctx context.Context, req memory.ListProjectionCheckpointsRequest) ([]memory.ProjectionCheckpoint, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	var (
		builder strings.Builder
		args    []any
		argN    = 1
	)
	builder.WriteString(`
		SELECT projection_name, tenant_id, shard_id, last_created_at, last_id, updated_at
		FROM projection_checkpoints
		WHERE tenant_id = $1`)
	args = append(args, req.TenantID)
	if strings.TrimSpace(req.ProjectionName) != "" {
		argN++
		fmt.Fprintf(&builder, " AND projection_name = $%d", argN)
		args = append(args, strings.TrimSpace(req.ProjectionName))
	}
	argN++
	fmt.Fprintf(&builder, " ORDER BY projection_name, shard_id LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list projection checkpoints query: %w", err)
	}
	defer rows.Close()

	result := make([]memory.ProjectionCheckpoint, 0, limit)
	for rows.Next() {
		var (
			item          memory.ProjectionCheckpoint
			lastCreatedAt sql.NullTime
			lastID        uuid.NullUUID
		)
		if err := rows.Scan(&item.ProjectionName, &item.TenantID, &item.ShardID, &lastCreatedAt, &lastID, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan projection checkpoint: %w", err)
		}
		if lastCreatedAt.Valid && lastID.Valid {
			item.LastCursor = &memory.CheckpointCursor{
				CreatedAt: lastCreatedAt.Time,
				ID:        lastID.UUID,
			}
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection checkpoint rows: %w", err)
	}
	return result, nil
}

// ListConsolidationRuns returns recent projection/consolidation batch records for one tenant.
func (s *Store) ListConsolidationRuns(ctx context.Context, req memory.ListConsolidationRunsRequest) ([]memory.ConsolidationRun, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	var (
		builder strings.Builder
		args    []any
		argN    = 1
	)
	builder.WriteString(`
		SELECT id, tenant_id, worker_id, batch_started_at, input_from_created_at, input_from_id,
		       input_to_created_at, input_to_id, status, projection_name, provider, model, error_text, created_at, completed_at
		FROM consolidation_runs
		WHERE tenant_id = $1`)
	args = append(args, req.TenantID)
	if strings.TrimSpace(req.ProjectionName) != "" {
		argN++
		fmt.Fprintf(&builder, " AND projection_name = $%d", argN)
		args = append(args, strings.TrimSpace(req.ProjectionName))
	}
	argN++
	fmt.Fprintf(&builder, " ORDER BY created_at DESC LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list consolidation runs query: %w", err)
	}
	defer rows.Close()

	result := make([]memory.ConsolidationRun, 0, limit)
	for rows.Next() {
		var (
			item           memory.ConsolidationRun
			inputFromTime  sql.NullTime
			inputFromID    uuid.NullUUID
			inputToTime    sql.NullTime
			inputToID      uuid.NullUUID
			projectionName sql.NullString
			provider       sql.NullString
			model          sql.NullString
			errorText      sql.NullString
			completedAt    sql.NullTime
		)
		if err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.WorkerID,
			&item.BatchStartedAt,
			&inputFromTime,
			&inputFromID,
			&inputToTime,
			&inputToID,
			&item.Status,
			&projectionName,
			&provider,
			&model,
			&errorText,
			&item.CreatedAt,
			&completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan consolidation run: %w", err)
		}
		if projectionName.Valid {
			item.ProjectionName = projectionName.String
		} else if model.Valid {
			item.ProjectionName = model.String
		}
		if provider.Valid {
			item.Provider = provider.String
		}
		if model.Valid {
			item.Model = model.String
		}
		if inputFromTime.Valid && inputFromID.Valid {
			item.InputFrom = &memory.CheckpointCursor{CreatedAt: inputFromTime.Time, ID: inputFromID.UUID}
		}
		if inputToTime.Valid && inputToID.Valid {
			item.InputTo = &memory.CheckpointCursor{CreatedAt: inputToTime.Time, ID: inputToID.UUID}
		}
		if errorText.Valid {
			value := errorText.String
			item.ErrorText = &value
		}
		if completedAt.Valid {
			value := completedAt.Time
			item.CompletedAt = &value
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consolidation run rows: %w", err)
	}
	return result, nil
}

// ListWorkerLeases returns the currently persisted worker lease rows.
func (s *Store) ListWorkerLeases(ctx context.Context, req memory.ListWorkerLeasesRequest) ([]memory.WorkerLease, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	rows, err := s.pool.Query(ctx,
		`SELECT lease_name, holder_id, expires_at, renewed_at, metadata
		 FROM worker_leases
		 ORDER BY renewed_at DESC, lease_name
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list worker leases query: %w", err)
	}
	defer rows.Close()

	result := make([]memory.WorkerLease, 0, limit)
	for rows.Next() {
		var (
			item         memory.WorkerLease
			metadataJSON []byte
		)
		if err := rows.Scan(&item.LeaseName, &item.HolderID, &item.ExpiresAt, &item.RenewedAt, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan worker lease: %w", err)
		}
		metadata, err := unmarshalJSONMap(metadataJSON)
		if err != nil {
			return nil, err
		}
		item.Metadata = metadata
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("worker lease rows: %w", err)
	}
	return result, nil
}
