package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgvector/pgvector-go"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// SearchMemories performs nearest-neighbor retrieval over memory_embeddings using the provided query vector.
func (s *Store) SearchMemories(ctx context.Context, req memory.RecallRequest, queryVector []float32) ([]memory.RecallHit, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("search memories: query vector must not be empty")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	var (
		builder strings.Builder
		args    []any
		argN    int
	)
	builder.WriteString(`
		SELECT m.id, m.tenant_id, m.agent_id, m.thread_id, m.kind, m.status, m.content, m.summary, m.attributes,
		       m.importance, m.confidence, m.supersedes_memory_id, m.last_observed_at, m.created_at, m.updated_at,
		       me.embedding <=> $1 AS distance
		FROM memory_embeddings me
		JOIN memories m ON m.id = me.memory_id
		WHERE me.tenant_id = $2`)
	args = append(args, pgvector.NewVector(queryVector), req.TenantID)
	argN = 2

	if req.AgentID != nil {
		argN++
		fmt.Fprintf(&builder, " AND me.agent_id = $%d", argN)
		args = append(args, *req.AgentID)
	}
	if req.ThreadID != nil {
		argN++
		fmt.Fprintf(&builder, " AND m.thread_id = $%d", argN)
		args = append(args, *req.ThreadID)
	}
	if strings.TrimSpace(req.Kind) != "" {
		argN++
		fmt.Fprintf(&builder, " AND m.kind = $%d", argN)
		args = append(args, strings.TrimSpace(req.Kind))
	}
	if strings.TrimSpace(req.Status) != "" {
		argN++
		fmt.Fprintf(&builder, " AND m.status = $%d", argN)
		args = append(args, strings.TrimSpace(req.Status))
	}

	argN++
	fmt.Fprintf(&builder, " ORDER BY me.embedding <=> $1, m.created_at, m.id LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("search memories query: %w", err)
	}
	defer rows.Close()

	hits := make([]memory.RecallHit, 0, limit)
	for rows.Next() {
		record, distance, err := scanRecallHit(rows)
		if err != nil {
			return nil, err
		}
		hits = append(hits, memory.RecallHit{
			Memory:   record,
			Distance: distance,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search memories rows: %w", err)
	}
	return hits, nil
}
