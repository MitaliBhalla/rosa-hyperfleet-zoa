package handler

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

// mockConfig wraps a real config.Config to inject test values without env vars.
type mockConfig struct {
	mode string
}

func (m *mockConfig) toConfig() *config.Config {
	return &config.Config{
		HandlerMode:               m.mode,
		TargetCluster:             "test-cluster",
		ReconcilerDeadlineSeconds: 55,
		ExecutionDeadlineSeconds:  295,
	}
}

func newMockHTTPEvent(method, path, query string) *events.APIGatewayV2HTTPRequest {
	return &events.APIGatewayV2HTTPRequest{
		RawPath:        path,
		RawQueryString: query,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: method,
				Path:   path,
			},
		},
		Headers: map[string]string{},
	}
}

// mockExecStore is a minimal store mock for handler tests.
type mockExecStore struct {
	getResult *store.Execution
	getErr    error
}

func (m *mockExecStore) Create(_ context.Context, _ *store.Execution) error { return nil }
func (m *mockExecStore) Get(_ context.Context, _ string) (*store.Execution, error) {
	return m.getResult, m.getErr
}
func (m *mockExecStore) List(_ context.Context, _ string, _ int, _ *store.ListFilter) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) TransitionStatus(_ context.Context, _ string, _, _ store.Status) error {
	return nil
}
func (m *mockExecStore) TransitionWithMetadata(_ context.Context, _ string, _, _ store.Status, _ map[string]interface{}) error {
	return nil
}
func (m *mockExecStore) QueryByStatus(_ context.Context, _ store.Status) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryByStatusAndClass(_ context.Context, _ store.Status, _ string) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryTerminal(_ context.Context, _ time.Duration) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) MarkCleaned(_ context.Context, _ string) error { return nil }
func (m *mockExecStore) ListByTargetAndAction(_ context.Context, _, _ string, _ time.Time) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) CountActiveByTarget(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockExecStore) QueryByTargetAndStatus(_ context.Context, _ string, _ store.Status) ([]*store.Execution, error) {
	return nil, nil
}
func (m *mockExecStore) QueryTerminalByTarget(_ context.Context, _ string, _ time.Duration) ([]*store.Execution, error) {
	return nil, nil
}
