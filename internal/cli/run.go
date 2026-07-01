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
	target    string
	namespace string
	allNS     bool
	selector  string
	verbose   bool
	name      string
	resource  string
	jira      string
	force     bool
	dryRun    bool
	noWait    bool
	params    []string
}

func newRunCommand(global *GlobalOptions) *cobra.Command {
	opts := &runOptions{}

	cmd := &cobra.Command{
		Use:   "run <action> [flags]",
		Short: "Execute a Trusted Action",
		Long: `Dispatch a Trusted Action against a target cluster and wait for completion.

The result (stdout of the TA script) is printed to stdout on success.
On failure, logs are printed to stderr. Use --no-wait to fire and forget.`,
		Example: `  # Basic read action
  zoa run get_nodes -t mc-useast1-1 --jira ROSAENG-1234

  # Namespaced with label selector (dedicated flag)
  zoa run get_pods -t mc-useast1-1 -n cert-manager -l app=cert-manager --jira ROSAENG-1234

  # Same as above using generic --param
  zoa run get_pods -t mc-useast1-1 -n cert-manager --param label_selector=app=cert-manager --jira ROSAENG-1234

  # All namespaces with verbose JSON, piped to jq
  zoa run get_pods -t mc-useast1-1 -A -v --jira ROSAENG-1234 | jq '.[] | select(.status != "Running")'

  # Custom parameters (field selector)
  zoa run get_events -t mc-useast1-1 -n cert-manager --param field_selector=reason=BackOff --jira ROSAENG-1234

  # Verbose output (full JSON from the action instead of compact summary)
  zoa run get_deployments -t mc-useast1-1 -n cert-manager -v --jira ROSAENG-1234

  # Generic resource query (any Kubernetes resource type)
  zoa run get_resource -t mc-useast1-1 --resource deployments -A --jira ROSAENG-1234

  # Write action
  zoa run rollout_restart -t mc-useast1-1 -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234

  # Write action with force (bypass cooldown and concurrency limits)
  zoa run rollout_restart -t mc-useast1-1 -n cert-manager --name cert-manager-webhook --jira ROSAENG-1234 --force

  # Dry run (executes the dry_run_action variant, no side effects)
  zoa run delete_pod -t mc-useast1-1 -n cert-manager --name cert-manager-webhook-abc123 --jira ROSAENG-1234 --dry-run

  # Destructive action
  zoa run delete_pod -t mc-useast1-1 -n cert-manager --name cert-manager-webhook-abc123 --jira ROSAENG-1234

  # Fire and forget (don't wait for completion)
  zoa run get_nodes -t mc-useast1-1 --jira ROSAENG-1234 --no-wait -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd.Context(), global, opts, args[0])
		},
	}

	cmd.Flags().StringVarP(&opts.target, "target", "t", "", "Target cluster (required)")
	cmd.Flags().StringVarP(&opts.namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVarP(&opts.allNS, "all-namespaces", "A", false, "All namespaces")
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Label selector")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Full JSON output from the action (no compact summary)")
	cmd.Flags().StringVar(&opts.name, "name", "", "Resource name")
	cmd.Flags().StringVar(&opts.resource, "resource", "", "Resource type (for generic actions)")
	cmd.Flags().StringVar(&opts.jira, "jira", "", "Jira ticket (required, e.g. ROSAENG-1234)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Bypass write cooldown and concurrency limits")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Execute dry-run variant of the action")
	cmd.Flags().BoolVar(&opts.noWait, "no-wait", false, "Don't wait for completion")
	cmd.Flags().StringArrayVar(&opts.params, "param", nil, "Additional parameters (key=value, repeatable)")

	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("jira")

	return cmd
}

func runAction(ctx context.Context, global *GlobalOptions, opts *runOptions, action string) error {
	c, err := newClient(global)
	if err != nil {
		return err
	}

	params := buildParams(opts)

	req := &client.DispatchRequest{
		TargetCluster: opts.target,
		Jira:          opts.jira,
		Params:        params,
		Force:         opts.force,
		DryRun:        opts.dryRun,
	}

	resp, err := c.Dispatch(ctx, action, req)
	if err != nil {
		return fmt.Errorf("dispatch failed: %w", err)
	}

	tags := formatTags(action, resp.ExecutedAction, opts.force, opts.dryRun)

	if opts.noWait {
		if global.OutputFormat == output.FormatJSON {
			return output.JSON(os.Stdout, resp)
		}
		fmt.Fprintf(os.Stderr, "✓ %s%s\n", resp.ID, tags)
		return nil
	}

	fmt.Fprintf(os.Stderr, "✓ %s%s\n", resp.ID, tags)

	result, err := poll(ctx, c, resp.ID)
	if err != nil {
		return err
	}

	return printRunResult(global, result)
}

func poll(ctx context.Context, c *client.Client, id string) (*client.Execution, error) {
	const interval = 5 * time.Second
	const timeout = 120 * time.Second
	start := time.Now()

	for {
		exec, err := c.GetExecution(ctx, id, "")
		if err != nil {
			return nil, fmt.Errorf("polling execution: %w", err)
		}

		switch exec.Status {
		case "succeeded", "failed", "error", "timed_out":
			fmt.Fprintf(os.Stderr, "\r\033[K")
			// Fetch full result with output/logs based on status
			include := ""
			if exec.OutputStatus == "uploaded" {
				if exec.Status == "succeeded" {
					include = "output"
				} else {
					include = "logs"
				}
			}
			if include != "" {
				full, err := c.GetExecution(ctx, id, include)
				if err == nil {
					return full, nil
				}
			}
			return exec, nil
		}

		elapsed := time.Since(start)
		if elapsed >= timeout {
			fmt.Fprintf(os.Stderr, "\r\033[K")
			return exec, fmt.Errorf("timed out after %s (status: %s)", elapsed.Round(time.Second), exec.Status)
		}

		if output.IsTerminal() {
			fmt.Fprintf(os.Stderr, "\r\033[K⠋ %s (%s)", exec.Status, elapsed.Round(time.Second))
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func printRunResult(global *GlobalOptions, exec *client.Execution) error {
	runnerS := 0
	uploadS := 0
	totalS := 0
	if exec.RunnerSeconds != nil {
		runnerS = *exec.RunnerSeconds
	}
	if exec.UploadSeconds != nil {
		uploadS = *exec.UploadSeconds
	}
	if exec.DurationSeconds != nil {
		totalS = *exec.DurationSeconds
	}
	dispatchS := totalS - runnerS - uploadS

	timing := fmt.Sprintf("%ds total · runner %ds · upload %ds · dispatch %ds", totalS, runnerS, uploadS, dispatchS)

	if exec.Status == "succeeded" {
		if exec.OutputStatus == "failed" {
			fmt.Fprintf(os.Stderr, "✓ %s ⚠ output upload failed\n", timing)
		} else {
			fmt.Fprintf(os.Stderr, "✓ %s\n", timing)
		}

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
	if exec.OutputStatus == "failed" {
		fmt.Fprintf(os.Stderr, "✗ %s · %s ⚠ output upload failed\n", exec.Status, timing)
	} else {
		fmt.Fprintf(os.Stderr, "✗ %s · %s\n", exec.Status, timing)
	}
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
