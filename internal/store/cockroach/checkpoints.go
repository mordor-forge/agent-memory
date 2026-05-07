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

// GetProjectionCheckpoint loads the saved CheckpointCursor for one projection shard.
func (s *Store) GetProjectionCheckpoint(ctx context.Context, projectionName string, tenantID uuid.UUID, shardID int64) (*memory.CheckpointCursor, error) {
	if projectionName == "" {
		return nil, fmt.Errorf("get projection checkpoint: projection name must not be empty")
	}
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("get projection checkpoint: tenant_id must not be nil")
	}

	var (
		lastCreatedAt sql.NullTime
		lastID        uuid.NullUUID
	)
	err := s.pool.QueryRow(ctx,
		`SELECT last_created_at, last_id
		 FROM projection_checkpoints
		 WHERE projection_name = $1 AND tenant_id = $2 AND shard_id = $3`,
		projectionName, tenantID, shardID,
	).Scan(&lastCreatedAt, &lastID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get projection checkpoint: %w", err)
	}
	if !lastCreatedAt.Valid || !lastID.Valid {
		return nil, nil
	}
	cursor := &memory.CheckpointCursor{
		CreatedAt: lastCreatedAt.Time,
		ID:        lastID.UUID,
	}
	return cursor, nil
}

// AdvanceProjectionCheckpoint upserts the projection cursor after successful processing.
func (s *Store) AdvanceProjectionCheckpoint(ctx context.Context, projectionName string, tenantID uuid.UUID, shardID int64, cursor memory.CheckpointCursor) error {
	if projectionName == "" {
		return fmt.Errorf("advance projection checkpoint: projection name must not be empty")
	}
	if tenantID == uuid.Nil {
		return fmt.Errorf("advance projection checkpoint: tenant_id must not be nil")
	}
	if cursor.ID == uuid.Nil {
		return fmt.Errorf("advance projection checkpoint: cursor id must not be nil")
	}
	if cursor.CreatedAt.IsZero() {
		return fmt.Errorf("advance projection checkpoint: cursor created_at must not be zero")
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO projection_checkpoints (
			projection_name,
			tenant_id,
			shard_id,
			last_created_at,
			last_id,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (projection_name, tenant_id, shard_id)
		DO UPDATE SET
			last_created_at = EXCLUDED.last_created_at,
			last_id = EXCLUDED.last_id,
			updated_at = now()`,
		projectionName, tenantID, shardID, cursor.CreatedAt, cursor.ID,
	)
	if err != nil {
		return fmt.Errorf("advance projection checkpoint: %w", err)
	}
	return nil
}
