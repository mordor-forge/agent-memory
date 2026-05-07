package recall

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type fakeSearcher struct {
	lastRequest memory.RecallRequest
	lastVector  []float32
	hits        []memory.RecallHit
	err         error
}

func (f *fakeSearcher) SearchMemories(_ context.Context, req memory.RecallRequest, queryVector []float32) ([]memory.RecallHit, error) {
	f.lastRequest = req
	f.lastVector = append([]float32(nil), queryVector...)
	return f.hits, f.err
}

func TestServiceRecall(t *testing.T) {
	t.Parallel()

	embedder, err := embed.NewDeterministicEmbedder(8)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	searcher := &fakeSearcher{
		hits: []memory.RecallHit{{Memory: memory.Memory{ID: uuid.New()}, Distance: 0.1}},
	}
	service, err := NewService(embedder, searcher)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	request := memory.RecallRequest{
		TenantID: uuid.New(),
		Query:    "resume study session",
		Limit:    5,
	}
	hits, err := service.Recall(context.Background(), request)
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("len(hits) = %d, want 1", len(hits))
	}
	if len(searcher.lastVector) != 8 {
		t.Fatalf("len(lastVector) = %d, want 8", len(searcher.lastVector))
	}
	if searcher.lastRequest.Limit != 20 {
		t.Fatalf("expanded search limit = %d, want 20", searcher.lastRequest.Limit)
	}
	if hits[0].Score == 0 {
		t.Fatal("expected ranked hit score to be populated")
	}
}

func TestServiceRecallPropagatesSearcherError(t *testing.T) {
	t.Parallel()

	embedder, err := embed.NewDeterministicEmbedder(8)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	searcher := &fakeSearcher{err: errors.New("boom")}
	service, err := NewService(embedder, searcher)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.Recall(context.Background(), memory.RecallRequest{
		TenantID: uuid.New(),
		Query:    "find notes",
	})
	if err == nil {
		t.Fatal("expected searcher error")
	}
}

func TestServiceRecallReranksByScore(t *testing.T) {
	t.Parallel()

	embedder, err := embed.NewDeterministicEmbedder(8)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	old := now.Add(-90 * 24 * time.Hour)
	recent := now.Add(-time.Hour)

	searcher := &fakeSearcher{
		hits: []memory.RecallHit{
			{
				Memory: memory.Memory{
					ID:         uuid.New(),
					Kind:       "episode_digest",
					Status:     "archived",
					Content:    "closest raw note",
					Importance: 0.1,
					Confidence: 0.2,
					UpdatedAt:  old,
				},
				Distance: 0.01,
			},
			{
				Memory: memory.Memory{
					ID:         uuid.New(),
					Kind:       "fact_merge",
					Status:     "active",
					Content:    "slightly farther but stronger fact",
					Importance: 0.9,
					Confidence: 0.9,
					UpdatedAt:  recent,
				},
				Distance: 0.15,
			},
		},
	}
	service, err := NewService(embedder, searcher)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.now = func() time.Time { return now }

	hits, err := service.Recall(context.Background(), memory.RecallRequest{
		TenantID: uuid.New(),
		Query:    "distributed sql retries",
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("len(hits) = %d, want 2", len(hits))
	}
	if hits[0].Memory.Content != "slightly farther but stronger fact" {
		t.Fatalf("top hit content = %q", hits[0].Memory.Content)
	}
	if hits[0].Score <= hits[1].Score {
		t.Fatalf("expected top hit score > second hit score, got %f <= %f", hits[0].Score, hits[1].Score)
	}
}
