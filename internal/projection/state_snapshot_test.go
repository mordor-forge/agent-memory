package projection

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestStateSnapshotTransformerAggregatesLatestPerKey(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	agentID := uuid.New()
	threadID := uuid.New()
	firstID := uuid.New()
	secondID := uuid.New()
	transformer := StateSnapshotTransformer{}
	firstCreatedAt := time.Now().UTC().Add(-time.Minute)
	secondCreatedAt := time.Now().UTC()

	records, err := transformer.Transform(context.Background(), []memory.Episode{
		{
			ID:        firstID,
			TenantID:  tenantID,
			AgentID:   agentID,
			ThreadID:  &threadID,
			Kind:      "state.update",
			Content:   "phase: practicing",
			Payload:   map[string]any{"snapshot_key": "resume_state", "snapshot_content": "practice mode"},
			CreatedAt: firstCreatedAt,
		},
		{
			ID:        secondID,
			TenantID:  tenantID,
			AgentID:   agentID,
			ThreadID:  &threadID,
			Kind:      "state.update",
			Content:   "phase: reviewing",
			Payload:   map[string]any{"snapshot_key": "resume_state", "snapshot_content": "review mode", "snapshot_summary": "resume in review"},
			CreatedAt: secondCreatedAt,
		},
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != "state_snapshot" || record.Status != "active" {
		t.Fatalf("unexpected snapshot record: %+v", record)
	}
	if record.Content != "review mode" {
		t.Fatalf("content = %q, want %q", record.Content, "review mode")
	}
	if record.Summary == nil || *record.Summary != "resume in review" {
		t.Fatalf("summary = %+v, want %q", record.Summary, "resume in review")
	}
	if len(record.SourceEpisodeIDs) != 2 {
		t.Fatalf("source episode ids = %v, want 2 entries", record.SourceEpisodeIDs)
	}
	if record.Attributes["snapshot_key"] != "resume_state" {
		t.Fatalf("attributes = %+v", record.Attributes)
	}
	if record.LastObservedAt == nil || !record.LastObservedAt.Equal(secondCreatedAt) {
		t.Fatalf("last observed at = %+v, want %v", record.LastObservedAt, secondCreatedAt)
	}
}
