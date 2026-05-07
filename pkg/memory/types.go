package memory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidRequest is returned when a request fails basic validation.
	ErrInvalidRequest = errors.New("memory: invalid request")
	// ErrMemoryNotFound is returned when a memory row does not exist.
	ErrMemoryNotFound = errors.New("memory: memory not found")
	// ErrTenantNotFound is returned when a tenant row does not exist.
	ErrTenantNotFound = errors.New("memory: tenant not found")
	// ErrIdempotencyConflict is returned when an idempotency key is reused for a different episode.
	ErrIdempotencyConflict = errors.New("memory: idempotency key already exists for a different episode")
	// ErrThreadMismatch is returned when a thread does not belong to the supplied tenant and agent.
	ErrThreadMismatch = errors.New("memory: thread does not belong to tenant and agent")
	// ErrAgentTenantMismatch is returned when an agent does not belong to the supplied tenant.
	ErrAgentTenantMismatch = errors.New("memory: agent does not belong to tenant")
)

// CheckpointCursor is the ordered resume position for append-heavy scans.
type CheckpointCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

// Tenant identifies one top-level memory owner boundary.
type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// Agent identifies a logical agent inside one tenant.
type Agent struct {
	ID          uuid.UUID      `json:"id"`
	TenantID    uuid.UUID      `json:"tenant_id"`
	ExternalRef string         `json:"external_ref,omitempty"`
	Name        string         `json:"name"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Thread identifies a conversation or workflow thread inside one agent.
type Thread struct {
	ID          uuid.UUID      `json:"id"`
	TenantID    uuid.UUID      `json:"tenant_id"`
	AgentID     uuid.UUID      `json:"agent_id"`
	ExternalRef string         `json:"external_ref,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Episode is one immutable recorded unit of agent activity.
type Episode struct {
	ID             uuid.UUID      `json:"id"`
	TenantID       uuid.UUID      `json:"tenant_id"`
	AgentID        uuid.UUID      `json:"agent_id"`
	ThreadID       *uuid.UUID     `json:"thread_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	Payload        map[string]any `json:"payload"`
	OccurredAt     *time.Time     `json:"occurred_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// Memory is one derived durable record produced from one or more episodes.
type Memory struct {
	ID                 uuid.UUID      `json:"id"`
	TenantID           uuid.UUID      `json:"tenant_id"`
	AgentID            uuid.UUID      `json:"agent_id"`
	ThreadID           *uuid.UUID     `json:"thread_id,omitempty"`
	Kind               string         `json:"kind"`
	Status             string         `json:"status"`
	Content            string         `json:"content"`
	Summary            *string        `json:"summary,omitempty"`
	Attributes         map[string]any `json:"attributes"`
	Importance         float64        `json:"importance"`
	Confidence         float64        `json:"confidence"`
	SupersedesMemoryID *uuid.UUID     `json:"supersedes_memory_id,omitempty"`
	LastObservedAt     *time.Time     `json:"last_observed_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// MemorySourceProvenance describes one episode that contributed to a memory.
type MemorySourceProvenance struct {
	EpisodeID           uuid.UUID `json:"episode_id"`
	EpisodeKind         string    `json:"episode_kind"`
	EpisodeContent      string    `json:"episode_content"`
	EpisodeCreatedAt    time.Time `json:"episode_created_at"`
	Role                string    `json:"role"`
	ConsolidationRunID  uuid.UUID `json:"consolidation_run_id"`
}

// MemoryProvenance describes why a memory exists.
type MemoryProvenance struct {
	MemoryID        uuid.UUID                `json:"memory_id"`
	TenantID        uuid.UUID                `json:"tenant_id"`
	ProjectionName  string                   `json:"projection_name"`
	HasEmbedding    bool                     `json:"has_embedding"`
	Sources         []MemorySourceProvenance `json:"sources"`
}

// ProjectionCheckpoint describes the last durable cursor for a projection shard.
type ProjectionCheckpoint struct {
	ProjectionName string            `json:"projection_name"`
	TenantID       uuid.UUID         `json:"tenant_id"`
	ShardID        int64             `json:"shard_id"`
	LastCursor     *CheckpointCursor `json:"last_cursor,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// ConsolidationRun describes one persisted projection or consolidation batch.
type ConsolidationRun struct {
	ID             uuid.UUID         `json:"id"`
	TenantID       uuid.UUID         `json:"tenant_id"`
	WorkerID       uuid.UUID         `json:"worker_id"`
	InputFrom      *CheckpointCursor `json:"input_from,omitempty"`
	InputTo        *CheckpointCursor `json:"input_to,omitempty"`
	Status         string            `json:"status"`
	Provider       string            `json:"provider"`
	Model          string            `json:"model"`
	ProjectionName string            `json:"projection_name"`
	ErrorText      *string           `json:"error_text,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	BatchStartedAt time.Time         `json:"batch_started_at"`
}

// WorkerLease describes one current lease holder record.
type WorkerLease struct {
	LeaseName string         `json:"lease_name"`
	HolderID  uuid.UUID      `json:"holder_id"`
	ExpiresAt time.Time      `json:"expires_at"`
	RenewedAt time.Time      `json:"renewed_at"`
	Metadata  map[string]any `json:"metadata"`
}

// TenantExport is a portable snapshot of one tenant's data and operational state.
type TenantExport struct {
	Tenant               Tenant                 `json:"tenant"`
	Agents               []Agent                `json:"agents"`
	Threads              []Thread               `json:"threads"`
	Episodes             []Episode              `json:"episodes"`
	Memories             []Memory               `json:"memories"`
	ProjectionCheckpoints []ProjectionCheckpoint `json:"projection_checkpoints"`
	ConsolidationRuns    []ConsolidationRun     `json:"consolidation_runs"`
	WorkerLeases         []WorkerLease          `json:"worker_leases"`
}

// TenantDeletionResult acknowledges tenant deletion.
type TenantDeletionResult struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Deleted  bool      `json:"deleted"`
}

// QueryMemoriesRequest lists durable memory rows by scope and status.
type QueryMemoriesRequest struct {
	TenantID uuid.UUID  `json:"tenant_id"`
	AgentID  *uuid.UUID `json:"agent_id,omitempty"`
	ThreadID *uuid.UUID `json:"thread_id,omitempty"`
	Kind     string     `json:"kind,omitempty"`
	Status   string     `json:"status,omitempty"`
	Limit    int        `json:"limit,omitempty"`
}

// Validate checks the memory query request.
func (r QueryMemoriesRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.AgentID != nil && *r.AgentID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("agent_id must not be nil when provided"))
	}
	if r.ThreadID != nil && *r.ThreadID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("thread_id must not be nil when provided"))
	}
	if r.Limit < 0 {
		return errors.Join(ErrInvalidRequest, errors.New("limit must be >= 0"))
	}
	return nil
}

// RecallRequest performs semantic nearest-neighbor retrieval over stored memory embeddings.
type RecallRequest struct {
	TenantID uuid.UUID  `json:"tenant_id"`
	AgentID  *uuid.UUID `json:"agent_id,omitempty"`
	ThreadID *uuid.UUID `json:"thread_id,omitempty"`
	Kind     string     `json:"kind,omitempty"`
	Status   string     `json:"status,omitempty"`
	Query    string     `json:"query"`
	Limit    int        `json:"limit,omitempty"`
}

// Validate checks the recall request.
func (r RecallRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.AgentID != nil && *r.AgentID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("agent_id must not be nil when provided"))
	}
	if r.ThreadID != nil && *r.ThreadID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("thread_id must not be nil when provided"))
	}
	if strings.TrimSpace(r.Query) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("query must not be empty"))
	}
	if r.Limit < 0 {
		return errors.Join(ErrInvalidRequest, errors.New("limit must be >= 0"))
	}
	return nil
}

// ListProjectionCheckpointsRequest lists projection cursor records.
type ListProjectionCheckpointsRequest struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	ProjectionName string    `json:"projection_name,omitempty"`
	Limit          int       `json:"limit,omitempty"`
}

// Validate checks the checkpoint list request.
func (r ListProjectionCheckpointsRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.Limit < 0 {
		return errors.Join(ErrInvalidRequest, errors.New("limit must be >= 0"))
	}
	return nil
}

// ListConsolidationRunsRequest lists recent projection/consolidation batches.
type ListConsolidationRunsRequest struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	ProjectionName string    `json:"projection_name,omitempty"`
	Limit          int       `json:"limit,omitempty"`
}

// Validate checks the run list request.
func (r ListConsolidationRunsRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.Limit < 0 {
		return errors.Join(ErrInvalidRequest, errors.New("limit must be >= 0"))
	}
	return nil
}

// ListWorkerLeasesRequest lists current worker leases.
type ListWorkerLeasesRequest struct {
	Limit int `json:"limit,omitempty"`
}

// Validate checks the worker lease list request.
func (r ListWorkerLeasesRequest) Validate() error {
	if r.Limit < 0 {
		return errors.Join(ErrInvalidRequest, errors.New("limit must be >= 0"))
	}
	return nil
}

// RecallHit is one semantically ranked memory match.
type RecallHit struct {
	Memory   Memory  `json:"memory"`
	Distance float64 `json:"distance"`
	Score    float64 `json:"score"`
}

// ProjectedMemory is a generic projection output that can be upserted deterministically.
// Key is projection-scoped and used to derive a stable memory ID.
type ProjectedMemory struct {
	Key            string         `json:"key"`
	TenantID       uuid.UUID      `json:"tenant_id"`
	AgentID        uuid.UUID      `json:"agent_id"`
	ThreadID       *uuid.UUID     `json:"thread_id,omitempty"`
	Kind           string         `json:"kind"`
	Status         string         `json:"status"`
	MergeStrategy  string         `json:"merge_strategy,omitempty"`
	Content        string         `json:"content"`
	Summary        *string        `json:"summary,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`
	Importance     float64        `json:"importance"`
	Confidence     float64        `json:"confidence"`
	LastObservedAt *time.Time     `json:"last_observed_at,omitempty"`
	SourceEpisodeIDs []uuid.UUID  `json:"source_episode_ids"`
	EmbedText      string         `json:"embed_text,omitempty"`
}

// Validate checks the projected memory payload before persistence.
func (r ProjectedMemory) Validate() error {
	if strings.TrimSpace(r.Key) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory key must not be empty"))
	}
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory tenant_id must not be nil"))
	}
	if r.AgentID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory agent_id must not be nil"))
	}
	if strings.TrimSpace(r.Kind) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory kind must not be empty"))
	}
	if strings.TrimSpace(r.Status) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory status must not be empty"))
	}
	if strings.TrimSpace(r.MergeStrategy) != "" {
		switch r.MergeStrategy {
		case "replace", "accumulate":
		default:
			return errors.Join(ErrInvalidRequest, errors.New("projected memory merge_strategy must be empty, replace, or accumulate"))
		}
	}
	if strings.TrimSpace(r.Content) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory content must not be empty"))
	}
	if len(r.SourceEpisodeIDs) == 0 {
		return errors.Join(ErrInvalidRequest, errors.New("projected memory must include at least one source episode"))
	}
	for _, id := range r.SourceEpisodeIDs {
		if id == uuid.Nil {
			return errors.Join(ErrInvalidRequest, errors.New("projected memory source episode ids must not contain nil"))
		}
	}
	return nil
}

// CreateTenantRequest creates one tenant boundary.
type CreateTenantRequest struct {
	Slug string `json:"slug"`
}

// Validate checks the tenant request.
func (r CreateTenantRequest) Validate() error {
	if strings.TrimSpace(r.Slug) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("slug must not be empty"))
	}
	return nil
}

// CreateAgentRequest creates one agent inside a tenant.
type CreateAgentRequest struct {
	TenantID    uuid.UUID      `json:"tenant_id"`
	ExternalRef string         `json:"external_ref,omitempty"`
	Name        string         `json:"name"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Validate checks the agent request.
func (r CreateAgentRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("name must not be empty"))
	}
	return nil
}

// CreateThreadRequest creates one thread inside an agent.
type CreateThreadRequest struct {
	TenantID    uuid.UUID      `json:"tenant_id"`
	AgentID     uuid.UUID      `json:"agent_id"`
	ExternalRef string         `json:"external_ref,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Validate checks the thread request.
func (r CreateThreadRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.AgentID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("agent_id must not be nil"))
	}
	return nil
}

// AppendEpisodeRequest records one immutable episode.
type AppendEpisodeRequest struct {
	TenantID       uuid.UUID      `json:"tenant_id"`
	AgentID        uuid.UUID      `json:"agent_id"`
	ThreadID       *uuid.UUID     `json:"thread_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Kind           string         `json:"kind"`
	Content        string         `json:"content"`
	Payload        map[string]any `json:"payload,omitempty"`
	OccurredAt     *time.Time     `json:"occurred_at,omitempty"`
}

// Validate checks the append request.
func (r AppendEpisodeRequest) Validate() error {
	if r.TenantID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("tenant_id must not be nil"))
	}
	if r.AgentID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("agent_id must not be nil"))
	}
	if r.ThreadID != nil && *r.ThreadID == uuid.Nil {
		return errors.Join(ErrInvalidRequest, errors.New("thread_id must not be nil when provided"))
	}
	if strings.TrimSpace(r.Kind) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("kind must not be empty"))
	}
	if strings.TrimSpace(r.Content) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("content must not be empty"))
	}
	return nil
}

// ResourceCreator creates the minimal top-level resources needed by the append path.
type ResourceCreator interface {
	CreateTenant(ctx context.Context, req CreateTenantRequest) (Tenant, error)
	CreateAgent(ctx context.Context, req CreateAgentRequest) (Agent, error)
	CreateThread(ctx context.Context, req CreateThreadRequest) (Thread, error)
	ExportTenant(ctx context.Context, tenantID uuid.UUID) (TenantExport, error)
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) (TenantDeletionResult, error)
}

// EpisodeWriter records immutable episodes.
type EpisodeWriter interface {
	AppendEpisode(ctx context.Context, req AppendEpisodeRequest) (Episode, error)
}

// MemoryReader lists durable memory rows.
type MemoryReader interface {
	QueryMemories(ctx context.Context, req QueryMemoriesRequest) ([]Memory, error)
}

// MemoryInspector exposes provenance and projection state for operators and tools.
type MemoryInspector interface {
	GetMemoryProvenance(ctx context.Context, memoryID uuid.UUID) (MemoryProvenance, error)
	ListProjectionCheckpoints(ctx context.Context, req ListProjectionCheckpointsRequest) ([]ProjectionCheckpoint, error)
	ListConsolidationRuns(ctx context.Context, req ListConsolidationRunsRequest) ([]ConsolidationRun, error)
	ListWorkerLeases(ctx context.Context, req ListWorkerLeasesRequest) ([]WorkerLease, error)
}

// Recaller performs semantic memory retrieval.
type Recaller interface {
	Recall(ctx context.Context, req RecallRequest) ([]RecallHit, error)
}
