package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// newLsCmd builds the `ls` subcommand: list active sessions (session-id, owner,
// scope summary, expires-at) from managed-object labels (FR-012). The shared
// ConfigFlags (cf) resolves the cluster once implemented; unused in the skeleton.
func newLsCmd(_ *genericclioptions.ConfigFlags) *cobra.Command {
	return &cobra.Command{
		Use:          "ls",
		Short:        "List active tessera sessions",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("ls: %w", errNotImplemented)
		},
	}
}
