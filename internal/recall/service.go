package recall

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// Searcher performs vector search using a precomputed query vector.
type Searcher interface {
	SearchMemories(ctx context.Context, req memory.RecallRequest, queryVector []float32) ([]memory.RecallHit, error)
}

// Service performs the end-to-end semantic recall flow.
type Service struct {
	embedder embed.Embedder
	searcher Searcher
	now      func() time.Time
}

// NewService constructs the semantic recall service.
func NewService(embedder embed.Embedder, searcher Searcher) (*Service, error) {
	if embedder == nil {
		return nil, errors.New("embedder must not be nil")
	}
	if searcher == nil {
		return nil, errors.New("searcher must not be nil")
	}
	return &Service{
		embedder: embedder,
		searcher: searcher,
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// Recall embeds the query text and then performs nearest-neighbor retrieval.
func (s *Service) Recall(ctx context.Context, req memory.RecallRequest) ([]memory.RecallHit, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	start := s.now()
	vectors, err := s.embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, errors.New("embedder returned unexpected vector count")
	}
	searchReq := req
	searchReq.Limit = expandedCandidateLimit(req.Limit)
	hits, err := s.searcher.SearchMemories(ctx, searchReq, vectors[0])
	if err != nil {
		return nil, err
	}
	ranked := rerankHits(hits, s.now())
	limit := effectiveRecallLimit(req.Limit)
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	attrs := []any{
		slog.String("tenant_id", req.TenantID.String()),
		slog.Int("requested_limit", limit),
		slog.Int("candidate_count", len(hits)),
		slog.Int("result_count", len(ranked)),
		slog.Int("query_length", len(req.Query)),
		slog.Int64("duration_ms", s.now().Sub(start).Milliseconds()),
	}
	if req.AgentID != nil {
		attrs = append(attrs, slog.String("agent_id", req.AgentID.String()))
	}
	if req.ThreadID != nil {
		attrs = append(attrs, slog.String("thread_id", req.ThreadID.String()))
	}
	if len(ranked) > 0 {
		attrs = append(attrs,
			slog.Float64("top_distance", ranked[0].Distance),
			slog.Float64("top_score", ranked[0].Score),
			slog.String("top_kind", ranked[0].Memory.Kind),
			slog.String("top_status", ranked[0].Memory.Status),
		)
	}
	slog.Default().Debug("recall completed", attrs...)
	return ranked, nil
}
