package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/metrics"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// S3Getter abstracts S3 read operations for testability.
type S3Getter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type Handler struct {
	cfg            *config.Config
	executionStore store.ExecutionStore
	auditStore     store.AuditStore
	executor       *executor.Executor
	s3Client       S3Getter
	logger         *slog.Logger
	mux            *http.ServeMux
}

func New(cfg *config.Config, execStore store.ExecutionStore, auditStore store.AuditStore, exec *executor.Executor, s3Client S3Getter, logger *slog.Logger) *Handler {
	h := &Handler{
		cfg:            cfg,
		executionStore: execStore,
		auditStore:     auditStore,
		executor:       exec,
		s3Client:       s3Client,
		logger:         logger,
		mux:            http.NewServeMux(),
	}
	h.registerRoutes()
	return h
}

func (h *Handler) registerRoutes() {
	// Health check endpoint
	h.mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h.mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		info := version.Get()
		resp := struct {
			version.Info
			Target string `json:"target"`
		}{
			Info:   info,
			Target: h.cfg.TargetCluster,
		}
		writeJSON(w, http.StatusOK, resp)
	})

	// TA execution
	h.mux.HandleFunc("POST /api/v0/trusted-actions/{action}/run", h.handleCreateRoute)

	// Execution queries
	h.mux.HandleFunc("GET /api/v0/trusted-actions/runs/{id}/output", h.handleOutputRoute)
	h.mux.HandleFunc("GET /api/v0/trusted-actions/runs/{id}/logs", h.handleLogsRoute)
	h.mux.HandleFunc("GET /api/v0/trusted-actions/runs/{id}", h.handleGetExecutionRoute)
	h.mux.HandleFunc("GET /api/v0/trusted-actions/runs", h.handleListExecutions)

	// TA metadata
	h.mux.HandleFunc("GET /api/v0/trusted-actions/{action}", h.handleDescribeActionRoute)
	h.mux.HandleFunc("GET /api/v0/trusted-actions", h.handleListActions)

	// Audit
	h.mux.HandleFunc("GET /api/v0/trusted-actions/audit", h.handleAudit)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	h.mux.ServeHTTP(rw, r)

	duration := time.Since(start)
	route := r.Method + " " + r.URL.Path

	dims := map[string]string{
		"Cluster": h.cfg.TargetCluster,
		"Route":   route,
	}
	mvs := map[string]metrics.MetricValue{
		"RequestDuration": metrics.Milliseconds(duration.Milliseconds()),
		"RequestCount":    metrics.Count(1),
	}
	if rw.statusCode >= 400 {
		mvs["ErrorCount"] = metrics.Count(1)
	}
	metrics.Emit(dims, mvs)
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// --- Route handlers (extract path params from request) ---

func (h *Handler) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	h.handleCreate(w, r, action)
}

func (h *Handler) handleGetExecutionRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.handleGetExecution(w, r, id)
}

func (h *Handler) handleOutputRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.recordAudit(r, http.StatusOK, "", id)
	h.handleDownloadOutput(w, r, id, "output")
}

func (h *Handler) handleLogsRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.recordAudit(r, http.StatusOK, "", id)
	h.handleDownloadOutput(w, r, id, "logs")
}

func (h *Handler) handleDescribeActionRoute(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	h.handleDescribeAction(w, r, action)
}

// --- Business logic handlers ---

func (h *Handler) handleGetExecution(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	exec, err := h.executionStore.Get(ctx, id)
	if err != nil {
		h.logger.Error("failed to get execution", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get execution")
		return
	}
	if exec == nil {
		writeError(w, http.StatusNotFound, "not_found", "execution not found")
		return
	}

	h.recordAudit(r, http.StatusOK, "", id)

	type enrichedExecution struct {
		*store.Execution
		DurationMs *int64          `json:"duration_ms"`
		Output     json.RawMessage `json:"output,omitempty"`
		Logs       string          `json:"logs,omitempty"`
	}
	resp := enrichedExecution{Execution: exec}

	if exec.Status.IsTerminal() {
		d := exec.DurationMs
		resp.DurationMs = &d
	}

	include := r.URL.Query().Get("include")
	if include != "" && exec.Status.IsTerminal() {
		includes := parseIncludes(include)
		if includes["output"] {
			data, _ := h.fetchArtifact(ctx, id, "output")
			if len(data) > 0 && json.Valid(data) {
				resp.Output = json.RawMessage(data)
			} else if len(data) > 0 {
				quoted, _ := json.Marshal(string(data))
				resp.Output = json.RawMessage(quoted)
			}
		}
		if includes["logs"] {
			data, _ := h.fetchArtifact(ctx, id, "logs")
			resp.Logs = string(data)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	accountID := r.Header.Get("X-Account-ID")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "missing_account", "X-Account-ID header required")
		return
	}

	q := r.URL.Query()
	filter := &store.ListFilter{}
	hasFilter := false

	if v := q.Get("status"); v != "" {
		s := store.Status(v)
		filter.Status = &s
		hasFilter = true
	}
	if v := q.Get("execution_mode"); v != "" {
		filter.ExecutionMode = &v
		hasFilter = true
	}
	if v := q.Get("action"); v != "" {
		filter.Action = &v
		hasFilter = true
	}
	if v := q.Get("type"); v != "" {
		filter.Type = &v
		hasFilter = true
	}
	if v := q.Get("scope"); v != "" {
		filter.Scope = &v
		hasFilter = true
	}
	if v := q.Get("operator"); v != "" {
		filter.Operator = &v
		hasFilter = true
	}
	if q.Get("dry_run") == "true" {
		dryRun := true
		filter.DryRun = &dryRun
		hasFilter = true
	}
	if q.Get("force") == "true" {
		force := true
		filter.Force = &force
		hasFilter = true
	}
	if v := q.Get("since"); v != "" {
		if t, err := parseSince(v); err == nil {
			filter.Since = &t
			hasFilter = true
		}
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &limit); n == 1 && err == nil {
			if limit > 100 {
				limit = 100
			}
		}
	}
	filter.Limit = limit

	var filterPtr *store.ListFilter
	if hasFilter || limit != 50 {
		filterPtr = filter
	}

	executions, err := h.executionStore.List(r.Context(), accountID, limit, filterPtr)
	if err != nil {
		h.logger.Error("failed to list executions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list executions")
		return
	}

	h.recordAudit(r, http.StatusOK, "", "")

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": executions, "count": len(executions), "next_token": nil})
}

func (h *Handler) handleDescribeAction(w http.ResponseWriter, _ *http.Request, actionName string) {
	action, ok := actions.Get(actionName)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action.Metadata())
}

func (h *Handler) handleListActions(w http.ResponseWriter, _ *http.Request) {
	allActions := actions.List()
	metas := make([]interface{}, 0, len(allActions))
	for _, a := range allActions {
		metas = append(metas, a.Metadata())
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": metas})
}

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request) {
	accountID := r.Header.Get("X-Account-ID")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "missing_account", "X-Account-ID header required")
		return
	}

	q := r.URL.Query()
	filter := &store.AuditFilter{}

	if v := q.Get("since"); v != "" {
		if t, err := parseSince(v); err == nil {
			filter.Since = &t
		}
	}
	if v := q.Get("action"); v != "" {
		filter.Action = &v
	}
	if v := q.Get("method"); v != "" {
		filter.Method = &v
	}
	if v := q.Get("operator"); v != "" {
		filter.Operator = &v
	}
	if q.Get("force") == "true" {
		force := true
		filter.Force = &force
	}
	if q.Get("dry_run") == "true" {
		dryRun := true
		filter.DryRun = &dryRun
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &limit); n == 1 && err == nil {
			if limit > 200 {
				limit = 200
			}
		}
	}
	filter.Limit = limit

	entries, err := h.auditStore.List(r.Context(), accountID, filter)
	if err != nil {
		h.logger.Error("failed to list audit entries", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list audit entries")
		return
	}

	h.recordAudit(r, http.StatusOK, "", "")

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": entries, "total": len(entries)})
}

func (h *Handler) fetchArtifact(ctx context.Context, id, artifact string) ([]byte, error) {
	if h.s3Client == nil {
		return nil, fmt.Errorf("s3 client not configured")
	}

	var key string
	switch artifact {
	case "output":
		key = fmt.Sprintf("executions/%s/output.json", id)
	case "logs":
		key = fmt.Sprintf("executions/%s/execution.log", id)
	default:
		return nil, fmt.Errorf("unknown artifact: %s", artifact)
	}

	out, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(h.cfg.ArtifactBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func parseIncludes(include string) map[string]bool {
	result := make(map[string]bool)
	for _, part := range strings.Split(include, ",") {
		result[strings.TrimSpace(part)] = true
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"kind": "Error", "code": code, "reason": message})
}

type auditOpts struct {
	jira   string
	force  bool
	dryRun bool
}

type AuditOption func(*auditOpts)

func withJira(jira string) AuditOption {
	return func(o *auditOpts) { o.jira = jira }
}

func withForce(force bool) AuditOption {
	return func(o *auditOpts) { o.force = force }
}

func withDryRun(dryRun bool) AuditOption {
	return func(o *auditOpts) { o.dryRun = dryRun }
}

func (h *Handler) recordAudit(r *http.Request, statusCode int, action, executionID string, opts ...AuditOption) {
	if h.auditStore == nil {
		return
	}
	var o auditOpts
	for _, opt := range opts {
		opt(&o)
	}
	accountID := r.Header.Get("X-Account-ID")
	operator := r.Header.Get("X-Operator")
	entry := &store.AuditEntry{
		AccountID:     accountID,
		Timestamp:     time.Now().Format(time.RFC3339Nano),
		Method:        r.Method,
		Path:          r.URL.Path,
		StatusCode:    statusCode,
		Operator:      operator,
		Action:        action,
		TargetCluster: h.cfg.TargetCluster,
		SourceIP:      r.Header.Get("X-Source-IP"),
		RequestID:     r.Header.Get("X-Request-ID"),
		UserAgent:     r.Header.Get("User-Agent"),
		ExecutionID:   executionID,
		Jira:          o.jira,
		Force:         o.force,
		DryRun:        o.dryRun,
	}
	if err := h.auditStore.Record(r.Context(), entry); err != nil {
		h.logger.Error("failed to record audit entry", "error", err)
	}
}

func parseSince(s string) (time.Time, error) {
	if len(s) < 2 {
		return time.Time{}, fmt.Errorf("invalid since: %s", s)
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	var n int
	if _, err := fmt.Sscanf(numStr, "%d", &n); err != nil {
		return time.Time{}, fmt.Errorf("invalid since: %s", s)
	}
	var d time.Duration
	switch unit {
	case 's':
		d = time.Duration(n) * time.Second
	case 'm':
		d = time.Duration(n) * time.Minute
	case 'h':
		d = time.Duration(n) * time.Hour
	case 'd':
		d = time.Duration(n) * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("invalid since unit: %c", unit)
	}
	return time.Now().Add(-d), nil
}
