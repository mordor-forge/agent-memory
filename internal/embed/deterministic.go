package embed

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
)

// DeterministicEmbedder produces stable vectors without external dependencies.
type DeterministicEmbedder struct {
	dimensions int
}

// NewDeterministicEmbedder constructs a stable local embedder.
func NewDeterministicEmbedder(dimensions int) (*DeterministicEmbedder, error) {
	if dimensions <= 0 {
		return nil, fmt.Errorf("dimensions must be > 0")
	}
	return &DeterministicEmbedder{dimensions: dimensions}, nil
}

// Provider returns the logical provider identifier.
func (e *DeterministicEmbedder) Provider() string {
	return "deterministic"
}

// Model returns the logical embedding model identifier.
func (e *DeterministicEmbedder) Model() string {
	return fmt.Sprintf("deterministic/%d", e.dimensions)
}

// Dimensions returns the number of vector dimensions produced.
func (e *DeterministicEmbedder) Dimensions() int {
	return e.dimensions
}

// Embed returns deterministic normalized vectors for the provided texts.
func (e *DeterministicEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors = append(vectors, deterministicVector(text, e.dimensions))
	}
	return vectors, nil
}

func deterministicVector(text string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	seed := sha256.Sum256([]byte(text))
	block := seed[:]

	var (
		norm  float64
		index int
		step  uint64
	)
	for index < dimensions {
		for offset := 0; offset+4 <= len(block) && index < dimensions; offset += 4 {
			value := binary.BigEndian.Uint32(block[offset : offset+4])
			scaled := (float64(value)/float64(math.MaxUint32))*2 - 1
			component := float32(scaled)
			vector[index] = component
			norm += float64(component * component)
			index++
		}
		step++
		nextInput := make([]byte, len(seed)+8)
		copy(nextInput, seed[:])
		binary.BigEndian.PutUint64(nextInput[len(seed):], step)
		next := sha256.Sum256(nextInput)
		block = next[:]
	}

	if norm == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}
