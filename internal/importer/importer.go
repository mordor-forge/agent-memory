package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

// FileImporter parses one source file format into normalized import records.
type FileImporter interface {
	Format() string
	ImportFile(ctx context.Context, req FileImportRequest) ([]Record, error)
}

// EpisodeWriter is the minimum persistence surface needed by the import runner.
type EpisodeWriter interface {
	memory.EpisodeWriter
}

// FileImportRequest describes the source file being imported.
type FileImportRequest struct {
	Path     string
	SourceID string
}

// Record is the normalized import unit before it is persisted as an episode.
type Record struct {
	Ordinal    int
	Kind       string
	Content    string
	Payload    map[string]any
	OccurredAt *time.Time
}

// ImportRequest describes a concrete import operation into one tenant/agent/thread scope.
type ImportRequest struct {
	Format   string
	FilePath string
	SourceID string
	TenantID uuid.UUID
	AgentID  uuid.UUID
	ThreadID *uuid.UUID
}

// Validate checks the import request.
func (r ImportRequest) Validate() error {
	if strings.TrimSpace(r.Format) == "" {
		return errors.New("import format must not be empty")
	}
	if strings.TrimSpace(r.FilePath) == "" {
		return errors.New("import file path must not be empty")
	}
	if r.TenantID == uuid.Nil {
		return errors.New("import tenant_id must not be nil")
	}
	if r.AgentID == uuid.Nil {
		return errors.New("import agent_id must not be nil")
	}
	if r.ThreadID != nil && *r.ThreadID == uuid.Nil {
		return errors.New("import thread_id must not be nil when provided")
	}
	return nil
}

// Report summarizes one import operation.
type Report struct {
	Format           string
	SourceID         string
	FilePath         string
	RecordsRead      int
	EpisodesImported int
}

// Runner coordinates format selection, import metadata, and episode persistence.
type Runner struct {
	writer    EpisodeWriter
	importers map[string]FileImporter
}

// NewRunner constructs a runner from the available format importers.
func NewRunner(writer EpisodeWriter, importers ...FileImporter) (*Runner, error) {
	if writer == nil {
		return nil, errors.New("episode writer must not be nil")
	}
	if len(importers) == 0 {
		return nil, errors.New("at least one importer must be provided")
	}
	byFormat := make(map[string]FileImporter, len(importers))
	for _, item := range importers {
		if item == nil {
			return nil, errors.New("importer must not be nil")
		}
		format := strings.TrimSpace(item.Format())
		if format == "" {
			return nil, errors.New("importer format must not be empty")
		}
		if _, exists := byFormat[format]; exists {
			return nil, fmt.Errorf("duplicate importer format %q", format)
		}
		byFormat[format] = item
	}
	return &Runner{
		writer:    writer,
		importers: byFormat,
	}, nil
}

// ImportFile parses a source file and persists the resulting episodes.
func (r *Runner) ImportFile(ctx context.Context, req ImportRequest) (Report, error) {
	if err := req.Validate(); err != nil {
		return Report{}, err
	}
	importer, ok := r.importers[strings.TrimSpace(req.Format)]
	if !ok {
		return Report{}, fmt.Errorf("unsupported import format %q", req.Format)
	}
	sourceID := strings.TrimSpace(req.SourceID)
	if sourceID == "" {
		sourceID = deriveSourceID(req.FilePath)
	}
	records, err := importer.ImportFile(ctx, FileImportRequest{
		Path:     req.FilePath,
		SourceID: sourceID,
	})
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Format:      req.Format,
		SourceID:    sourceID,
		FilePath:    req.FilePath,
		RecordsRead: len(records),
	}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		payload := mergeImportPayload(record.Payload, req.Format, sourceID, req.FilePath, record.Ordinal)
		_, err := r.writer.AppendEpisode(ctx, memory.AppendEpisodeRequest{
			TenantID:       req.TenantID,
			AgentID:        req.AgentID,
			ThreadID:       req.ThreadID,
			IdempotencyKey: importIdempotencyKey(req.Format, sourceID, record.Ordinal),
			Kind:           record.Kind,
			Content:        record.Content,
			Payload:        payload,
			OccurredAt:     record.OccurredAt,
		})
		if err != nil {
			return report, fmt.Errorf("import record %d: %w", record.Ordinal, err)
		}
		report.EpisodesImported++
	}
	return report, nil
}

func mergeImportPayload(payload map[string]any, format, sourceID, filePath string, ordinal int) map[string]any {
	result := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		result[key] = value
	}
	result["import"] = map[string]any{
		"format":      format,
		"source_id":   sourceID,
		"source_path": filePath,
		"ordinal":     ordinal,
	}
	return result
}

func importIdempotencyKey(format, sourceID string, ordinal int) string {
	return fmt.Sprintf("import:%s:%s:%d", strings.TrimSpace(format), strings.TrimSpace(sourceID), ordinal)
}

func deriveSourceID(filePath string) string {
	base := strings.TrimSpace(strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)))
	base = strings.ReplaceAll(base, " ", "_")
	if base != "" {
		return base
	}
	sum := sha256.Sum256([]byte(filePath))
	return "source-" + hex.EncodeToString(sum[:8])
}
