# Timeout & Deadline Tuning Guide

This document explains the timeout architecture, how values relate to each other, and how to safely adjust them.

## Three Layers of Timeouts

```
┌───────────────────────────────────────────────────────────┐
│  Lambda Function Timeout (hard ceiling, AWS-enforced)     │
│  Default: 300s. Configured in Terraform.                  │
│  ┌───────────────────────────────────────────────────────┐│
│  │ Code-level deadline (context.WithTimeout)             ││
│  │ Default: 295s (execution) / 55s (reconciler/GC)      ││
│  │ Configured via env vars without code change.         ││
│  │ ┌─────────────────────────────────────────────────┐  ││
│  │ │ Per-TA TimeoutSeconds (metadata in Go code)     │  ││
│  │ │ Overridable via CLI: --timeout 60               │  ││
│  │ │ Must be <= code-level deadline.                 │  ││
│  │ └─────────────────────────────────────────────────┘  ││
│  └───────────────────────────────────────────────────────┘│
└───────────────────────────────────────────────────────────┘
```

## Current Defaults

| Parameter | Default | Where | What it controls |
|---|---|---|---|
| `lambda_api_timeout` | 300s | Terraform variable | API Lambda hard ceiling |
| `lambda_worker_timeout` | 300s | Terraform variable | Worker Lambda hard ceiling |
| `EXECUTION_DEADLINE_SECONDS` | 295s | Lambda env var | Code deadline for TA execution |
| `RECONCILER_DEADLINE_SECONDS` | 55s | Lambda env var | Code deadline for reconciler/GC |
| `MAX_BATCH_PER_TICK` | 30 | Lambda env var | Items per scheduled phase |
| Per-TA `TimeoutSeconds` | 120s | Go code | Individual TA max runtime |

## Constraint Rules

1. **Per-TA TimeoutSeconds <= EXECUTION_DEADLINE_SECONDS** (enforced by conformance test — applies to sync TAs only; async TAs use K8s `activeDeadlineSeconds` and can exceed this)
2. **EXECUTION_DEADLINE_SECONDS < lambda_worker_timeout** (5s safety buffer for cleanup/metrics)
3. **RECONCILER_DEADLINE_SECONDS < lambda_worker_timeout** (must finish before Lambda kills the process)
4. **Lambda timeout <= 900s** (AWS hard limit — only constrains sync execution)

## Sync vs Async Timeout Constraints

**Sync TAs** execute inside a Lambda invocation — their maximum runtime is bounded by the Lambda timeout ceiling (max 900s per AWS). This is the constraint that matters.

**Async TAs** execute inside a K8s Job — the Lambda only creates the Job (seconds), then the reconciler polls status every 1m. The TA's `TimeoutSeconds` is set as the Job's `activeDeadlineSeconds`, enforced by Kubernetes (kills the Pod, marks Job as `DeadlineExceeded`). Lambda timeout is irrelevant for async — a 10-minute TA works fine with a 300s Lambda.

## How to Increase Timeouts

### Scenario: A new sync TA needs 10 minutes

1. Increase `lambda_worker_timeout` in Terraform to 900 (max):
   ```hcl
   lambda_worker_timeout = 900
   ```
2. `EXECUTION_DEADLINE_SECONDS` auto-updates (it's `lambda_worker_timeout - 5 = 895`)
3. Set the TA's `TimeoutSeconds` to 600 in its Go registration
4. Update the test ceiling in `rbac_validator_test.go`:
   ```go
   const executionDeadlineCeiling = 895
   ```
5. Run `go test ./pkg/actions/` to verify compliance

### Scenario: A new async TA needs 30 minutes

No Lambda changes needed. Just set the TA's `TimeoutSeconds` to 1800 in its Go registration. This becomes the Job's `activeDeadlineSeconds`. The conformance test only checks sync ceiling (`EXECUTION_DEADLINE_SECONDS`), so async TAs can exceed it freely.

### Scenario: Reconciler needs more time (many items per tick)

1. Option A: Increase `MAX_BATCH_PER_TICK` env var (allows more items per tick)
2. Option B: Increase `RECONCILER_DEADLINE_SECONDS` env var (allows more time per tick)
3. Both can be changed without code deployment — just update the Lambda env vars

### Scenario: Fast-forward — reduce costs

Reduce `lambda_worker_timeout` to 60s if all TAs are confirmed fast. Adjust env vars accordingly.

## How Deadlines Work in Code

### API Lambda (sync TAs)
```
Request arrives → EXECUTION_DEADLINE_SECONDS context → Execute TA → Upload artifacts → Respond
```
If the TA exceeds its per-TA timeout or the code deadline fires, the TA is interrupted and marked `failed`.

### Worker Lambda (scheduled routes)
```
EventBridge trigger → RECONCILER_DEADLINE_SECONDS context → Run phase → Emit metrics → Return
```
Each phase (reconciler/GC) checks `ctx.Err()` between items. If deadline fires mid-batch, remaining items are deferred to the next tick.

### Worker Lambda (self-invoked TA execution)
```
Self-invoke → EXECUTION_DEADLINE_SECONDS context → Load execution → Execute TA → Update DynamoDB → Return
```
Per-TA timeout is enforced by the executor. If the code deadline fires before TA completes, the execution is marked `failed`.

## Billing Implications

Lambda bills per-millisecond of actual execution time, NOT per-timeout-value. A Lambda with timeout=300s that finishes in 3s is billed for 3s. The timeout only determines the hard ceiling before AWS kills the process.

## Discoverability

- **Terraform**: All timeout variables have `description` fields explaining their purpose and safe ranges
- **Env vars**: Each env var has a structured log line on Lambda startup showing its value
- **Tests**: `TestAllRegisteredActions_TimeoutCompliance` breaks the build if any TA exceeds the ceiling
- **This doc**: The canonical reference for timeout decisions and adjustment procedures
