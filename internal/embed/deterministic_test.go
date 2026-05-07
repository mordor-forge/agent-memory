package embed

import (
	"context"
	"testing"
)

func TestDeterministicEmbedderRepeatable(t *testing.T) {
	t.Parallel()

	embedder, err := NewDeterministicEmbedder(8)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}

	first, err := embedder.Embed(context.Background(), []string{"cockroach memory"})
	if err != nil {
		t.Fatalf("Embed(first) error = %v", err)
	}
	second, err := embedder.Embed(context.Background(), []string{"cockroach memory"})
	if err != nil {
		t.Fatalf("Embed(second) error = %v", err)
	}

	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected vector counts: %d %d", len(first), len(second))
	}
	if len(first[0]) != 8 {
		t.Fatalf("len(vector) = %d, want 8", len(first[0]))
	}
	for i := range first[0] {
		if first[0][i] != second[0][i] {
			t.Fatalf("vectors differ at %d: %f != %f", i, first[0][i], second[0][i])
		}
	}
}
