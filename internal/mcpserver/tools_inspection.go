package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type memoryIDInput struct {
	MemoryID string `json:"memoryId" jsonschema:"Memory UUID"`
}

type listProjectionCheckpointsInput struct {
	TenantID       string `json:"tenantId" jsonschema:"Tenant UUID"`
	ProjectionName string `json:"projectionName,omitempty" jsonschema:"Optional projection name filter"`
	Limit          int    `json:"limit,omitempty" jsonschema:"Maximum number of checkpoint rows to return"`
}

type listProjectionRunsInput struct {
	TenantID       string `json:"tenantId" jsonschema:"Tenant UUID"`
	ProjectionName string `json:"projectionName,omitempty" jsonschema:"Optional projection name filter"`
	Limit          int    `json:"limit,omitempty" jsonschema:"Maximum number of run rows to return"`
}

type listLeasesInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of lease rows to return"`
}

type provenanceResult struct {
	MemoryID       string                   `json:"memoryId"`
	TenantID       string                   `json:"tenantId"`
	ProjectionName string                   `json:"projectionName"`
	HasEmbedding   bool                     `json:"hasEmbedding"`
	Sources        []provenanceSourceResult `json:"sources"`
}

type provenanceSourceResult struct {
	EpisodeID          string `json:"episodeId"`
	EpisodeKind        string `json:"episodeKind"`
	EpisodeContent     string `json:"episodeContent"`
	EpisodeCreatedAt   string `json:"episodeCreatedAt"`
	Role               string `json:"role"`
	ConsolidationRunID string `json:"consolidationRunId"`
}

type projectionCheckpointsResult struct {
	Count int                          `json:"count"`
	Items []projectionCheckpointResult `json:"items"`
}

type projectionCheckpointResult struct {
	ProjectionName string                  `json:"projectionName"`
	TenantID       string                  `json:"tenantId"`
	ShardID        int64                   `json:"shardId"`
	LastCursor     *checkpointCursorResult `json:"lastCursor,omitempty"`
	UpdatedAt      string                  `json:"updatedAt"`
}

type consolidationRunsResult struct {
	Count int                      `json:"count"`
	Items []consolidationRunResult `json:"items"`
}

type consolidationRunResult struct {
	ID             string                  `json:"id"`
	TenantID       string                  `json:"tenantId"`
	WorkerID       string                  `json:"workerId"`
	InputFrom      *checkpointCursorResult `json:"inputFrom,omitempty"`
	InputTo        *checkpointCursorResult `json:"inputTo,omitempty"`
	Status         string                  `json:"status"`
	Provider       string                  `json:"provider"`
	Model          string                  `json:"model"`
	ProjectionName string                  `json:"projectionName"`
	ErrorText      string                  `json:"errorText,omitempty"`
	CreatedAt      string                  `json:"createdAt"`
	CompletedAt    string                  `json:"completedAt,omitempty"`
	BatchStartedAt string                  `json:"batchStartedAt"`
}

type workerLeasesResult struct {
	Count int                 `json:"count"`
	Items []workerLeaseResult `json:"items"`
}

type workerLeaseResult struct {
	LeaseName string         `json:"leaseName"`
	HolderID  string         `json:"holderId"`
	ExpiresAt string         `json:"expiresAt"`
	RenewedAt string         `json:"renewedAt"`
	Metadata  map[string]any `json:"metadata"`
}

type checkpointCursorResult struct {
	CreatedAt string `json:"createdAt"`
	ID        string `json:"id"`
}

func (s *Server) registerInspectionTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "get_memory_provenance",
		Description: "Explain why a memory exists by returning its source episodes, projection name, and embedding status.",
	}, s.handleGetMemoryProvenance)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_projection_checkpoints",
		Description: "List persisted projection checkpoint cursors for a tenant.",
	}, s.handleListProjectionCheckpoints)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_projection_runs",
		Description: "List recent projection or consolidation runs for a tenant.",
	}, s.handleListProjectionRuns)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_leases",
		Description: "List currently persisted worker lease rows.",
	}, s.handleListLeases)
}

func (s *Server) handleGetMemoryProvenance(ctx context.Context, _ *mcp.CallToolRequest, input memoryIDInput) (*mcp.CallToolResult, provenanceResult, error) {
	memoryID, err := uuid.Parse(input.MemoryID)
	if err != nil {
		return nil, provenanceResult{}, fmt.Errorf("parse memory id: %w", err)
	}
	record, err := s.inspector.GetMemoryProvenance(ctx, memoryID)
	if err != nil {
		return nil, provenanceResult{}, fmt.Errorf("get memory provenance: %w", err)
	}
	result := provenanceToResult(record)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Projection: %s\nSources: %d\nHas embedding: %t", result.ProjectionName, len(result.Sources), result.HasEmbedding)},
		},
	}, result, nil
}

func (s *Server) handleListProjectionCheckpoints(ctx context.Context, _ *mcp.CallToolRequest, input listProjectionCheckpointsInput) (*mcp.CallToolResult, projectionCheckpointsResult, error) {
	req, err := projectionCheckpointsRequestFromInput(input)
	if err != nil {
		return nil, projectionCheckpointsResult{}, fmt.Errorf("projection checkpoint input: %w", err)
	}
	items, err := s.inspector.ListProjectionCheckpoints(ctx, req)
	if err != nil {
		return nil, projectionCheckpointsResult{}, fmt.Errorf("list projection checkpoints: %w", err)
	}
	result := projectionCheckpointsToResult(items)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Projection checkpoints returned: %d", result.Count)},
		},
	}, result, nil
}

func (s *Server) handleListProjectionRuns(ctx context.Context, _ *mcp.CallToolRequest, input listProjectionRunsInput) (*mcp.CallToolResult, consolidationRunsResult, error) {
	req, err := consolidationRunsRequestFromInput(input)
	if err != nil {
		return nil, consolidationRunsResult{}, fmt.Errorf("projection runs input: %w", err)
	}
	items, err := s.inspector.ListConsolidationRuns(ctx, req)
	if err != nil {
		return nil, consolidationRunsResult{}, fmt.Errorf("list projection runs: %w", err)
	}
	result := consolidationRunsToResult(items)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Projection runs returned: %d", result.Count)},
		},
	}, result, nil
}

func (s *Server) handleListLeases(ctx context.Context, _ *mcp.CallToolRequest, input listLeasesInput) (*mcp.CallToolResult, workerLeasesResult, error) {
	req := memory.ListWorkerLeasesRequest{Limit: input.Limit}
	if err := req.Validate(); err != nil {
		return nil, workerLeasesResult{}, fmt.Errorf("lease input: %w", err)
	}
	items, err := s.inspector.ListWorkerLeases(ctx, req)
	if err != nil {
		return nil, workerLeasesResult{}, fmt.Errorf("list leases: %w", err)
	}
	result := workerLeasesToResult(items)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Worker leases returned: %d", result.Count)},
		},
	}, result, nil
}

func provenanceToResult(value memory.MemoryProvenance) provenanceResult {
	result := provenanceResult{
		MemoryID:       value.MemoryID.String(),
		TenantID:       value.TenantID.String(),
		ProjectionName: value.ProjectionName,
		HasEmbedding:   value.HasEmbedding,
		Sources:        make([]provenanceSourceResult, 0, len(value.Sources)),
	}
	for _, source := range value.Sources {
		result.Sources = append(result.Sources, provenanceSourceResult{
			EpisodeID:          source.EpisodeID.String(),
			EpisodeKind:        source.EpisodeKind,
			EpisodeContent:     source.EpisodeContent,
			EpisodeCreatedAt:   source.EpisodeCreatedAt.Format(time.RFC3339),
			Role:               source.Role,
			ConsolidationRunID: source.ConsolidationRunID.String(),
		})
	}
	return result
}

func projectionCheckpointsToResult(items []memory.ProjectionCheckpoint) projectionCheckpointsResult {
	result := projectionCheckpointsResult{Count: len(items), Items: make([]projectionCheckpointResult, 0, len(items))}
	for _, item := range items {
		entry := projectionCheckpointResult{
			ProjectionName: item.ProjectionName,
			TenantID:       item.TenantID.String(),
			ShardID:        item.ShardID,
			UpdatedAt:      item.UpdatedAt.Format(time.RFC3339),
		}
		if item.LastCursor != nil {
			entry.LastCursor = &checkpointCursorResult{
				CreatedAt: item.LastCursor.CreatedAt.Format(time.RFC3339),
				ID:        item.LastCursor.ID.String(),
			}
		}
		result.Items = append(result.Items, entry)
	}
	return result
}

func consolidationRunsToResult(items []memory.ConsolidationRun) consolidationRunsResult {
	result := consolidationRunsResult{Count: len(items), Items: make([]consolidationRunResult, 0, len(items))}
	for _, item := range items {
		entry := consolidationRunResult{
			ID:             item.ID.String(),
			TenantID:       item.TenantID.String(),
			WorkerID:       item.WorkerID.String(),
			Status:         item.Status,
			Provider:       item.Provider,
			Model:          item.Model,
			ProjectionName: item.ProjectionName,
			CreatedAt:      item.CreatedAt.Format(time.RFC3339),
			BatchStartedAt: item.BatchStartedAt.Format(time.RFC3339),
		}
		if item.InputFrom != nil {
			entry.InputFrom = &checkpointCursorResult{CreatedAt: item.InputFrom.CreatedAt.Format(time.RFC3339), ID: item.InputFrom.ID.String()}
		}
		if item.InputTo != nil {
			entry.InputTo = &checkpointCursorResult{CreatedAt: item.InputTo.CreatedAt.Format(time.RFC3339), ID: item.InputTo.ID.String()}
		}
		if item.ErrorText != nil {
			entry.ErrorText = *item.ErrorText
		}
		if item.CompletedAt != nil {
			entry.CompletedAt = item.CompletedAt.Format(time.RFC3339)
		}
		result.Items = append(result.Items, entry)
	}
	return result
}

func workerLeasesToResult(items []memory.WorkerLease) workerLeasesResult {
	result := workerLeasesResult{Count: len(items), Items: make([]workerLeaseResult, 0, len(items))}
	for _, item := range items {
		result.Items = append(result.Items, workerLeaseResult{
			LeaseName: item.LeaseName,
			HolderID:  item.HolderID.String(),
			ExpiresAt: item.ExpiresAt.Format(time.RFC3339),
			RenewedAt: item.RenewedAt.Format(time.RFC3339),
			Metadata:  nonNilMap(item.Metadata),
		})
	}
	return result
}

func projectionCheckpointsRequestFromInput(input listProjectionCheckpointsInput) (memory.ListProjectionCheckpointsRequest, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return memory.ListProjectionCheckpointsRequest{}, err
	}
	req := memory.ListProjectionCheckpointsRequest{
		TenantID:       tenantID,
		ProjectionName: input.ProjectionName,
		Limit:          input.Limit,
	}
	return req, req.Validate()
}

func consolidationRunsRequestFromInput(input listProjectionRunsInput) (memory.ListConsolidationRunsRequest, error) {
	tenantID, err := uuid.Parse(input.TenantID)
	if err != nil {
		return memory.ListConsolidationRunsRequest{}, err
	}
	req := memory.ListConsolidationRunsRequest{
		TenantID:       tenantID,
		ProjectionName: input.ProjectionName,
		Limit:          input.Limit,
	}
	return req, req.Validate()
}
