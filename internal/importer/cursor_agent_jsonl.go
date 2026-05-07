package importer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const cursorAgentJSONLFormat = "cursor-agent-jsonl"

// CursorAgentJSONLImporter imports Cursor agent transcript JSONL files.
type CursorAgentJSONLImporter struct{}

type cursorTranscriptEnvelope struct {
	Role    string `json:"role"`
	Message struct {
		Content []map[string]any `json:"content"`
	} `json:"message"`
}

// Format returns the importer format name.
func (CursorAgentJSONLImporter) Format() string {
	return cursorAgentJSONLFormat
}

// ImportFile parses a Cursor agent transcript JSONL file into normalized records.
func (CursorAgentJSONLImporter) ImportFile(ctx context.Context, req FileImportRequest) ([]Record, error) {
	file, err := os.Open(req.Path)
	if err != nil {
		return nil, fmt.Errorf("open transcript file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	records := make([]Record, 0)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		record, err := parseCursorTranscriptRecord(line, lineNumber)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan transcript file: %w", err)
	}
	return records, nil
}

func parseCursorTranscriptRecord(line []byte, ordinal int) (Record, error) {
	var envelope cursorTranscriptEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return Record{}, fmt.Errorf("parse transcript line %d: %w", ordinal, err)
	}

	content, payload := summarizeCursorTranscript(envelope.Role, envelope.Message.Content)
	return Record{
		Ordinal: ordinal,
		Kind:    cursorTranscriptKind(envelope.Role),
		Content: content,
		Payload: payload,
	}, nil
}

func summarizeCursorTranscript(role string, blocks []map[string]any) (string, map[string]any) {
	texts := make([]string, 0, len(blocks))
	toolNames := make([]string, 0)
	normalizedBlocks := make([]map[string]any, 0, len(blocks))

	for _, block := range blocks {
		normalized := make(map[string]any, len(block))
		for key, value := range block {
			normalized[key] = value
		}
		normalizedBlocks = append(normalizedBlocks, normalized)

		blockType, _ := block["type"].(string)
		switch strings.TrimSpace(blockType) {
		case "text":
			if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		case "tool_use":
			if name, ok := block["name"].(string); ok && strings.TrimSpace(name) != "" {
				toolNames = append(toolNames, strings.TrimSpace(name))
			}
		}
	}

	content := strings.Join(texts, "\n\n")
	if strings.TrimSpace(content) == "" && len(toolNames) > 0 {
		content = fmt.Sprintf("%s used tools: %s", cursorRoleLabel(role), strings.Join(toolNames, ", "))
	}
	if strings.TrimSpace(content) == "" {
		content = fmt.Sprintf("%s transcript message", cursorRoleLabel(role))
	}

	payload := map[string]any{
		"transcript_role":     strings.TrimSpace(role),
		"content_blocks":      normalizedBlocks,
		"content_block_count": len(normalizedBlocks),
	}
	if len(toolNames) > 0 {
		payload["tool_names"] = toolNames
	}
	return content, payload
}

func cursorTranscriptKind(role string) string {
	switch strings.TrimSpace(role) {
	case "user":
		return "conversation.user_message"
	case "assistant":
		return "conversation.assistant_message"
	default:
		return "conversation.message"
	}
}

func cursorRoleLabel(role string) string {
	switch strings.TrimSpace(role) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	default:
		return "conversation"
	}
}
