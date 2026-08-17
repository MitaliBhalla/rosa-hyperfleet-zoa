package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newLogsCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logs <execution-id>",
		Short: "Show execution log (raw from S3)",
		Long:  "Retrieve the full execution log. Available even after Job/Pod garbage collection.",
		Example: `  # View logs for a completed execution
  zoa logs fa65418c-f4eb-4f5c-8314-baaeb695ba7d

  # Pipe to less for large outputs
  zoa logs fa65418c-f4eb-4f5c-8314-baaeb695ba7d | less`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showLogs(cmd.Context(), global, args[0])
		},
	}
}

func showLogs(ctx context.Context, global *GlobalOptions, id string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	exec, err := c.GetExecution(ctx, id, "logs")
	if err != nil {
		return err
	}

	if exec.Logs == "" {
		fmt.Fprintln(os.Stderr, "no logs available")
		return nil
	}

	fmt.Print(exec.Logs)
	return nil
}
