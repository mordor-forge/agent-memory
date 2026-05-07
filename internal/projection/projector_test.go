package projection

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type fakeTransformer struct {
	records []memory.ProjectedMemory
}

func (f fakeTransformer) Transform(_ context.Context, _ []memory.Episode) ([]memory.ProjectedMemory, error) {
	return f.records, nil
}

type fakeWriter struct {
	projectionName   string
	workerID         uuid.UUID
	inputFrom        *memory.CheckpointCursor
	inputTo          *memory.CheckpointCursor
	records          []memory.ProjectedMemory
	embeddings       [][]float32
	embedderProvider string
	embeddingModel   string
}

func (f *fakeWriter) UpsertProjectedMemories(_ context.Context, projectionName string, workerID uuid.UUID, inputFrom, inputTo *memory.CheckpointCursor, records []memory.ProjectedMemory, embeddings [][]float32, embedderProvider, embeddingModel string) error {
	f.projectionName = projectionName
	f.workerID = workerID
	if inputFrom != nil {
		copy := *inputFrom
		f.inputFrom = &copy
	}
	if inputTo != nil {
		copy := *inputTo
		f.inputTo = &copy
	}
	f.records = append([]memory.ProjectedMemory(nil), records...)
	f.embeddings = make([][]float32, len(embeddings))
	for i := range embeddings {
		f.embeddings[i] = append([]float32(nil), embeddings[i]...)
	}
	f.embedderProvider = embedderProvider
	f.embeddingModel = embeddingModel
	return nil
}

func TestMemoryProjectorProcessEpisodes(t *testing.T) {
	t.Parallel()

	embedder, err := embed.NewDeterministicEmbedder(8)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	writer := &fakeWriter{}
	record := memory.ProjectedMemory{
		Key:              "episode:1",
		TenantID:         uuid.New(),
		AgentID:          uuid.New(),
		Kind:             "episode_digest",
		Status:           "active",
		Content:          "hello world",
		Importance:       0.5,
		Confidence:       1.0,
		LastObservedAt:   ptrTime(time.Now().UTC()),
		SourceEpisodeIDs: []uuid.UUID{uuid.New()},
	}
	projector, err := NewMemoryProjector("generic", writer, embedder, fakeTransformer{records: []memory.ProjectedMemory{record}})
	if err != nil {
		t.Fatalf("NewMemoryProjector() error = %v", err)
	}

	episode := memory.Episode{ID: uuid.New(), CreatedAt: time.Now().UTC()}
	err = projector.ProcessEpisodes(context.Background(), uuid.New(), []memory.Episode{episode})
	if err != nil {
		t.Fatalf("ProcessEpisodes() error = %v", err)
	}
	if writer.projectionName != "generic" {
		t.Fatalf("projectionName = %q, want %q", writer.projectionName, "generic")
	}
	if len(writer.records) != 1 || writer.records[0].Key != record.Key {
		t.Fatalf("records = %+v", writer.records)
	}
	if len(writer.embeddings) != 1 || len(writer.embeddings[0]) != 8 {
		t.Fatalf("embeddings = %+v", writer.embeddings)
	}
	if writer.inputFrom == nil || writer.inputFrom.ID != episode.ID {
		t.Fatalf("inputFrom = %+v, want episode %s", writer.inputFrom, episode.ID)
	}
	if writer.inputTo == nil || writer.inputTo.ID != episode.ID {
		t.Fatalf("inputTo = %+v, want episode %s", writer.inputTo, episode.ID)
	}
	if writer.embedderProvider != embedder.Provider() {
		t.Fatalf("embedderProvider = %q, want %q", writer.embedderProvider, embedder.Provider())
	}
	if writer.embeddingModel != embedder.Model() {
		t.Fatalf("embeddingModel = %q, want %q", writer.embeddingModel, embedder.Model())
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
