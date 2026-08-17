package cli

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func TestGetExecution_WhenTerminalStatus_ItShouldReturnNilError(t *testing.T) {
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, id string, include string) (*client.Execution, error) {
			callCount++
			if callCount == 1 && include != "" {
				t.Errorf("first call should have empty include, got %q", include)
			}
			dur := int64(1500)
			return &client.Execution{
				ID:            "exec-get-1",
				Action:        "get_pods",
				TargetCluster: "mc-useast1-1",
				Status:        "succeeded",
				ExecutionMode: "sync",
				Scope:         "kube-api",
				Type:          "read",
				DurationMs:    &dur,
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := getExecution(context.Background(), global, "exec-get-1", getOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetExecution_WhenIncludeOutput_ItShouldFetchOutput(t *testing.T) {
	var includeOnSecondCall string
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, include string) (*client.Execution, error) {
			callCount++
			if callCount == 2 {
				includeOnSecondCall = include
			}
			return &client.Execution{
				ID:     "exec-get-2",
				Status: "succeeded",
				Output: client.FlexString(`[{"name":"pod-1"}]`),
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := getExecution(context.Background(), global, "exec-get-2", getOpts{includeOutput: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if includeOnSecondCall != "output" {
		t.Errorf("expected second call include='output', got %q", includeOnSecondCall)
	}
}

func TestGetExecution_WhenIncludeAll_ItShouldFetchOutputAndLogs(t *testing.T) {
	var includeOnSecondCall string
	callCount := 0
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, include string) (*client.Execution, error) {
			callCount++
			if callCount == 2 {
				includeOnSecondCall = include
			}
			return &client.Execution{
				ID:     "exec-get-3",
				Status: "succeeded",
				Output: client.FlexString(`{}`),
				Logs:   "some logs",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	err := getExecution(context.Background(), global, "exec-get-3", getOpts{
		includeOutput: true,
		includeLogs:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if includeOnSecondCall != "output,logs" {
		t.Errorf("expected include='output,logs', got %q", includeOnSecondCall)
	}
}

func TestGetExecution_WhenJSONFormat_ItShouldReturnNilError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return &client.Execution{
				ID:     "exec-get-json",
				Status: "succeeded",
			}, nil
		},
	}

	global := newMockGlobalOpts(mock)
	global.OutputFormat = output.FormatJSON
	err := getExecution(context.Background(), global, "exec-get-json", getOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetExecution_WhenClientReturnsError_ItShouldPropagateError(t *testing.T) {
	mock := &mockClient{
		getExecutionFn: func(_ context.Context, _ string, _ string) (*client.Execution, error) {
			return nil, &client.APIError{Code: "not_found", Reason: "execution not found"}
		},
	}

	global := newMockGlobalOpts(mock)
	err := getExecution(context.Background(), global, "missing", getOpts{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetOpts_WhenWaitTimeout_ItShouldDefaultTo5Minutes(t *testing.T) {
	defaultTimeout := 5 * time.Minute
	if defaultTimeout != 5*time.Minute {
		t.Errorf("expected 5m default, got %v", defaultTimeout)
	}
}

func TestGetOpts_WhenWaitPollInterval_ItShouldDefaultTo30Seconds(t *testing.T) {
	defaultInterval := 30 * time.Second
	if defaultInterval != 30*time.Second {
		t.Errorf("expected 30s default, got %v", defaultInterval)
	}
}
