// Package mcpserver exposes agent-memory operations as MCP tools over stdio.
package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// WriteAPI exposes the write-side operations needed by the MCP tools.
type WriteAPI interface {
	memory.ResourceCreator
	memory.EpisodeWriter
}

// MemoryReader lists durable memory rows for MCP inspection tools.
type MemoryReader interface {
	memory.MemoryReader
}

// Inspector exposes provenance and projection state for MCP introspection tools.
type Inspector interface {
	memory.MemoryInspector
}

// Recaller performs semantic memory retrieval for MCP tools.
type Recaller interface {
	memory.Recaller
}

// Server wraps an MCP server and routes tool calls to memory operations.
type Server struct {
	mcp                *mcp.Server
	write              WriteAPI
	memories           MemoryReader
	inspector          Inspector
	recaller           Recaller
	version            string
	embedderProvider   string
	embedderModel      string
	embedderDimensions int
}

// Options configures MCP metadata and exposed config details.
type Options struct {
	Version            string
	EmbedderProvider   string
	EmbedderModel      string
	EmbedderDimensions int
}

// New creates an MCP server with memory tools.
func New(write WriteAPI, memories MemoryReader, inspector Inspector, recaller Recaller) *Server {
	return NewWithOptions(write, memories, inspector, recaller, Options{
		Version:            "dev",
		EmbedderProvider:   "unknown",
		EmbedderModel:      "unknown",
		EmbedderDimensions: 0,
	})
}

// NewWithOptions creates an MCP server with explicit metadata.
func NewWithOptions(write WriteAPI, memories MemoryReader, inspector Inspector, recaller Recaller, opts Options) *Server {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.EmbedderProvider == "" {
		opts.EmbedderProvider = "unknown"
	}
	if opts.EmbedderModel == "" {
		opts.EmbedderModel = "unknown"
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "agent-memory-mcp",
		Version: opts.Version,
	}, nil)

	s := &Server{
		mcp:                mcpServer,
		write:              write,
		memories:           memories,
		inspector:          inspector,
		recaller:           recaller,
		version:            opts.Version,
		embedderProvider:   opts.EmbedderProvider,
		embedderModel:      opts.EmbedderModel,
		embedderDimensions: opts.EmbedderDimensions,
	}

	if write != nil {
		s.registerWriteTools()
	}
	if memories != nil {
		s.registerMemoryTools()
	}
	if inspector != nil {
		s.registerInspectionTools()
	}
	if recaller != nil {
		s.registerRecallTools()
	}
	s.registerConfigTools()

	return s
}

// Run starts the MCP server on stdio and blocks until the client disconnects or context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// MCPServer returns the underlying server for tests.
func (s *Server) MCPServer() *mcp.Server {
	return s.mcp
}
