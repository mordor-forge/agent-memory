package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

const (
	defaultBatchSize = 100
	defaultLeaseTTL  = 30 * time.Second
)

// Store is the minimum persistence surface the projection engine needs.
type Store interface {
	TryAcquireLease(ctx context.Context, leaseName string, holderID uuid.UUID, ttl time.Duration) (bool, error)
	ReleaseLease(ctx context.Context, leaseName string, holderID uuid.UUID) error
	GetProjectionCheckpoint(ctx context.Context, projectionName string, tenantID uuid.UUID, shardID int64) (*memory.CheckpointCursor, error)
	AdvanceProjectionCheckpoint(ctx context.Context, projectionName string, tenantID uuid.UUID, shardID int64, cursor memory.CheckpointCursor) error
	ListEpisodesAfter(ctx context.Context, tenantID uuid.UUID, after *memory.CheckpointCursor, limit int) ([]memory.Episode, error)
}

// Processor performs the non-transactional work for one episode batch.
type Processor interface {
	ProcessEpisodes(ctx context.Context, workerID uuid.UUID, episodes []memory.Episode) error
}

// Engine coordinates lease acquisition, ordered scanning, processing, and checkpoint advancement.
type Engine struct {
	projectionName string
	store          Store
	processor      Processor
	batchSize      int
	leaseTTL       time.Duration
}

// Name returns the projection name associated with this engine.
func (e *Engine) Name() string {
	return e.projectionName
}

// Result describes one batch execution.
type Result struct {
	LeaseAcquired      bool
	EpisodesRead       int
	CheckpointAdvanced bool
	LastCursor         *memory.CheckpointCursor
}

// NewEngine constructs a projection engine with safe defaults.
func NewEngine(projectionName string, store Store, processor Processor, batchSize int, leaseTTL time.Duration) (*Engine, error) {
	if projectionName == "" {
		return nil, errors.New("projection name must not be empty")
	}
	if store == nil {
		return nil, errors.New("store must not be nil")
	}
	if processor == nil {
		return nil, errors.New("processor must not be nil")
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if leaseTTL <= 0 {
		leaseTTL = defaultLeaseTTL
	}
	return &Engine{
		projectionName: projectionName,
		store:          store,
		processor:      processor,
		batchSize:      batchSize,
		leaseTTL:       leaseTTL,
	}, nil
}

// RunBatch processes at most one ordered batch for a tenant/shard pair.
func (e *Engine) RunBatch(ctx context.Context, tenantID uuid.UUID, shardID int64, workerID uuid.UUID) (result Result, err error) {
	if tenantID == uuid.Nil {
		return Result{}, errors.New("tenant_id must not be nil")
	}
	if workerID == uuid.Nil {
		return Result{}, errors.New("worker_id must not be nil")
	}

	leaseName := fmt.Sprintf("projection:%s:%s:%d", e.projectionName, tenantID.String(), shardID)
	acquired, err := e.store.TryAcquireLease(ctx, leaseName, workerID, e.leaseTTL)
	if err != nil {
		return Result{}, err
	}
	if !acquired {
		return Result{LeaseAcquired: false}, nil
	}
	result.LeaseAcquired = true

	released := false
	defer func() {
		if released {
			return
		}
		releaseErr := e.store.ReleaseLease(context.Background(), leaseName, workerID)
		if releaseErr != nil && err == nil {
			err = releaseErr
		}
	}()

	cursor, err := e.store.GetProjectionCheckpoint(ctx, e.projectionName, tenantID, shardID)
	if err != nil {
		return result, err
	}

	episodes, err := e.store.ListEpisodesAfter(ctx, tenantID, cursor, e.batchSize)
	if err != nil {
		return result, err
	}
	result.EpisodesRead = len(episodes)
	if len(episodes) == 0 {
		if releaseErr := e.store.ReleaseLease(ctx, leaseName, workerID); releaseErr != nil {
			return result, releaseErr
		}
		released = true
		return result, nil
	}

	if err := e.processor.ProcessEpisodes(ctx, workerID, episodes); err != nil {
		return result, err
	}

	lastCursor := memory.CheckpointCursor{
		CreatedAt: episodes[len(episodes)-1].CreatedAt,
		ID:        episodes[len(episodes)-1].ID,
	}
	if err := e.store.AdvanceProjectionCheckpoint(ctx, e.projectionName, tenantID, shardID, lastCursor); err != nil {
		return result, err
	}
	result.CheckpointAdvanced = true
	result.LastCursor = &lastCursor

	if releaseErr := e.store.ReleaseLease(ctx, leaseName, workerID); releaseErr != nil {
		return result, releaseErr
	}
	released = true
	return result, nil
}
