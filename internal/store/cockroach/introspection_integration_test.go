//go:build integration

package cockroach_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestGetMemoryProvenance(t *testing.T) {
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

	episode, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "note",
		Content:  "CockroachDB retries are normal in distributed SQL",
	})
	if err != nil {
		t.Fatalf("AppendEpisode() error = %v", err)
	}

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	vectors, err := embedder.Embed(ctx, []string{episode.Content})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if err := store.UpsertEpisodeMemoriesWithEmbeddings(ctx, "episode-digests", uuid.New(), []memory.Episode{episode}, vectors, embedder.Provider(), embedder.Model()); err != nil {
		t.Fatalf("UpsertEpisodeMemoriesWithEmbeddings() error = %v", err)
	}

	memories, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{TenantID: tenant.ID, Limit: 10})
	if err != nil {
		t.Fatalf("QueryMemories() error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}

	provenance, err := store.GetMemoryProvenance(ctx, memories[0].ID)
	if err != nil {
		t.Fatalf("GetMemoryProvenance() error = %v", err)
	}
	if provenance.ProjectionName != "episode-digests" {
		t.Fatalf("ProjectionName = %q, want %q", provenance.ProjectionName, "episode-digests")
	}
	if !provenance.HasEmbedding {
		t.Fatal("expected HasEmbedding = true")
	}
	if len(provenance.Sources) != 1 || provenance.Sources[0].EpisodeID != episode.ID {
		t.Fatalf("unexpected sources: %+v", provenance.Sources)
	}
}

func TestListProjectionStateAndLeases(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	cursor := memory.CheckpointCursor{
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		ID:        uuid.New(),
	}
	if err := store.AdvanceProjectionCheckpoint(ctx, "episode-digests", tenant.ID, 0, cursor); err != nil {
		t.Fatalf("AdvanceProjectionCheckpoint() error = %v", err)
	}

	checkpoints, err := store.ListProjectionCheckpoints(ctx, memory.ListProjectionCheckpointsRequest{
		TenantID: tenant.ID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListProjectionCheckpoints() error = %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].ProjectionName != "episode-digests" {
		t.Fatalf("unexpected checkpoints: %+v", checkpoints)
	}

	holderID := uuid.New()
	acquired, err := store.TryAcquireLease(ctx, "projection:episode-digests:"+tenant.ID.String()+":0", holderID, time.Minute)
	if err != nil {
		t.Fatalf("TryAcquireLease() error = %v", err)
	}
	if !acquired {
		t.Fatal("expected lease acquisition to succeed")
	}

	leases, err := store.ListWorkerLeases(ctx, memory.ListWorkerLeasesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerLeases() error = %v", err)
	}
	if len(leases) == 0 {
		t.Fatal("expected at least one lease")
	}

	runs, err := store.ListConsolidationRuns(ctx, memory.ListConsolidationRunsRequest{
		TenantID: tenant.ID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListConsolidationRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no runs for this tenant yet, got %+v", runs)
	}
}

func TestListConsolidationRunsIncludesProjectionAndBatchBounds(t *testing.T) {
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

	episodeA, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "note",
		Content:  "Projection runs should capture the first processed episode",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(episodeA) error = %v", err)
	}
	episodeB, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "note",
		Content:  "Projection runs should capture the last processed episode",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(episodeB) error = %v", err)
	}

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	vectors, err := embedder.Embed(ctx, []string{episodeA.Content, episodeB.Content})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	workerID := uuid.New()
	if err := store.UpsertEpisodeMemoriesWithEmbeddings(ctx, "episode-digests", workerID, []memory.Episode{episodeA, episodeB}, vectors, embedder.Provider(), embedder.Model()); err != nil {
		t.Fatalf("UpsertEpisodeMemoriesWithEmbeddings() error = %v", err)
	}

	runs, err := store.ListConsolidationRuns(ctx, memory.ListConsolidationRunsRequest{
		TenantID:       tenant.ID,
		ProjectionName: "episode-digests",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListConsolidationRuns() error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.ProjectionName != "episode-digests" {
		t.Fatalf("ProjectionName = %q, want %q", run.ProjectionName, "episode-digests")
	}
	if run.Provider != embedder.Provider() {
		t.Fatalf("Provider = %q, want %q", run.Provider, embedder.Provider())
	}
	if run.Model != embedder.Model() {
		t.Fatalf("Model = %q, want %q", run.Model, embedder.Model())
	}
	if run.WorkerID != workerID {
		t.Fatalf("WorkerID = %s, want %s", run.WorkerID, workerID)
	}
	if run.InputFrom == nil || run.InputFrom.ID != episodeA.ID || !run.InputFrom.CreatedAt.Equal(episodeA.CreatedAt) {
		t.Fatalf("InputFrom = %+v, want episodeA cursor", run.InputFrom)
	}
	if run.InputTo == nil || run.InputTo.ID != episodeB.ID || !run.InputTo.CreatedAt.Equal(episodeB.CreatedAt) {
		t.Fatalf("InputTo = %+v, want episodeB cursor", run.InputTo)
	}
	if run.BatchStartedAt.IsZero() {
		t.Fatal("expected BatchStartedAt to be set")
	}
	if run.CompletedAt == nil || run.CompletedAt.IsZero() {
		t.Fatalf("CompletedAt = %+v, want non-zero time", run.CompletedAt)
	}
}
