package cockroach

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func marshalJSONMap(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json payload: %w", err)
	}
	return payload, nil
}

func unmarshalJSONMap(value []byte) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, fmt.Errorf("unmarshal json payload: %w", err)
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func agentBelongsToTenant(ctx context.Context, q queryRower, tenantID, agentID uuid.UUID) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM agents
			WHERE id = $1 AND tenant_id = $2
		)`,
		agentID, tenantID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check agent scope: %w", err)
	}
	return exists, nil
}

func threadBelongsToScope(ctx context.Context, q queryRower, tenantID, agentID, threadID uuid.UUID) (bool, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM threads
			WHERE id = $1 AND tenant_id = $2 AND agent_id = $3
		)`,
		threadID, tenantID, agentID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check thread scope: %w", err)
	}
	return exists, nil
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func scanEpisode(scanner rowScanner) (memory.Episode, error) {
	var (
		episode        memory.Episode
		threadID       uuid.NullUUID
		idempotencyKey sql.NullString
		payloadJSON    []byte
		occurredAt     sql.NullTime
	)

	if err := scanner.Scan(
		&episode.ID,
		&episode.TenantID,
		&episode.AgentID,
		&threadID,
		&idempotencyKey,
		&episode.Kind,
		&episode.Content,
		&payloadJSON,
		&occurredAt,
		&episode.CreatedAt,
	); err != nil {
		return memory.Episode{}, fmt.Errorf("scan episode: %w", err)
	}

	if threadID.Valid {
		episode.ThreadID = &threadID.UUID
	}
	if idempotencyKey.Valid {
		episode.IdempotencyKey = idempotencyKey.String
	}
	if occurredAt.Valid {
		occurredTime := occurredAt.Time
		episode.OccurredAt = &occurredTime
	}

	payload, err := unmarshalJSONMap(payloadJSON)
	if err != nil {
		return memory.Episode{}, err
	}
	episode.Payload = payload
	return episode, nil
}

func cursorFromEpisode(episode memory.Episode) memory.CheckpointCursor {
	return memory.CheckpointCursor{
		CreatedAt: episode.CreatedAt,
		ID:        episode.ID,
	}
}

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func scanMemory(scanner rowScanner) (memory.Memory, error) {
	var (
		record             memory.Memory
		threadID           uuid.NullUUID
		summary            sql.NullString
		attributesJSON     []byte
		supersedesMemoryID uuid.NullUUID
		lastObservedAt     sql.NullTime
	)

	if err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&record.AgentID,
		&threadID,
		&record.Kind,
		&record.Status,
		&record.Content,
		&summary,
		&attributesJSON,
		&record.Importance,
		&record.Confidence,
		&supersedesMemoryID,
		&lastObservedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return memory.Memory{}, fmt.Errorf("scan memory: %w", err)
	}

	if threadID.Valid {
		record.ThreadID = &threadID.UUID
	}
	if summary.Valid {
		summaryText := summary.String
		record.Summary = &summaryText
	}
	if supersedesMemoryID.Valid {
		record.SupersedesMemoryID = &supersedesMemoryID.UUID
	}
	if lastObservedAt.Valid {
		lastObserved := lastObservedAt.Time
		record.LastObservedAt = &lastObserved
	}
	attributes, err := unmarshalJSONMap(attributesJSON)
	if err != nil {
		return memory.Memory{}, err
	}
	record.Attributes = attributes
	return record, nil
}

func scanRecallHit(scanner rowScanner) (memory.Memory, float64, error) {
	var distance float64
	record, err := scanMemoryWithDistance(scanner, &distance)
	if err != nil {
		return memory.Memory{}, 0, err
	}
	return record, distance, nil
}

func scanMemoryWithDistance(scanner rowScanner, distance *float64) (memory.Memory, error) {
	var (
		record             memory.Memory
		threadID           uuid.NullUUID
		summary            sql.NullString
		attributesJSON     []byte
		supersedesMemoryID uuid.NullUUID
		lastObservedAt     sql.NullTime
	)

	if err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&record.AgentID,
		&threadID,
		&record.Kind,
		&record.Status,
		&record.Content,
		&summary,
		&attributesJSON,
		&record.Importance,
		&record.Confidence,
		&supersedesMemoryID,
		&lastObservedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
		distance,
	); err != nil {
		return memory.Memory{}, fmt.Errorf("scan memory with distance: %w", err)
	}

	if threadID.Valid {
		record.ThreadID = &threadID.UUID
	}
	if summary.Valid {
		summaryText := summary.String
		record.Summary = &summaryText
	}
	if supersedesMemoryID.Valid {
		record.SupersedesMemoryID = &supersedesMemoryID.UUID
	}
	if lastObservedAt.Valid {
		lastObserved := lastObservedAt.Time
		record.LastObservedAt = &lastObserved
	}
	attributes, err := unmarshalJSONMap(attributesJSON)
	if err != nil {
		return memory.Memory{}, err
	}
	record.Attributes = attributes
	return record, nil
}
