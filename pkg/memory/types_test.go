package memory

import (
	"testing"

	"github.com/google/uuid"
)

func TestCreateTenantRequestValidate(t *testing.T) {
	t.Parallel()

	if err := (CreateTenantRequest{}).Validate(); err == nil {
		t.Fatal("expected validation error for empty slug")
	}

	if err := (CreateTenantRequest{Slug: "mordor-forge"}).Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestCreateAgentRequestValidate(t *testing.T) {
	t.Parallel()

	valid := CreateAgentRequest{
		TenantID: uuid.New(),
		Name:     "study-skill",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (CreateAgentRequest{Name: "missing-tenant"}).Validate(); err == nil {
		t.Fatal("expected validation error for nil tenant id")
	}
}

func TestCreateThreadRequestValidate(t *testing.T) {
	t.Parallel()

	valid := CreateThreadRequest{
		TenantID: uuid.New(),
		AgentID:  uuid.New(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (CreateThreadRequest{TenantID: uuid.New()}).Validate(); err == nil {
		t.Fatal("expected validation error for nil agent id")
	}
}

func TestAppendEpisodeRequestValidate(t *testing.T) {
	t.Parallel()

	threadID := uuid.New()
	valid := AppendEpisodeRequest{
		TenantID:   uuid.New(),
		AgentID:    uuid.New(),
		ThreadID:   &threadID,
		Kind:       "study.break",
		Content:    "session paused after chapter 3",
		Payload:    map[string]any{"phase": "reviewing"},
		OccurredAt: nil,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (AppendEpisodeRequest{
		TenantID: uuid.New(),
		AgentID:  uuid.New(),
		Content:  "missing kind",
	}).Validate(); err == nil {
		t.Fatal("expected validation error for empty kind")
	}

	zero := uuid.Nil
	if err := (AppendEpisodeRequest{
		TenantID: uuid.New(),
		AgentID:  uuid.New(),
		ThreadID: &zero,
		Kind:     "tool_result",
		Content:  "bad thread id",
	}).Validate(); err == nil {
		t.Fatal("expected validation error for zero thread id")
	}
}

func TestQueryMemoriesRequestValidate(t *testing.T) {
	t.Parallel()

	agentID := uuid.New()
	threadID := uuid.New()
	valid := QueryMemoriesRequest{
		TenantID: uuid.New(),
		AgentID:  &agentID,
		ThreadID: &threadID,
		Kind:     "episode_digest",
		Status:   "active",
		Limit:    10,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (QueryMemoriesRequest{TenantID: uuid.Nil}).Validate(); err == nil {
		t.Fatal("expected validation error for nil tenant id")
	}
	if err := (QueryMemoriesRequest{TenantID: uuid.New(), Limit: -1}).Validate(); err == nil {
		t.Fatal("expected validation error for negative limit")
	}
}

func TestRecallRequestValidate(t *testing.T) {
	t.Parallel()

	valid := RecallRequest{
		TenantID: uuid.New(),
		Query:    "resume study session",
		Limit:    5,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (RecallRequest{TenantID: uuid.New(), Query: ""}).Validate(); err == nil {
		t.Fatal("expected validation error for empty query")
	}
}

func TestProjectedMemoryValidate(t *testing.T) {
	t.Parallel()

	record := ProjectedMemory{
		Key:              "episode:123",
		TenantID:         uuid.New(),
		AgentID:          uuid.New(),
		Kind:             "episode_digest",
		Status:           "active",
		Content:          "hello",
		SourceEpisodeIDs: []uuid.UUID{uuid.New()},
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if err := (ProjectedMemory{TenantID: uuid.New(), AgentID: uuid.New(), Kind: "k", Status: "s", Content: "c", SourceEpisodeIDs: []uuid.UUID{uuid.New()}}).Validate(); err == nil {
		t.Fatal("expected validation error for empty key")
	}
}
