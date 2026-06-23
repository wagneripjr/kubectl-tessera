package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/wagneripjr/kubectl-tessera/internal/kubeconfig"
	"github.com/wagneripjr/kubectl-tessera/internal/labels"
	"github.com/wagneripjr/kubectl-tessera/internal/output"
	"github.com/wagneripjr/kubectl-tessera/internal/preflight"
	"github.com/wagneripjr/kubectl-tessera/internal/rbac"
	"github.com/wagneripjr/kubectl-tessera/internal/scope"
	"github.com/wagneripjr/kubectl-tessera/internal/session"
	"github.com/wagneripjr/kubectl-tessera/internal/subshell"
	"github.com/wagneripjr/kubectl-tessera/internal/token"
)

func (o *mintOptions) run(cmd *cobra.Command) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	if err := o.validate(); err != nil {
		return err
	}

	interactive := !o.printKubeconfig && !o.dryRun && o.output != output.FormatJSON

	restCfg, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("resolving REST config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}

	if err := token.RequireSupported(cs.Discovery()); err != nil {
		return err
	}
	mapper, err := o.configFlags.ToRESTMapper()
	if err != nil {
		return fmt.Errorf("building REST mapper: %w", err)
	}

	nsScope, err := o.resolveNamespaceScope()
	if err != nil {
		return err
	}

	clusterWide := o.clusterScoped || nsScope.all
	owner := sanitizeDNS1123(resolveOwner(ctx, cs))

	resolution, err := scope.Resolve(scope.Request{
		Verbs:         o.verbs,
		Resources:     o.resources,
		ResourceNames: o.resourceNames,
		APIGroup:      o.apiGroup,
		ClusterScoped: o.clusterScoped,
	}, mapper)
	if err != nil {
		return fmt.Errorf("resolving scope: %w", err)
	}

	pf, err := preflight.Check(ctx, cs, o.buildGrantAttributes(resolution.Resources, clusterWide, nsScope.namespaces))
	if err != nil {
		return fmt.Errorf("pre-flight authorization: %w", err)
	}
	if !pf.AllAllowed() {
		preflight.RenderTable(stderr, pf)
		return fmt.Errorf("mint refused: requested scope exceeds your permissions")
	}

	sessionID := newSessionID()
	name := baseName(owner, sessionID)
	expires := time.Now().UTC().Add(o.ttl)
	objLabels := labels.Set(owner, sessionID)
	objAnnotations := map[string]string{labels.ExpiresAtKey: expires.Format(time.RFC3339)}

	saNamespace := "default"
	if !clusterWide {
		saNamespace = nsScope.namespaces[0]
	}
	bindNamespaces := nsScope.namespaces

	createPF, err := preflight.Check(ctx, cs, buildCreateAttributes(clusterWide, saNamespace, bindNamespaces))
	if err != nil {
		return fmt.Errorf("pre-flight create authorization: %w", err)
	}
	if !createPF.AllAllowed() {
		preflight.RenderMissingCreate(stderr, createPF.Denied)
		return fmt.Errorf("mint refused: you lack permission to create the required RBAC objects")
	}

	nsLabel := strings.Join(nsScope.namespaces, ",")
	if nsScope.all {
		nsLabel = "*"
	}
	audit := func(effExpires time.Time) string {
		return auditLine(sessionID, owner, o.verbs, o.resources, o.resourceNames, nsLabel, o.ttl, effExpires, o.clusterScoped)
	}

	if nsScope.all {
		_, _ = fmt.Fprintln(stderr, "tessera: warning: this session grants the requested scope across ALL namespaces (cluster-wide), including namespaces created later")
	}

	if o.isWildcardResource() {
		_, _ = fmt.Fprintf(stderr, "tessera: warning: this session grants ALL resources (apiGroups=*, resources=*) for verbs [%s] — admin-equivalent for those verbs\n", strings.Join(o.verbs, ","))
	}

	var descNamespaces []string
	switch {
	case o.clusterScoped:

	case nsScope.all:
		descNamespaces = []string{"*"}
	default:
		descNamespaces = nsScope.namespaces
	}

	if o.dryRun {

		intended := session.Descriptor{
			SessionID:      sessionID,
			Owner:          owner,
			Scope:          scopeSummary(o.verbs, o.resources),
			ExpiresAt:      expires.Format(time.RFC3339),
			Namespaces:     descNamespaces,
			CreatedObjects: createdObjectNames(name, clusterWide),
		}
		if o.output == output.FormatJSON {
			if err := output.JSON(stdout, intended); err != nil {
				return err
			}
		} else {
			_, _ = fmt.Fprintln(stdout, "tessera: dry-run — would create (nothing was created):")
			for _, obj := range intended.CreatedObjects {
				_, _ = fmt.Fprintf(stdout, "  %s\n", obj)
			}
			if len(intended.Namespaces) > 0 {
				_, _ = fmt.Fprintf(stdout, "  namespaces: %s\n", strings.Join(intended.Namespaces, ", "))
			}
			_, _ = fmt.Fprintf(stdout, "  scope: %s  expires: %s\n", intended.Scope, intended.ExpiresAt)
		}
		_, _ = fmt.Fprintln(stderr, audit(expires))
		return nil
	}

	created, err := rbac.Create(ctx, cs, rbac.Spec{
		BaseName:      name,
		Namespace:     saNamespace,
		Namespaces:    bindNamespaces,
		ClusterScoped: clusterWide,
		Rules:         resolution.Rules,
		Labels:        objLabels,
		Annotations:   objAnnotations,
	})
	if err != nil {
		return fmt.Errorf("creating RBAC objects: %w", err)
	}

	minted, err := token.Mint(ctx, cs, created.ServiceAccountNamespace, created.ServiceAccountName, o.ttl)
	if err != nil {
		_ = rbac.Rollback(ctx, cs, created)
		return fmt.Errorf("minting token: %w", err)
	}
	if minted.Floored {
		_, _ = fmt.Fprintf(stderr, "tessera: warning: requested ttl %s floored to cluster minimum %s\n",
			o.ttl, token.MinTTL)
	}
	if minted.Clamped {
		_, _ = fmt.Fprintf(stderr, "tessera: warning: requested ttl %s clamped by cluster to %s\n",
			o.ttl, minted.ExpirationTimestamp.UTC().Format(time.RFC3339))
	}

	kcNamespace := ""
	if !clusterWide && len(nsScope.namespaces) == 1 {
		kcNamespace = nsScope.namespaces[0]
	}
	kcfg := kubeconfig.Build(kubeconfig.Params{
		RESTConfig: restCfg,
		Token:      minted.Token,
		Namespace:  kcNamespace,
		SessionID:  sessionID,
	})
	path, err := kubeconfig.Write(kcfg, sessionID)
	if err != nil {
		_ = rbac.Rollback(ctx, cs, created)
		return fmt.Errorf("writing kubeconfig: %w", err)
	}

	_, _ = fmt.Fprintln(stderr, audit(minted.ExpirationTimestamp.UTC()))

	if interactive {

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}

		_, _ = fmt.Fprintf(stderr, "tessera: kubeconfig=%s\n", path)

		exitCode, err := subshell.Run(ctx, subshell.Config{
			Shell:  shell,
			Env:    append(os.Environ(), "KUBECONFIG="+path),
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Cleanup: func(cleanupCtx context.Context) {
				if rbErr := rbac.Rollback(cleanupCtx, cs, created); rbErr != nil {
					_, _ = fmt.Fprintf(stderr, "tessera: warning: cleanup of session %s incomplete: %v\n", sessionID, rbErr)
				}
				if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
					_, _ = fmt.Fprintf(stderr, "tessera: warning: removing kubeconfig %s: %v\n", path, rmErr)
				}
			},
		})
		if err != nil {
			return fmt.Errorf("running subshell: %w", err)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	}

	if o.output == output.FormatJSON {
		desc := session.Descriptor{
			SessionID:      sessionID,
			Owner:          owner,
			Scope:          scopeSummary(o.verbs, o.resources),
			ExpiresAt:      minted.ExpirationTimestamp.UTC().Format(time.RFC3339),
			Namespaces:     descNamespaces,
			KubeconfigPath: path,
			CreatedObjects: createdObjectNames(name, clusterWide),
		}
		return output.JSON(stdout, desc)
	}

	_, _ = fmt.Fprintln(stdout, path)
	return nil
}

func (o *mintOptions) isWildcardResource() bool {
	return len(o.resources) == 1 && o.resources[0] == "*"
}

func (o *mintOptions) validate() error {
	if len(o.resources) == 0 {
		return fmt.Errorf("--resource is required")
	}
	if o.isWildcardResource() {
		if len(o.resourceNames) > 0 {
			return fmt.Errorf("--resource '*' cannot be combined with --resource-name")
		}
		if o.apiGroup != "" {
			return fmt.Errorf("--resource '*' cannot be combined with --api-group; the wildcard already spans all groups")
		}
	}
	if o.printKubeconfig && o.exec {
		return fmt.Errorf("--print-kubeconfig and --exec are mutually exclusive")
	}
	if err := output.Validate(o.output); err != nil {
		return err
	}

	if o.output == output.FormatJSON && o.exec {
		return fmt.Errorf("--exec and -o json are mutually exclusive")
	}
	return nil
}

func buildCreateAttributes(clusterWide bool, saNamespace string, bindNamespaces []string) []preflight.Attribute {
	const rbacGroup = "rbac.authorization.k8s.io"
	if clusterWide {
		return []preflight.Attribute{
			{Verb: "create", Group: "", Resource: "serviceaccounts", Namespace: saNamespace},
			{Verb: "create", Group: rbacGroup, Resource: "clusterroles"},
			{Verb: "create", Group: rbacGroup, Resource: "clusterrolebindings"},
		}
	}
	attrs := []preflight.Attribute{{Verb: "create", Group: "", Resource: "serviceaccounts", Namespace: saNamespace}}
	for _, ns := range bindNamespaces {
		attrs = append(
			attrs,
			preflight.Attribute{Verb: "create", Group: rbacGroup, Resource: "roles", Namespace: ns},
			preflight.Attribute{Verb: "create", Group: rbacGroup, Resource: "rolebindings", Namespace: ns},
		)
	}
	return attrs
}

func scopeSummary(verbs, resources []string) string {
	return strings.Join(verbs, ",") + ":" + strings.Join(resources, ",")
}

func createdObjectNames(name string, clusterScoped bool) []string {
	if clusterScoped {
		return []string{"serviceaccount/" + name, "clusterrole/" + name, "clusterrolebinding/" + name}
	}
	return []string{"serviceaccount/" + name, "role/" + name, "rolebinding/" + name}
}

type namespaceScope struct {
	namespaces []string
	all        bool
}

func (o *mintOptions) resolveNamespaceScope() (namespaceScope, error) {
	raw := ""
	if o.configFlags.Namespace != nil {
		raw = strings.TrimSpace(*o.configFlags.Namespace)
	}
	wildcard := o.allNamespaces || raw == "*"

	if o.clusterScoped {
		if wildcard {
			return namespaceScope{}, fmt.Errorf("--cluster-scoped cannot be combined with --all-namespaces (-A): --cluster-scoped is for cluster-scoped resource types, -A is for namespaced resources in every namespace")
		}
		if raw != "" {
			return namespaceScope{}, fmt.Errorf("--namespace cannot be used with --cluster-scoped; omit -n")
		}
		return namespaceScope{}, nil
	}
	if wildcard {
		if raw != "" && raw != "*" {
			return namespaceScope{}, fmt.Errorf("--all-namespaces (-A) cannot be combined with an explicit --namespace list")
		}
		return namespaceScope{all: true}, nil
	}
	if raw == "" {
		if ns, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace(); err == nil && ns != "" {
			return namespaceScope{namespaces: []string{ns}}, nil
		}
		return namespaceScope{namespaces: []string{"default"}}, nil
	}
	parts, err := parseNamespaceList(raw)
	if err != nil {
		return namespaceScope{}, err
	}
	return namespaceScope{namespaces: parts}, nil
}

func parseNamespaceList(raw string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("empty namespace in --namespace %q", raw)
		}
		if p == "*" {
			return nil, fmt.Errorf("wildcard '*' cannot be mixed into a --namespace list; use -A/--all-namespaces on its own")
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func (o *mintOptions) buildGrantAttributes(resources []scope.ResolvedResource, clusterWide bool, namespaces []string) []preflight.Attribute {
	if clusterWide {
		return buildAttributes(resources, o.verbs, o.resourceNames, "")
	}
	var attrs []preflight.Attribute
	for _, ns := range namespaces {
		attrs = append(attrs, buildAttributes(resources, o.verbs, o.resourceNames, ns)...)
	}
	return attrs
}

func resolveOwner(ctx context.Context, cs kubernetes.Interface) string {
	ssr, err := cs.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err == nil && ssr.Status.UserInfo.Username != "" {
		return ssr.Status.UserInfo.Username
	}
	return "unknown"
}

func buildAttributes(resources []scope.ResolvedResource, verbs, names []string, namespace string) []preflight.Attribute {
	var attrs []preflight.Attribute
	for _, r := range resources {
		ns := namespace
		if !r.Namespaced {
			ns = ""
		}
		if len(names) == 0 {
			for _, v := range verbs {
				attrs = append(attrs, preflight.Attribute{Verb: v, Group: r.Group, Resource: r.Resource, Namespace: ns})
			}
			continue
		}
		for _, v := range verbs {
			for _, n := range names {
				attrs = append(attrs, preflight.Attribute{Verb: v, Group: r.Group, Resource: r.Resource, Namespace: ns, Name: n})
			}
		}
	}
	return attrs
}

func auditLine(sessionID, owner string, verbs, resources, names []string, namespace string, ttl time.Duration, expires time.Time, clusterScoped bool) string {
	ns := namespace
	if clusterScoped {
		ns = "cluster"
	}
	scopeStr := strings.Join(verbs, ",") + ":" + strings.Join(resources, ",")
	if len(names) > 0 {
		scopeStr += ":" + strings.Join(names, ",")
	}
	return fmt.Sprintf("tessera: session-id=%s owner=%s scope=%s ns=%s ttl=%s expires=%s cluster-scoped=%t",
		sessionID, owner, scopeStr, ns, ttl, expires.Format(time.RFC3339), clusterScoped)
}
