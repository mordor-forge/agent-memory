package cockroach

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// GetCachedEmbedding looks up one cached embedding by provider, model, and text hash.
func (s *Store) GetCachedEmbedding(ctx context.Context, provider, model, textHash string) ([]float32, bool, error) {
	var vector pgvector.Vector
	err := s.pool.QueryRow(ctx,
		`SELECT embedding
		 FROM embedding_cache
		 WHERE provider = $1 AND model = $2 AND text_hash = $3`,
		provider, model, textHash,
	).Scan(&vector)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get cached embedding: %w", err)
	}
	return vector.Slice(), true, nil
}

// PutCachedEmbedding stores one embedding cache row. Duplicate writes are harmless.
func (s *Store) PutCachedEmbedding(ctx context.Context, provider, model, textHash, text string, embedding []float32) error {
	if len(embedding) == 0 {
		return fmt.Errorf("put cached embedding: embedding must not be empty")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO embedding_cache (provider, model, text_hash, text, embedding)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (provider, model, text_hash)
		 DO UPDATE SET
			text = EXCLUDED.text,
			embedding = EXCLUDED.embedding,
			created_at = now()`,
		provider, model, textHash, text, pgvector.NewVector(embedding),
	)
	if err != nil {
		return fmt.Errorf("put cached embedding: %w", err)
	}
	return nil
}
