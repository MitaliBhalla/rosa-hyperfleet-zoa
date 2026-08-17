package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newOutputCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "output <execution-id>",
		Short: "Show execution output",
		Long:  "Retrieve the TA output (result data). Available even after Job/Pod garbage collection.",
		Example: `  # View output for a completed execution
  zoa output fa65418c-f4eb-4f5c-8314-baaeb695ba7d

  # Raw JSON output
  zoa output fa65418c-f4eb-4f5c-8314-baaeb695ba7d -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showOutput(cmd.Context(), global, args[0])
		},
	}
}

func showOutput(ctx context.Context, global *GlobalOptions, id string) error {
	c, err := getClient(global)
	if err != nil {
		return err
	}

	exec, err := c.GetExecution(ctx, id, "output")
	if err != nil {
		return err
	}

	if exec.Output == "" {
		fmt.Fprintln(os.Stderr, "no output available")
		return nil
	}

	if global.OutputFormat == output.FormatJSON {
		fmt.Println(string(exec.Output))
		return nil
	}

	output.PrintTAOutput(os.Stdout, string(exec.Output))
	return nil
}
