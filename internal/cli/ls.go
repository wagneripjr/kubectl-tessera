package cli

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/wagneripjr/kubectl-tessera/internal/output"
	"github.com/wagneripjr/kubectl-tessera/internal/session"
)

func newLsCmd(cf *genericclioptions.ConfigFlags) *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List active tessera sessions",
		Example: "  # List active tessera sessions\n" +
			"  kubectl tessera ls\n\n" +
			"  # Machine-readable output\n" +
			"  kubectl tessera ls -o json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := output.Validate(outputFormat); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			restCfg, err := cf.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("resolving REST config: %w", err)
			}
			cs, err := kubernetes.NewForConfig(restCfg)
			if err != nil {
				return fmt.Errorf("building clientset: %w", err)
			}

			sessions, err := session.List(ctx, cs)
			if err != nil {
				return fmt.Errorf("listing sessions: %w", err)
			}

			stdout := cmd.OutOrStdout()
			if outputFormat == output.FormatJSON {
				return output.JSON(stdout, sessions)
			}
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "SESSION-ID\tOWNER\tSCOPE\tEXPIRES-AT")
			for _, s := range sessions {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.SessionID, s.Owner, s.Scope, s.ExpiresAt)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "output format (e.g. json)")
	return cmd
}
