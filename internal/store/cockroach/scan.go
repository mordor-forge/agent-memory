package cockroach

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

const defaultBatchLimit = 100

// ListEpisodesAfter returns episodes for one tenant ordered by CheckpointCursor.
func (s *Store) ListEpisodesAfter(ctx context.Context, tenantID uuid.UUID, after *memory.CheckpointCursor, limit int) ([]memory.Episode, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("list episodes after: tenant_id must not be nil")
	}
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	query := `
		SELECT id, tenant_id, agent_id, thread_id, idempotency_key, kind, content, payload, occurred_at, created_at
		FROM episodes
		WHERE tenant_id = $1
		ORDER BY created_at, id
		LIMIT $2
	`
	args := []any{tenantID, limit}
	if after != nil {
		query = `
			SELECT id, tenant_id, agent_id, thread_id, idempotency_key, kind, content, payload, occurred_at, created_at
			FROM episodes
			WHERE tenant_id = $1
			  AND (
				created_at > $2
				OR (created_at = $2 AND id > $3)
			  )
			ORDER BY created_at, id
			LIMIT $4
		`
		args = []any{tenantID, after.CreatedAt, after.ID, limit}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list episodes after query: %w", err)
	}
	defer rows.Close()

	episodes := make([]memory.Episode, 0, limit)
	for rows.Next() {
		episode, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, episode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list episodes after rows: %w", err)
	}
	return episodes, nil
}
