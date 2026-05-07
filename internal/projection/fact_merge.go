package projection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

var whitespaceRe = regexp.MustCompile(`\s+`)

// FactMergeTransformer builds one durable fact memory per logical fact key and
// accumulates repeated supporting observations over time.
//
// Episodes opt in by including either:
//   - fact_merge: true
//   - fact_key: "<stable-logical-key>"
//
// Optional payload fields:
//   - fact_content (string)
//   - fact_summary (string)
//   - fact_kind (string, default "fact_merge")
//   - fact_status (string, default "active")
//   - fact_embed_text (string)
//   - fact_attributes (object)
//   - fact_importance (number, default 0.6)
type FactMergeTransformer struct{}

type factAccumulator struct {
	record          memory.ProjectedMemory
	firstObservedAt time.Time
	lastObservedAt  time.Time
	count           int
}

// Transform converts opt-in episodes into mergeable projected fact memories.
func (FactMergeTransformer) Transform(_ context.Context, episodes []memory.Episode) ([]memory.ProjectedMemory, error) {
	byKey := make(map[string]*factAccumulator)

	for _, episode := range episodes {
		if hasSnapshotPayload(episode.Payload) {
			continue
		}
		if !factMergeEnabled(episode.Payload) {
			continue
		}

		content := strings.TrimSpace(episode.Content)
		if value, ok := stringPayloadField(episode.Payload, "fact_content"); ok && strings.TrimSpace(value) != "" {
			content = strings.TrimSpace(value)
		}
		if content == "" {
			continue
		}

		kind := "fact_merge"
		if value, ok := stringPayloadField(episode.Payload, "fact_kind"); ok && strings.TrimSpace(value) != "" {
			kind = strings.TrimSpace(value)
		}
		status := "active"
		if value, ok := stringPayloadField(episode.Payload, "fact_status"); ok && strings.TrimSpace(value) != "" {
			status = strings.TrimSpace(value)
		}

		summary := truncateSummary(content, 160)
		if value, ok := stringPayloadField(episode.Payload, "fact_summary"); ok && strings.TrimSpace(value) != "" {
			summary = strings.TrimSpace(value)
		}

		importance := 0.6
		if value, ok := floatPayloadField(episode.Payload, "fact_importance"); ok {
			importance = value
		}

		normalized := normalizeFactContent(content)
		if normalized == "" {
			continue
		}
		key := factLogicalKey(episode, explicitOrDerivedFactKey(episode, normalized))

		observedAt := episode.CreatedAt
		if episode.OccurredAt != nil {
			observedAt = *episode.OccurredAt
		}

		attributes := map[string]any{
			"normalized_content": normalized,
			"source_kind":        episode.Kind,
		}
		if explicitKey, ok := stringPayloadField(episode.Payload, "fact_key"); ok && strings.TrimSpace(explicitKey) != "" {
			attributes["fact_key"] = strings.TrimSpace(explicitKey)
		}
		if extra, ok := mapPayloadField(episode.Payload, "fact_attributes"); ok {
			for key, value := range extra {
				attributes[key] = value
			}
		}

		record := memory.ProjectedMemory{
			Key:           key,
			TenantID:      episode.TenantID,
			AgentID:       episode.AgentID,
			ThreadID:      episode.ThreadID,
			Kind:          kind,
			Status:        status,
			MergeStrategy: "accumulate",
			Content:       content,
			Summary:       &summary,
			Attributes:    attributes,
			Importance:    importance,
			Confidence:    confidenceFromObservationCount(1),
			LastObservedAt: func() *time.Time {
				value := observedAt
				return &value
			}(),
			SourceEpisodeIDs: []uuid.UUID{episode.ID},
			EmbedText:        factEmbedText(episode, content),
		}
		if value, ok := stringPayloadField(episode.Payload, "fact_embed_text"); ok && strings.TrimSpace(value) != "" {
			record.EmbedText = strings.TrimSpace(value)
		}

		existing, exists := byKey[key]
		if !exists {
			byKey[key] = &factAccumulator{
				record:          record,
				firstObservedAt: observedAt,
				lastObservedAt:  observedAt,
				count:           1,
			}
			continue
		}

		existing.record.SourceEpisodeIDs = append(existing.record.SourceEpisodeIDs, episode.ID)
		existing.count++
		if observedAt.Before(existing.firstObservedAt) {
			existing.firstObservedAt = observedAt
		}
		if observedAt.After(existing.lastObservedAt) {
			existing.lastObservedAt = observedAt
			existing.record.Content = content
			existing.record.Summary = &summary
			existing.record.Status = status
		}
		for attrKey, attrValue := range attributes {
			existing.record.Attributes[attrKey] = attrValue
		}
		if importance > existing.record.Importance {
			existing.record.Importance = importance
		}
		if existing.record.LastObservedAt == nil || observedAt.After(*existing.record.LastObservedAt) {
			observedAtCopy := observedAt
			existing.record.LastObservedAt = &observedAtCopy
		}
		existing.record.Confidence = confidenceFromObservationCount(existing.count)
	}

	records := make([]memory.ProjectedMemory, 0, len(byKey))
	for _, entry := range byKey {
		first := entry.firstObservedAt.UTC().Format(time.RFC3339)
		last := entry.lastObservedAt.UTC().Format(time.RFC3339)
		entry.record.SourceEpisodeIDs = dedupeUUIDs(entry.record.SourceEpisodeIDs)
		entry.record.Attributes["observation_count"] = entry.count
		entry.record.Attributes["first_observed_at"] = first
		entry.record.Attributes["last_observed_at"] = last
		entry.record.Confidence = confidenceFromObservationCount(entry.count)
		records = append(records, entry.record)
	}

	return records, nil
}

func factMergeEnabled(payload map[string]any) bool {
	if explicitKey, ok := stringPayloadField(payload, "fact_key"); ok && strings.TrimSpace(explicitKey) != "" {
		return true
	}
	if payload == nil {
		return false
	}
	value, ok := payload["fact_merge"]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func hasSnapshotPayload(payload map[string]any) bool {
	if payload == nil {
		return false
	}
	_, ok := payload["snapshot_key"]
	return ok
}

func explicitOrDerivedFactKey(episode memory.Episode, normalized string) string {
	if explicitKey, ok := stringPayloadField(episode.Payload, "fact_key"); ok && strings.TrimSpace(explicitKey) != "" {
		return strings.TrimSpace(explicitKey)
	}
	hash := sha256.Sum256([]byte(episode.Kind + ":" + normalized))
	return episode.Kind + ":" + hex.EncodeToString(hash[:])
}

func factLogicalKey(episode memory.Episode, key string) string {
	if episode.ThreadID != nil {
		return fmt.Sprintf("thread:%s:%s", episode.ThreadID.String(), key)
	}
	return fmt.Sprintf("agent:%s:%s", episode.AgentID.String(), key)
}

func normalizeFactContent(content string) string {
	normalized := strings.ToLower(strings.TrimSpace(content))
	normalized = whitespaceRe.ReplaceAllString(normalized, " ")
	return normalized
}

func confidenceFromObservationCount(count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(count) / float64(count+2)
}

func factEmbedText(episode memory.Episode, content string) string {
	return fmt.Sprintf("%s %s", episode.Kind, content)
}
