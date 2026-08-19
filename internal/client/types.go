package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Execution struct {
	ID              string            `json:"id"`
	Action          string            `json:"action"`
	RequestedAction string            `json:"requested_action,omitempty"`
	TargetCluster   string            `json:"target_cluster"`
	Status          string            `json:"status"`
	ExecutionMode   string            `json:"execution_mode,omitempty"`
	Scope           string            `json:"scope"`
	Type            string            `json:"type"`
	DryRun          bool              `json:"dry_run"`
	Force           bool              `json:"force"`
	Jira            string            `json:"jira,omitempty"`
	Operator        string            `json:"operator,omitempty"`
	Revision        string            `json:"revision,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	CreatedAt       *time.Time        `json:"created_at,omitempty"`
	DispatchedAt    *time.Time        `json:"dispatched_at,omitempty"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	DurationMs      *int64            `json:"duration_ms,omitempty"`
	OutputBytes     *int64            `json:"output_bytes,omitempty"`
	LogBytes        *int64            `json:"log_bytes,omitempty"`
	OutputFormat    string            `json:"output_format,omitempty"`
	Output          FlexString        `json:"output,omitempty"`
	Logs            string            `json:"logs,omitempty"`
}

// FlexString handles API fields that may be a string, array, or object.
// When unmarshaled, it stores the raw string content for human rendering.
// When marshaled back to JSON, it re-parses to emit proper nested JSON.
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(string(data))
	return nil
}

func (f FlexString) MarshalJSON() ([]byte, error) {
	s := string(f)
	if s == "" {
		return []byte("null"), nil
	}
	trimmed := strings.TrimSpace(s)
	if (strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{")) && json.Valid([]byte(trimmed)) {
		return []byte(trimmed), nil
	}
	return json.Marshal(s)
}

func (f FlexString) String() string {
	return string(f)
}

type ExecutionList struct {
	Items []Execution `json:"items"`
	Total int         `json:"total,omitempty"`
}

type DispatchRequest struct {
	Jira           string            `json:"jira"`
	Params         map[string]string `json:"params,omitempty"`
	Force          bool              `json:"force"`
	DryRun         bool              `json:"dry_run"`
	ExecutionMode  string            `json:"execution_mode,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

type DispatchResponse struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Target         string     `json:"target"`
	ExecutedAction string     `json:"executed_action,omitempty"`
	ExecutionMode  string     `json:"execution_mode,omitempty"`
	Output         FlexString `json:"output,omitempty"`
	Logs           string     `json:"logs,omitempty"`
	DurationMs     *int64     `json:"duration_ms,omitempty"`
}

type ActionParam struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

type ActionAuthorization struct {
	Approval string `json:"approval,omitempty"`
}

type Action struct {
	Name                 string              `json:"name"`
	Scope                string              `json:"scope"`
	Type                 string              `json:"type"`
	ExecutionMode        string              `json:"execution_mode,omitempty"`
	Description          string              `json:"description"`
	Params               []ActionParam       `json:"parameters,omitempty"`
	Authorization        ActionAuthorization `json:"authorization,omitempty"`
	DryRunAction         string              `json:"dry_run_action,omitempty"`
	WriteCooldownSeconds int                 `json:"write_cooldown_seconds,omitempty"`
	TimeoutSeconds       int                 `json:"timeout_seconds,omitempty"`
}

type ActionList struct {
	Items []Action `json:"items"`
}

type AuditEntry struct {
	Timestamp     string `json:"timestamp"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	StatusCode    int    `json:"status_code"`
	Operator      string `json:"operator"`
	Action        string `json:"action,omitempty"`
	TargetCluster string `json:"target_cluster,omitempty"`
	Jira          string `json:"jira,omitempty"`
	Force         bool   `json:"force,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
	ApprovalState string `json:"approval_state,omitempty"`
	ExecutionID   string `json:"execution_id,omitempty"`
}

func (a AuditEntry) ShortPath() string {
	if strings.HasPrefix(a.Path, "/api/v0/trusted-actions/") {
		return a.Path[len("/api/v0/trusted-actions/"):]
	}
	return a.Path
}

type AuditList struct {
	Items []AuditEntry `json:"items"`
}

type ServerVersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	Target    string `json:"target"`
}

type APIError struct {
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// LambdaRuntimeError is returned by AWS Lambda Function URLs when the Lambda
// function crashes, times out, or returns an unhandled error. The format differs
// from ZOA's APIError.
type LambdaRuntimeError struct {
	ErrorMessage string `json:"errorMessage"`
	ErrorType    string `json:"errorType"`
}

func (e *LambdaRuntimeError) Error() string {
	switch e.ErrorType {
	case "Runtime.ExitError":
		return "ZOA API is unavailable (Lambda failed to start — check CloudWatch logs for startup health failures)"
	case "Task timed out after", "Runtime.DeadlineExceeded":
		return "ZOA API timed out (Lambda execution deadline exceeded)"
	default:
		if e.ErrorMessage != "" {
			return fmt.Sprintf("ZOA API error [%s]: %s", e.ErrorType, e.ErrorMessage)
		}
		return fmt.Sprintf("ZOA API error: %s", e.ErrorType)
	}
}

func (e *LambdaRuntimeError) IsUnavailable() bool {
	return e.ErrorType == "Runtime.ExitError"
}
