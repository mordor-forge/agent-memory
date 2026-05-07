package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type createTenantInput struct {
	Slug string `json:"slug" jsonschema:"Tenant slug used as a stable top-level memory boundary"`
}

type tenantIDInput struct {
	TenantID string `json:"tenantId" jsonschema:"Tenant UUID"`
}

type createAgentInput struct {
	TenantID    string         `json:"tenantId" jsonschema:"Tenant UUID returned by create_tenant"`
	ExternalRef string         `json:"externalRef,omitempty" jsonschema:"Optional external identifier for this agent"`
	Name        string         `json:"name" jsonschema:"Human-readable agent name"`
	Metadata    map[string]any `json:"metadata,omitempty" jsonschema:"Optional metadata for the agent"`
}

type createThreadInput struct {
	TenantID    string         `json:"tenantId" jsonschema:"Tenant UUID returned by create_tenant"`
	AgentID     string         `json:"agentId" jsonschema:"Agent UUID returned by create_agent"`
	ExternalRef string         `json:"externalRef,omitempty" jsonschema:"Optional external identifier for this thread"`
	Metadata    map[string]any `json:"metadata,omitempty" jsonschema:"Optional metadata for the thread"`
}

type rememberInput struct {
	TenantID       string         `json:"tenantId" jsonschema:"Tenant UUID"`
	AgentID        string         `json:"agentId" jsonschema:"Agent UUID"`
	ThreadID       string         `json:"threadId,omitempty" jsonschema:"Optional thread UUID"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty" jsonschema:"Optional idempotency key for safe retries"`
	Kind           string         `json:"kind" jsonschema:"Episode kind such as note, observation, tool_result, or state.update"`
	Content        string         `json:"content" jsonschema:"Raw episode content to record"`
	Payload        map[string]any `json:"payload,omitempty" jsonschema:"Optional structured payload for projections"`
	OccurredAt     string         `json:"occurredAt,omitempty" jsonschema:"Optional RFC3339 timestamp representing when the event originally occurred"`
}

type listMemoriesInput struct {
	TenantID string `json:"tenantId" jsonschema:"Tenant UUID"`
	AgentID  string `json:"agentId,omitempty" jsonschema:"Optional agent UUID filter"`
	ThreadID string `json:"threadId,omitempty" jsonschema:"Optional thread UUID filter"`
	Kind     string `json:"kind,omitempty" jsonschema:"Optional memory kind filter"`
	Status   string `json:"status,omitempty" jsonschema:"Optional memory status filter"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum number of rows to return"`
}

type recallInput struct {
	TenantID string `json:"tenantId" jsonschema:"Tenant UUID"`
	AgentID  string `json:"agentId,omitempty" jsonschema:"Optional agent UUID filter"`
	ThreadID string `json:"threadId,omitempty" jsonschema:"Optional thread UUID filter"`
	Kind     string `json:"kind,omitempty" jsonschema:"Optional memory kind filter"`
	Status   string `json:"status,omitempty" jsonschema:"Optional memory status filter"`
	Query    string `json:"query" jsonschema:"Semantic recall query"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum number of recall hits to return"`
}

type listMemoriesResult struct {
	Count int            `json:"count"`
	Items []memoryResult `json:"items"`
}

type recallResult struct {
	Count int               `json:"count"`
	Items []recallHitResult `json:"items"`
}

type tenantResult struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"createdAt"`
}

type agentResult struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenantId"`
	ExternalRef string         `json:"externalRef,omitempty"`
	Name        string         `json:"name"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   string         `json:"createdAt"`
}

type threadResult struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenantId"`
	AgentID     string         `json:"agentId"`
	ExternalRef string         `json:"externalRef,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   string         `json:"createdAt"`
}

type episodeResult struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenantId"`
	AgentID        string         `json:"agentId"`
	ThreadID       string         `json:"threadId,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	Payload        map[string]any `json:"payload"`
	OccurredAt     string         `json:"occurredAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

type tenantExportResult struct {
	Tenant                tenantResult                 `json:"tenant"`
	Agents                []agentResult                `json:"agents"`
	Threads               []threadResult               `json:"threads"`
	Episodes              []episodeResult              `json:"episodes"`
	Memories              []memoryResult               `json:"memories"`
	ProjectionCheckpoints []projectionCheckpointResult `json:"projectionCheckpoints"`
	ConsolidationRuns     []consolidationRunResult     `json:"consolidationRuns"`
	WorkerLeases          []workerLeaseResult          `json:"workerLeases"`
}

type tenantDeletionResult struct {
	TenantID string `json:"tenantId"`
	Deleted  bool   `json:"deleted"`
}

type memoryResult struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	AgentID            string         `json:"agentId"`
	ThreadID           string         `json:"threadId,omitempty"`
	Kind               string         `json:"kind"`
	Status             string         `json:"status"`
	Content            string         `json:"content"`
	Summary            string         `json:"summary,omitempty"`
	Attributes         map[string]any `json:"attributes"`
	Importance         float64        `json:"importance"`
	Confidence         float64        `json:"confidence"`
	SupersedesMemoryID string         `json:"supersedesMemoryId,omitempty"`
	LastObservedAt     string         `json:"lastObservedAt,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type recallHitResult struct {
	Memory   memoryResult `json:"memory"`
	Distance float64      `json:"distance"`
	Score    float64      `json:"score"`
}

func (s *Server) registerWriteTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_tenant",
		Description: "Create a tenant boundary for memory data.",
	}, s.handleCreateTenant)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_agent",
		Description: "Create an agent inside a tenant.",
	}, s.handleCreateAgent)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "create_thread",
		Description: "Create a thread inside an agent.",
	}, s.handleCreateThread)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "remember",
		Description: "Record one immutable episode of agent activity for later projection and recall.",
	}, s.handleRemember)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "export_tenant",
		Description: "Export tenant-scoped data and operational state for backup or inspection.",
	}, s.handleExportTenant)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "delete_tenant",
		Description: "Delete a tenant and its associated data. Destructive operation.",
	}, s.handleDeleteTenant)
}

func (s *Server) registerMemoryTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_memories",
		Description: "List durable memory rows filtered by tenant and optional agent/thread/kind/status.",
	}, s.handleListMemories)
}

func (s *Server) registerRecallTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "recall",
		Description: "Semantically search stored memory embeddings and return the nearest matching memories.",
	}, s.handleRecall)
}

func (s *Server) handleCreateTenant(ctx context.Context, _ *mcp.CallToolRequest, input createTenantInput) (*mcp.CallToolResult, tenantResult, error) {
	tenant, err := s.write.CreateTenant(ctx, memory.CreateTenantRequest{Slug: input.Slug})
	if err != nil {
		return nil, tenantResult{}, fmt.Errorf("create tenant: %w", err)
	}
	result := tenantToResult(tenant)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Tenant created.\n\nID: %s\nSlug: %s", tenant.ID, tenant.Slug)},
		},
	}, result, nil
}

func (s *Server) handleCreateAgent(ctx context.Context, _ *mcp.CallToolRequest, input createAgentInput) (*mcp.CallToolResult, agentResult, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, agentResult{}, fmt.Errorf("parse tenant id: %w", err)
	}
	agent, err := s.write.CreateAgent(ctx, memory.CreateAgentRequest{
		TenantID:    tenantID,
		ExternalRef: input.ExternalRef,
		Name:        input.Name,
		Metadata:    input.Metadata,
	})
	if err != nil {
		return nil, agentResult{}, fmt.Errorf("create agent: %w", err)
	}
	result := agentToResult(agent)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Agent created.\n\nID: %s\nTenant: %s\nName: %s", agent.ID, agent.TenantID, agent.Name)},
		},
	}, result, nil
}

func (s *Server) handleCreateThread(ctx context.Context, _ *mcp.CallToolRequest, input createThreadInput) (*mcp.CallToolResult, threadResult, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, threadResult{}, fmt.Errorf("parse tenant id: %w", err)
	}
	agentID, err := uuid.Parse(input.AgentID)
	if err != nil {
		return nil, threadResult{}, fmt.Errorf("parse agent id: %w", err)
	}
	thread, err := s.write.CreateThread(ctx, memory.CreateThreadRequest{
		TenantID:    tenantID,
		AgentID:     agentID,
		ExternalRef: input.ExternalRef,
		Metadata:    input.Metadata,
	})
	if err != nil {
		return nil, threadResult{}, fmt.Errorf("create thread: %w", err)
	}
	result := threadToResult(thread)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Thread created.\n\nID: %s\nTenant: %s\nAgent: %s", thread.ID, thread.TenantID, thread.AgentID)},
		},
	}, result, nil
}

func (s *Server) handleRemember(ctx context.Context, _ *mcp.CallToolRequest, input rememberInput) (*mcp.CallToolResult, episodeResult, error) {
	req, err := appendEpisodeRequestFromInput(input)
	if err != nil {
		return nil, episodeResult{}, fmt.Errorf("remember input: %w", err)
	}
	episode, err := s.write.AppendEpisode(ctx, req)
	if err != nil {
		return nil, episodeResult{}, fmt.Errorf("remember: %w", err)
	}
	result := episodeToResult(episode)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Episode recorded.\n\nID: %s\nKind: %s\nTenant: %s", episode.ID, episode.Kind, episode.TenantID)},
		},
	}, result, nil
}

func (s *Server) handleExportTenant(ctx context.Context, _ *mcp.CallToolRequest, input tenantIDInput) (*mcp.CallToolResult, tenantExportResult, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, tenantExportResult{}, fmt.Errorf("parse tenant id: %w", err)
	}
	exported, err := s.write.ExportTenant(ctx, tenantID)
	if err != nil {
		return nil, tenantExportResult{}, fmt.Errorf("export tenant: %w", err)
	}
	result := tenantExportToResult(exported)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Tenant export generated.\n\nTenant: %s\nAgents: %d\nEpisodes: %d\nMemories: %d", result.Tenant.ID, len(result.Agents), len(result.Episodes), len(result.Memories))},
		},
	}, result, nil
}

func (s *Server) handleDeleteTenant(ctx context.Context, _ *mcp.CallToolRequest, input tenantIDInput) (*mcp.CallToolResult, tenantDeletionResult, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return nil, tenantDeletionResult{}, fmt.Errorf("parse tenant id: %w", err)
	}
	deletion, err := s.write.DeleteTenant(ctx, tenantID)
	if err != nil {
		return nil, tenantDeletionResult{}, fmt.Errorf("delete tenant: %w", err)
	}
	result := tenantDeletionResult{
		TenantID: deletion.TenantID.String(),
		Deleted:  deletion.Deleted,
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Tenant deleted.\n\nTenant: %s", result.TenantID)},
		},
	}, result, nil
}

func (s *Server) handleListMemories(ctx context.Context, _ *mcp.CallToolRequest, input listMemoriesInput) (*mcp.CallToolResult, listMemoriesResult, error) {
	req, err := queryMemoriesRequestFromInput(input)
	if err != nil {
		return nil, listMemoriesResult{}, fmt.Errorf("list memories input: %w", err)
	}
	items, err := s.memories.QueryMemories(ctx, req)
	if err != nil {
		return nil, listMemoriesResult{}, fmt.Errorf("list memories: %w", err)
	}
	result := listMemoriesResult{
		Count: len(items),
		Items: toMemoryResults(items),
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Memory rows returned: %d", result.Count)},
		},
	}, result, nil
}

func (s *Server) handleRecall(ctx context.Context, _ *mcp.CallToolRequest, input recallInput) (*mcp.CallToolResult, recallResult, error) {
	req, err := recallRequestFromInput(input)
	if err != nil {
		return nil, recallResult{}, fmt.Errorf("recall input: %w", err)
	}
	items, err := s.recaller.Recall(ctx, req)
	if err != nil {
		return nil, recallResult{}, fmt.Errorf("recall: %w", err)
	}
	result := recallResult{
		Count: len(items),
		Items: toRecallHitResults(items),
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Recall hits returned: %d", result.Count)},
		},
	}, result, nil
}

func tenantToResult(value memory.Tenant) tenantResult {
	return tenantResult{
		ID:        value.ID.String(),
		Slug:      value.Slug,
		CreatedAt: value.CreatedAt.Format(time.RFC3339),
	}
}

func agentToResult(value memory.Agent) agentResult {
	return agentResult{
		ID:          value.ID.String(),
		TenantID:    value.TenantID.String(),
		ExternalRef: value.ExternalRef,
		Name:        value.Name,
		Metadata:    nonNilMap(value.Metadata),
		CreatedAt:   value.CreatedAt.Format(time.RFC3339),
	}
}

func threadToResult(value memory.Thread) threadResult {
	result := threadResult{
		ID:          value.ID.String(),
		TenantID:    value.TenantID.String(),
		AgentID:     value.AgentID.String(),
		ExternalRef: value.ExternalRef,
		Metadata:    nonNilMap(value.Metadata),
		CreatedAt:   value.CreatedAt.Format(time.RFC3339),
	}
	return result
}

func episodeToResult(value memory.Episode) episodeResult {
	result := episodeResult{
		ID:             value.ID.String(),
		TenantID:       value.TenantID.String(),
		AgentID:        value.AgentID.String(),
		IdempotencyKey: value.IdempotencyKey,
		Kind:           value.Kind,
		Content:        value.Content,
		Payload:        nonNilMap(value.Payload),
		CreatedAt:      value.CreatedAt.Format(time.RFC3339),
	}
	if value.ThreadID != nil {
		result.ThreadID = value.ThreadID.String()
	}
	if value.OccurredAt != nil {
		result.OccurredAt = value.OccurredAt.Format(time.RFC3339)
	}
	return result
}

func memoryToResult(value memory.Memory) memoryResult {
	result := memoryResult{
		ID:         value.ID.String(),
		TenantID:   value.TenantID.String(),
		AgentID:    value.AgentID.String(),
		Kind:       value.Kind,
		Status:     value.Status,
		Content:    value.Content,
		Attributes: nonNilMap(value.Attributes),
		Importance: value.Importance,
		Confidence: value.Confidence,
		CreatedAt:  value.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  value.UpdatedAt.Format(time.RFC3339),
	}
	if value.ThreadID != nil {
		result.ThreadID = value.ThreadID.String()
	}
	if value.Summary != nil {
		result.Summary = *value.Summary
	}
	if value.SupersedesMemoryID != nil {
		result.SupersedesMemoryID = value.SupersedesMemoryID.String()
	}
	if value.LastObservedAt != nil {
		result.LastObservedAt = value.LastObservedAt.Format(time.RFC3339)
	}
	return result
}

func toMemoryResults(items []memory.Memory) []memoryResult {
	results := make([]memoryResult, 0, len(items))
	for _, item := range items {
		results = append(results, memoryToResult(item))
	}
	return results
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func toRecallHitResults(items []memory.RecallHit) []recallHitResult {
	results := make([]recallHitResult, 0, len(items))
	for _, item := range items {
		results = append(results, recallHitResult{
			Memory:   memoryToResult(item.Memory),
			Distance: item.Distance,
			Score:    item.Score,
		})
	}
	return results
}

func tenantExportToResult(value memory.TenantExport) tenantExportResult {
	result := tenantExportResult{
		Tenant:                tenantToResult(value.Tenant),
		Agents:                make([]agentResult, 0, len(value.Agents)),
		Threads:               make([]threadResult, 0, len(value.Threads)),
		Episodes:              make([]episodeResult, 0, len(value.Episodes)),
		Memories:              make([]memoryResult, 0, len(value.Memories)),
		ProjectionCheckpoints: make([]projectionCheckpointResult, 0, len(value.ProjectionCheckpoints)),
		ConsolidationRuns:     make([]consolidationRunResult, 0, len(value.ConsolidationRuns)),
		WorkerLeases:          make([]workerLeaseResult, 0, len(value.WorkerLeases)),
	}
	for _, item := range value.Agents {
		result.Agents = append(result.Agents, agentToResult(item))
	}
	for _, item := range value.Threads {
		result.Threads = append(result.Threads, threadToResult(item))
	}
	for _, item := range value.Episodes {
		result.Episodes = append(result.Episodes, episodeToResult(item))
	}
	for _, item := range value.Memories {
		result.Memories = append(result.Memories, memoryToResult(item))
	}
	result.ProjectionCheckpoints = projectionCheckpointsToResult(value.ProjectionCheckpoints).Items
	result.ConsolidationRuns = consolidationRunsToResult(value.ConsolidationRuns).Items
	result.WorkerLeases = workerLeasesToResult(value.WorkerLeases).Items
	return result
}

func appendEpisodeRequestFromInput(input rememberInput) (memory.AppendEpisodeRequest, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return memory.AppendEpisodeRequest{}, err
	}
	agentID, err := uuid.Parse(input.AgentID)
	if err != nil {
		return memory.AppendEpisodeRequest{}, err
	}
	req := memory.AppendEpisodeRequest{
		TenantID:       tenantID,
		AgentID:        agentID,
		IdempotencyKey: input.IdempotencyKey,
		Kind:           input.Kind,
		Content:        input.Content,
		Payload:        input.Payload,
	}
	if input.ThreadID != "" {
		threadID, err := uuid.Parse(input.ThreadID)
		if err != nil {
			return memory.AppendEpisodeRequest{}, err
		}
		req.ThreadID = &threadID
	}
	if input.OccurredAt != "" {
		occurredAt, err := time.Parse(time.RFC3339, input.OccurredAt)
		if err != nil {
			return memory.AppendEpisodeRequest{}, err
		}
		req.OccurredAt = &occurredAt
	}
	return req, nil
}

func queryMemoriesRequestFromInput(input listMemoriesInput) (memory.QueryMemoriesRequest, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return memory.QueryMemoriesRequest{}, err
	}
	req := memory.QueryMemoriesRequest{
		TenantID: tenantID,
		Kind:     input.Kind,
		Status:   input.Status,
		Limit:    input.Limit,
	}
	if input.AgentID != "" {
		agentID, err := uuid.Parse(input.AgentID)
		if err != nil {
			return memory.QueryMemoriesRequest{}, err
		}
		req.AgentID = &agentID
	}
	if input.ThreadID != "" {
		threadID, err := uuid.Parse(input.ThreadID)
		if err != nil {
			return memory.QueryMemoriesRequest{}, err
		}
		req.ThreadID = &threadID
	}
	return req, nil
}

func recallRequestFromInput(input recallInput) (memory.RecallRequest, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return memory.RecallRequest{}, err
	}
	req := memory.RecallRequest{
		TenantID: tenantID,
		Kind:     input.Kind,
		Status:   input.Status,
		Query:    input.Query,
		Limit:    input.Limit,
	}
	if input.AgentID != "" {
		agentID, err := uuid.Parse(input.AgentID)
		if err != nil {
			return memory.RecallRequest{}, err
		}
		req.AgentID = &agentID
	}
	if input.ThreadID != "" {
		threadID, err := uuid.Parse(input.ThreadID)
		if err != nil {
			return memory.RecallRequest{}, err
		}
		req.ThreadID = &threadID
	}
	return req, nil
}
