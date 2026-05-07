//go:build integration

package importer_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/embed"
	"github.com/mordor-forge/agent-memory/internal/importer"
	"github.com/mordor-forge/agent-memory/internal/projection"
	"github.com/mordor-forge/agent-memory/internal/recall"
	storepkg "github.com/mordor-forge/agent-memory/internal/store/cockroach"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

func TestCursorAgentJSONLImportSmokeFlow(t *testing.T) {
	store := newImporterIntegrationStore(t)
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

	runner, err := importer.NewRunner(store, importer.CursorAgentJSONLImporter{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	report, err := runner.ImportFile(ctx, importer.ImportRequest{
		Format:   "cursor-agent-jsonl",
		FilePath: filepath.Join("testdata", "cursor_agent_transcript.jsonl"),
		SourceID: "smoke-transcript",
		TenantID: tenant.ID,
		AgentID:  agent.ID,
		ThreadID: &thread.ID,
	})
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}
	if report.RecordsRead != 3 || report.EpisodesImported != 3 {
		t.Fatalf("unexpected import report: %+v", report)
	}

	embedder, err := embed.NewDeterministicEmbedder(1536)
	if err != nil {
		t.Fatalf("NewDeterministicEmbedder() error = %v", err)
	}
	projector, err := projection.NewEpisodeDigestProjector("episode-digests", store, embedder)
	if err != nil {
		t.Fatalf("NewEpisodeDigestProjector() error = %v", err)
	}
	engine, err := projection.NewEngine("episode-digests", store, projector, 100, time.Second)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	result, err := engine.RunBatch(ctx, tenant.ID, 0, uuid.New())
	if err != nil {
		t.Fatalf("RunBatch() error = %v", err)
	}
	if !result.CheckpointAdvanced || result.EpisodesRead != 3 {
		t.Fatalf("unexpected projection result: %+v", result)
	}

	memories, err := store.QueryMemories(ctx, memory.QueryMemoriesRequest{
		TenantID: tenant.ID,
		ThreadID: &thread.ID,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("QueryMemories() error = %v", err)
	}
	if len(memories) != 3 {
		t.Fatalf("len(memories) = %d, want 3", len(memories))
	}

	recallService, err := recall.NewService(embedder, store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	hits, err := recallService.Recall(ctx, memory.RecallRequest{
		TenantID: tenant.ID,
		ThreadID: &thread.ID,
		Query:    "CockroachDB",
		Limit:    3,
	})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one recall hit")
	}

	var importedMemory *memory.Memory
	for i := range memories {
		if strings.Contains(memories[i].Content, "CockroachDB") {
			importedMemory = &memories[i]
			break
		}
	}
	if importedMemory == nil {
		t.Fatalf("expected a CockroachDB memory in %+v", memories)
	}

	provenance, err := store.GetMemoryProvenance(ctx, importedMemory.ID)
	if err != nil {
		t.Fatalf("GetMemoryProvenance() error = %v", err)
	}
	if provenance.ProjectionName != "episode-digests" {
		t.Fatalf("ProjectionName = %q, want %q", provenance.ProjectionName, "episode-digests")
	}
	if len(provenance.Sources) != 1 {
		t.Fatalf("len(provenance.Sources) = %d, want 1", len(provenance.Sources))
	}
}

func newImporterIntegrationStore(t *testing.T) *storepkg.Store {
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
