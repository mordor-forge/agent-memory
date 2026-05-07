package cockroach

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListTenantIDs returns tenant IDs ordered by creation time.
func (s *Store) ListTenantIDs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id
		 FROM tenants
		 ORDER BY created_at, id
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list tenant ids query: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tenant id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tenant ids rows: %w", err)
	}
	return ids, nil
}
