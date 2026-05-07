package embed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/observability"
)

// CacheStore is the persistence surface needed for optional embedding caching.
type CacheStore interface {
	GetCachedEmbedding(ctx context.Context, provider, model, textHash string) ([]float32, bool, error)
	PutCachedEmbedding(ctx context.Context, provider, model, textHash, text string, embedding []float32) error
}

// NewRuntimeEmbedder constructs a configured embedder wrapped with observability
// and optional Cockroach-backed caching.
func NewRuntimeEmbedder(cfg config.Config, cache CacheStore) (Embedder, error) {
	base, err := NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	providerName := cfg.EmbeddingProvider
	if providerName == "" {
		providerName = "deterministic"
	}
	observed := &observedEmbedder{
		provider: providerName,
		inner:    base,
	}
	if cache == nil {
		return observed, nil
	}
	return &cachedEmbedder{
		provider: providerName,
		inner:    observed,
		cache:    cache,
		maxBatch: cfg.EmbeddingMaxBatch,
	}, nil
}

type observedEmbedder struct {
	provider string
	inner    Embedder
}

func (e *observedEmbedder) Provider() string {
	return e.provider
}

func (e *observedEmbedder) Model() string {
	return e.inner.Model()
}

func (e *observedEmbedder) Dimensions() int {
	return e.inner.Dimensions()
}

func (e *observedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	start := time.Now()
	vectors, err := e.inner.Embed(ctx, texts)
	duration := time.Since(start)
	observability.RecordEmbedProviderRequest(e.provider, e.inner.Model(), len(texts), duration, err)
	if err != nil {
		slog.Default().Error("embedding provider request failed",
			slog.String("provider", e.provider),
			slog.String("model", e.inner.Model()),
			slog.Int("text_count", len(texts)),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("error", err.Error()),
		)
	} else {
		slog.Default().Debug("embedding provider request complete",
			slog.String("provider", e.provider),
			slog.String("model", e.inner.Model()),
			slog.Int("text_count", len(texts)),
			slog.Int64("duration_ms", duration.Milliseconds()),
		)
	}
	return vectors, err
}

type cachedEmbedder struct {
	provider string
	inner    Embedder
	cache    CacheStore
	maxBatch int
}

func (e *cachedEmbedder) Provider() string {
	return e.provider
}

func (e *cachedEmbedder) Model() string {
	return e.inner.Model()
}

func (e *cachedEmbedder) Dimensions() int {
	return e.inner.Dimensions()
}

func (e *cachedEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	type miss struct {
		text string
		hash string
	}

	results := make([][]float32, len(texts))
	cacheByHash := make(map[string][]float32, len(texts))
	missesByHash := make(map[string]miss)
	hits := 0
	misses := 0
	cacheErrors := 0
	writeErrors := 0

	for idx, text := range texts {
		hash := textHash(text)

		if vector, ok := cacheByHash[hash]; ok {
			observability.RecordEmbedCacheLookup(e.provider, e.inner.Model(), "hit")
			results[idx] = vector
			hits++
			continue
		}

		vector, found, err := e.cache.GetCachedEmbedding(ctx, e.provider, e.inner.Model(), hash)
		if err != nil {
			observability.RecordEmbedCacheLookup(e.provider, e.inner.Model(), "error")
			cacheErrors++
		} else if found {
			observability.RecordEmbedCacheLookup(e.provider, e.inner.Model(), "hit")
			cacheByHash[hash] = vector
			results[idx] = vector
			hits++
			continue
		} else {
			observability.RecordEmbedCacheLookup(e.provider, e.inner.Model(), "miss")
			misses++
		}

		if _, exists := missesByHash[hash]; !exists {
			missesByHash[hash] = miss{text: text, hash: hash}
		}
	}

	if len(missesByHash) == 0 {
		return results, nil
	}

	orderedMisses := make([]miss, 0, len(missesByHash))
	missTexts := make([]string, 0, len(missesByHash))
	for _, item := range missesByHash {
		orderedMisses = append(orderedMisses, item)
		missTexts = append(missTexts, item.text)
	}

	vectors, err := e.embedInChunks(ctx, missTexts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(orderedMisses) {
		return nil, fmt.Errorf("cache wrapper expected %d vectors, got %d", len(orderedMisses), len(vectors))
	}

	for i, item := range orderedMisses {
		cacheByHash[item.hash] = vectors[i]
		if err := e.cache.PutCachedEmbedding(ctx, e.provider, e.inner.Model(), item.hash, item.text, vectors[i]); err != nil {
			observability.RecordEmbedCacheLookup(e.provider, e.inner.Model(), "write_error")
			writeErrors++
		}
	}

	for idx, text := range texts {
		hash := textHash(text)
		vector, ok := cacheByHash[hash]
		if !ok {
			return nil, fmt.Errorf("cache wrapper missing vector for input index %d", idx)
		}
		results[idx] = vector
	}
	slog.Default().Debug("embedding cache lookup complete",
		slog.String("provider", e.provider),
		slog.String("model", e.inner.Model()),
		slog.Int("text_count", len(texts)),
		slog.Int("hit_count", hits),
		slog.Int("miss_count", misses),
		slog.Int("cache_error_count", cacheErrors),
		slog.Int("write_error_count", writeErrors),
	)
	return results, nil
}

func (e *cachedEmbedder) embedInChunks(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	maxBatch := e.maxBatch
	if maxBatch <= 0 || len(texts) <= maxBatch {
		return e.inner.Embed(ctx, texts)
	}

	slog.Default().Info("splitting embedding batch",
		slog.String("provider", e.provider),
		slog.String("model", e.inner.Model()),
		slog.Int("text_count", len(texts)),
		slog.Int("max_batch", maxBatch),
	)

	results := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxBatch {
		end := start + maxBatch
		if end > len(texts) {
			end = len(texts)
		}
		vectors, err := e.inner.Embed(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		results = append(results, vectors...)
	}
	return results, nil
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
