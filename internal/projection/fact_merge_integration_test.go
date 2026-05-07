//go:build integration

package projection_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/projection"
	storepkg "github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/internal/worker"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestFactMergeProjectionAccumulatesAcrossWorkerPasses(t *testing.T) {
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

	appendFact := func(content string) {
		t.Helper()
		_, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
			TenantID: tenant.ID,
			AgentID:  agent.ID,
			ThreadID: &thread.ID,
			Kind:     "observation",
			Content:  content,
			Payload: map[string]any{
				"fact_merge": true,
			},
		})
		if err != nil {
			t.Fatalf("AppendEpisode(%q) error = %v", content, err)
		}
	}

	appendFact("CockroachDB retries are normal in distributed SQL")
	time.Sleep(10 * time.Millisecond)
	appendFact("cockroachdb   retries are normal in distributed sql")

	if err := runWorkerOnce(ctx, store); err != nil {
		t.Fatalf("runWorkerOnce(first) error = %v", err)
	}

	memories, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		ThreadID: &thread.ID,
		Kind:     "fact_merge",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("QueryMemories(first) error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].Attributes["observation_count"] != float64(2) {
		t.Fatalf("observation_count = %v, want 2", memories[0].Attributes["observation_count"])
	}

	appendFact("COCKROACHDB retries are normal in distributed SQL")
	if err := runWorkerOnce(ctx, store); err != nil {
		t.Fatalf("runWorkerOnce(second) error = %v", err)
	}

	memories, err = store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		ThreadID: &thread.ID,
		Kind:     "fact_merge",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("QueryMemories(second) error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("len(memories) = %d, want 1", len(memories))
	}
	if memories[0].Attributes["observation_count"] != float64(3) {
		t.Fatalf("observation_count = %v, want 3", memories[0].Attributes["observation_count"])
	}

	provenance, err := store.GetMemoryProvenance(ctx, memories[0].ID)
	if err != nil {
		t.Fatalf("GetMemoryProvenance() error = %v", err)
	}
	if len(provenance.Sources) != 3 {
		t.Fatalf("len(provenance.Sources) = %d, want 3", len(provenance.Sources))
	}
}

func runWorkerOnce(ctx context.Context, store *storepkg.Store) error {
	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		return err
	}
	digestProjector, err := projection.NewEpisodeDigestProjector("episode-digests", store, embedder)
	if err != nil {
		return err
	}
	digestEngine, err := projection.NewEngine("episode-digests", store, digestProjector, 100, time.Minute)
	if err != nil {
		return err
	}
	snapshotProjector, err := projection.NewMemoryProjector("state-snapshots", store, embedder, projection.StateSnapshotTransformer{})
	if err != nil {
		return err
	}
	snapshotEngine, err := projection.NewEngine("state-snapshots", store, snapshotProjector, 100, time.Minute)
	if err != nil {
		return err
	}
	factProjector, err := projection.NewMemoryProjector("fact-merges", store, embedder, projection.FactMergeTransformer{})
	if err != nil {
		return err
	}
	factEngine, err := projection.NewEngine("fact-merges", store, factProjector, 100, time.Minute)
	if err != nil {
		return err
	}
	runner, err := worker.NewRunner(uuid.New(), time.Minute, 100, nil, store, digestEngine, snapshotEngine, factEngine)
	if err != nil {
		return err
	}
	return runner.RunOnce(ctx)
}
