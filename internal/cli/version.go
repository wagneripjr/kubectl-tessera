package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		Example: "  # Print version, commit, and build date\n" +
			"  kubectl tessera version",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"kubectl-tessera %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
			return err
		},
	}
}
