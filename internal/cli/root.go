package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/client"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/output"
	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/version"
)

type GlobalOptions struct {
	APIURL       string
	OutputFormat output.Format
}

func NewRootCommand() *cobra.Command {
	opts := &GlobalOptions{}

	cmd := &cobra.Command{
		Use:   "zoa",
		Short: "ZOA — Zero Operator Access CLI",
		Long: `ZOA executes audited Trusted Actions against target clusters via the Platform API.

All operations are authenticated with AWS SigV4 using your current credentials.
Set API_URL to your regional API Gateway endpoint.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			name := cmd.Name()
			if name == "version" || name == "completion" || name == "help" {
				return nil
			}
			if cmd.CalledAs() == "__complete" || cmd.CalledAs() == "__completeNoDesc" {
				return nil
			}
			if opts.APIURL == "" {
				return fmt.Errorf("API_URL not set\n\n  export API_URL=\"https://<id>.execute-api.<region>.amazonaws.com/prod\"")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.APIURL, "api-url", os.Getenv("API_URL"), "Platform API Gateway URL (env: API_URL)")
	var outputFlag string
	cmd.PersistentFlags().StringVarP(&outputFlag, "output", "o", "table", "Output format: table, json")
	cobra.OnInitialize(func() {
		opts.OutputFormat = output.ParseFormat(outputFlag)
	})

	cmd.AddCommand(
		newRunCommand(opts),
		newGetCommand(opts),
		newLogsCommand(opts),
		newRunsCommand(opts),
		newActionsCommand(opts),
		newDescribeCommand(opts),
		newAuditCommand(opts),
		newVersionCommand(opts),
		newCompletionCommand(),
	)

	return cmd
}

func newClient(opts *GlobalOptions) (*client.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return client.New(opts.APIURL, cfg.Credentials)
}

func newVersionCommand(global *GlobalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			if global.OutputFormat == output.FormatJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(info)
				return
			}
			fmt.Println(info.String())
		},
	}
}

func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts (usage: zoa completion [bash|zsh|fish])",
		Long: `Generate shell completion scripts for zoa.

To load completions:

  # bash (current session)
  source <(zoa completion bash)

  # bash (persistent — add to ~/.bashrc)
  zoa completion bash > /etc/bash_completion.d/zoa

  # zsh (current session)
  source <(zoa completion zsh)

  # zsh (persistent — place in $fpath)
  zoa completion zsh > "${fpath[1]}/_zoa"

  # fish
  zoa completion fish | source`,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			default:
				return fmt.Errorf("unsupported shell: %s (valid: bash, zsh, fish)", args[0])
			}
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{"bash", "zsh", "fish"}, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}
