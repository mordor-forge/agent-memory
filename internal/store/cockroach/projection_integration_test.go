//go:build integration

package cockroach_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestListEpisodesAfterUsesCheckpointCursor(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	agent, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	first, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.start",
		Content:  "start study session",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(first) error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	second, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.note",
		Content:  "captured note",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(second) error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	third, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.break",
		Content:  "pause session",
	})
	if err != nil {
		t.Fatalf("AppendEpisode(third) error = %v", err)
	}

	page, err := store.ListEpisodesAfter(ctx, tenant.ID, nil, 2)
	if err != nil {
		t.Fatalf("ListEpisodesAfter(page1) error = %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("len(page1) = %d, want 2", len(page))
	}
	if page[0].ID != first.ID || page[1].ID != second.ID {
		t.Fatalf("page1 ids = [%s %s], want [%s %s]", page[0].ID, page[1].ID, first.ID, second.ID)
	}

	cursor := memory.CheckpointCursor{CreatedAt: page[1].CreatedAt, ID: page[1].ID}
	next, err := store.ListEpisodesAfter(ctx, tenant.ID, &cursor, 2)
	if err != nil {
		t.Fatalf("ListEpisodesAfter(page2) error = %v", err)
	}
	if len(next) != 1 {
		t.Fatalf("len(page2) = %d, want 1", len(next))
	}
	if next[0].ID != third.ID {
		t.Fatalf("page2 id = %s, want %s", next[0].ID, third.ID)
	}
}

func TestProjectionCheckpointRoundTrip(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	cursor := memory.CheckpointCursor{
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		ID:        uuid.New(),
	}
	if err := store.AdvanceProjectionCheckpoint(ctx, "projection-test", tenant.ID, 0, cursor); err != nil {
		t.Fatalf("AdvanceProjectionCheckpoint() error = %v", err)
	}

	got, err := store.GetProjectionCheckpoint(ctx, "projection-test", tenant.ID, 0)
	if err != nil {
		t.Fatalf("GetProjectionCheckpoint() error = %v", err)
	}
	if got == nil {
		t.Fatal("expected checkpoint to exist")
	}
	if got.ID != cursor.ID || !got.CreatedAt.Equal(cursor.CreatedAt) {
		t.Fatalf("checkpoint = %+v, want %+v", got, cursor)
	}
}

func TestLeaseAcquireRelease(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	leaseName := "lease-" + uuid.NewString()
	holderA := uuid.New()
	holderB := uuid.New()

	acquired, err := store.TryAcquireLease(ctx, leaseName, holderA, time.Minute)
	if err != nil {
		t.Fatalf("TryAcquireLease(holderA) error = %v", err)
	}
	if !acquired {
		t.Fatal("expected first lease acquire to succeed")
	}

	acquired, err = store.TryAcquireLease(ctx, leaseName, holderB, time.Minute)
	if err != nil {
		t.Fatalf("TryAcquireLease(holderB) error = %v", err)
	}
	if acquired {
		t.Fatal("expected second holder acquire to fail while lease is active")
	}

	if err := store.ReleaseLease(ctx, leaseName, holderA); err != nil {
		t.Fatalf("ReleaseLease(holderA) error = %v", err)
	}

	acquired, err = store.TryAcquireLease(ctx, leaseName, holderB, time.Minute)
	if err != nil {
		t.Fatalf("TryAcquireLease(holderB-after-release) error = %v", err)
	}
	if !acquired {
		t.Fatal("expected second holder acquire to succeed after release")
	}
}
