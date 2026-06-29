package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
)

func newActionsCommand(global *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "actions [action]",
		Short:   "List available Trusted Actions",
		Aliases: []string{"catalog"},
		Example: `  # List all available Trusted Actions
  zoa actions

  # Show details for a specific action (alias for describe)
  zoa actions get_pods

  # Output as JSON (for scripting)
  zoa actions -o json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return describeAction(cmd.Context(), global, args[0])
			}
			return listActions(cmd.Context(), global)
		},
	}
	return cmd
}

func newDescribeCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <action>",
		Short: "Show Trusted Action details",
		Example: `  # Show action details (parameters, scope, approval requirements)
  zoa describe get_pods

  # Output as JSON
  zoa describe rollout_restart -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return describeAction(cmd.Context(), global, args[0])
		},
	}
}

func listActions(ctx context.Context, global *GlobalOptions) error {
	c, err := newClient(global)
	if err != nil {
		return err
	}

	list, err := c.ListActions(ctx)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, list)
	}

	tw := output.NewTable(os.Stdout)
	fmt.Fprintf(tw, "NAME\tSCOPE\tTYPE\tDESCRIPTION\n")
	for _, a := range list.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Name, a.Scope, a.Type, a.Description)
	}
	return tw.Flush()
}

func describeAction(ctx context.Context, global *GlobalOptions, name string) error {
	c, err := newClient(global)
	if err != nil {
		return err
	}

	action, err := c.GetAction(ctx, name)
	if err != nil {
		return err
	}

	if global.OutputFormat == output.FormatJSON {
		return output.JSON(os.Stdout, action)
	}

	fmt.Printf("NAME:        %s\n", action.Name)
	fmt.Printf("SCOPE:       %s\n", action.Scope)
	fmt.Printf("TYPE:        %s\n", action.Type)
	fmt.Printf("DESCRIPTION: %s\n", action.Description)
	fmt.Printf("APPROVAL:    %s\n", output.Dash(action.Authorization.Approval))

	if action.WriteCooldownSeconds > 0 {
		fmt.Printf("COOLDOWN:    %ds\n", action.WriteCooldownSeconds)
	}
	if action.DryRunAction != "" {
		fmt.Printf("DRY-RUN:     %s\n", action.DryRunAction)
	}

	if len(action.RequiredFields) > 0 {
		fmt.Printf("\nREQUIRED FIELDS:\n")
		for _, f := range action.RequiredFields {
			fmt.Printf("  %s\n", f)
		}
	}

	if len(action.Params) > 0 {
		fmt.Printf("\nPARAMETERS:\n")
		tw := output.NewTable(os.Stdout)
		fmt.Fprintf(tw, "  NAME\tREQUIRED\tDEFAULT\tDESCRIPTION\n")
		for _, p := range action.Params {
			req := ""
			if p.Required {
				req = "*"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
				p.Name, req, output.Dash(p.Default), strings.TrimSpace(p.Description))
		}
		tw.Flush()
	} else {
		fmt.Printf("\nPARAMETERS:  (none)\n")
	}

	return nil
}
