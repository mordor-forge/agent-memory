package projection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// StateSnapshotTransformer builds one latest-state memory per logical snapshot key.
//
// Episodes opt in by including a payload field:
//   - snapshot_key (string, required)
//
// Optional payload fields:
//   - snapshot_content (string)
//   - snapshot_summary (string)
//   - snapshot_kind (string, default "state_snapshot")
//   - snapshot_status (string, default "active")
//   - snapshot_embed_text (string)
//   - snapshot_importance (number, default 0.7)
//   - snapshot_confidence (number, default 1.0)
//   - snapshot_attributes (object)
type StateSnapshotTransformer struct{}

type snapshotAccumulator struct {
	record memory.ProjectedMemory
	cursor memory.CheckpointCursor
}

// Transform aggregates snapshot episodes down to one latest-state memory per logical key.
func (StateSnapshotTransformer) Transform(_ context.Context, episodes []memory.Episode) ([]memory.ProjectedMemory, error) {
	byKey := make(map[string]*snapshotAccumulator)

	for _, episode := range episodes {
		rawKey, ok := stringPayloadField(episode.Payload, "snapshot_key")
		if !ok || strings.TrimSpace(rawKey) == "" {
			continue
		}

		logicalKey := snapshotLogicalKey(episode, rawKey)
		content := episode.Content
		if value, ok := stringPayloadField(episode.Payload, "snapshot_content"); ok && strings.TrimSpace(value) != "" {
			content = value
		}
		summary := truncateSummary(content, 160)
		if value, ok := stringPayloadField(episode.Payload, "snapshot_summary"); ok && strings.TrimSpace(value) != "" {
			summary = value
		}
		kind := "state_snapshot"
		if value, ok := stringPayloadField(episode.Payload, "snapshot_kind"); ok && strings.TrimSpace(value) != "" {
			kind = value
		}
		status := "active"
		if value, ok := stringPayloadField(episode.Payload, "snapshot_status"); ok && strings.TrimSpace(value) != "" {
			status = value
		}
		importance := 0.7
		if value, ok := floatPayloadField(episode.Payload, "snapshot_importance"); ok {
			importance = value
		}
		confidence := 1.0
		if value, ok := floatPayloadField(episode.Payload, "snapshot_confidence"); ok {
			confidence = value
		}

		attributes := map[string]any{
			"snapshot_key": rawKey,
			"source_kind":  episode.Kind,
		}
		if extra, ok := mapPayloadField(episode.Payload, "snapshot_attributes"); ok {
			for key, value := range extra {
				attributes[key] = value
			}
		}

		lastObservedAt := episode.CreatedAt
		if episode.OccurredAt != nil {
			lastObservedAt = *episode.OccurredAt
		}
		record := memory.ProjectedMemory{
			Key:        logicalKey,
			TenantID:   episode.TenantID,
			AgentID:    episode.AgentID,
			ThreadID:   episode.ThreadID,
			Kind:       kind,
			Status:     status,
			Content:    content,
			Summary:    &summary,
			Attributes: attributes,
			Importance: importance,
			Confidence: confidence,
			LastObservedAt: func() *time.Time {
				value := lastObservedAt
				return &value
			}(),
			SourceEpisodeIDs: []uuid.UUID{episode.ID},
			EmbedText:        embedTextForSnapshot(episode, content),
		}
		if value, ok := stringPayloadField(episode.Payload, "snapshot_embed_text"); ok && strings.TrimSpace(value) != "" {
			record.EmbedText = value
		}

		cursor := memory.CheckpointCursor{
			CreatedAt: episode.CreatedAt,
			ID:        episode.ID,
		}
		existing, exists := byKey[logicalKey]
		if !exists {
			byKey[logicalKey] = &snapshotAccumulator{
				record: record,
				cursor: cursor,
			}
			continue
		}

		existing.record.SourceEpisodeIDs = append(existing.record.SourceEpisodeIDs, episode.ID)
		if cursorAfter(cursor, existing.cursor) {
			record.SourceEpisodeIDs = dedupeUUIDs(existing.record.SourceEpisodeIDs)
			existing.record = record
			existing.cursor = cursor
			continue
		}
		existing.record.SourceEpisodeIDs = dedupeUUIDs(existing.record.SourceEpisodeIDs)
	}

	records := make([]memory.ProjectedMemory, 0, len(byKey))
	for _, entry := range byKey {
		entry.record.SourceEpisodeIDs = dedupeUUIDs(entry.record.SourceEpisodeIDs)
		records = append(records, entry.record)
	}
	return records, nil
}

func snapshotLogicalKey(episode memory.Episode, rawKey string) string {
	if episode.ThreadID != nil {
		return fmt.Sprintf("thread:%s:%s", episode.ThreadID.String(), rawKey)
	}
	return fmt.Sprintf("agent:%s:%s", episode.AgentID.String(), rawKey)
}

func embedTextForSnapshot(episode memory.Episode, content string) string {
	return fmt.Sprintf("%s %s", episode.Kind, content)
}

func stringPayloadField(payload map[string]any, key string) (string, bool) {
	if payload == nil {
		return "", false
	}
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func mapPayloadField(payload map[string]any, key string) (map[string]any, bool) {
	if payload == nil {
		return nil, false
	}
	value, ok := payload[key]
	if !ok {
		return nil, false
	}
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

func floatPayloadField(payload map[string]any, key string) (float64, bool) {
	if payload == nil {
		return 0, false
	}
	value, ok := payload[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func cursorAfter(left, right memory.CheckpointCursor) bool {
	if left.CreatedAt.After(right.CreatedAt) {
		return true
	}
	if left.CreatedAt.Equal(right.CreatedAt) && left.ID.String() > right.ID.String() {
		return true
	}
	return false
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
