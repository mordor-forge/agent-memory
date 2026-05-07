package embed

import (
	"fmt"

	"github.com/mordor-forge/agent-memory/internal/config"
)

// NewFromConfig constructs an embedder from process configuration.
func NewFromConfig(cfg config.Config) (Embedder, error) {
	switch cfg.EmbeddingProvider {
	case "", "deterministic":
		return NewDeterministicEmbedder(cfg.EmbeddingDimensions)
	case "openai":
		return NewOpenAIEmbedder(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
	default:
		return nil, fmt.Errorf("unsupported embedder provider %q", cfg.EmbeddingProvider)
	}
}
