package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type fakeStore struct {
	acquiredLease bool
	checkpoint    *memory.CheckpointCursor
	episodes      []memory.Episode

	tryAcquireCalls int
	releaseCalls    int
	advanceCalls    int

	advancedCursor *memory.CheckpointCursor
}

func (f *fakeStore) TryAcquireLease(_ context.Context, _ string, _ uuid.UUID, _ time.Duration) (bool, error) {
	f.tryAcquireCalls++
	return f.acquiredLease, nil
}

func (f *fakeStore) ReleaseLease(_ context.Context, _ string, _ uuid.UUID) error {
	f.releaseCalls++
	return nil
}

func (f *fakeStore) GetProjectionCheckpoint(_ context.Context, _ string, _ uuid.UUID, _ int64) (*memory.CheckpointCursor, error) {
	return f.checkpoint, nil
}

func (f *fakeStore) AdvanceProjectionCheckpoint(_ context.Context, _ string, _ uuid.UUID, _ int64, cursor memory.CheckpointCursor) error {
	f.advanceCalls++
	cursorCopy := cursor
	f.advancedCursor = &cursorCopy
	return nil
}

func (f *fakeStore) ListEpisodesAfter(_ context.Context, _ uuid.UUID, _ *memory.CheckpointCursor, _ int) ([]memory.Episode, error) {
	return f.episodes, nil
}

type fakeProcessor struct {
	calls     int
	workerIDs []uuid.UUID
	lastBatch []memory.Episode
	err       error
}

func (f *fakeProcessor) ProcessEpisodes(_ context.Context, workerID uuid.UUID, episodes []memory.Episode) error {
	f.calls++
	f.workerIDs = append(f.workerIDs, workerID)
	f.lastBatch = append([]memory.Episode(nil), episodes...)
	return f.err
}

func TestEngineRunBatchSkipsWhenLeaseNotAcquired(t *testing.T) {
	t.Parallel()

	store := &fakeStore{acquiredLease: false}
	processor := &fakeProcessor{}
	engine, err := NewEngine("test-projection", store, processor, 10, time.Second)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	workerID := uuid.New()
	result, err := engine.RunBatch(context.Background(), uuid.New(), 0, workerID)
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if result.LeaseAcquired {
		t.Fatal("expected lease not acquired")
	}
	if processor.calls != 0 {
		t.Fatalf("processor calls = %d, want 0", processor.calls)
	}
	if store.releaseCalls != 0 {
		t.Fatalf("release calls = %d, want 0", store.releaseCalls)
	}
}

func TestEngineRunBatchAdvancesCheckpointOnSuccess(t *testing.T) {
	t.Parallel()

	first := memory.Episode{ID: uuid.New(), CreatedAt: time.Now().UTC().Add(-time.Minute)}
	second := memory.Episode{ID: uuid.New(), CreatedAt: time.Now().UTC()}
	store := &fakeStore{
		acquiredLease: true,
		episodes:      []memory.Episode{first, second},
	}
	processor := &fakeProcessor{}
	engine, err := NewEngine("test-projection", store, processor, 10, time.Second)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	workerID := uuid.New()
	result, err := engine.RunBatch(context.Background(), uuid.New(), 0, workerID)
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if !result.LeaseAcquired {
		t.Fatal("expected lease acquired")
	}
	if !result.CheckpointAdvanced {
		t.Fatal("expected checkpoint to advance")
	}
	if result.EpisodesRead != 2 {
		t.Fatalf("EpisodesRead = %d, want 2", result.EpisodesRead)
	}
	if store.advanceCalls != 1 {
		t.Fatalf("advance calls = %d, want 1", store.advanceCalls)
	}
	if len(processor.workerIDs) != 1 || processor.workerIDs[0] != workerID {
		t.Fatalf("workerIDs = %v, want [%s]", processor.workerIDs, workerID)
	}
	if store.advancedCursor == nil || store.advancedCursor.ID != second.ID {
		t.Fatalf("advanced cursor = %+v, want last episode id %s", store.advancedCursor, second.ID)
	}
	if store.releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", store.releaseCalls)
	}
}

func TestEngineRunBatchDoesNotAdvanceOnProcessorError(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		acquiredLease: true,
		episodes:      []memory.Episode{{ID: uuid.New(), CreatedAt: time.Now().UTC()}},
	}
	processor := &fakeProcessor{err: errors.New("boom")}
	engine, err := NewEngine("test-projection", store, processor, 10, time.Second)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	_, err = engine.RunBatch(context.Background(), uuid.New(), 0, uuid.New())
	if err == nil {
		t.Fatal("expected processor error")
	}
	if store.advanceCalls != 0 {
		t.Fatalf("advance calls = %d, want 0", store.advanceCalls)
	}
	if store.releaseCalls != 1 {
		t.Fatalf("release calls = %d, want 1", store.releaseCalls)
	}
}
