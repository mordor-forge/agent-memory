package cockroach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	"github.com/mordor-forge/agent-memory/pkg/memory"
)

var (
	memoryNamespaceUUID           = uuid.MustParse("3f1fb9b0-a25f-4d42-9d7f-a4c9a9075f5d")
	consolidationRunNamespaceUUID = uuid.MustParse("8f9c522b-4a72-4ee4-886b-71a3641bcbf2")
)

// UpsertEpisodeMemories materializes one stable memory row per episode for a named projection.
func (s *Store) UpsertEpisodeMemories(ctx context.Context, projectionName string, workerID uuid.UUID, episodes []memory.Episode) error {
	records := make([]memory.ProjectedMemory, 0, len(episodes))
	for _, episode := range episodes {
		summary := summarize(episode.Content, 160)
		lastObservedAt := episode.CreatedAt
		if episode.OccurredAt != nil {
			lastObservedAt = *episode.OccurredAt
		}
		records = append(records, memory.ProjectedMemory{
			Key:      episode.ID.String(),
			TenantID: episode.TenantID,
			AgentID:  episode.AgentID,
			ThreadID: episode.ThreadID,
			Kind:     "episode_digest",
			Status:   "active",
			Content:  episode.Content,
			Summary:  &summary,
			Attributes: map[string]any{
				"source_kind": episode.Kind,
			},
			Importance:       0.5,
			Confidence:       1.0,
			LastObservedAt:   &lastObservedAt,
			SourceEpisodeIDs: []uuid.UUID{episode.ID},
			EmbedText:        episode.Content,
		})
	}
	inputFrom, inputTo := episodeCursorBounds(episodes)
	return s.UpsertProjectedMemories(ctx, projectionName, workerID, inputFrom, inputTo, records, nil, "", "")
}

// UpsertEpisodeMemoriesWithEmbeddings materializes episode-derived memories and their embeddings.
func (s *Store) UpsertEpisodeMemoriesWithEmbeddings(ctx context.Context, projectionName string, workerID uuid.UUID, episodes []memory.Episode, embeddings [][]float32, embeddingProvider, embeddingModel string) error {
	records := make([]memory.ProjectedMemory, 0, len(episodes))
	for _, episode := range episodes {
		summary := summarize(episode.Content, 160)
		lastObservedAt := episode.CreatedAt
		if episode.OccurredAt != nil {
			lastObservedAt = *episode.OccurredAt
		}
		records = append(records, memory.ProjectedMemory{
			Key:      episode.ID.String(),
			TenantID: episode.TenantID,
			AgentID:  episode.AgentID,
			ThreadID: episode.ThreadID,
			Kind:     "episode_digest",
			Status:   "active",
			Content:  episode.Content,
			Summary:  &summary,
			Attributes: map[string]any{
				"source_kind": episode.Kind,
			},
			Importance:       0.5,
			Confidence:       1.0,
			LastObservedAt:   &lastObservedAt,
			SourceEpisodeIDs: []uuid.UUID{episode.ID},
			EmbedText:        episode.Content,
		})
	}
	inputFrom, inputTo := episodeCursorBounds(episodes)
	return s.UpsertProjectedMemories(ctx, projectionName, workerID, inputFrom, inputTo, records, embeddings, embeddingProvider, embeddingModel)
}

// UpsertProjectedMemories persists generic projected memory records and optional embeddings.
func (s *Store) UpsertProjectedMemories(ctx context.Context, projectionName string, workerID uuid.UUID, inputFrom, inputTo *memory.CheckpointCursor, records []memory.ProjectedMemory, embeddings [][]float32, embeddingProvider, embeddingModel string) error {
	if projectionName == "" {
		return fmt.Errorf("upsert projected memories: projection name must not be empty")
	}
	if workerID == uuid.Nil {
		return fmt.Errorf("upsert projected memories: worker_id must not be nil")
	}
	if len(records) == 0 {
		return nil
	}
	if embeddings != nil && len(embeddings) != len(records) {
		return fmt.Errorf("upsert projected memories: embeddings count %d does not match record count %d", len(embeddings), len(records))
	}
	if embeddings != nil && strings.TrimSpace(embeddingProvider) == "" {
		return fmt.Errorf("upsert projected memories: embedding provider must not be empty when embeddings are provided")
	}
	if embeddings != nil && strings.TrimSpace(embeddingModel) == "" {
		return fmt.Errorf("upsert projected memories: embedding model must not be empty when embeddings are provided")
	}

	tenantID := records[0].TenantID
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
		if record.TenantID != tenantID {
			return fmt.Errorf("upsert projected memories: batch spans multiple tenants")
		}
	}

	firstSourceID := records[0].SourceEpisodeIDs[0]
	if inputFrom != nil {
		firstSourceID = inputFrom.ID
	}
	lastRecord := records[len(records)-1]
	lastSourceID := lastRecord.SourceEpisodeIDs[len(lastRecord.SourceEpisodeIDs)-1]
	if inputTo != nil {
		lastSourceID = inputTo.ID
	}
	runID := deterministicConsolidationRunID(projectionName, firstSourceID, lastSourceID)
	now := time.Now().UTC()
	var (
		inputFromCreatedAt any
		inputFromIDArg     any
		inputToCreatedAt   any
		inputToIDArg       any
	)
	if inputFrom != nil {
		inputFromCreatedAt = inputFrom.CreatedAt
		inputFromIDArg = inputFrom.ID
	}
	if inputTo != nil {
		inputToCreatedAt = inputTo.CreatedAt
		inputToIDArg = inputTo.ID
	}

	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO consolidation_runs (
				id,
				tenant_id,
				worker_id,
				batch_started_at,
				input_from_created_at,
				input_from_id,
				input_to_created_at,
				input_to_id,
				status,
				projection_name,
				provider,
				model,
				error_text,
				created_at,
				completed_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'completed', $9, $10, $11, NULL, now(), now())
			ON CONFLICT (id)
			DO UPDATE SET
				worker_id = EXCLUDED.worker_id,
				batch_started_at = EXCLUDED.batch_started_at,
				input_from_created_at = EXCLUDED.input_from_created_at,
				input_from_id = EXCLUDED.input_from_id,
				input_to_created_at = EXCLUDED.input_to_created_at,
				input_to_id = EXCLUDED.input_to_id,
				status = EXCLUDED.status,
				projection_name = EXCLUDED.projection_name,
				provider = EXCLUDED.provider,
				model = EXCLUDED.model,
				completed_at = EXCLUDED.completed_at`,
			runID,
			tenantID,
			workerID,
			now,
			inputFromCreatedAt,
			inputFromIDArg,
			inputToCreatedAt,
			inputToIDArg,
			projectionName,
			nullableString(embeddingProvider),
			nullableString(embeddingModel),
		); err != nil {
			return fmt.Errorf("upsert consolidation run: %w", err)
		}

		for i, record := range records {
			memoryID := deterministicMemoryID(projectionName, record.Key)
			persistRecord, err := s.mergeProjectedMemoryIfNeeded(ctx, tx, memoryID, record)
			if err != nil {
				return err
			}
			attributesMap := make(map[string]any, len(record.Attributes)+2)
			for key, value := range persistRecord.Attributes {
				attributesMap[key] = value
			}
			attributesMap["projection"] = projectionName
			if len(persistRecord.SourceEpisodeIDs) == 1 {
				attributesMap["source_episode_id"] = persistRecord.SourceEpisodeIDs[0].String()
			}
			attributes, err := marshalJSONMap(attributesMap)
			if err != nil {
				return err
			}

			if _, err := tx.Exec(ctx,
				`INSERT INTO memories (
					id,
					tenant_id,
					agent_id,
					thread_id,
					kind,
					status,
					content,
					summary,
					attributes,
					importance,
					confidence,
					supersedes_memory_id,
					last_observed_at,
					created_at,
					updated_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, $12, now(), now())
				ON CONFLICT (id)
				DO UPDATE SET
					content = EXCLUDED.content,
					summary = EXCLUDED.summary,
					attributes = EXCLUDED.attributes,
					importance = EXCLUDED.importance,
					confidence = EXCLUDED.confidence,
					last_observed_at = EXCLUDED.last_observed_at,
					updated_at = now()`,
				memoryID,
				persistRecord.TenantID,
				persistRecord.AgentID,
				persistRecord.ThreadID,
				persistRecord.Kind,
				persistRecord.Status,
				persistRecord.Content,
				persistRecord.Summary,
				attributes,
				persistRecord.Importance,
				persistRecord.Confidence,
				persistRecord.LastObservedAt,
			); err != nil {
				return fmt.Errorf("upsert projected memory for key %s: %w", persistRecord.Key, err)
			}

			for _, episodeID := range persistRecord.SourceEpisodeIDs {
				if _, err := tx.Exec(ctx,
					`INSERT INTO memory_provenance (memory_id, episode_id, consolidation_run_id, role)
					 VALUES ($1, $2, $3, $4)
					 ON CONFLICT (memory_id, episode_id) DO NOTHING`,
					memoryID,
					episodeID,
					runID,
					"source",
				); err != nil {
					return fmt.Errorf("insert projected memory provenance for key %s: %w", persistRecord.Key, err)
				}
			}

			if embeddings != nil {
				if _, err := tx.Exec(ctx,
					`INSERT INTO memory_embeddings (memory_id, tenant_id, agent_id, embedding_model, embedding, created_at)
					 VALUES ($1, $2, $3, $4, $5, now())
					 ON CONFLICT (memory_id)
					 DO UPDATE SET
						tenant_id = EXCLUDED.tenant_id,
						agent_id = EXCLUDED.agent_id,
						embedding_model = EXCLUDED.embedding_model,
						embedding = EXCLUDED.embedding,
						created_at = now()`,
					memoryID,
					persistRecord.TenantID,
					persistRecord.AgentID,
					embeddingModel,
					pgvector.NewVector(embeddings[i]),
				); err != nil {
					return fmt.Errorf("upsert memory embedding for key %s: %w", persistRecord.Key, err)
				}
			}
		}

		return nil
	})
}

// ListMemoriesByTenant returns durable memories for one tenant ordered by creation time.
func (s *Store) ListMemoriesByTenant(ctx context.Context, tenantID uuid.UUID, limit int) ([]memory.Memory, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("list memories by tenant: tenant_id must not be nil")
	}
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_id, thread_id, kind, status, content, summary, attributes,
		        importance, confidence, supersedes_memory_id, last_observed_at, created_at, updated_at
		 FROM memories
		 WHERE tenant_id = $1
		 ORDER BY created_at, id
		 LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list memories by tenant query: %w", err)
	}
	defer rows.Close()

	memories := make([]memory.Memory, 0, limit)
	for rows.Next() {
		record, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memories by tenant rows: %w", err)
	}
	return memories, nil
}

// QueryMemories lists durable memories using the first inspection-oriented filter set.
func (s *Store) QueryMemories(ctx context.Context, req memory.QueryMemoriesRequest) ([]memory.Memory, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	var (
		builder strings.Builder
		args    []any
		argN    = 1
	)
	builder.WriteString(`
		SELECT id, tenant_id, agent_id, thread_id, kind, status, content, summary, attributes,
		       importance, confidence, supersedes_memory_id, last_observed_at, created_at, updated_at
		FROM memories
		WHERE tenant_id = $1`)
	args = append(args, req.TenantID)

	if req.AgentID != nil {
		argN++
		builder.WriteString(fmt.Sprintf(" AND agent_id = $%d", argN))
		args = append(args, *req.AgentID)
	}
	if req.ThreadID != nil {
		argN++
		builder.WriteString(fmt.Sprintf(" AND thread_id = $%d", argN))
		args = append(args, *req.ThreadID)
	}
	if strings.TrimSpace(req.Kind) != "" {
		argN++
		builder.WriteString(fmt.Sprintf(" AND kind = $%d", argN))
		args = append(args, strings.TrimSpace(req.Kind))
	}
	if strings.TrimSpace(req.Status) != "" {
		argN++
		builder.WriteString(fmt.Sprintf(" AND status = $%d", argN))
		args = append(args, strings.TrimSpace(req.Status))
	}

	argN++
	builder.WriteString(fmt.Sprintf(" ORDER BY created_at, id LIMIT $%d", argN))
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query memories: %w", err)
	}
	defer rows.Close()

	memories := make([]memory.Memory, 0, limit)
	for rows.Next() {
		record, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query memories rows: %w", err)
	}
	return memories, nil
}

func episodeCursorBounds(episodes []memory.Episode) (*memory.CheckpointCursor, *memory.CheckpointCursor) {
	if len(episodes) == 0 {
		return nil, nil
	}
	first := memory.CheckpointCursor{
		CreatedAt: episodes[0].CreatedAt,
		ID:        episodes[0].ID,
	}
	last := memory.CheckpointCursor{
		CreatedAt: episodes[len(episodes)-1].CreatedAt,
		ID:        episodes[len(episodes)-1].ID,
	}
	return &first, &last
}

func deterministicMemoryID(projectionName string, key string) uuid.UUID {
	return uuid.NewSHA1(memoryNamespaceUUID, []byte(projectionName+":"+key))
}

func deterministicConsolidationRunID(projectionName string, firstEpisodeID, lastEpisodeID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(consolidationRunNamespaceUUID, []byte(projectionName+":"+firstEpisodeID.String()+":"+lastEpisodeID.String()))
}

// CountMemoryEmbeddingsByTenant returns the number of embedding rows for one tenant.
func (s *Store) CountMemoryEmbeddingsByTenant(ctx context.Context, tenantID uuid.UUID) (int, error) {
	if tenantID == uuid.Nil {
		return 0, fmt.Errorf("count memory embeddings by tenant: tenant_id must not be nil")
	}
	var count int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM memory_embeddings WHERE tenant_id = $1`,
		tenantID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count memory embeddings by tenant: %w", err)
	}
	return count, nil
}

func summarize(content string, maxLen int) string {
	runes := []rune(content)
	if maxLen <= 0 || len(runes) <= maxLen {
		return content
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
