//go:build integration

package cockroach_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestSearchMemoriesReturnsNearestHit(t *testing.T) {
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
		Kind:     "study.note",
		Content:  "CockroachDB uses serializable transactions",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(episodeA) error = %v", err)
	}
	episodeB, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.note",
		Content:  "NotebookLM is useful for book-driven studying",
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
	if err := store.UpsertEpisodeMemoriesWithEmbeddings(ctx, "episode-digests", uuid.New(), []memory.Episode{episodeA, episodeB}, vectors, embedder.Provider(), embedder.Model()); err != nil {
		t.Fatalf("UpsertEpisodeMemoriesWithEmbeddings() error = %v", err)
	}

	queryVector, err := embedder.Embed(ctx, []string{episodeB.Content})
	if err != nil {
		t.Fatalf("Embed(query) error = %v", err)
	}
	hits, err := store.SearchMemories(ctx, memory.RecallRequest{
		TenantID: tenant.ID,
		Query:    episodeB.Content,
		Limit:    2,
	}, queryVector[0])
	if err != nil {
		t.Fatalf("SearchMemories() error = %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("len(hits) = %d, want 2", len(hits))
	}
	if hits[0].Memory.Content != episodeB.Content {
		t.Fatalf("top hit content = %q, want %q", hits[0].Memory.Content, episodeB.Content)
	}
	if hits[0].Distance > hits[1].Distance {
		t.Fatalf("expected top hit distance <= second hit distance, got %f > %f", hits[0].Distance, hits[1].Distance)
	}
}
