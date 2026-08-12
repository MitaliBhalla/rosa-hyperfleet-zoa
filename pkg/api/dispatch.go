package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

var jiraPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)

type createRequest struct {
	Jira           string            `json:"jira"`
	Params         map[string]string `json:"params,omitempty"`
	Force          bool              `json:"force"`
	DryRun         bool              `json:"dry_run"`
	ExecutionMode  string            `json:"execution_mode,omitempty"`  // "sync" or "async" — override TA default
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"` // override TA's default timeout (bounded by server max)
}

type createResponse struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"`
	TargetCluster  string          `json:"target"`
	ExecutedAction string          `json:"executed_action,omitempty"`
	ExecutionMode  string          `json:"execution_mode"`
	Output         json.RawMessage `json:"output,omitempty"`
	Logs           string          `json:"logs,omitempty"`
	DurationMs     *int64          `json:"duration_ms,omitempty"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request, actionName string) {
	ctx := r.Context()
	// TODO: Replace with identity resolution from ECS task ARN once rosa-boundary is integrated
	operator := r.Header.Get("X-Operator")
	accountID := r.Header.Get("X-Account-ID")

	if accountID == "" {
		writeError(w, http.StatusBadRequest, "missing_account", "X-Account-ID header required")
		return
	}

	action, ok := actions.Get(actionName)
	if !ok {
		h.recordAudit(r, http.StatusNotFound, actionName, "")
		writeError(w, http.StatusNotFound, "action_not_found", fmt.Sprintf("action %q not found", actionName))
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.recordAudit(r, http.StatusBadRequest, actionName, "")
		writeError(w, http.StatusBadRequest, "invalid_body", "failed to parse request body")
		return
	}

	if req.Jira != "" && !jiraPattern.MatchString(req.Jira) {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
		writeError(w, http.StatusBadRequest, "invalid-jira", fmt.Sprintf("jira ticket %q must match format PROJECT-123", req.Jira))
		return
	}

	meta := action.Metadata()

	if err := validateParams(meta, req.Params); err != nil {
		h.recordAudit(r, http.StatusBadRequest, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
		writeError(w, http.StatusBadRequest, "invalid_params", err.Error())
		return
	}

	executedAction := actionName
	if req.DryRun && meta.DryRunAction != "" {
		for k, v := range meta.DryRunExtraParams {
			if _, exists := req.Params[k]; !exists {
				if req.Params == nil {
					req.Params = make(map[string]string)
				}
				req.Params[k] = v
			}
		}
		executedAction = meta.DryRunAction
		dryAction, dryOk := actions.Get(executedAction)
		if !dryOk {
			writeError(w, http.StatusInternalServerError, "dry_run_misconfigured", fmt.Sprintf("dry-run action %q not found", executedAction))
			return
		}
		action = dryAction
		meta = action.Metadata()
	}

	// Write cooldown check (prevents duplicate SRE requests at UX level)
	if meta.Type == "write" && !req.Force {
		cooldown := h.cfg.WriteCooldownSeconds
		if meta.WriteCooldownSeconds > 0 {
			cooldown = meta.WriteCooldownSeconds
		}
		since := time.Now().Add(-time.Duration(cooldown) * time.Second)
		recent, err := h.executionStore.ListByTargetAndAction(ctx, h.cfg.TargetCluster, actionName, since)
		if err != nil {
			h.logger.Error("failed to check write cooldown", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to check write cooldown")
			return
		}
		if len(recent) > 0 {
			h.recordAudit(r, http.StatusTooManyRequests, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
			writeError(w, http.StatusTooManyRequests, "write_cooldown",
				fmt.Sprintf("action %q was executed within the last %ds; use force=true to override", actionName, cooldown))
			return
		}
	}

	// Max concurrent check (prevents overloading a single cluster)
	if !req.Force {
		activeCount, err := h.executionStore.CountActiveByTarget(ctx, h.cfg.TargetCluster)
		if err != nil {
			h.logger.Error("failed to count active executions", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to check concurrent limit")
			return
		}
		if activeCount >= h.cfg.MaxConcurrentPerTarget {
			h.recordAudit(r, http.StatusTooManyRequests, actionName, "", withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))
			writeError(w, http.StatusTooManyRequests, "max_concurrent",
				fmt.Sprintf("target %q has %d active executions (max %d); use force=true to override", h.cfg.TargetCluster, activeCount, h.cfg.MaxConcurrentPerTarget))
			return
		}
	}

	// Determine execution mode: CLI override > TA default
	execMode := meta.ExecutionMode
	if req.ExecutionMode != "" {
		if req.ExecutionMode != "sync" && req.ExecutionMode != "async" {
			writeError(w, http.StatusBadRequest, "invalid_execution_mode",
				fmt.Sprintf("execution_mode must be 'sync' or 'async', got %q", req.ExecutionMode))
			return
		}
		if req.ExecutionMode != meta.ExecutionMode {
			h.logger.Info("execution mode overridden",
				"action", actionName,
				"default", meta.ExecutionMode,
				"requested", req.ExecutionMode,
				"operator", operator,
			)
		}
		execMode = req.ExecutionMode
	}
	if execMode == "" {
		execMode = "sync"
	}

	// Resolve timeout: CLI override > TA default, bounded by server max
	timeoutSeconds := meta.TimeoutSeconds
	if req.TimeoutSeconds > 0 {
		if req.TimeoutSeconds > h.cfg.ExecutionDeadlineSeconds {
			writeError(w, http.StatusBadRequest, "timeout_exceeded",
				fmt.Sprintf("requested timeout %ds exceeds server maximum %ds", req.TimeoutSeconds, h.cfg.ExecutionDeadlineSeconds))
			return
		}
		timeoutSeconds = req.TimeoutSeconds
	}

	executionID := uuid.New().String()
	now := time.Now().Format(time.RFC3339Nano)

	var requestedAction string
	if executedAction != actionName {
		requestedAction = actionName
	}

	exec := &store.Execution{
		ID:              executionID,
		Action:          executedAction,
		RequestedAction: requestedAction,
		AccountID:       accountID,
		TargetCluster:   h.cfg.TargetCluster,
		Status:          store.StatusDispatched,
		ExecutionMode:   execMode,
		Scope:           meta.Scope,
		Type:            meta.Type,
		DryRun:          req.DryRun,
		Force:           req.Force,
		Jira:            req.Jira,
		Operator:        operator,
		Params:          req.Params,
		Revision:        version.GitCommit,
		TimeoutSeconds:  timeoutSeconds,
		CreatedAt:       now,
		DispatchedAt:    now,
	}

	if err := h.executionStore.Create(ctx, exec); err != nil {
		h.logger.Error("failed to create execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create execution")
		return
	}

	h.recordAudit(r, http.StatusAccepted, executedAction, executionID, withJira(req.Jira), withForce(req.Force), withDryRun(req.DryRun))

	if execMode == "sync" {
		h.executeSyncAndRespond(w, ctx, exec, action, req.Params, req.Force)
		return
	}

	// Async: dispatch resources and fire-and-forget (reconciler will manage lifecycle)
	if err := h.executor.DispatchAsync(ctx, exec, action); err != nil {
		h.logger.Error("async dispatch failed", "error", err, "execution_id", executionID)
		_ = h.executionStore.TransitionWithMetadata(ctx, executionID, store.StatusDispatched, store.StatusFailed,
			map[string]interface{}{
				"completedAt":     time.Now().Format(time.RFC3339Nano),
				"durationMs": int64(0),
			})
		writeJSON(w, http.StatusOK, createResponse{
			ID:             executionID,
			TargetCluster:  h.cfg.TargetCluster,
			Status:         string(store.StatusFailed),
			ExecutedAction: executedAction,
			ExecutionMode: execMode,
		})
		return
	}

	writeJSON(w, http.StatusAccepted, createResponse{
		ID:             executionID,
		TargetCluster:  h.cfg.TargetCluster,
		Status:         string(store.StatusDispatched),
		ExecutedAction: executedAction,
		ExecutionMode: execMode,
	})
}

func (h *Handler) executeSyncAndRespond(w http.ResponseWriter, ctx context.Context, exec *store.Execution, action actions.Action, params map[string]string, force bool) {
	timeout := time.Duration(exec.TimeoutSeconds) * time.Second
	if timeout == 0 || timeout > time.Duration(h.cfg.ExecutionDeadlineSeconds)*time.Second {
		timeout = time.Duration(h.cfg.ExecutionDeadlineSeconds) * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, execErr := h.executor.ExecuteSync(execCtx, exec.ID, action, params, &executor.SyncContext{
		Operator:      exec.Operator,
		TargetCluster: exec.TargetCluster,
		Force:         force,
	})

	startTime, _ := time.Parse(time.RFC3339Nano, exec.CreatedAt)
	durationMs := time.Since(startTime).Milliseconds()

	finalStatus := store.StatusSucceeded
	if execCtx.Err() == context.DeadlineExceeded {
		finalStatus = store.StatusTimedOut
	} else if execErr != nil || (result != nil && !result.Success) {
		finalStatus = store.StatusFailed
	}

	updates := map[string]interface{}{
		"durationMs": durationMs,
	}
	if result != nil {
		updates["outputBytes"] = result.OutputBytes
		updates["logBytes"] = result.LogBytes
		updates["outputFormat"] = "json"
	}

	if err := h.executionStore.TransitionWithMetadata(ctx, exec.ID, store.StatusDispatched, finalStatus, updates); err != nil {
		h.logger.Error("failed to transition execution status after sync completion",
			"execution_id", exec.ID, "target_status", string(finalStatus), "error", err)
	}

	resp := createResponse{
		ID:             exec.ID,
		TargetCluster:  h.cfg.TargetCluster,
		Status:         string(finalStatus),
		ExecutedAction: exec.Action,
		ExecutionMode:  exec.ExecutionMode,
		DurationMs:     &durationMs,
	}
	if result != nil {
		if finalStatus == store.StatusSucceeded {
			resp.Output = result.Output
		} else {
			resp.Logs = result.Logs
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func validateParams(meta actions.ActionMetadata, params map[string]string) error {
	defined := make(map[string]bool)
	for _, p := range meta.Parameters {
		defined[p.Name] = true
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				return fmt.Errorf("required parameter %q is missing", p.Name)
			}
		}
	}

	for k := range params {
		if !defined[k] {
			return fmt.Errorf("unknown parameter %q", k)
		}
	}

	return nil
}
