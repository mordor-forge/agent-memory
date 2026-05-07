package embed

import (
	"testing"

	"github.com/mordor-forge/agent-memory/internal/config"
)

func TestNewFromConfigDeterministic(t *testing.T) {
	t.Parallel()

	embedder, err := NewFromConfig(config.Config{
		EmbeddingProvider:   "deterministic",
		EmbeddingDimensions: 64,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if embedder.Model() != "deterministic/64" {
		t.Fatalf("Model() = %q, want %q", embedder.Model(), "deterministic/64")
	}
}

func TestNewFromConfigOpenAIValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewFromConfig(config.Config{
		EmbeddingProvider:   "openai",
		EmbeddingModel:      "text-embedding-3-small",
		EmbeddingDimensions: 1536,
	}); err == nil {
		t.Fatal("expected error for missing api key")
	}
}

func TestNewFromConfigOpenAI(t *testing.T) {
	t.Parallel()

	embedder, err := NewFromConfig(config.Config{
		EmbeddingProvider:   "openai",
		EmbeddingModel:      "text-embedding-3-small",
		EmbeddingDimensions: 1536,
		OpenAIBaseURL:       "https://api.openai.com/v1",
		OpenAIAPIKey:        "test-key",
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if embedder.Model() != "text-embedding-3-small" {
		t.Fatalf("Model() = %q", embedder.Model())
	}
}
