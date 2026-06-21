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

// newGcCmd builds the `gc` subcommand: delete expired managed objects across all
// namespaces (FR-011). It is the safety net for --print-kubeconfig sessions (no
// foreground trap) and for sessions killed with SIGKILL (which bypasses the trap).
// Idempotent and cron-safe — the shipped CronJob (deploy/gc-cronjob.yaml) runs it on
// a schedule. The shared ConfigFlags (cf) resolves the cluster exactly like kubectl.
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
			// Summary on the diagnostic stream; stdout stays clean (NFR-008). Printed
			// before surfacing any error so a partial sweep still reports its progress.
			_, _ = fmt.Fprintf(stderr,
				"tessera: gc swept %d expired, kept %d unexpired, skipped %d unparseable (scanned %d)\n",
				res.Deleted, res.SkippedFresh, res.SkippedUnparseable, res.Scanned)
			if sweepErr != nil {
				// Non-zero so a cron run surfaces the failure; the orphans it could not
				// reclaim are picked up next sweep (idempotent).
				return fmt.Errorf("gc: %w", sweepErr)
			}
			return nil // exit 0 even on an empty sweep — cron-safe
		},
	}
}
