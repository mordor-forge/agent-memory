package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/apicontract"
	"github.com/mordor-forge/agent-memory/internal/auth"
	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/health"
	"github.com/mordor-forge/agent-memory/internal/quota"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

type fakeMemoryReader struct {
	lastRequest memory.QueryMemoriesRequest
	items       []memory.Memory
}

func (f *fakeMemoryReader) QueryMemories(_ context.Context, req memory.QueryMemoriesRequest) ([]memory.Memory, error) {
	f.lastRequest = req
	return f.items, nil
}

type fakeWriteAPI struct {
	lastTenantRequest  memory.CreateTenantRequest
	lastAgentRequest   memory.CreateAgentRequest
	lastThreadRequest  memory.CreateThreadRequest
	lastEpisodeRequest memory.AppendEpisodeRequest
	lastExportTenantID uuid.UUID
	lastDeleteTenantID uuid.UUID

	tenant         memory.Tenant
	agent          memory.Agent
	thread         memory.Thread
	episode        memory.Episode
	exportedTenant memory.TenantExport
	deleteResult   memory.TenantDeletionResult
	err            error
}

func (f *fakeWriteAPI) CreateTenant(_ context.Context, req memory.CreateTenantRequest) (memory.Tenant, error) {
	f.lastTenantRequest = req
	return f.tenant, f.err
}

func (f *fakeWriteAPI) CreateAgent(_ context.Context, req memory.CreateAgentRequest) (memory.Agent, error) {
	f.lastAgentRequest = req
	return f.agent, f.err
}

func (f *fakeWriteAPI) CreateThread(_ context.Context, req memory.CreateThreadRequest) (memory.Thread, error) {
	f.lastThreadRequest = req
	return f.thread, f.err
}

func (f *fakeWriteAPI) AppendEpisode(_ context.Context, req memory.AppendEpisodeRequest) (memory.Episode, error) {
	f.lastEpisodeRequest = req
	return f.episode, f.err
}

func (f *fakeWriteAPI) ExportTenant(_ context.Context, tenantID uuid.UUID) (memory.TenantExport, error) {
	f.lastExportTenantID = tenantID
	return f.exportedTenant, f.err
}

func (f *fakeWriteAPI) DeleteTenant(_ context.Context, tenantID uuid.UUID) (memory.TenantDeletionResult, error) {
	f.lastDeleteTenantID = tenantID
	return f.deleteResult, f.err
}

type fakeRecaller struct {
	lastRequest memory.RecallRequest
	hits        []memory.RecallHit
}

func (f *fakeRecaller) Recall(_ context.Context, req memory.RecallRequest) ([]memory.RecallHit, error) {
	f.lastRequest = req
	return f.hits, nil
}

type fakeInspector struct {
	provenance        memory.MemoryProvenance
	checkpoints       []memory.ProjectionCheckpoint
	runs              []memory.ConsolidationRun
	leases            []memory.WorkerLease
	lastMemoryID      uuid.UUID
	lastCheckpointReq memory.ListProjectionCheckpointsRequest
	lastRunsReq       memory.ListConsolidationRunsRequest
	lastLeasesReq     memory.ListWorkerLeasesRequest
	err               error
}

func (f *fakeInspector) GetMemoryProvenance(_ context.Context, memoryID uuid.UUID) (memory.MemoryProvenance, error) {
	f.lastMemoryID = memoryID
	return f.provenance, f.err
}

func (f *fakeInspector) ListProjectionCheckpoints(_ context.Context, req memory.ListProjectionCheckpointsRequest) ([]memory.ProjectionCheckpoint, error) {
	f.lastCheckpointReq = req
	return f.checkpoints, f.err
}

func (f *fakeInspector) ListConsolidationRuns(_ context.Context, req memory.ListConsolidationRunsRequest) ([]memory.ConsolidationRun, error) {
	f.lastRunsReq = req
	return f.runs, f.err
}

func (f *fakeInspector) ListWorkerLeases(_ context.Context, req memory.ListWorkerLeasesRequest) ([]memory.WorkerLease, error) {
	f.lastLeasesReq = req
	return f.leases, f.err
}

func TestMemoriesEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	agentID := uuid.New()
	reader := &fakeMemoryReader{
		items: []memory.Memory{
			{ID: uuid.New(), TenantID: tenantID, AgentID: agentID, Kind: "episode_digest", Status: "active", Content: "memory"},
		},
	}

	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, reader, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/memories?tenant_id="+tenantID.String()+"&agent_id="+agentID.String()+"&kind=episode_digest&status=active&limit=5", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertV1ContractHeaders(t, rec)

	if reader.lastRequest.TenantID != tenantID {
		t.Fatalf("TenantID = %s, want %s", reader.lastRequest.TenantID, tenantID)
	}
	if reader.lastRequest.AgentID == nil || *reader.lastRequest.AgentID != agentID {
		t.Fatalf("AgentID = %+v, want %s", reader.lastRequest.AgentID, agentID)
	}
	if reader.lastRequest.Kind != "episode_digest" || reader.lastRequest.Status != "active" || reader.lastRequest.Limit != 5 {
		t.Fatalf("unexpected request: %+v", reader.lastRequest)
	}

	var response struct {
		Data struct {
			Items []memory.Memory `json:"items"`
		} `json:"data"`
		Meta struct {
			Count int `json:"count"`
			Limit int `json:"limit"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Meta.Count != 1 || response.Meta.Limit != 5 || len(response.Data.Items) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRequestLoggingAddsRequestIDHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, logger, []health.Checker{}, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected X-Request-Id header to be set")
	}
	if !strings.Contains(buf.String(), `"path":"/"`) || !strings.Contains(buf.String(), `"status":200`) {
		t.Fatalf("unexpected request log output: %s", buf.String())
	}
}

func TestMemoriesEndpointRejectsMissingTenantID(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, &fakeMemoryReader{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/memories", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var response struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "invalid_request" {
		t.Fatalf("error code = %q, want %q", response.Error.Code, "invalid_request")
	}
}

func TestV1RejectsUnsupportedAccept(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, &fakeMemoryReader{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/memories?tenant_id="+tenantID.String(), nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotAcceptable)
	}
	assertV1ContractHeaders(t, rec)

	var response struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "not_acceptable" {
		t.Fatalf("error code = %q, want %q", response.Error.Code, "not_acceptable")
	}
}

func TestV1RejectsUnsupportedContentType(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, &fakeWriteAPI{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"slug":"mordor"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
	assertV1ContractHeaders(t, rec)

	var response struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "unsupported_media_type" {
		t.Fatalf("error code = %q, want %q", response.Error.Code, "unsupported_media_type")
	}
}

func TestV1UnknownPathUsesJSONNotFoundEnvelope(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	assertV1ContractHeaders(t, rec)

	var response struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want %q", response.Error.Code, "not_found")
	}
}

func TestRecallEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	recaller := &fakeRecaller{
		hits: []memory.RecallHit{{Memory: memory.Memory{ID: uuid.New(), TenantID: tenantID}, Distance: 0, Score: 0.9}},
	}

	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, nil, recaller)
	req := httptest.NewRequest(http.MethodGet, "/v1/recall?tenant_id="+tenantID.String()+"&query=resume+study&limit=3", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if recaller.lastRequest.TenantID != tenantID || recaller.lastRequest.Query != "resume study" || recaller.lastRequest.Limit != 3 {
		t.Fatalf("unexpected recall request: %+v", recaller.lastRequest)
	}

	var response struct {
		Data struct {
			Items []memory.RecallHit `json:"items"`
		} `json:"data"`
		Meta struct {
			Count int `json:"count"`
			Limit int `json:"limit"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Meta.Count != 1 || response.Meta.Limit != 3 || len(response.Data.Items) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Data.Items[0].Score != 0.9 {
		t.Fatalf("score = %f, want %f", response.Data.Items[0].Score, 0.9)
	}
}

func TestCreateTenantEndpoint(t *testing.T) {
	t.Parallel()

	writeAPI := &fakeWriteAPI{
		tenant: memory.Tenant{ID: uuid.New(), Slug: "mordor"},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"slug":"mordor"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	assertV1ContractHeaders(t, rec)
	if writeAPI.lastTenantRequest.Slug != "mordor" {
		t.Fatalf("unexpected request: %+v", writeAPI.lastTenantRequest)
	}

	var response struct {
		Data memory.Tenant `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Data.Slug != "mordor" {
		t.Fatalf("tenant slug = %q, want %q", response.Data.Slug, "mordor")
	}
}

func TestCreateTenantEndpointAcceptsVendorMediaType(t *testing.T) {
	t.Parallel()

	writeAPI := &fakeWriteAPI{
		tenant: memory.Tenant{ID: uuid.New(), Slug: "mordor"},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"slug":"mordor"}`))
	req.Header.Set("Content-Type", apicontract.HTTPMediaType)
	req.Header.Set("Accept", apicontract.HTTPMediaType)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	assertV1ContractHeaders(t, rec)
}

func TestAppendEpisodeEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	agentID := uuid.New()
	writeAPI := &fakeWriteAPI{
		episode: memory.Episode{ID: uuid.New(), TenantID: tenantID, AgentID: agentID, Kind: "study.break", Content: "pause"},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/episodes", strings.NewReader(`{"tenant_id":"`+tenantID.String()+`","agent_id":"`+agentID.String()+`","kind":"study.break","content":"pause"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if writeAPI.lastEpisodeRequest.TenantID != tenantID || writeAPI.lastEpisodeRequest.AgentID != agentID {
		t.Fatalf("unexpected request: %+v", writeAPI.lastEpisodeRequest)
	}
}

func TestAppendEpisodeEndpointConflictEnvelope(t *testing.T) {
	t.Parallel()

	writeAPI := &fakeWriteAPI{err: memory.ErrIdempotencyConflict}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/episodes", strings.NewReader(`{"tenant_id":"`+uuid.NewString()+`","agent_id":"`+uuid.NewString()+`","kind":"study.break","content":"pause"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	var response struct {
		Error apiErrorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "conflict" {
		t.Fatalf("error code = %q, want %q", response.Error.Code, "conflict")
	}
}

func TestMemoryProvenanceEndpoint(t *testing.T) {
	t.Parallel()

	memoryID := uuid.New()
	inspector := &fakeInspector{
		provenance: memory.MemoryProvenance{
			MemoryID:       memoryID,
			TenantID:       uuid.New(),
			ProjectionName: "episode-digests",
			HasEmbedding:   true,
		},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, inspector, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/memories/"+memoryID.String()+"/provenance", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if inspector.lastMemoryID != memoryID {
		t.Fatalf("lastMemoryID = %s, want %s", inspector.lastMemoryID, memoryID)
	}
}

func TestProjectionCheckpointsEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	inspector := &fakeInspector{
		checkpoints: []memory.ProjectionCheckpoint{{ProjectionName: "episode-digests", TenantID: tenantID, ShardID: 0}},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, inspector, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/projections/checkpoints?tenant_id="+tenantID.String()+"&limit=2", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if inspector.lastCheckpointReq.TenantID != tenantID || inspector.lastCheckpointReq.Limit != 2 {
		t.Fatalf("unexpected request: %+v", inspector.lastCheckpointReq)
	}
}

func TestProjectionRunsEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	now := time.Now().UTC()
	inspector := &fakeInspector{
		runs: []memory.ConsolidationRun{{
			ID:             uuid.New(),
			TenantID:       tenantID,
			WorkerID:       uuid.New(),
			Status:         "completed",
			Provider:       "deterministic",
			Model:          "deterministic/1536",
			ProjectionName: "episode-digests",
			BatchStartedAt: now,
			CreatedAt:      now,
		}},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, inspector, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/projections/runs?tenant_id="+tenantID.String()+"&projection_name=episode-digests&limit=2", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertV1ContractHeaders(t, rec)
	if inspector.lastRunsReq.TenantID != tenantID || inspector.lastRunsReq.ProjectionName != "episode-digests" || inspector.lastRunsReq.Limit != 2 {
		t.Fatalf("unexpected request: %+v", inspector.lastRunsReq)
	}

	var response struct {
		Data struct {
			Items []memory.ConsolidationRun `json:"items"`
		} `json:"data"`
		Meta struct {
			Count int `json:"count"`
			Limit int `json:"limit"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Meta.Count != 1 || response.Meta.Limit != 2 || len(response.Data.Items) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Data.Items[0].ProjectionName != "episode-digests" || response.Data.Items[0].Model != "deterministic/1536" {
		t.Fatalf("unexpected run response: %+v", response.Data.Items[0])
	}
}

func TestWorkerLeasesEndpoint(t *testing.T) {
	t.Parallel()

	inspector := &fakeInspector{
		leases: []memory.WorkerLease{{LeaseName: "projection:demo"}},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, nil, nil, inspector, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/leases?limit=4", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if inspector.lastLeasesReq.Limit != 4 {
		t.Fatalf("unexpected lease request: %+v", inspector.lastLeasesReq)
	}
}

func TestExportTenantEndpoint(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	authorizer := mustAuthorizer(t, tenantID)
	writeAPI := &fakeWriteAPI{
		exportedTenant: memory.TenantExport{
			Tenant: memory.Tenant{ID: tenantID, Slug: "demo"},
		},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, authorizer, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/tenants/"+tenantID.String()+"/export", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if writeAPI.lastExportTenantID != tenantID {
		t.Fatalf("lastExportTenantID = %s, want %s", writeAPI.lastExportTenantID, tenantID)
	}
}

func TestDeleteTenantEndpointRequiresAdmin(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	authorizer, err := auth.NewFromConfig(config.Config{
		HTTPAuthMode:           "api_key",
		HTTPAuthHeader:         "Authorization",
		HTTPAuthScheme:         "Bearer",
		HTTPAuthPrincipalsJSON: `[{"token":"secret","subject":"user","tenant_grants":["` + tenantID.String() + `"]}]`,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, authorizer, nil, &fakeWriteAPI{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/v1/tenants/"+tenantID.String(), nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEndpointRejectsMissingAuthWhenEnabled(t *testing.T) {
	t.Parallel()

	authorizer := mustAuthorizer(t, uuid.New())
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, authorizer, nil, &fakeWriteAPI{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"slug":"mordor"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestEndpointRejectsTenantWithoutGrant(t *testing.T) {
	t.Parallel()

	allowedTenant := uuid.New()
	targetTenant := uuid.New()
	authorizer := mustAuthorizer(t, allowedTenant)
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, authorizer, nil, &fakeWriteAPI{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(`{"tenant_id":"`+targetTenant.String()+`","name":"demo-agent"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEndpointAllowsDevBypassWhenAuthDisabled(t *testing.T) {
	t.Parallel()

	writeAPI := &fakeWriteAPI{
		tenant: memory.Tenant{ID: uuid.New(), Slug: "mordor"},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, nil, nil, writeAPI, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/tenants", strings.NewReader(`{"slug":"mordor"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestEndpointRateLimited(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	authorizer := mustAuthorizer(t, tenantID)
	limiter := quota.NewHTTPRateLimiter(config.Config{
		HTTPRateLimitRPS:   0.0001,
		HTTPRateLimitBurst: 1,
	})
	writeAPI := &fakeWriteAPI{
		tenant: memory.Tenant{ID: uuid.New(), Slug: "mordor"},
	}
	srv := New(config.Config{HTTPAddr: ":0", HealthTimeout: time.Second}, slog.Default(), []health.Checker{}, authorizer, limiter, writeAPI, nil, nil, nil)

	makeReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/agents", strings.NewReader(`{"tenant_id":"`+tenantID.String()+`","name":"demo-agent"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		return rec
	}

	first := makeReq()
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusCreated)
	}
	second := makeReq()
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func mustAuthorizer(t *testing.T, tenantID uuid.UUID) *auth.Authorizer {
	t.Helper()

	authorizer, err := auth.NewFromConfig(config.Config{
		HTTPAuthMode:           "api_key",
		HTTPAuthHeader:         "Authorization",
		HTTPAuthScheme:         "Bearer",
		HTTPAuthPrincipalsJSON: `[{"token":"secret","subject":"test-user","tenant_grants":["` + tenantID.String() + `"]}]`,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	return authorizer
}

func assertV1ContractHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Content-Type"); got != apicontract.HTTPMediaType {
		t.Fatalf("Content-Type = %q, want %q", got, apicontract.HTTPMediaType)
	}
	if got := rec.Header().Get(apicontract.HTTPVersionHeader); got != apicontract.HTTPAPIVersion {
		t.Fatalf("%s = %q, want %q", apicontract.HTTPVersionHeader, got, apicontract.HTTPAPIVersion)
	}
	if got := rec.Header().Get(apicontract.HTTPStabilityHeader); got != apicontract.CompatibilityStability {
		t.Fatalf("%s = %q, want %q", apicontract.HTTPStabilityHeader, got, apicontract.CompatibilityStability)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept") {
		t.Fatalf("Vary = %q, want to include %q", got, "Accept")
	}
}
