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
		Example: `  # Read-only interactive shell on pods in the current namespace (default verbs get,list,watch)
  kubectl tessera --resource pods

  # Hand an AI agent a self-contained, auto-expiring read-only kubeconfig for prod
  kubectl tessera --resource pods,deployments,events --namespace prod --ttl 1h --print-kubeconfig

  # Ephemeral cluster-wide reader across every resource type (quote the wildcard)
  kubectl tessera --resource '*' --all-namespaces --print-kubeconfig

  # Scoped write for an incident: edit one named deployment in prod
  kubectl tessera --verb get,list,update,patch --resource deployments --resource-name web --namespace prod --ttl 30m

  # Preview what would be created without creating anything
  kubectl tessera --resource pods --namespace prod --dry-run`,
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "kubectl tessera",
		},
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
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
