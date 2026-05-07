package cockroach

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func (s *Store) mergeProjectedMemoryIfNeeded(ctx context.Context, tx pgx.Tx, memoryID uuid.UUID, record memory.ProjectedMemory) (memory.ProjectedMemory, error) {
	if record.MergeStrategy != "accumulate" {
		return record, nil
	}

	existing, err := getMemoryByIDTx(ctx, tx, memoryID)
	if err != nil {
		if errors.Is(err, memory.ErrMemoryNotFound) {
			return record, nil
		}
		return memory.ProjectedMemory{}, err
	}

	existingCount := intAttr(existing.Attributes, "observation_count", 0)
	newCount := intAttr(record.Attributes, "observation_count", 1)
	totalCount := existingCount + newCount

	firstObservedAt := earliestTime(
		parseAttrTime(existing.Attributes, "first_observed_at"),
		existing.LastObservedAt,
		parseAttrTime(record.Attributes, "first_observed_at"),
		record.LastObservedAt,
	)
	lastObservedAt := latestTime(
		parseAttrTime(existing.Attributes, "last_observed_at"),
		existing.LastObservedAt,
		parseAttrTime(record.Attributes, "last_observed_at"),
		record.LastObservedAt,
	)

	mergedAttributes := make(map[string]any, len(existing.Attributes)+len(record.Attributes)+3)
	for key, value := range existing.Attributes {
		mergedAttributes[key] = value
	}
	for key, value := range record.Attributes {
		mergedAttributes[key] = value
	}
	mergedAttributes["observation_count"] = totalCount
	if firstObservedAt != nil {
		mergedAttributes["first_observed_at"] = firstObservedAt.UTC().Format(time.RFC3339)
	}
	if lastObservedAt != nil {
		mergedAttributes["last_observed_at"] = lastObservedAt.UTC().Format(time.RFC3339)
	}

	record.Attributes = mergedAttributes
	record.Importance = maxFloat64(existing.Importance, record.Importance)
	record.Confidence = confidenceFromObservationCount(totalCount)
	record.LastObservedAt = lastObservedAt
	record.SourceEpisodeIDs = dedupeUUIDs(record.SourceEpisodeIDs)
	return record, nil
}

func getMemoryByIDTx(ctx context.Context, tx pgx.Tx, memoryID uuid.UUID) (memory.Memory, error) {
	record, err := scanMemory(tx.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, thread_id, kind, status, content, summary, attributes,
		        importance, confidence, supersedes_memory_id, last_observed_at, created_at, updated_at
		 FROM memories
		 WHERE id = $1`,
		memoryID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return memory.Memory{}, memory.ErrMemoryNotFound
		}
		return memory.Memory{}, fmt.Errorf("get memory by id in tx: %w", err)
	}
	return record, nil
}

func intAttr(attrs map[string]any, key string, fallback int) int {
	if attrs == nil {
		return fallback
	}
	value, ok := attrs[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return fallback
	}
}

func parseAttrTime(attrs map[string]any, key string) *time.Time {
	if attrs == nil {
		return nil
	}
	value, ok := attrs[key]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil
	}
	return &parsed
}

func earliestTime(values ...*time.Time) *time.Time {
	var best *time.Time
	for _, value := range values {
		if value == nil {
			continue
		}
		if best == nil || value.Before(*best) {
			copy := *value
			best = &copy
		}
	}
	return best
}

func latestTime(values ...*time.Time) *time.Time {
	var best *time.Time
	for _, value := range values {
		if value == nil {
			continue
		}
		if best == nil || value.After(*best) {
			copy := *value
			best = &copy
		}
	}
	return best
}

func maxFloat64(left, right float64) float64 {
	if right > left {
		return right
	}
	return left
}

func confidenceFromObservationCount(count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(count) / float64(count+2)
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	result := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
