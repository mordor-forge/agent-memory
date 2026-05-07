//go:build integration

package projection_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/projection"
	storepkg "github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestEpisodeMemoryProjectionCreatesMemoriesAndAdvancesCheckpoint(t *testing.T) {
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

	if _, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.start",
		Content:  "beginning the study session",
	}); err != nil {
		t.Fatalf("AppendEpisode(first) error = %v", err)
	}
	if _, err := store.AppendEpisode(ctx, memory.AppendEpisodeRequest{
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		Kind:     "study.break",
		Content:  "paused after exercises",
	}); err != nil {
		t.Fatalf("AppendEpisode(second) error = %v", err)
	}

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	processor, err := projection.NewEpisodeDigestProjector("episode-digests", store, embedder)
	if err != nil {
		t.Fatalf("NewEpisodeDigestProjector() error = %v", err)
	}
	engine, err := projection.NewEngine("episode-digests", store, processor, 100, time.Minute)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	result, err := engine.RunBatch(ctx, tenant.ID, 0, uuid.New())
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if !result.LeaseAcquired || !result.CheckpointAdvanced {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.EpisodesRead != 2 {
		t.Fatalf("EpisodesRead = %d, want 2", result.EpisodesRead)
	}

	memories, err := store.ListMemoriesByTenant(ctx, tenant.ID, 10)
	if err != nil {
		t.Fatalf("ListMemoriesByTenant() error = %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("len(memories) = %d, want 2", len(memories))
	}
	if memories[0].Kind != "episode_digest" || memories[1].Kind != "episode_digest" {
		t.Fatalf("unexpected memory kinds: %+v", memories)
	}
	embeddingCount, err := store.CountMemoryEmbeddingsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("CountMemoryEmbeddingsByTenant() error = %v", err)
	}
	if embeddingCount != 2 {
		t.Fatalf("embeddingCount = %d, want 2", embeddingCount)
	}

	checkpoint, err := store.GetProjectionCheckpoint(ctx, "episode-digests", tenant.ID, 0)
	if err != nil {
		t.Fatalf("GetProjectionCheckpoint() error = %v", err)
	}
	if checkpoint == nil {
		t.Fatal("expected checkpoint to be recorded")
	}
	if result.LastCursor == nil || checkpoint.ID != result.LastCursor.ID {
		t.Fatalf("checkpoint = %+v, result.LastCursor = %+v", checkpoint, result.LastCursor)
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
