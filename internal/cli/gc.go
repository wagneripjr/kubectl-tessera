package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// newGcCmd builds the `gc` subcommand: delete expired managed objects across all
// namespaces. Idempotent and cron-safe (FR-011). The shared ConfigFlags (cf) will
// resolve the cluster for the sweep once implemented; unused in the skeleton.
func newGcCmd(_ *genericclioptions.ConfigFlags) *cobra.Command {
	return &cobra.Command{
		Use:          "gc",
		Short:        "Delete expired managed RBAC objects across namespaces",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("gc: %w", errNotImplemented)
		},
	}
}
