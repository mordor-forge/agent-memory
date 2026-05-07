package cockroach

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// AppendEpisode records one immutable episode. If an idempotency key is supplied,
// repeated calls with the same tenant and key return the existing row.
func (s *Store) AppendEpisode(ctx context.Context, req memory.AppendEpisodeRequest) (memory.Episode, error) {
	if err := req.Validate(); err != nil {
		return memory.Episode{}, err
	}

	payload, err := marshalJSONMap(req.Payload)
	if err != nil {
		return memory.Episode{}, err
	}

	var episode memory.Episode
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		ok, err := agentBelongsToTenant(ctx, tx, req.TenantID, req.AgentID)
		if err != nil {
			return err
		}
		if !ok {
			return memory.ErrAgentTenantMismatch
		}

		if req.ThreadID != nil {
			ok, err = threadBelongsToScope(ctx, tx, req.TenantID, req.AgentID, *req.ThreadID)
			if err != nil {
				return err
			}
			if !ok {
				return memory.ErrThreadMismatch
			}
		}

		threadArg := any(nil)
		if req.ThreadID != nil {
			threadArg = *req.ThreadID
		}

		query := `
			INSERT INTO episodes (
				tenant_id,
				agent_id,
				thread_id,
				idempotency_key,
				kind,
				content,
				payload,
				occurred_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, tenant_id, agent_id, thread_id, idempotency_key, kind, content, payload, occurred_at, created_at
		`
		args := []any{
			req.TenantID,
			req.AgentID,
			threadArg,
			nullableString(req.IdempotencyKey),
			strings.TrimSpace(req.Kind),
			req.Content,
			payload,
			req.OccurredAt,
		}
		if strings.TrimSpace(req.IdempotencyKey) != "" {
			query = `
				INSERT INTO episodes (
					tenant_id,
					agent_id,
					thread_id,
					idempotency_key,
					kind,
					content,
					payload,
					occurred_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (tenant_id, idempotency_key)
				DO UPDATE SET idempotency_key = episodes.idempotency_key
				RETURNING id, tenant_id, agent_id, thread_id, idempotency_key, kind, content, payload, occurred_at, created_at
			`
		}

		episode, err = scanEpisode(tx.QueryRow(ctx, query, args...))
		if err != nil {
			return fmt.Errorf("append episode: %w", err)
		}
		if strings.TrimSpace(req.IdempotencyKey) != "" {
			if episode.AgentID != req.AgentID ||
				episode.Kind != strings.TrimSpace(req.Kind) ||
				episode.Content != req.Content ||
				!sameOptionalUUID(episode.ThreadID, req.ThreadID) {
				return memory.ErrIdempotencyConflict
			}
		}
		return nil
	})
	if err != nil {
		return memory.Episode{}, err
	}

	return episode, nil
}
