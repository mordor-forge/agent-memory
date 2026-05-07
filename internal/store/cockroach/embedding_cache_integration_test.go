//go:build integration

package cockroach_test

import (
	"context"
	"testing"

	"github.com/mordor-forge/agent-memory/internal/embed"
)

func TestEmbeddingCacheRoundTrip(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	vector, err := embedder.Embed(ctx, []string{"cached text"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	hash := "test-hash"
	if err := store.PutCachedEmbedding(ctx, "deterministic", embedder.Model(), hash, "cached text", vector[0]); err != nil {
		t.Fatalf("PutCachedEmbedding() error = %v", err)
	}
	got, found, err := store.GetCachedEmbedding(ctx, "deterministic", embedder.Model(), hash)
	if err != nil {
		t.Fatalf("GetCachedEmbedding() error = %v", err)
	}
	if !found {
		t.Fatal("expected cached embedding to be found")
	}
	if len(got) != len(vector[0]) || got[0] != vector[0][0] {
		t.Fatalf("unexpected cached vector: %+v", got)
	}
}
