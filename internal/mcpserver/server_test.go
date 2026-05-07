package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mordor-forge/agent-memory/internal/apicontract"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type mockWriteAPI struct {
	tenant         memory.Tenant
	agent          memory.Agent
	thread         memory.Thread
	episode        memory.Episode
	exportedTenant memory.TenantExport
	deleteResult   memory.TenantDeletionResult
}

func (m *mockWriteAPI) CreateTenant(_ context.Context, req memory.CreateTenantRequest) (memory.Tenant, error) {
	m.tenant.Slug = req.Slug
	return m.tenant, nil
}

func (m *mockWriteAPI) CreateAgent(_ context.Context, req memory.CreateAgentRequest) (memory.Agent, error) {
	m.agent.TenantID = req.TenantID
	m.agent.Name = req.Name
	return m.agent, nil
}

func (m *mockWriteAPI) CreateThread(_ context.Context, req memory.CreateThreadRequest) (memory.Thread, error) {
	m.thread.TenantID = req.TenantID
	m.thread.AgentID = req.AgentID
	return m.thread, nil
}

func (m *mockWriteAPI) AppendEpisode(_ context.Context, req memory.AppendEpisodeRequest) (memory.Episode, error) {
	m.episode.TenantID = req.TenantID
	m.episode.AgentID = req.AgentID
	m.episode.Kind = req.Kind
	m.episode.Content = req.Content
	return m.episode, nil
}

func (m *mockWriteAPI) ExportTenant(_ context.Context, tenantID uuid.UUID) (memory.TenantExport, error) {
	if m.exportedTenant.Tenant.ID == uuid.Nil {
		m.exportedTenant.Tenant.ID = tenantID
	}
	return m.exportedTenant, nil
}

func (m *mockWriteAPI) DeleteTenant(_ context.Context, tenantID uuid.UUID) (memory.TenantDeletionResult, error) {
	if m.deleteResult.TenantID == uuid.Nil {
		m.deleteResult = memory.TenantDeletionResult{TenantID: tenantID, Deleted: true}
	}
	return m.deleteResult, nil
}

type mockMemoryReader struct {
	items []memory.Memory
}

func (m *mockMemoryReader) QueryMemories(_ context.Context, _ memory.QueryMemoriesRequest) ([]memory.Memory, error) {
	return m.items, nil
}

type mockRecaller struct {
	hits []memory.RecallHit
}

func (m *mockRecaller) Recall(_ context.Context, _ memory.RecallRequest) ([]memory.RecallHit, error) {
	return m.hits, nil
}

type mockInspector struct {
	provenance  memory.MemoryProvenance
	checkpoints []memory.ProjectionCheckpoint
	runs        []memory.ConsolidationRun
	leases      []memory.WorkerLease
}

func (m *mockInspector) GetMemoryProvenance(_ context.Context, _ uuid.UUID) (memory.MemoryProvenance, error) {
	return m.provenance, nil
}

func (m *mockInspector) ListProjectionCheckpoints(_ context.Context, _ memory.ListProjectionCheckpointsRequest) ([]memory.ProjectionCheckpoint, error) {
	return m.checkpoints, nil
}

func (m *mockInspector) ListConsolidationRuns(_ context.Context, _ memory.ListConsolidationRunsRequest) ([]memory.ConsolidationRun, error) {
	return m.runs, nil
}

func (m *mockInspector) ListWorkerLeases(_ context.Context, _ memory.ListWorkerLeasesRequest) ([]memory.WorkerLease, error) {
	return m.leases, nil
}

func TestNew_CreatesServer(t *testing.T) {
	srv := New(nil, nil, nil, nil)
	if srv == nil {
		t.Fatal("New returned nil")
	}
	if srv.mcp == nil {
		t.Fatal("underlying MCP server is nil")
	}
}

func TestToolsRegistered(t *testing.T) {
	write := &mockWriteAPI{}
	memories := &mockMemoryReader{}
	recaller := &mockRecaller{}
	inspector := &mockInspector{}
	session := connectTestClient(t, NewWithOptions(write, memories, inspector, recaller, Options{
		Version:            "test-version",
		EmbedderProvider:   "deterministic",
		EmbedderModel:      "deterministic/8",
		EmbedderDimensions: 8,
	}))

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	wantTools := map[string]bool{
		"create_tenant":               false,
		"create_agent":                false,
		"create_thread":               false,
		"remember":                    false,
		"export_tenant":               false,
		"delete_tenant":               false,
		"list_memories":               false,
		"recall":                      false,
		"get_memory_provenance":       false,
		"list_projection_checkpoints": false,
		"list_projection_runs":        false,
		"list_leases":                 false,
		"get_config":                  false,
	}
	for _, tool := range result.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
	}
	for name, found := range wantTools {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestGetConfig(t *testing.T) {
	session := connectTestClient(t, NewWithOptions(nil, nil, nil, nil, Options{
		Version:            "test-version",
		EmbedderProvider:   "deterministic",
		EmbedderModel:      "deterministic/8",
		EmbedderDimensions: 8,
	}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_config"})
	if err != nil {
		t.Fatalf("CallTool(get_config): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	assertStructuredField(t, res, "backend", "cockroachdb")
	assertStructuredField(t, res, "version", "test-version")
	assertStructuredField(t, res, "httpApiVersion", apicontract.HTTPAPIVersion)
	assertStructuredField(t, res, "httpMediaType", apicontract.HTTPMediaType)
	assertStructuredField(t, res, "compatibilityStability", apicontract.CompatibilityStability)
	assertStructuredField(t, res, "embedderProvider", "deterministic")
}

func TestRemember(t *testing.T) {
	write := &mockWriteAPI{
		episode: memory.Episode{
			ID: uuid.New(),
		},
	}
	session := connectTestClient(t, NewWithOptions(write, nil, nil, nil, Options{
		Version: "test-version",
	}))

	tenantID := uuid.New()
	agentID := uuid.New()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "remember",
		Arguments: map[string]any{
			"tenantId": tenantID.String(),
			"agentId":  agentID.String(),
			"kind":     "note",
			"content":  "remember this",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(remember): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	assertStructuredField(t, res, "kind", "note")
	assertStructuredField(t, res, "content", "remember this")
}

func TestExportTenant(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	write := &mockWriteAPI{
		exportedTenant: memory.TenantExport{
			Tenant: memory.Tenant{ID: tenantID, Slug: "demo"},
		},
	}
	session := connectTestClient(t, NewWithOptions(write, nil, nil, nil, Options{Version: "test-version"}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "export_tenant",
		Arguments: map[string]any{
			"tenantId": tenantID.String(),
		},
	})
	if err != nil {
		t.Fatalf("CallTool(export_tenant): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	if res.StructuredContent == nil {
		t.Fatal("StructuredContent is nil")
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var payload struct {
		Tenant struct {
			Slug string `json:"slug"`
		} `json:"tenant"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	if payload.Tenant.Slug != "demo" {
		t.Fatalf("tenant slug = %q, want %q", payload.Tenant.Slug, "demo")
	}
}

func TestListProjectionCheckpoints(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	inspector := &mockInspector{
		checkpoints: []memory.ProjectionCheckpoint{
			{ProjectionName: "episode-digests", TenantID: tenantID, ShardID: 0},
		},
	}
	session := connectTestClient(t, NewWithOptions(nil, nil, inspector, nil, Options{Version: "test-version"}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_projection_checkpoints",
		Arguments: map[string]any{
			"tenantId": tenantID.String(),
			"limit":    5,
		},
	})
	if err != nil {
		t.Fatalf("CallTool(list_projection_checkpoints): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	assertStructuredValue(t, res, "count", float64(1))
}

func TestListProjectionRuns(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	now := time.Now().UTC()
	inspector := &mockInspector{
		runs: []memory.ConsolidationRun{
			{
				ID:             uuid.New(),
				TenantID:       tenantID,
				WorkerID:       uuid.New(),
				Status:         "completed",
				Provider:       "deterministic",
				Model:          "deterministic/1536",
				ProjectionName: "episode-digests",
				CreatedAt:      now,
				BatchStartedAt: now,
			},
		},
	}
	session := connectTestClient(t, NewWithOptions(nil, nil, inspector, nil, Options{Version: "test-version"}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "list_projection_runs",
		Arguments: map[string]any{
			"tenantId":       tenantID.String(),
			"projectionName": "episode-digests",
			"limit":          5,
		},
	})
	if err != nil {
		t.Fatalf("CallTool(list_projection_runs): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	if res.StructuredContent == nil {
		t.Fatal("StructuredContent is nil")
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var payload struct {
		Count float64 `json:"count"`
		Items []struct {
			ProjectionName string `json:"projectionName"`
			Model          string `json:"model"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	if payload.Count != 1 || len(payload.Items) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Items[0].ProjectionName != "episode-digests" || payload.Items[0].Model != "deterministic/1536" {
		t.Fatalf("unexpected run item: %+v", payload.Items[0])
	}
}

func TestGetMemoryProvenance(t *testing.T) {
	t.Parallel()

	memoryID := uuid.New()
	inspector := &mockInspector{
		provenance: memory.MemoryProvenance{
			MemoryID:       memoryID,
			TenantID:       uuid.New(),
			ProjectionName: "episode-digests",
			HasEmbedding:   true,
		},
	}
	session := connectTestClient(t, NewWithOptions(nil, nil, inspector, nil, Options{Version: "test-version"}))

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_memory_provenance",
		Arguments: map[string]any{
			"memoryId": memoryID.String(),
		},
	})
	if err != nil {
		t.Fatalf("CallTool(get_memory_provenance): %v", err)
	}
	if res.IsError {
		t.Fatal("expected success, got error result")
	}
	assertStructuredField(t, res, "projectionName", "episode-digests")
}

func connectTestClient(t *testing.T, srv *Server) *mcp.ClientSession {
	t.Helper()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		_ = srv.mcp.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	return session
}

func assertStructuredField(t *testing.T, res *mcp.CallToolResult, key, want string) {
	t.Helper()
	assertStructuredValue(t, res, key, want)
}

func assertStructuredValue(t *testing.T, res *mcp.CallToolResult, key string, want any) {
	t.Helper()

	if res.StructuredContent == nil {
		t.Fatal("StructuredContent is nil")
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	got, ok := payload[key]
	if !ok {
		t.Fatalf("structured field %q missing in %+v", key, payload)
	}
	if got != want {
		t.Fatalf("structured field %q = %v, want %v", key, got, want)
	}
}
