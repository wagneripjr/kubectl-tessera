package cli

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type mintOptions struct {
	configFlags *genericclioptions.ConfigFlags

	verbs           []string
	resources       []string
	resourceNames   []string
	apiGroup        string
	ttl             time.Duration
	clusterScoped   bool
	allNamespaces   bool
	exec            bool
	printKubeconfig bool
	dryRun          bool
	output          string
}

func Execute(info BuildInfo) {
	if err := newRootCmd(info).Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(info BuildInfo) *cobra.Command {
	cf := genericclioptions.NewConfigFlags(true)
	o := &mintOptions{configFlags: cf}

	cmd := &cobra.Command{
		Use:   "kubectl-tessera",
		Short: "Mint ephemeral, scope-narrowed, TTL-bound Kubernetes credentials",
		Long: "kubectl-tessera mints an ephemeral, scope-narrowed, TTL-bound credential " +
			"for the current cluster, running as the invoking user, with a SelfSubjectAccessReview " +
			"pre-flight and automatic cleanup of the RBAC objects it creates.",
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.run(cmd)
		},
	}

	cf.AddFlags(cmd.PersistentFlags())

	f := cmd.Flags()
	f.StringSliceVar(&o.verbs, "verb", []string{"get", "list", "watch"}, "verbs to grant (comma-separated)")
	f.StringSliceVar(&o.resources, "resource", nil, "resources to scope to (comma-separated; required)")
	f.StringSliceVar(&o.resourceNames, "resource-name", nil, "narrow to named objects (comma-separated)")
	f.StringVar(&o.apiGroup, "api-group", "", "API group, when a resource is ambiguous across groups")
	f.DurationVar(&o.ttl, "ttl", 15*time.Minute, "credential lifetime (Go duration)")

	f.BoolVar(&o.clusterScoped, "cluster-scoped", false, "scope over cluster-scoped resources (ClusterRole/Binding)")

	f.BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "grant the scope in every namespace, including future ones (ClusterRole/Binding)")
	f.BoolVar(&o.exec, "exec", false, "spawn a subshell with KUBECONFIG set; cleanup on exit (default mode)")
	f.BoolVar(&o.printKubeconfig, "print-kubeconfig", false, "print the kubeconfig path to stdout; leave objects for gc")
	f.BoolVar(&o.dryRun, "dry-run", false, "pre-flight and print intended objects; create nothing")
	f.StringVarP(&o.output, "output", "o", "", "output format (e.g. json)")

	cmd.AddCommand(newVersionCmd(info), newGcCmd(cf), newLsCmd(cf))
	return cmd
}
