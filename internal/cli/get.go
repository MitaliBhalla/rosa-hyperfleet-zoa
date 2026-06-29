package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newGetCommand(global *GlobalOptions) *cobra.Command {
	var includeOutput, includeLogs, includeAll bool

	cmd := &cobra.Command{
		Use:   "get <execution-id>",
		Short: "Get execution details",
		Example: `  # Get execution status and metadata
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d

  # Include the TA output
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --include-output

  # Include execution logs (stderr/debug logs)
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --include-logs

  # Include everything (output + logs)
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d --include-all

  # JSON output for scripting
  zoa get fa65418c-f4eb-4f5c-8314-baaeb695ba7d -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if includeAll {
				includeOutput = true
				includeLogs = true
			}
			return getExecution(cmd.Context(), global, args[0], includeOutput, includeLogs)
		},
	}

	cmd.Flags().BoolVar(&includeOutput, "include-output", false, "Include execution output")
	cmd.Flags().BoolVar(&includeLogs, "include-logs", false, "Include execution logs")
	cmd.Flags().BoolVar(&includeAll, "include-all", false, "Include output + logs")

	return cmd
}

func getExecution(ctx context.Context, global *GlobalOptions, id string, includeOutput, includeLogs bool) error {
	c, err := newClient(global)
	if err != nil {
		return err
	}

	include := ""
	if includeOutput && includeLogs {
		include = "output,logs"
	} else if includeOutput {
		include = "output"
	} else if includeLogs {
		include = "logs"
	}

	exec, err := c.GetExecution(ctx, id, include)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, exec)
	}

	// Human-readable key-value output
	fmt.Printf("ID:        %s\n", exec.ID)
	actionStr := exec.Action
	if exec.DryRun && exec.ExecutedAction != "" {
		actionStr += fmt.Sprintf(" (dry-run → %s)", exec.ExecutedAction)
	}
	fmt.Printf("ACTION:    %s\n", actionStr)
	fmt.Printf("TARGET:    %s\n", exec.TargetCluster)
	fmt.Printf("STATUS:    %s\n", exec.Status)
	fmt.Printf("APPROVAL:  %s\n", output.Dash(exec.ApprovalState))
	fmt.Printf("OUTPUT:    %s\n", output.Dash(exec.OutputStatus))
	fmt.Printf("DRY-RUN:   %s\n", output.FormatBool(exec.DryRun))
	fmt.Printf("FORCE:     %s\n", output.FormatBool(exec.Force))
	fmt.Printf("JIRA:      %s\n", output.Dash(exec.Jira))
	fmt.Printf("OPERATOR:  %s\n", output.Dash(exec.Operator))
	if len(exec.Params) > 0 {
		parts := make([]string, 0, len(exec.Params))
		for k, v := range exec.Params {
			parts = append(parts, k+"="+v)
		}
		fmt.Printf("PARAMS:    %s\n", strings.Join(parts, " "))
	} else {
		fmt.Printf("PARAMS:    -\n")
	}
	fmt.Printf("CREATED:   %s\n", output.FormatTimestamp(exec.CreatedAt))
	fmt.Printf("COMPLETED: %s\n", output.FormatTimestamp(exec.CompletedAt))
	fmt.Printf("DURATION:  %s\n", output.FormatDuration(exec.DurationSeconds))

	if includeOutput && exec.Output.String() != "" {
		fmt.Println("---")
		output.PrintTAOutput(os.Stdout, exec.Output.String())
	}
	if includeLogs && exec.Logs != "" {
		fmt.Println("---")
		fmt.Print(exec.Logs)
	}

	return nil
}
