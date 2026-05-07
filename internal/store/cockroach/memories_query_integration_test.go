//go:build integration

package cockroach_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestQueryMemoriesFilters(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	agentA, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-a",
	})
	if err != nil {
		t.Fatalf("CreateAgent(agentA) error = %v", err)
	}
	agentB, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-b",
	})
	if err != nil {
		t.Fatalf("CreateAgent(agentB) error = %v", err)
	}
	threadA, err := store.CreateThread(ctx, memory.CreateThreadRequest{
		TenantID: tenant.ID,
		AgentID:  agentA.ID,
	})
	if err != nil {
		t.Fatalf("CreateThread(threadA) error = %v", err)
	}

	episodeA, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agentA.ID,
		ThreadID: &threadA.ID,
		Kind:     "study.note",
		Content:  "agent A note",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(episodeA) error = %v", err)
	}
	episodeB, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agentB.ID,
		Kind:     "study.break",
		Content:  "agent B break",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(episodeB) error = %v", err)
	}

	if err := store.UpsertEpisodeMemories(ctx, "episode-digests", uuid.New(), []memory.Episode{episodeA, episodeB}); err != nil {
		t.Fatalf("UpsertEpisodeMemories() error = %v", err)
	}

	all, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
	})
	if err != nil {
		t.Fatalf("QueryMemories(all) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}

	filteredByAgent, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		AgentID:  &agentA.ID,
	})
	if err != nil {
		t.Fatalf("QueryMemories(filteredByAgent) error = %v", err)
	}
	if len(filteredByAgent) != 1 || filteredByAgent[0].AgentID != agentA.ID {
		t.Fatalf("unexpected agent-filtered memories: %+v", filteredByAgent)
	}

	filteredByThread, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		ThreadID: &threadA.ID,
		Kind:     "episode_digest",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("QueryMemories(filteredByThread) error = %v", err)
	}
	if len(filteredByThread) != 1 || filteredByThread[0].ThreadID == nil || *filteredByThread[0].ThreadID != threadA.ID {
		t.Fatalf("unexpected thread-filtered memories: %+v", filteredByThread)
	}
}
