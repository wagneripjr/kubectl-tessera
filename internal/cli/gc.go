package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/wagneripjr/kubectl-tessera/internal/gc"
)

func newGcCmd(cf *genericclioptions.ConfigFlags) *cobra.Command {
	return &cobra.Command{
		Use:          "gc",
		Short:        "Delete expired managed RBAC objects across namespaces",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			stderr := cmd.ErrOrStderr()

			restCfg, err := cf.ToRESTConfig()
			if err != nil {
				return fmt.Errorf("resolving REST config: %w", err)
			}
			cs, err := kubernetes.NewForConfig(restCfg)
			if err != nil {
				return fmt.Errorf("building clientset: %w", err)
			}

			res, sweepErr := gc.Sweep(ctx, cs, time.Now().UTC())

			_, _ = fmt.Fprintf(stderr,
				"tessera: gc swept %d expired, kept %d unexpired, skipped %d unparseable (scanned %d)\n",
				res.Deleted, res.SkippedFresh, res.SkippedUnparseable, res.Scanned)
			if sweepErr != nil {
				return fmt.Errorf("gc: %w", sweepErr)
			}
			return nil
		},
	}
}
