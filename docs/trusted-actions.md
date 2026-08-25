# Trusted Actions — Authoring Guide

This guide explains how to create new Trusted Actions (TAs) in Go for the ZOA Lambda architecture.

## Overview

Trusted Actions are Go packages that implement the `actions.Action` interface. Each action:

- Has metadata (name, scope, type, parameters, RBAC requirements)
- Self-registers via `init()` into the global registry
- Executes against a Kubernetes or AWS API using injected clients
- Returns structured JSON output

## Interface

```go
type Action interface {
    Metadata() ActionMetadata
    Validate(ctx context.Context, params *ExecutionParams) error
    Execute(ctx context.Context, params *ExecutionParams) (*ActionResult, error)
}
```

## Creating a New Action

### 1. Create the file

```
pkg/actions/myaction.go
pkg/actions/myaction_test.go
```

### 2. Implement the interface

```go
package myaction

import (
    "context"
    "github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func init() {
    actions.Register(&myAction{})
}

type myAction struct{}

func (a *myAction) Metadata() actions.ActionMetadata {
    return actions.ActionMetadata{
        Name:        "my_action",
        Scope:       "kube-api",   // or "aws-api"
        Type:        "read",       // or "write"
        Description: "Does something useful",
        Parameters: []actions.ParameterDef{
            {Name: "namespace", Required: true, Description: "Target namespace"},
            {Name: "name", Required: false, Description: "Resource name"},
        },
        RBAC: &actions.RBACConfig{
            ClusterScoped: false,
            Rules: []actions.RBACRule{
                {APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
            },
        },
    }
}

func (a *myAction) Validate(ctx context.Context, params *actions.ExecutionParams) error {
    // Validate params beyond basic required/unknown checks
    return nil
}

func (a *myAction) Execute(ctx context.Context, params *actions.ExecutionParams) (*actions.ActionResult, error) {
    // Use params.KubeClient for Kubernetes resources
    // Use params.DynamicClient for unstructured resources
    // Use params.AWSConfig for AWS API calls

    return &actions.ActionResult{
        Success: true,
        Output:  myResults,
        Summary: "Found 5 pods",
    }, nil
}
```

### 3. Self-register via `init()`

The `init()` function in your file automatically registers the action when the package is loaded. No additional import wiring needed — all TA files in `pkg/actions/` are compiled together.

### 4. Write tests

Use `k8s.io/client-go/kubernetes/fake` for Kubernetes actions:

```go
func TestMyAction(t *testing.T) {
    client := fake.NewSimpleClientset(
        &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"}},
    )
    params := &actions.ExecutionParams{
        Params:     map[string]string{"namespace": "default"},
        KubeClient: client,
        Logger:     slog.Default(),
    }
    action := &myAction{}
    result, err := action.Execute(context.Background(), params)
    // assert...
}
```

## Action Types

### Read Actions (`type: "read"`)

- No write cooldown
- Subject to max concurrent per target (bypassable with `--force`)
- DryRunAction not applicable
- Lightweight RBAC (get, list, watch)

### Write Actions (`type: "write"`)

- Subject to write cooldown (`WriteCooldownSeconds`, bypassable with `--force`)
- Subject to max concurrent per target (bypassable with `--force`)
- Can optionally define `DryRunAction` (a read action for preview, advisable but not required)
- Must include safety checks (e.g., owner references, replica count)
- RBAC includes mutating verbs (delete, patch, update)

## Scope

| Scope | Client Available | Use Case |
|-------|-----------------|----------|
| `kube-api` | `params.KubeClient`, `params.DynamicClient` | Kubernetes resources |
| `aws-api` | `params.AWSConfig` | AWS SDK calls |

## Safety Guidelines

1. **Secret data visibility** — by default show keys only; when `verbose=true`, show decoded (raw) values. The Go K8s client returns decoded data natively, so we expose it directly rather than re-encoding to base64
2. **Block HCP namespaces** — reject operations on `clusters-*` namespaces for secrets
3. **Require owner references** — refuse to delete standalone resources
4. **Verify state before mutating** — check replicas > 0, resource exists, etc.
5. **Return affected resources** — populate `ActionResult.AffectedResources` for audit

## Discovering Actions

Run `zoa actions` to see all registered TAs and `zoa describe <name>` for details. All TAs live as flat files in `pkg/actions/` — look at existing implementations as reference.

## Testing Patterns

- Use `"When ... it should ..."` format for test case names
- Test validation failures (missing params, safety blocks)
- Test happy path with fake clients
- Test edge cases (empty results, not found, timeouts)
- Run with `-race` flag

## Conformance Gate

The conformance test suite (`pkg/actions/conformance_test.go`) runs on every PR and enforces:

- Required metadata (name, scope, type, mode, description, timeout)
- Snake_case naming
- Scope-RBAC consistency (kube-api must declare RBAC, aws-api must not)
- Write-TA rules (cooldown > 0, DryRunAction if defined must be a read TA)
- Timeout ceiling (no TA exceeds `EXECUTION_DEADLINE_SECONDS`)
- Parameter uniqueness and descriptions
- Every registered TA must have a corresponding test file

New TAs that fail conformance will break the build.
