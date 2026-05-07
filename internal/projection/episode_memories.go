package projection

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// MemoryWriter persists generic projection outputs and optional embeddings.
type MemoryWriter interface {
	UpsertProjectedMemories(ctx context.Context, projectionName string, workerID uuid.UUID, inputFrom, inputTo *memory.CheckpointCursor, records []memory.ProjectedMemory, embeddings [][]float32, embeddingProvider, embeddingModel string) error
}

// Transformer converts source episodes into generic projected memories.
type Transformer interface {
	Transform(ctx context.Context, episodes []memory.Episode) ([]memory.ProjectedMemory, error)
}

// MemoryProjector runs a transformer, embeds the outputs, and persists them.
type MemoryProjector struct {
	projectionName string
	writer         MemoryWriter
	embedder       embed.Embedder
	transformer    Transformer
}

// NewMemoryProjector constructs a generic reusable projection pipeline.
func NewMemoryProjector(projectionName string, writer MemoryWriter, embedder embed.Embedder, transformer Transformer) (*MemoryProjector, error) {
	if projectionName == "" {
		return nil, errors.New("projection name must not be empty")
	}
	if writer == nil {
		return nil, errors.New("writer must not be nil")
	}
	if embedder == nil {
		return nil, errors.New("embedder must not be nil")
	}
	if transformer == nil {
		return nil, errors.New("transformer must not be nil")
	}
	return &MemoryProjector{
		projectionName: projectionName,
		writer:         writer,
		embedder:       embedder,
		transformer:    transformer,
	}, nil
}

// ProcessEpisodes stores transformed projected memories and their embeddings for the batch.
func (p *MemoryProjector) ProcessEpisodes(ctx context.Context, workerID uuid.UUID, episodes []memory.Episode) error {
	records, err := p.transformer.Transform(ctx, episodes)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Key < records[j].Key
	})

	texts := make([]string, 0, len(records))
	for _, record := range records {
		text := strings.TrimSpace(record.EmbedText)
		if text == "" {
			text = record.Content
		}
		texts = append(texts, text)
	}
	vectors, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return err
	}
	inputFrom, inputTo := batchBounds(episodes)
	return p.writer.UpsertProjectedMemories(ctx, p.projectionName, workerID, inputFrom, inputTo, records, vectors, p.embedder.Provider(), p.embedder.Model())
}

// EpisodeDigestTransformer materializes one generic digest memory per episode.
type EpisodeDigestTransformer struct{}

// Transform converts each episode into one stable generic projected memory.
func (EpisodeDigestTransformer) Transform(_ context.Context, episodes []memory.Episode) ([]memory.ProjectedMemory, error) {
	records := make([]memory.ProjectedMemory, 0, len(episodes))
	for _, episode := range episodes {
		summary := truncateSummary(episode.Content, 160)
		lastObservedAt := episode.CreatedAt
		if episode.OccurredAt != nil {
			lastObservedAt = *episode.OccurredAt
		}
		records = append(records, memory.ProjectedMemory{
			Key:      episode.ID.String(),
			TenantID: episode.TenantID,
			AgentID:  episode.AgentID,
			ThreadID: episode.ThreadID,
			Kind:     "episode_digest",
			Status:   "active",
			Content:  episode.Content,
			Summary:  &summary,
			Attributes: map[string]any{
				"source_kind": episode.Kind,
			},
			Importance:     0.5,
			Confidence:     1.0,
			LastObservedAt: &lastObservedAt,
			SourceEpisodeIDs: []uuid.UUID{
				episode.ID,
			},
			EmbedText: episode.Content,
		})
	}
	return records, nil
}

// NewEpisodeDigestProjector builds the default generic digest projection.
func NewEpisodeDigestProjector(projectionName string, writer MemoryWriter, embedder embed.Embedder) (*MemoryProjector, error) {
	return NewMemoryProjector(projectionName, writer, embedder, EpisodeDigestTransformer{})
}

func truncateSummary(content string, maxLen int) string {
	runes := []rune(content)
	if maxLen <= 0 || len(runes) <= maxLen {
		return content
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func batchBounds(episodes []memory.Episode) (*memory.CheckpointCursor, *memory.CheckpointCursor) {
	if len(episodes) == 0 {
		return nil, nil
	}
	first := memory.CheckpointCursor{
		CreatedAt: episodes[0].CreatedAt,
		ID:        episodes[0].ID,
	}
	last := memory.CheckpointCursor{
		CreatedAt: episodes[len(episodes)-1].CreatedAt,
		ID:        episodes[len(episodes)-1].ID,
	}
	return &first, &last
}
