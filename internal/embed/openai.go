package embed

import (
	"context"
	"errors"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIEmbedder uses the OpenAI-compatible embeddings API.
type OpenAIEmbedder struct {
	client     openai.Client
	model      string
	dimensions int
}

// NewOpenAIEmbedder constructs an OpenAI-compatible embedder.
func NewOpenAIEmbedder(baseURL, apiKey, model string, dimensions int) (*OpenAIEmbedder, error) {
	if apiKey == "" {
		return nil, errors.New("openai api key is required")
	}
	if model == "" {
		return nil, errors.New("openai embedding model is required")
	}
	if dimensions <= 0 {
		return nil, errors.New("embedding dimensions must be > 0")
	}

	options := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		options = append(options, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(options...)
	return &OpenAIEmbedder{
		client:     client,
		model:      model,
		dimensions: dimensions,
	}, nil
}

// Provider returns the logical provider identifier.
func (e *OpenAIEmbedder) Provider() string {
	return "openai"
}

// Model returns the configured model identifier.
func (e *OpenAIEmbedder) Model() string {
	return e.model
}

// Dimensions returns the configured embedding dimensionality.
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dimensions
}

// Embed performs batch embedding against the configured OpenAI-compatible endpoint.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		Model:      e.model,
		Dimensions: openai.Int(int64(e.dimensions)),
	})
	if err != nil {
		return nil, err
	}

	vectors := make([][]float32, 0, len(resp.Data))
	for _, item := range resp.Data {
		vector := make([]float32, len(item.Embedding))
		for i, value := range item.Embedding {
			vector[i] = float32(value)
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}
