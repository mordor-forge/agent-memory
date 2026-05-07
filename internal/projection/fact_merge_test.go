package projection

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestFactMergeTransformerAggregatesNormalizedContent(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	agentID := uuid.New()
	threadID := uuid.New()
	now := time.Now().UTC()

	transformer := FactMergeTransformer{}
	records, err := transformer.Transform(context.Background(), []memory.Episode{
		{
			ID:        uuid.New(),
			TenantID:  tenantID,
			AgentID:   agentID,
			ThreadID:  &threadID,
			Kind:      "observation",
			Content:   "Hello   World",
			Payload:   map[string]any{"fact_merge": true},
			CreatedAt: now.Add(-time.Minute),
		},
		{
			ID:        uuid.New(),
			TenantID:  tenantID,
			AgentID:   agentID,
			ThreadID:  &threadID,
			Kind:      "observation",
			Content:   "hello world",
			Payload:   map[string]any{"fact_merge": true},
			CreatedAt: now,
		},
		{
			ID:        uuid.New(),
			TenantID:  tenantID,
			AgentID:   agentID,
			ThreadID:  &threadID,
			Kind:      "state.update",
			Content:   "resume later",
			Payload:   map[string]any{"snapshot_key": "resume_state", "fact_merge": true},
			CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}

	record := records[0]
	if record.Kind != "fact_merge" || record.MergeStrategy != "accumulate" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Attributes["observation_count"] != 2 {
		t.Fatalf("observation_count = %v, want 2", record.Attributes["observation_count"])
	}
	if record.Attributes["normalized_content"] != "hello world" {
		t.Fatalf("normalized_content = %v, want %q", record.Attributes["normalized_content"], "hello world")
	}
	if len(record.SourceEpisodeIDs) != 2 {
		t.Fatalf("source episode ids = %v, want 2 entries", record.SourceEpisodeIDs)
	}
}
