//go:build integration

package cockroach_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestExportAndDeleteTenant(t *testing.T) {
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
	thread, err := store.CreateThread(ctx, memory.CreateThreadRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}
	episode, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		ThreadID: &thread.ID,
		Kind:     "note",
		Content:  "tenant export test",
	})
	if err != nil {
		t.Fatalf("AppendEpisode() error = %v", err)
	}

	exported, err := store.ExportTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ExportTenant() error = %v", err)
	}
	if exported.Tenant.ID != tenant.ID {
		t.Fatalf("exported tenant id = %s, want %s", exported.Tenant.ID, tenant.ID)
	}
	if len(exported.Agents) != 1 || len(exported.Threads) != 1 || len(exported.Episodes) != 1 {
		t.Fatalf("unexpected export sizes: agents=%d threads=%d episodes=%d", len(exported.Agents), len(exported.Threads), len(exported.Episodes))
	}
	if exported.Episodes[0].ID != episode.ID {
		t.Fatalf("exported episode id = %s, want %s", exported.Episodes[0].ID, episode.ID)
	}

	deletion, err := store.DeleteTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("DeleteTenant() error = %v", err)
	}
	if !deletion.Deleted || deletion.TenantID != tenant.ID {
		t.Fatalf("unexpected deletion result: %+v", deletion)
	}

	_, err = store.ExportTenant(ctx, tenant.ID)
	if err != memory.ErrTenantNotFound {
		t.Fatalf("ExportTenant() after delete err = %v, want %v", err, memory.ErrTenantNotFound)
	}
}
