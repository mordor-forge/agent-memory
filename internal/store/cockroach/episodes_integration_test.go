//go:build integration

package cockroach_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/config"
	storepkg "github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestAppendEpisodeIdempotent(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	agent, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "study-skill",
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

	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	request := memory.AppendEpisodeRequest{
		TenantID:       tenant.ID,
		AgentID:        agent.ID,
		ThreadID:       &thread.ID,
		IdempotencyKey: "episode-" + uuid.NewString(),
		Kind:           "study.break",
		Content:        "paused after chapter 3 review",
		Payload: map[string]any{
			"phase":          "reviewing",
			"pending_action": "resume exercises",
		},
		OccurredAt: &occurredAt,
	}

	first, err := store.AppendEpisode(ctx, request)
	if err != nil {
		t.Fatalf("AppendEpisode(first) error = %v", err)
	}
	second, err := store.AppendEpisode(ctx, request)
	if err != nil {
		t.Fatalf("AppendEpisode(second) error = %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected idempotent append to return same id, got %s and %s", first.ID, second.ID)
	}
	if first.ThreadID == nil || *first.ThreadID != thread.ID {
		t.Fatalf("expected thread id %s, got %+v", thread.ID, first.ThreadID)
	}
	if first.Payload["phase"] != "reviewing" {
		t.Fatalf("expected phase payload to round-trip, got %#v", first.Payload["phase"])
	}
	if first.OccurredAt == nil || !first.OccurredAt.Equal(occurredAt) {
		t.Fatalf("expected occurred_at %v, got %+v", occurredAt, first.OccurredAt)
	}
}

func TestAppendEpisodeRejectsThreadMismatch(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()

	tenant, err := store.CreateTenant(ctx, memory.CreateTenantRequest{Slug: "tenant-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	agentA, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-a",
	})
	if err != nil {
		t.Fatalf("CreateAgent(agentA) error = %v", err)
	}
	agentB, err := store.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID: tenant.ID,
		Name:     "agent-b",
	})
	if err != nil {
		t.Fatalf("CreateAgent(agentB) error = %v", err)
	}
	thread, err := store.CreateThread(ctx, memory.CreateThreadRequest{
		TenantID: tenant.ID,
		AgentID:  agentA.ID,
	})
	if err != nil {
		t.Fatalf("CreateThread() error = %v", err)
	}

	_, err = store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agentB.ID,
		ThreadID: &thread.ID,
		Kind:     "tool_result",
		Content:  "thread mismatch",
	})
	if !errors.Is(err, memory.ErrThreadMismatch) {
		t.Fatalf("expected ErrThreadMismatch, got %v", err)
	}
}

func newIntegrationStore(t *testing.T) *storepkg.Store {
	t.Helper()

	dbURL := os.Getenv("COCKROACH_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("COCKROACH_TEST_DATABASE_URL is not set")
	}

	cfg := config.Config{
		HTTPAddr:        ":0",
		DatabaseURL:     dbURL,
		LogLevel:        "error",
		ShutdownTimeout: 5 * time.Second,
		HealthTimeout:   2 * time.Second,
		DBMaxConns:      4,
		DBMinConns:      1,
	}

	ctx := context.Background()
	store, err := storepkg.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(store.Close)

	if err := store.Exec(ctx, "SET CLUSTER SETTING feature.vector_index.enabled = true"); err != nil {
		t.Fatalf("enable vector indexes: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return store
}
