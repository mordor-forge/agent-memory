package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mordor-forge/agent-memory/internal/apicontract"
	"github.com/mordor-forge/agent-memory/internal/auth"
	"github.com/mordor-forge/agent-memory/internal/config"
	"github.com/mordor-forge/agent-memory/internal/health"
	"github.com/mordor-forge/agent-memory/internal/observability"
	"github.com/mordor-forge/agent-memory/internal/quota"
	"github.com/mordor-forge/agent-memory/internal/version"
	"github.com/mordor-forge/agent-memory/pkg/memory"
)

const maxJSONBodyBytes int64 = 1 << 20
const defaultListLimit = 100

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiEnvelope struct {
	Data  any           `json:"data,omitempty"`
	Error *apiErrorBody `json:"error,omitempty"`
	Meta  any           `json:"meta,omitempty"`
}

type listMeta struct {
	Count int `json:"count"`
	Limit int `json:"limit"`
}

type requestContextKey string

const requestIDContextKey requestContextKey = "request-id"
const requestLogStateContextKey requestContextKey = "request-log-state"

type requestLogState struct {
	requestID string
	principal *auth.Principal
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// WriteAPI exposes the first write-side HTTP operations.
type WriteAPI interface {
	CreateTenant(ctx context.Context, req memory.CreateTenantRequest) (memory.Tenant, error)
	CreateAgent(ctx context.Context, req memory.CreateAgentRequest) (memory.Agent, error)
	CreateThread(ctx context.Context, req memory.CreateThreadRequest) (memory.Thread, error)
	AppendEpisode(ctx context.Context, req memory.AppendEpisodeRequest) (memory.Episode, error)
	ExportTenant(ctx context.Context, tenantID uuid.UUID) (memory.TenantExport, error)
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) (memory.TenantDeletionResult, error)
}

// MemoryReader lists durable memory rows for inspection.
type MemoryReader interface {
	QueryMemories(ctx context.Context, req memory.QueryMemoriesRequest) ([]memory.Memory, error)
}

// MemoryInspector exposes provenance and projection state for operator/debug surfaces.
type MemoryInspector interface {
	GetMemoryProvenance(ctx context.Context, memoryID uuid.UUID) (memory.MemoryProvenance, error)
	ListProjectionCheckpoints(ctx context.Context, req memory.ListProjectionCheckpointsRequest) ([]memory.ProjectionCheckpoint, error)
	ListConsolidationRuns(ctx context.Context, req memory.ListConsolidationRunsRequest) ([]memory.ConsolidationRun, error)
	ListWorkerLeases(ctx context.Context, req memory.ListWorkerLeasesRequest) ([]memory.WorkerLease, error)
}

// Recaller performs semantic memory retrieval.
type Recaller interface {
	Recall(ctx context.Context, req memory.RecallRequest) ([]memory.RecallHit, error)
}

// RateLimiter provides request throttling for authenticated principals.
type RateLimiter interface {
	Allow(ctx context.Context, principal auth.Principal) error
}

type authedHandler func(http.ResponseWriter, *http.Request, auth.Principal)

// New builds the HTTP server used by the memory service.
func New(cfg config.Config, logger *slog.Logger, checkers []health.Checker, authorizer *auth.Authorizer, limiter RateLimiter, writeAPI WriteAPI, memoryReader MemoryReader, inspector MemoryInspector, recaller Recaller) *http.Server {
	mux := http.NewServeMux()
	build := version.Current()

	mux.Handle("/metrics", observability.MetricsHandler())
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": build.Name,
			"version": build.Version,
		})
	})
	mux.HandleFunc("/buildz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, build)
	})
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
		defer cancel()

		report := health.Evaluate(ctx, checkers)
		statusCode := http.StatusOK
		if report.Status != "ok" {
			statusCode = http.StatusServiceUnavailable
		}
		writeJSON(w, statusCode, report)
	})
	if writeAPI != nil {
		mux.HandleFunc("/v1/tenants", wrapV1(authorizer, limiter, logger, http.MethodPost, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			if !principal.CanAdmin() {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to create tenants")
				return
			}
			var req memory.CreateTenantRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			tenant, err := writeAPI.CreateTenant(ctx, req)
			if err != nil {
				writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
				return
			}
			writeAPISuccess(w, http.StatusCreated, tenant, nil)
		}))
		mux.HandleFunc("/v1/agents", wrapV1(authorizer, limiter, logger, http.MethodPost, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			var req memory.CreateAgentRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			agent, err := writeAPI.CreateAgent(ctx, req)
			if err != nil {
				writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
				return
			}
			writeAPISuccess(w, http.StatusCreated, agent, nil)
		}))
		mux.HandleFunc("/v1/threads", wrapV1(authorizer, limiter, logger, http.MethodPost, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			var req memory.CreateThreadRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			thread, err := writeAPI.CreateThread(ctx, req)
			if err != nil {
				writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
				return
			}
			writeAPISuccess(w, http.StatusCreated, thread, nil)
		}))
		mux.HandleFunc("/v1/episodes", wrapV1(authorizer, limiter, logger, http.MethodPost, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			var req memory.AppendEpisodeRequest
			if err := decodeJSONBody(w, r, &req); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			episode, err := writeAPI.AppendEpisode(ctx, req)
			if err != nil {
				writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
				return
			}
			writeAPISuccess(w, http.StatusCreated, episode, nil)
		}))
		mux.HandleFunc("/v1/tenants/", func(w http.ResponseWriter, r *http.Request) {
			tenantID, exportPath, err := tenantGovernancePath(r.URL.Path)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			switch r.Method {
			case http.MethodGet:
				wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
					if !exportPath {
						writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
						return
					}
					if !principal.CanAccessTenant(tenantID) {
						writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
						return
					}

					ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
					defer cancel()

					exported, err := writeAPI.ExportTenant(ctx, tenantID)
					if err != nil {
						writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
						return
					}
					writeAPISuccess(w, http.StatusOK, exported, nil)
				})(w, r)
			case http.MethodDelete:
				wrapV1(authorizer, limiter, logger, http.MethodDelete, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
					if exportPath {
						writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
						return
					}
					if !principal.CanAdmin() {
						writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to delete tenants")
						return
					}

					ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
					defer cancel()

					result, err := writeAPI.DeleteTenant(ctx, tenantID)
					if err != nil {
						writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
						return
					}
					writeAPISuccess(w, http.StatusOK, result, nil)
				})(w, r)
			default:
				writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			}
		})
	}
	if memoryReader != nil {
		mux.HandleFunc("/v1/memories", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			req, err := queryMemoriesRequestFromHTTP(r)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			items, err := memoryReader.QueryMemories(ctx, req)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeAPISuccess(w, http.StatusOK, map[string]any{
				"items": items,
			}, listMeta{
				Count: len(items),
				Limit: effectiveLimit(req.Limit),
			})
		}))
	}
	if inspector != nil {
		mux.HandleFunc("/v1/memories/", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			memoryID, err := memoryIDFromProvenancePath(r.URL.Path)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			provenance, err := inspector.GetMemoryProvenance(ctx, memoryID)
			if err != nil {
				writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
				return
			}
			if !principal.CanAccessTenant(provenance.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}
			writeAPISuccess(w, http.StatusOK, provenance, nil)
		}))

		mux.HandleFunc("/v1/projections/checkpoints", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			req, err := checkpointsRequestFromHTTP(r)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			items, err := inspector.ListProjectionCheckpoints(ctx, req)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeAPISuccess(w, http.StatusOK, map[string]any{"items": items}, listMeta{
				Count: len(items),
				Limit: effectiveLimit(req.Limit),
			})
		}))

		mux.HandleFunc("/v1/projections/runs", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			req, err := runsRequestFromHTTP(r)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			items, err := inspector.ListConsolidationRuns(ctx, req)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeAPISuccess(w, http.StatusOK, map[string]any{"items": items}, listMeta{
				Count: len(items),
				Limit: effectiveLimit(req.Limit),
			})
		}))

		mux.HandleFunc("/v1/leases", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			if !principal.CanAdmin() {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to inspect leases")
				return
			}
			req, err := leasesRequestFromHTTP(r)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			items, err := inspector.ListWorkerLeases(ctx, req)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeAPISuccess(w, http.StatusOK, map[string]any{"items": items}, listMeta{
				Count: len(items),
				Limit: effectiveLimit(req.Limit),
			})
		}))
	}
	if recaller != nil {
		mux.HandleFunc("/v1/recall", wrapV1(authorizer, limiter, logger, http.MethodGet, func(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
			req, err := recallRequestFromHTTP(r)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
			if !principal.CanAccessTenant(req.TenantID) {
				writeAPIError(w, http.StatusForbidden, "forbidden", "principal is not allowed to access this tenant")
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), cfg.HealthTimeout)
			defer cancel()

			hits, err := recaller.Recall(ctx, req)
			if err != nil {
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			writeAPISuccess(w, http.StatusOK, map[string]any{
				"items": hits,
			}, listMeta{
				Count: len(hits),
				Limit: effectiveLimit(req.Limit),
			})
		}))
	}
	mux.HandleFunc("/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, http.StatusNotFound, "not_found", "endpoint not found")
	})
	mux.HandleFunc("/v1/", func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, http.StatusNotFound, "not_found", "endpoint not found")
	})

	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           requestLogMiddleware(logger, v1ContractMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPISuccess(w http.ResponseWriter, status int, data any, meta any) {
	writeJSON(w, status, apiEnvelope{
		Data: data,
		Meta: meta,
	})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiEnvelope{
		Error: &apiErrorBody{
			Code:    code,
			Message: message,
		},
	})
}

func requestLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", requestID)
		state := &requestLogState{requestID: requestID}
		ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
		ctx = context.WithValue(ctx, requestLogStateContextKey, state)
		r = r.WithContext(ctx)

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		level := logger.Info
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			level = logger.Debug
		}
		duration := time.Since(start)
		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.Int64("duration_ms", duration.Milliseconds()),
		}
		if state.principal != nil {
			principal := *state.principal
			attrs = append(attrs,
				slog.String("subject", principal.Subject),
				slog.Bool("auth_bypass", principal.Bypass),
			)
		}
		level("http request completed", attrs...)
	})
}

func v1ContractMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isV1Path(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		applyV1ResponseHeaders(w.Header())
		if !acceptsV1JSON(r.Header.Get("Accept")) {
			writeAPIError(w, http.StatusNotAcceptable, "not_acceptable", "Accept must allow application/json or "+apicontract.HTTPMediaType)
			return
		}
		if methodHasJSONBody(r.Method) && !hasSupportedV1ContentType(r.Header.Get("Content-Type")) {
			writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json or "+apicontract.HTTPMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func wrapV1(authorizer *auth.Authorizer, limiter RateLimiter, logger *slog.Logger, method string, next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		principal, err := authorizer.Authenticate(r)
		if err != nil {
			writeAPIError(w, statusFromError(err), codeFromError(err), err.Error())
			return
		}
		r = r.WithContext(auth.WithPrincipal(r.Context(), principal))
		if state, ok := r.Context().Value(requestLogStateContextKey).(*requestLogState); ok {
			state.principal = &principal
		}
		if limiter != nil {
			if err := limiter.Allow(r.Context(), principal); err != nil {
				if errors.Is(err, quota.ErrThrottled) {
					observability.RecordHTTPRateLimitDenied(r.URL.Path)
					logger.Warn("http request throttled",
						slog.String("path", r.URL.Path),
						slog.String("subject", principal.Subject),
						slog.Bool("auth_bypass", principal.Bypass),
					)
					writeAPIError(w, http.StatusTooManyRequests, "rate_limited", "request rate limit exceeded")
					return
				}
				writeAPIError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
		}
		next(w, r, principal)
	}
}

func isV1Path(rawPath string) bool {
	return rawPath == "/v1" || strings.HasPrefix(rawPath, "/v1/")
}

func applyV1ResponseHeaders(header http.Header) {
	header.Set(apicontract.HTTPVersionHeader, apicontract.HTTPAPIVersion)
	header.Set(apicontract.HTTPStabilityHeader, apicontract.CompatibilityStability)
	header.Set("Content-Type", apicontract.HTTPMediaType)
	appendVary(header, "Accept")
}

func appendVary(header http.Header, value string) {
	existing := header.Values("Vary")
	for _, item := range existing {
		for _, part := range strings.Split(item, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func acceptsV1JSON(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	for _, part := range strings.Split(raw, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		switch mediaType {
		case "*/*", "application/*", "application/json", apicontract.HTTPMediaType:
			return true
		}
	}
	return false
}

func methodHasJSONBody(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
}

func hasSupportedV1ContentType(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	switch mediaType {
	case "application/json", apicontract.HTTPMediaType:
		return true
	default:
		return false
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func statusFromError(err error) int {
	switch {
	case errors.Is(err, memory.ErrInvalidRequest),
		errors.Is(err, memory.ErrAgentTenantMismatch),
		errors.Is(err, memory.ErrThreadMismatch):
		return http.StatusBadRequest
	case errors.Is(err, auth.ErrUnauthenticated):
		return http.StatusUnauthorized
	case errors.Is(err, auth.ErrUnauthorized):
		return http.StatusForbidden
	case errors.Is(err, memory.ErrMemoryNotFound):
		return http.StatusNotFound
	case errors.Is(err, memory.ErrTenantNotFound):
		return http.StatusNotFound
	case errors.Is(err, memory.ErrIdempotencyConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func codeFromError(err error) string {
	switch {
	case errors.Is(err, memory.ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, auth.ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, auth.ErrUnauthorized):
		return "forbidden"
	case errors.Is(err, memory.ErrAgentTenantMismatch),
		errors.Is(err, memory.ErrThreadMismatch):
		return "scope_violation"
	case errors.Is(err, memory.ErrMemoryNotFound):
		return "not_found"
	case errors.Is(err, memory.ErrTenantNotFound):
		return "not_found"
	case errors.Is(err, memory.ErrIdempotencyConflict):
		return "conflict"
	default:
		return "internal_error"
	}
}

func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	return limit
}

func queryMemoriesRequestFromHTTP(r *http.Request) (memory.QueryMemoriesRequest, error) {
	query := r.URL.Query()
	req := memory.QueryMemoriesRequest{
		Kind:   query.Get("kind"),
		Status: query.Get("status"),
	}

	tenantValue := query.Get("tenant_id")
	if tenantValue == "" {
		return memory.QueryMemoriesRequest{}, memory.ErrInvalidRequest
	}
	tenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return memory.QueryMemoriesRequest{}, err
	}
	req.TenantID = tenantID

	if value := query.Get("agent_id"); value != "" {
		agentID, err := uuid.Parse(value)
		if err != nil {
			return memory.QueryMemoriesRequest{}, err
		}
		req.AgentID = &agentID
	}
	if value := query.Get("thread_id"); value != "" {
		threadID, err := uuid.Parse(value)
		if err != nil {
			return memory.QueryMemoriesRequest{}, err
		}
		req.ThreadID = &threadID
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memory.QueryMemoriesRequest{}, err
		}
		req.Limit = limit
	}

	if err := req.Validate(); err != nil {
		return memory.QueryMemoriesRequest{}, err
	}
	return req, nil
}

func recallRequestFromHTTP(r *http.Request) (memory.RecallRequest, error) {
	query := r.URL.Query()
	req := memory.RecallRequest{
		Kind:   query.Get("kind"),
		Status: query.Get("status"),
		Query:  query.Get("query"),
	}

	tenantValue := query.Get("tenant_id")
	if tenantValue == "" {
		return memory.RecallRequest{}, memory.ErrInvalidRequest
	}
	tenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return memory.RecallRequest{}, err
	}
	req.TenantID = tenantID

	if value := query.Get("agent_id"); value != "" {
		agentID, err := uuid.Parse(value)
		if err != nil {
			return memory.RecallRequest{}, err
		}
		req.AgentID = &agentID
	}
	if value := query.Get("thread_id"); value != "" {
		threadID, err := uuid.Parse(value)
		if err != nil {
			return memory.RecallRequest{}, err
		}
		req.ThreadID = &threadID
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memory.RecallRequest{}, err
		}
		req.Limit = limit
	}

	if err := req.Validate(); err != nil {
		return memory.RecallRequest{}, err
	}
	return req, nil
}

func checkpointsRequestFromHTTP(r *http.Request) (memory.ListProjectionCheckpointsRequest, error) {
	query := r.URL.Query()
	tenantValue := query.Get("tenant_id")
	if tenantValue == "" {
		return memory.ListProjectionCheckpointsRequest{}, memory.ErrInvalidRequest
	}
	tenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return memory.ListProjectionCheckpointsRequest{}, err
	}
	req := memory.ListProjectionCheckpointsRequest{
		TenantID:       tenantID,
		ProjectionName: query.Get("projection_name"),
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memory.ListProjectionCheckpointsRequest{}, err
		}
		req.Limit = limit
	}
	if err := req.Validate(); err != nil {
		return memory.ListProjectionCheckpointsRequest{}, err
	}
	return req, nil
}

func runsRequestFromHTTP(r *http.Request) (memory.ListConsolidationRunsRequest, error) {
	query := r.URL.Query()
	tenantValue := query.Get("tenant_id")
	if tenantValue == "" {
		return memory.ListConsolidationRunsRequest{}, memory.ErrInvalidRequest
	}
	tenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return memory.ListConsolidationRunsRequest{}, err
	}
	req := memory.ListConsolidationRunsRequest{
		TenantID:       tenantID,
		ProjectionName: query.Get("projection_name"),
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memory.ListConsolidationRunsRequest{}, err
		}
		req.Limit = limit
	}
	if err := req.Validate(); err != nil {
		return memory.ListConsolidationRunsRequest{}, err
	}
	return req, nil
}

func leasesRequestFromHTTP(r *http.Request) (memory.ListWorkerLeasesRequest, error) {
	query := r.URL.Query()
	req := memory.ListWorkerLeasesRequest{}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil {
			return memory.ListWorkerLeasesRequest{}, err
		}
		req.Limit = limit
	}
	if err := req.Validate(); err != nil {
		return memory.ListWorkerLeasesRequest{}, err
	}
	return req, nil
}

func memoryIDFromProvenancePath(rawPath string) (uuid.UUID, error) {
	clean := path.Clean(rawPath)
	if !strings.HasPrefix(clean, "/v1/memories/") || !strings.HasSuffix(clean, "/provenance") {
		return uuid.UUID{}, memory.ErrInvalidRequest
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(clean, "/v1/memories/"), "/provenance")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return uuid.UUID{}, memory.ErrInvalidRequest
	}
	return uuid.Parse(trimmed)
}

func tenantGovernancePath(rawPath string) (uuid.UUID, bool, error) {
	clean := path.Clean(rawPath)
	if !strings.HasPrefix(clean, "/v1/tenants/") {
		return uuid.UUID{}, false, memory.ErrInvalidRequest
	}
	trimmed := strings.TrimPrefix(clean, "/v1/tenants/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return uuid.UUID{}, false, memory.ErrInvalidRequest
	}
	if strings.HasSuffix(trimmed, "/export") {
		tenantPart := strings.TrimSuffix(trimmed, "/export")
		tenantPart = strings.Trim(tenantPart, "/")
		if tenantPart == "" || strings.Contains(tenantPart, "/") {
			return uuid.UUID{}, false, memory.ErrInvalidRequest
		}
		id, err := uuid.Parse(tenantPart)
		return id, true, err
	}
	if strings.Contains(trimmed, "/") {
		return uuid.UUID{}, false, memory.ErrInvalidRequest
	}
	id, err := uuid.Parse(trimmed)
	return id, false, err
}
