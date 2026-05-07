//go:build integration

package projection_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/projection"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestStateSnapshotProjectionMaintainsLatestStatePerLogicalKey(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	agent, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}
	thread, err := store.CreateThread(ctx, memory.CreateThreadRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}

	if _, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		ThreadID: &thread.ID,
		Kind:     "state.update",
		Content:  "resume in practice",
		Payload: map[string]any{
			"snapshot_key":     "resume_state",
			"snapshot_content": "resume in practice",
		},
	}); err != nil {
		t.Fatalf("AppendEpisode(first) error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		ThreadID: &thread.ID,
		Kind:     "state.update",
		Content:  "resume in review",
		Payload: map[string]any{
			"snapshot_key":     "resume_state",
			"snapshot_content": "resume in review",
			"snapshot_summary": "latest resume state",
		},
	}); err != nil {
		t.Fatalf("AppendEpisode(second) error = %v", err)
	}

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	projector, err := projection.NewMemoryProjector("state-snapshots", store, embedder, projection.StateSnapshotTransformer{})
	if err != nil {
		t.Fatalf("NewMemoryProjector() error = %v", err)
	}
	engine, err := projection.NewEngine("state-snapshots", store, projector, 100, time.Minute)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	result, err := engine.RunBatch(ctx, tenant.ID, 0, uuid.New())
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if !result.CheckpointAdvanced {
		t.Fatalf("unexpected result: %+v", result)
	}

	memories, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		ThreadID: &thread.ID,
		Kind:     "state_snapshot",
	})
	if err != nil {
		t.Fatalf("QueryMemories() error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].Content != "resume in review" {
		t.Fatalf("memory content = %q, want %q", memories[0].Content, "resume in review")
	}
	if memories[0].Summary == nil || *memories[0].Summary != "latest resume state" {
		t.Fatalf("memory summary = %+v", memories[0].Summary)
	}
	if memories[0].Attributes["snapshot_key"] != "resume_state" {
		t.Fatalf("memory attributes = %+v", memories[0].Attributes)
	}
}
