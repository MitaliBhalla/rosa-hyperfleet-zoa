package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
)

type mockClient struct {
	dispatchFn       func(ctx context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error)
	getExecutionFn   func(ctx context.Context, id string, include string) (*client.Execution, error)
	listExecutionsFn func(ctx context.Context, query url.Values) (*client.ExecutionList, error)
	getActionFn      func(ctx context.Context, name string) (*client.Action, error)
	listActionsFn    func(ctx context.Context) (*client.ActionList, error)
	listAuditFn      func(ctx context.Context, query url.Values) (*client.AuditList, error)
	serverVersionFn  func(ctx context.Context) (*client.ServerVersionInfo, error)
	rawGetFn         func(ctx context.Context, path string) (*http.Response, error)
}

func (m *mockClient) Dispatch(ctx context.Context, action string, req *client.DispatchRequest) (*client.DispatchResponse, error) {
	if m.dispatchFn != nil {
		return m.dispatchFn(ctx, action, req)
	}
	return nil, fmt.Errorf("Dispatch not mocked")
}

func (m *mockClient) GetExecution(ctx context.Context, id string, include string) (*client.Execution, error) {
	if m.getExecutionFn != nil {
		return m.getExecutionFn(ctx, id, include)
	}
	return nil, fmt.Errorf("GetExecution not mocked")
}

func (m *mockClient) ListExecutions(ctx context.Context, query url.Values) (*client.ExecutionList, error) {
	if m.listExecutionsFn != nil {
		return m.listExecutionsFn(ctx, query)
	}
	return nil, fmt.Errorf("ListExecutions not mocked")
}

func (m *mockClient) GetAction(ctx context.Context, name string) (*client.Action, error) {
	if m.getActionFn != nil {
		return m.getActionFn(ctx, name)
	}
	return nil, fmt.Errorf("GetAction not mocked")
}

func (m *mockClient) ListActions(ctx context.Context) (*client.ActionList, error) {
	if m.listActionsFn != nil {
		return m.listActionsFn(ctx)
	}
	return nil, fmt.Errorf("ListActions not mocked")
}

func (m *mockClient) ListAudit(ctx context.Context, query url.Values) (*client.AuditList, error) {
	if m.listAuditFn != nil {
		return m.listAuditFn(ctx, query)
	}
	return nil, fmt.Errorf("ListAudit not mocked")
}

func (m *mockClient) ServerVersion(ctx context.Context) (*client.ServerVersionInfo, error) {
	if m.serverVersionFn != nil {
		return m.serverVersionFn(ctx)
	}
	return nil, fmt.Errorf("ServerVersion not mocked")
}

func (m *mockClient) RawGet(ctx context.Context, path string) (*http.Response, error) {
	if m.rawGetFn != nil {
		return m.rawGetFn(ctx, path)
	}
	return nil, fmt.Errorf("RawGet not mocked")
}

func newMockGlobalOpts(mock *mockClient) *GlobalOptions {
	return &GlobalOptions{
		APIURL: "https://test.lambda-url.us-east-1.on.aws",
		ClientFactory: func(_ *GlobalOptions) (APIClient, error) {
			return mock, nil
		},
	}
}
