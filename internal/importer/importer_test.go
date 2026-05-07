package importer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type fakeEpisodeWriter struct {
	requests []memory.AppendEpisodeRequest
}

func (f *fakeEpisodeWriter) AppendEpisode(_ context.Context, req memory.AppendEpisodeRequest) (memory.Episode, error) {
	f.requests = append(f.requests, req)
	return memory.Episode{
		ID:       uuid.New(),
		TenantID: req.TenantID,
		AgentID:  req.AgentID,
		ThreadID: req.ThreadID,
		Kind:     req.Kind,
		Content:  req.Content,
		Payload:  req.Payload,
	}, nil
}

func TestCursorAgentJSONLImporterImportFile(t *testing.T) {
	t.Parallel()

	importer := CursorAgentJSONLImporter{}
	records, err := importer.ImportFile(context.Background(), FileImportRequest{
		Path: filepath.Join("testdata", "cursor_agent_transcript.jsonl"),
	})
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}
	if records[0].Kind != "conversation.user_message" {
		t.Fatalf("records[0].Kind = %q", records[0].Kind)
	}
	if records[1].Kind != "conversation.assistant_message" {
		t.Fatalf("records[1].Kind = %q", records[1].Kind)
	}
	if records[2].Content != "assistant used tools: ReadFile" {
		t.Fatalf("records[2].Content = %q, want %q", records[2].Content, "assistant used tools: ReadFile")
	}
	toolNames, ok := records[2].Payload["tool_names"].([]string)
	if !ok || len(toolNames) != 1 || toolNames[0] != "ReadFile" {
		t.Fatalf("unexpected tool_names payload: %+v", records[2].Payload["tool_names"])
	}
}

func TestRunnerImportFileAppendsEpisodesWithImportMetadata(t *testing.T) {
	t.Parallel()

	writer := &fakeEpisodeWriter{}
	runner, err := NewRunner(writer, CursorAgentJSONLImporter{})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	tenantID := uuid.New()
	agentID := uuid.New()
	threadID := uuid.New()
	filePath := filepath.Join("testdata", "cursor_agent_transcript.jsonl")
	report, err := runner.ImportFile(context.Background(), ImportRequest{
		Format:   cursorAgentJSONLFormat,
		FilePath: filePath,
		SourceID: "session-123",
		TenantID: tenantID,
		AgentID:  agentID,
		ThreadID: &threadID,
	})
	if err != nil {
		t.Fatalf("ImportFile() error = %v", err)
	}
	if report.RecordsRead != 3 || report.EpisodesImported != 3 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(writer.requests) != 3 {
		t.Fatalf("len(writer.requests) = %d, want 3", len(writer.requests))
	}
	if writer.requests[0].IdempotencyKey != "import:cursor-agent-jsonl:session-123:1" {
		t.Fatalf("IdempotencyKey = %q", writer.requests[0].IdempotencyKey)
	}
	if writer.requests[0].TenantID != tenantID || writer.requests[0].AgentID != agentID {
		t.Fatalf("unexpected scope on first request: %+v", writer.requests[0])
	}
	if writer.requests[0].ThreadID == nil || *writer.requests[0].ThreadID != threadID {
		t.Fatalf("unexpected thread on first request: %+v", writer.requests[0].ThreadID)
	}

	importMeta, ok := writer.requests[0].Payload["import"].(map[string]any)
	if !ok {
		t.Fatalf("missing import metadata: %+v", writer.requests[0].Payload)
	}
	if importMeta["format"] != cursorAgentJSONLFormat || importMeta["source_id"] != "session-123" {
		t.Fatalf("unexpected import metadata: %+v", importMeta)
	}
	if importMeta["source_path"] != filePath {
		t.Fatalf("source_path = %v, want %q", importMeta["source_path"], filePath)
	}
	if importMeta["ordinal"] != 1 {
		t.Fatalf("ordinal = %v, want %d", importMeta["ordinal"], 1)
	}
	if writer.requests[2].Content != "assistant used tools: ReadFile" {
		t.Fatalf("unexpected imported content: %q", writer.requests[2].Content)
	}
}
