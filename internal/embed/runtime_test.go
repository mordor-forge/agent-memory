package embed

import (
	"context"
	"errors"
	"testing"

	"github.com/mordor-forge/agent-memory/internal/config"
)

type fakeBaseEmbedder struct {
	provider   string
	model      string
	dimensions int
	calls      int
	texts      []string
	vectors    [][]float32
	err        error
}

func (f *fakeBaseEmbedder) Provider() string { return f.provider }
func (f *fakeBaseEmbedder) Model() string    { return f.model }
func (f *fakeBaseEmbedder) Dimensions() int  { return f.dimensions }
func (f *fakeBaseEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	f.texts = append([]string(nil), texts...)
	if f.err != nil {
		return nil, f.err
	}
	return f.vectors, nil
}

type fakeCacheStore struct {
	items    map[string][]float32
	putCalls int
	getCalls int
	putErr   error
	getErr   error
}

func (f *fakeCacheStore) GetCachedEmbedding(_ context.Context, provider, model, textHash string) ([]float32, bool, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	vector, ok := f.items[key(provider, model, textHash)]
	return vector, ok, nil
}

func (f *fakeCacheStore) PutCachedEmbedding(_ context.Context, provider, model, textHash, _ string, embedding []float32) error {
	f.putCalls++
	if f.putErr != nil {
		return f.putErr
	}
	if f.items == nil {
		f.items = make(map[string][]float32)
	}
	f.items[key(provider, model, textHash)] = append([]float32(nil), embedding...)
	return nil
}

func TestCachedEmbedderUsesCacheAndDedupesMisses(t *testing.T) {
	t.Parallel()

	base := &fakeBaseEmbedder{
		provider:   "deterministic",
		model:      "test-model",
		dimensions: 2,
		vectors: [][]float32{
			{0.1, 0.2},
		},
	}
	cache := &fakeCacheStore{
		items: map[string][]float32{
			key("deterministic", "test-model", textHash("cached")): {0.9, 0.8},
		},
	}
	embedder := &cachedEmbedder{
		provider: "deterministic",
		inner:    base,
		cache:    cache,
	}

	results, err := embedder.Embed(context.Background(), []string{"cached", "miss", "miss"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if base.calls != 1 {
		t.Fatalf("base calls = %d, want 1", base.calls)
	}
	if len(base.texts) != 1 || base.texts[0] != "miss" {
		t.Fatalf("base texts = %v, want [miss]", base.texts)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache put calls = %d, want 1", cache.putCalls)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	if results[0][0] != 0.9 || results[1][0] != 0.1 || results[2][0] != 0.1 {
		t.Fatalf("unexpected vectors: %+v", results)
	}
}

func TestObservedEmbedderPropagatesError(t *testing.T) {
	t.Parallel()

	base := &fakeBaseEmbedder{
		provider:   "deterministic",
		model:      "test-model",
		dimensions: 2,
		err:        errors.New("boom"),
	}
	embedder := &observedEmbedder{
		provider: "deterministic",
		inner:    base,
	}

	if _, err := embedder.Embed(context.Background(), []string{"x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCachedEmbedderSplitsLargeMissBatch(t *testing.T) {
	t.Parallel()

	base := &fakeBaseEmbedder{
		provider:   "deterministic",
		model:      "test-model",
		dimensions: 2,
		vectors: [][]float32{
			{0.1, 0.2},
			{0.3, 0.4},
		},
	}
	cache := &fakeCacheStore{}
	embedder := &cachedEmbedder{
		provider: "deterministic",
		inner:    base,
		cache:    cache,
		maxBatch: 2,
	}

	_, err := embedder.Embed(context.Background(), []string{"a", "b", "c", "d"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if base.calls != 2 {
		t.Fatalf("base calls = %d, want 2", base.calls)
	}
	if cache.putCalls != 4 {
		t.Fatalf("cache put calls = %d, want 4", cache.putCalls)
	}
}

func TestNewRuntimeEmbedderUsesFactory(t *testing.T) {
	t.Parallel()

	embedder, err := NewRuntimeEmbedder(config.Config{
		EmbeddingProvider:   "deterministic",
		EmbeddingDimensions: 8,
	}, nil)
	if err != nil {
		t.Fatalf("NewRuntimeEmbedder() error = %v", err)
	}
	if embedder.Model() != "deterministic/8" {
		t.Fatalf("Model() = %q", embedder.Model())
	}
	if embedder.Provider() != "deterministic" {
		t.Fatalf("Provider() = %q", embedder.Provider())
	}
}

func key(provider, model, hash string) string {
	return provider + "|" + model + "|" + hash
}
