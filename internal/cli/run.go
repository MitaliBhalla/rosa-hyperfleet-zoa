package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

type runOptions struct {
	namespace      string
	allNS          bool
	selector       string
	verbose        bool
	name           string
	resource       string
	jira           string
	force          bool
	dryRun         bool
	noWait         bool
	wait           bool
	waitTimeout    time.Duration
	pollInterval   time.Duration
	timeout        time.Duration
	executionMode  string
	params         []string
}

func newRunCommand(global *GlobalOptions) *cobra.Command {
	opts := &runOptions{}

	cmd := &cobra.Command{
		Use:   "run <action> [flags]",
		Short: "Execute a Trusted Action",
		Long: `Dispatch a Trusted Action against a target cluster and wait for completion.

The target cluster is determined by the Lambda endpoint (ZOA_API_URL). Each Lambda
serves exactly one EKS cluster — set ZOA_API_URL to point at the right one.

The result (stdout of the TA script) is printed to stdout on success.
On failure, logs are printed to stderr. Use --no-wait to fire and forget.`,
		Example: `  # Basic read action
  zoa run get_resource --resource pods -n cert-manager --jira ROSAENG-1234

  # All namespaces with verbose JSON, piped to jq
  zoa run get_resource --resource pods -A -v --jira ROSAENG-1234 | jq '.[] | select(.status != "Running")'

  # Write action
  zoa run rollout_restart --resource deployment -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234

  # Write action with force (bypass cooldown and concurrency limits)
  zoa run rollout_restart --resource deployment -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234 --force

  # Dry run (executes the dry_run_action variant, no side effects)
  zoa run delete_pod -n cert-manager --name cert-manager-webhook-abc123 --jira ROSAENG-1234 --dry-run

  # Destructive action
  zoa run delete_pod -n cert-manager --name cert-manager-webhook-abc123 --jira ROSAENG-1234

  # Fire and forget (don't wait for completion)
  zoa run get_resource --resource nodes --jira ROSAENG-1234 --no-wait -o json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("missing action name\n\n  Usage: zoa run <action> --jira <ticket>\n  Run 'zoa actions' to see available actions")
			}
			if len(args) > 1 {
			return fmt.Errorf("unexpected argument %q — action parameters must be passed as flags\n\n  Example: zoa run %s --resource %s --jira <ticket>\n  Run 'zoa describe %s' to see available parameters",
				args[1], args[0], args[1], args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd.Context(), global, opts, args[0])
		},
	}

	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVarP(&opts.allNS, "all-namespaces", "A", false, "All namespaces")
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Label selector")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Full JSON output from the action (no compact summary)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Resource name")
	cmd.Flags().StringVar(&opts.resource, "resource", "", "Resource type (for generic actions)")
	cmd.Flags().StringVar(&opts.jira, "jira", "", "Jira ticket (required, e.g. ROSAENG-1234)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Bypass write cooldown and concurrency limits")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Execute dry-run variant of the action")
	cmd.Flags().BoolVar(&opts.noWait, "no-wait", false, "Return ID immediately, skip output display (sync: skips output fetch; async: already default)")
	cmd.Flags().BoolVar(&opts.wait, "wait", false, "Poll until async execution completes (no effect on sync — sync returns inline)")
	cmd.Flags().DurationVar(&opts.waitTimeout, "wait-timeout", 5*time.Minute, "Max poll duration when --wait is active")
	cmd.Flags().DurationVar(&opts.pollInterval, "wait-poll-interval", 30*time.Second, "Poll frequency when --wait is active")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 0, "Server-side TA execution timeout (e.g. 60s, 3m; bounded by server max 295s)")
	cmd.Flags().StringVar(&opts.executionMode, "execution-mode", "", "Override execution class: 'sync' or 'async' (default: TA's declared class)")
	cmd.Flags().StringArrayVar(&opts.params, "param", nil, "Additional parameters (key=value, repeatable)")

	_ = cmd.MarkFlagRequired("jira")

	return cmd
}

func runAction(ctx context.Context, global *GlobalOptions, opts *runOptions, action string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	params := buildParams(opts)

	req := &client.DispatchRequest{
		Jira:           opts.jira,
		Params:         params,
		Force:          opts.force,
		DryRun:         opts.dryRun,
		ExecutionMode:  opts.executionMode,
		TimeoutSeconds: int(opts.timeout.Seconds()),
	}

	resp, err := c.Dispatch(ctx, action, req)
	if err != nil {
		return fmt.Errorf("dispatch failed: %w", err)
	}

	tags := formatTags(action, resp.ExecutedAction, opts.force, opts.dryRun)

	isAsync := resp.ExecutionMode == "async"
	fireAndForget := opts.noWait || (isAsync && !opts.wait)

	if fireAndForget {
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, resp)
		}
		fmt.Fprintf(os.Stderr, "✓ %s [%s]%s\n", resp.ID, resp.Target, tags)
		if isAsync {
			fmt.Fprintf(os.Stderr, "  dispatched (async) — use 'zoa get %s' or --wait to track\n", resp.ID)
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "✓ %s [%s]%s\n", resp.ID, resp.Target, tags)

	if isTerminalStatus(resp.Status) {
		if resp.Output.String() != "" || resp.Logs != "" {
			exec := &client.Execution{
				ID:            resp.ID,
				Status:        resp.Status,
				ExecutionMode: resp.ExecutionMode,
				DurationMs:    resp.DurationMs,
				Output:        resp.Output,
				Logs:          resp.Logs,
			}
			return printRunResult(global, exec)
		}
		include := "output"
		if resp.Status != "succeeded" {
			include = "logs"
		}
		full, err := c.GetExecution(ctx, resp.ID, include)
		if err != nil {
			full = &client.Execution{ID: resp.ID, Status: resp.Status}
		}
		return printRunResult(global, full)
	}

	// Only async with --wait reaches here; sync always returns terminal status inline.
	if !isAsync {
		return fmt.Errorf("unexpected non-terminal status %q from sync execution %s (server bug)", resp.Status, resp.ID)
	}

	result, err := poll(ctx, c, resp.ID, pollConfig{
		interval: opts.pollInterval,
		timeout:  opts.waitTimeout,
	})
	if err != nil {
		return err
	}

	return printRunResult(global, result)
}

func isTerminalStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "timed_out", "rejected":
		return true
	}
	return false
}

type pollConfig struct {
	interval time.Duration
	timeout  time.Duration
}

func poll(ctx context.Context, c APIClient, id string, cfg pollConfig) (*client.Execution, error) {
	start := time.Now()

	for {
		exec, err := c.GetExecution(ctx, id, "")
		if err != nil {
			return nil, fmt.Errorf("polling execution: %w", err)
		}

		if isTerminalStatus(exec.Status) {
			fmt.Fprintf(os.Stderr, "\r\033[K")
			include := "output"
			if exec.Status != "succeeded" {
				include = "logs"
			}
			full, err := c.GetExecution(ctx, id, include)
			if err == nil {
				return full, nil
			}
			return exec, nil
		}

		elapsed := time.Since(start)
		if elapsed >= cfg.timeout {
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return exec, fmt.Errorf("timed out after %s (status: %s)", elapsed.Round(time.Second), exec.Status)
		}

		if output.IsTerminal() {
			fmt.Fprintf(os.Stderr, "\r\033[K⠋ %s (%s)", exec.Status, elapsed.Round(time.Second))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.interval):
		}
	}
}

func printRunResult(global *GlobalOptions, exec *client.Execution) error {
	timing := fmt.Sprintf("%s · mode=%s", output.FormatDuration(exec.DurationMs), exec.ExecutionMode)

	if exec.Status == "succeeded" {
		fmt.Fprintf(os.Stderr, "✓ %s\n", timing)

		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, exec)
		}
		if exec.Output.String() != "" {
			fmt.Fprintf(os.Stderr, "---\n")
		}
		output.PrintTAOutput(os.Stdout, exec.Output.String())
		return nil
	}

	// Failed execution
	fmt.Fprintf(os.Stderr, "✗ %s · %s\n", exec.Status, timing)
	if exec.Logs != "" {
		fmt.Fprint(os.Stderr, exec.Logs)
	}
	return fmt.Errorf("execution %s: %s", exec.ID, exec.Status)
}

func buildParams(opts *runOptions) map[string]string {
	params := make(map[string]string)

	if opts.namespace != "" {
		params["namespace"] = opts.namespace
	}
	if opts.allNS {
		params["all_namespaces"] = "true"
	}
	if opts.selector != "" {
		params["label_selector"] = opts.selector
	}
	if opts.name != "" {
		params["name"] = opts.name
	}
	if opts.resource != "" {
		params["resource"] = opts.resource
	}
	if opts.verbose {
		params["verbose"] = "true"
	}

	for _, p := range opts.params {
		key, val, ok := strings.Cut(p, "=")
		if ok {
			if _, exists := params[key]; !exists {
				params[key] = val
			}
		}
	}

	if len(params) == 0 {
		return nil
	}
	return params
}

func formatTags(action, executedAction string, force, dryRun bool) string {
	var parts []string
	if dryRun && executedAction != "" {
		parts = append(parts, fmt.Sprintf("dry-run:%s→%s", action, executedAction))
	}
	if force {
		parts = append(parts, "forced")
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, ", ") + "]"
}
