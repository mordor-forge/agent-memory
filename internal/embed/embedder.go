package embed

import "context"

// Embedder converts text into a fixed-size vector space.
type Embedder interface {
	Provider() string
	Model() string
	Dimensions() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
