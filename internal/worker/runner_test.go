package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/projection"
)

type fakeTenantLister struct {
	tenantIDs []uuid.UUID
	err       error
}

func (f *fakeTenantLister) ListTenantIDs(_ context.Context, _ int) ([]uuid.UUID, error) {
	return f.tenantIDs, f.err
}

type fakeEngine struct {
	name      string
	tenantIDs []uuid.UUID
	err       error
	calls     int
}

func (f *fakeEngine) Name() string {
	return f.name
}

func (f *fakeEngine) RunBatch(_ context.Context, tenantID uuid.UUID, _ int64, _ uuid.UUID) (projection.Result, error) {
	f.calls++
	f.tenantIDs = append(f.tenantIDs, tenantID)
	return projection.Result{LeaseAcquired: true}, f.err
}

func TestRunnerRunOnce(t *testing.T) {
	t.Parallel()

	tenantA := uuid.New()
	tenantB := uuid.New()
	engineA := &fakeEngine{name: "a"}
	engineB := &fakeEngine{name: "b"}
	runner, err := NewRunner(uuid.New(), time.Second, 10, slog.Default(), &fakeTenantLister{
		tenantIDs: []uuid.UUID{tenantA, tenantB},
	}, engineA, engineB)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(engineA.tenantIDs) != 2 || engineA.tenantIDs[0] != tenantA || engineA.tenantIDs[1] != tenantB {
		t.Fatalf("engineA tenant calls = %v", engineA.tenantIDs)
	}
	if len(engineB.tenantIDs) != 2 || engineB.tenantIDs[0] != tenantA || engineB.tenantIDs[1] != tenantB {
		t.Fatalf("engineB tenant calls = %v", engineB.tenantIDs)
	}
}

func TestRunnerRunOncePropagatesEngineError(t *testing.T) {
	t.Parallel()

	runner, err := NewRunner(uuid.New(), time.Second, 10, slog.Default(), &fakeTenantLister{
		tenantIDs: []uuid.UUID{uuid.New()},
	}, &fakeEngine{name: "bad", err: errors.New("boom")})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	if err := runner.RunOnce(context.Background()); err == nil {
		t.Fatal("expected engine error")
	}
}
