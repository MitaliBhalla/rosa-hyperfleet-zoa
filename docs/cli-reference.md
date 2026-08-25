# CLI Reference

The `zoa` CLI communicates with the Lambda API via SigV4-signed HTTPS requests.

## Design Philosophy

The CLI is deliberately built around **SRE muscle memory** — the patterns and conventions that operators have internalized from years of daily use of `kubectl`, `aws-cli`, and `oc`. This means:

- **Familiar flags**: `-n` for namespace, `-o json` for machine output, `-A` for all-namespaces, `-v` for verbose — no learning curve
- **Composable output**: table by default (human), JSON for piping into `jq`, same pattern as `kubectl get -o json | jq`
- **Subcommand structure**: verb-first (`run`, `get`, `describe`, `logs`) mirrors kubectl's mental model
- **Predictable behavior**: `--dry-run` previews, `--force` overrides safety, `--wait` blocks until done — exactly what you'd expect
- **Shell completion**: full zsh/bash/fish completion so discoverability is instant
- **Zero configuration**: inherit AWS credentials from the environment (same as aws-cli), single env var for endpoint

The goal: an SRE who has never seen ZOA should be productive within 30 seconds of reading `zoa --help`.

## Configuration

```bash
export ZOA_API_URL="https://<id>.lambda-url.<region>.on.aws"
```

AWS credentials must be available in the environment (e.g., via `aws configure export-credentials`).

## Commands

| Command | Description |
|---------|-------------|
| `run <action>` | Execute a Trusted Action |
| `runs` | List recent executions |
| `get <id>` | Get execution details |
| `output <id>` | Show execution output |
| `logs <id>` | Show execution logs |
| `download <id>` | Download output to a local file |
| `actions` | List all available Trusted Actions |
| `describe <action>` | Show TA details (params, scope, timeout) |
| `audit` | View audit trail |
| `version` | Print client and server version |
| `completion` | Generate shell completion scripts |

## Examples

```bash
# Discover
zoa actions
zoa describe get_resource

# Read resources (sync, default)
zoa run get_resource --jira OSD-123 --namespace kube-system --resource pods
zoa run get_resource --jira OSD-123 --resource nodes
zoa run get_resource --jira OSD-123 --namespace cert-manager --resource deployments

# Read with verbose output (full API objects)
zoa run get_resource --jira OSD-123 --resource pods -A --verbose

# AWS API reads
zoa run list_eks_clusters --jira OSD-123
zoa run describe_eks_cluster --jira OSD-123 --name my-cluster

# Write actions (have cooldown)
zoa run delete_pod --jira OSD-123 --namespace grafana --name grafana-abc123
zoa run rollout_restart --jira OSD-123 --namespace cert-manager --resource deployment --name cert-manager

# Dry-run a write action (executes the read preview)
zoa run delete_pod --jira OSD-123 --namespace grafana --name grafana-abc123 --dry-run

# Force (bypass cooldown)
zoa run delete_pod --jira OSD-123 --namespace grafana --name grafana-abc123 --force

# Async execution (for longer-running operations)
zoa run get_resource --jira OSD-123 --namespace kube-system --resource pods --async
zoa run get_resource --jira OSD-123 --namespace kube-system --resource pods --async --wait

# View results
zoa runs --limit 10
zoa runs -o wide              # Full details: dispatched/completed timestamps, log bytes
zoa get <exec-id> --include-output
zoa output <exec-id>
zoa logs <exec-id>
zoa download <exec-id>
zoa download <exec-id> -f /tmp/output.json

# JSON output (for piping)
zoa runs -o json
zoa get <exec-id> -o json

# Audit
zoa audit --limit 20
```

## Global Flags

| Flag | Short | Env Var | Description |
|------|-------|---------|-------------|
| `--api-url` | | `ZOA_API_URL` | ZOA endpoint URL (Function URL, API Gateway, or CNAME) |
| `--output` | `-o` | | Output format: `table` (default), `wide`, `json` |
| `--region` | | `AWS_REGION` | AWS region override for custom CNAME endpoints |
| `--help` | `-h` | | Help for any command |

## `run` Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--jira` | | Jira ticket (required, e.g. ROSAENG-1234) |
| `--namespace` | `-n` | Target namespace |
| `--all-namespaces` | `-A` | All namespaces |
| `--name` | | Resource name |
| `--resource` | | Resource type (for generic actions) |
| `--selector` | `-l` | Label selector |
| `--verbose` | `-v` | Full JSON output (no compact summary) |
| `--param` | | Additional parameters (key=value, repeatable) |
| `--force` | | Bypass write cooldown and concurrency limits |
| `--dry-run` | | Execute the dry-run variant of the action |
| `--execution-mode` | | Override execution class: `sync` or `async` (default: TA's declared class) |
| `--timeout` | | Server-side TA execution timeout (e.g. 60s, 3m; bounded by server max 295s) |
| `--no-wait` | | Return ID immediately, skip output display |
| `--wait` | | Poll until async execution completes (no effect on sync — sync returns inline) |
| `--wait-timeout` | | Max poll duration when `--wait` is active (default 5m) |
| `--wait-poll-interval` | | Poll frequency when `--wait` is active (default 30s) |

## `runs` Filters

All filters are combinable:

| Flag | Short | Description |
|------|-------|-------------|
| `--status` | | Filter by status (dispatched, succeeded, failed, timed_out) |
| `--action` | | Filter by action name |
| `--operator` | | Filter by operator |
| `--target` | `-t` | Filter by target cluster |
| `--scope` | | Filter by scope (kube-api, aws-api) |
| `--type` | | Filter by type (read, write) |
| `--execution-mode` | | Filter by execution class (sync, async) |
| `--since` | | Filter by time window (e.g. 1h, 24h, 7d) |
| `--dry-run` | | Show only dry-run executions |
| `--limit` | | Max results (max 100, default 20) |

## `get` Flags

| Flag | Description |
|------|-------------|
| `--include-output` | Include execution output in display |
| `--include-logs` | Include execution logs |
| `--include-all` | Include both output and logs |
| `--wait` | Poll until execution reaches terminal status (useful to reconnect) |
| `--wait-timeout` | Max poll duration when `--wait` is active (default 5m) |
| `--wait-poll-interval` | Poll frequency when `--wait` is active (default 30s) |

## `audit` Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--action` | | Filter by action name |
| `--operator` | | Filter by operator |
| `--target` | `-t` | Filter by target cluster |
| `--method` | | Filter by HTTP method (GET, POST) |
| `--approval` | | Filter by approval state |
| `--since` | | Filter by time window (e.g. 1h, 24h, 7d) |
| `--limit` | | Max results (max 200, default 50) |

## `download` Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Destination file path (default: `zoa-<id>-<artifact>.json`) |
| `--artifact` | | Which artifact to download: `output` (default), `logs` |

## Shell Completion

```bash
source <(zoa completion zsh)     # zsh
source <(zoa completion bash)    # bash
zoa completion fish | source     # fish
```
