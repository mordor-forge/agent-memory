package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/projection"
)

const defaultTenantScanLimit = 100

// TenantLister lists tenant IDs that should be processed.
type TenantLister interface {
	ListTenantIDs(ctx context.Context, limit int) ([]uuid.UUID, error)
}

// Engine runs one projection batch for one tenant.
type Engine interface {
	Name() string
	RunBatch(ctx context.Context, tenantID uuid.UUID, shardID int64, workerID uuid.UUID) (projection.Result, error)
}

// Runner coordinates the recurring worker loop.
type Runner struct {
	workerID        uuid.UUID
	pollInterval    time.Duration
	tenantScanLimit int
	logger          *slog.Logger
	tenants         TenantLister
	engines         []Engine
}

// NewRunner constructs a new worker runner.
func NewRunner(workerID uuid.UUID, pollInterval time.Duration, tenantScanLimit int, logger *slog.Logger, tenants TenantLister, engines ...Engine) (*Runner, error) {
	if workerID == uuid.Nil {
		return nil, errors.New("worker id must not be nil")
	}
	if pollInterval <= 0 {
		return nil, errors.New("poll interval must be > 0")
	}
	if tenantScanLimit <= 0 {
		tenantScanLimit = defaultTenantScanLimit
	}
	if logger == nil {
		logger = slog.Default()
	}
	if tenants == nil {
		return nil, errors.New("tenant lister must not be nil")
	}
	if len(engines) == 0 {
		return nil, errors.New("at least one engine must be provided")
	}
	for _, engine := range engines {
		if engine == nil {
			return nil, errors.New("engine must not be nil")
		}
	}
	return &Runner{
		workerID:        workerID,
		pollInterval:    pollInterval,
		tenantScanLimit: tenantScanLimit,
		logger:          logger,
		tenants:         tenants,
		engines:         engines,
	}, nil
}

// RunOnce processes the current tenant list one time.
func (r *Runner) RunOnce(ctx context.Context) error {
	tenantIDs, err := r.tenants.ListTenantIDs(ctx, r.tenantScanLimit)
	if err != nil {
		r.logger.Error("worker tenant scan failed", slog.String("error", err.Error()))
		return err
	}
	r.logger.Info("worker tenant scan complete",
		slog.Int("tenant_count", len(tenantIDs)),
		slog.Int("engine_count", len(r.engines)),
	)
	for _, tenantID := range tenantIDs {
		if ctx.Err() != nil {
			r.logger.Warn("worker run interrupted by context", slog.String("tenant_id", tenantID.String()))
			return ctx.Err()
		}
		for _, engine := range r.engines {
			result, err := engine.RunBatch(ctx, tenantID, 0, r.workerID)
			if err != nil {
				r.logger.Error("worker projection batch failed",
					slog.String("projection", engine.Name()),
					slog.String("tenant_id", tenantID.String()),
					slog.String("worker_id", r.workerID.String()),
					slog.String("error", err.Error()),
				)
				return err
			}
			attrs := []any{
				slog.String("projection", engine.Name()),
				slog.String("tenant_id", tenantID.String()),
				slog.String("worker_id", r.workerID.String()),
				slog.Bool("lease_acquired", result.LeaseAcquired),
				slog.Int("episodes_read", result.EpisodesRead),
				slog.Bool("checkpoint_advanced", result.CheckpointAdvanced),
			}
			if result.LastCursor != nil {
				attrs = append(attrs,
					slog.String("last_cursor_id", result.LastCursor.ID.String()),
					slog.Time("last_cursor_created_at", result.LastCursor.CreatedAt),
				)
			}
			switch {
			case result.CheckpointAdvanced || result.EpisodesRead > 0:
				r.logger.Info("worker projection batch processed", attrs...)
			case !result.LeaseAcquired:
				r.logger.Debug("worker projection batch skipped", attrs...)
			default:
				r.logger.Debug("worker projection batch no-op", attrs...)
			}
		}
	}
	return nil
}

// RunLoop processes tenants repeatedly until the context is cancelled.
func (r *Runner) RunLoop(ctx context.Context) error {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		if err := r.RunOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
