package client

import (
	"encoding/json"
	"strings"
	"time"
)

type Execution struct {
	ID              string            `json:"id"`
	Action          string            `json:"action"`
	ExecutedAction  string            `json:"executed_action,omitempty"`
	TargetCluster   string            `json:"target_cluster"`
	Status          string            `json:"status"`
	OutputStatus    string            `json:"output_status,omitempty"`
	ApprovalState   string            `json:"approval_state,omitempty"`
	Scope           string            `json:"scope"`
	Type            string            `json:"type"`
	DryRun          bool              `json:"dry_run"`
	Force           bool              `json:"force"`
	Jira            string            `json:"jira,omitempty"`
	Operator        string            `json:"operator,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	CreatedAt       *time.Time        `json:"created_at,omitempty"`
	UpdatedAt       *time.Time        `json:"updated_at,omitempty"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	DurationSeconds *int              `json:"duration_seconds,omitempty"`
	RunnerSeconds   *int              `json:"runner_seconds,omitempty"`
	UploadSeconds   *int              `json:"upload_seconds,omitempty"`
	Output          FlexString        `json:"output,omitempty"`
	Logs            string            `json:"logs,omitempty"`
}

// FlexString handles API fields that may be a string, array, or object.
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*f = ""
		return nil
	}
	// Try string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}
	// Try array of objects — pretty-print each item
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		lines := make([]string, 0, len(arr))
		for _, item := range arr {
			var m map[string]interface{}
			if json.Unmarshal(item, &m) == nil {
				b, _ := json.Marshal(m)
				lines = append(lines, string(b))
			} else {
				lines = append(lines, string(item))
			}
		}
		*f = FlexString(strings.Join(lines, "\n"))
		return nil
	}
	// Fallback: raw JSON
	*f = FlexString(string(data))
	return nil
}

func (f FlexString) String() string {
	return string(f)
}

type ExecutionList struct {
	Items []Execution `json:"items"`
	Total int         `json:"total,omitempty"`
}

type DispatchRequest struct {
	TargetCluster string            `json:"target_cluster"`
	Jira          string            `json:"jira"`
	Params        map[string]string `json:"params,omitempty"`
	Force         bool              `json:"force"`
	DryRun        bool              `json:"dry_run"`
}

type DispatchResponse struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	ExecutedAction string `json:"executed_action,omitempty"`
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
	Description          string              `json:"description"`
	Params               []ActionParam       `json:"params,omitempty"`
	RequiredFields       []string            `json:"required_fields,omitempty"`
	Authorization        ActionAuthorization `json:"authorization,omitempty"`
	DryRunAction         string              `json:"dry_run_action,omitempty"`
	WriteCooldownSeconds int                 `json:"write_cooldown_seconds,omitempty"`
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
