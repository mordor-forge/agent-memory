package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mordor-forge/agent-memory/internal/apicontract"
)

// EmptyInput is used for tools that take no arguments.
type EmptyInput struct{}

type configResult struct {
	Version                string `json:"version"`
	Backend                string `json:"backend"`
	HTTPAPIVersion         string `json:"httpApiVersion"`
	HTTPMediaType          string `json:"httpMediaType"`
	CompatibilityStability string `json:"compatibilityStability"`
	EmbedderProvider       string `json:"embedderProvider"`
	EmbedderModel          string `json:"embedderModel"`
	EmbedderDimensions     int    `json:"embedderDimensions"`
}

func (s *Server) registerConfigTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "get_config",
		Description: "Show current server configuration including backend, build version, HTTP API version, and active embedder.",
	}, s.handleGetConfig)
}

func (s *Server) handleGetConfig(_ context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, configResult, error) {
	result := configResult{
		Version:                s.version,
		Backend:                "cockroachdb",
		HTTPAPIVersion:         apicontract.HTTPAPIVersion,
		HTTPMediaType:          apicontract.HTTPMediaType,
		CompatibilityStability: apicontract.CompatibilityStability,
		EmbedderProvider:       s.embedderProvider,
		EmbedderModel:          s.embedderModel,
		EmbedderDimensions:     s.embedderDimensions,
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(
				"Backend: %s\nVersion: %s\nHTTP API version: %s\nHTTP media type: %s\nCompatibility stability: %s\nEmbedder provider: %s\nEmbedder model: %s\nEmbedder dimensions: %d",
				result.Backend,
				result.Version,
				result.HTTPAPIVersion,
				result.HTTPMediaType,
				result.CompatibilityStability,
				result.EmbedderProvider,
				result.EmbedderModel,
				result.EmbedderDimensions,
			)},
		},
	}, result, nil
}
