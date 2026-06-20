package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/wagneripjr/kubectl-tessera/internal/kubeconfig"
	"github.com/wagneripjr/kubectl-tessera/internal/labels"
	"github.com/wagneripjr/kubectl-tessera/internal/preflight"
	"github.com/wagneripjr/kubectl-tessera/internal/rbac"
	"github.com/wagneripjr/kubectl-tessera/internal/scope"
	"github.com/wagneripjr/kubectl-tessera/internal/token"
)

// run orchestrates the full mint flow (FR-001): resolve scope (FR-002), SSAR
// pre-flight (FR-003), create RBAC as the invoking user (FR-004) with rollback
// (FR-005), mint the token (FR-006), write a 0600 kubeconfig (FR-007), emit the
// audit line (FR-014) and print the path (FR-008). All diagnostics go to stderr;
// stdout carries only the kubeconfig path.
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
	if o.exec {
		return fmt.Errorf("mint: --exec is not implemented yet; use --print-kubeconfig")
	}
	if !o.printKubeconfig && !o.dryRun {
		return fmt.Errorf("mint: specify --print-kubeconfig (--exec is not implemented yet)")
	}

	restCfg, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("resolving REST config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}
	mapper, err := o.configFlags.ToRESTMapper()
	if err != nil {
		return fmt.Errorf("building REST mapper: %w", err)
	}

	targetNS := o.targetNamespace()
	owner := sanitizeDNS1123(resolveOwner(ctx, cs))

	resolution, err := scope.Resolve(scope.Request{
		Verbs:         o.verbs,
		Resources:     o.resources,
		ResourceNames: o.resourceNames,
		APIGroup:      o.apiGroup,
		ClusterScoped: o.clusterScoped,
		Namespace:     targetNS,
	}, mapper)
	if err != nil {
		return fmt.Errorf("resolving scope: %w", err)
	}

	pf, err := preflight.Check(ctx, cs, buildAttributes(resolution.Resources, o.verbs, o.resourceNames, targetNS))
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

	saNamespace := targetNS
	if o.clusterScoped {
		saNamespace = "default"
	}

	audit := func(effExpires time.Time) string {
		return auditLine(sessionID, owner, o.verbs, o.resources, o.resourceNames, targetNS, o.ttl, effExpires, o.clusterScoped)
	}

	if o.dryRun {
		fmt.Fprintf(stderr, "tessera: dry-run: would create service account, %s and binding %q in %q with %d rule(s)\n",
			roleKind(o.clusterScoped), name, saNamespace, len(resolution.Rules))
		fmt.Fprintln(stderr, audit(expires))
		return nil
	}

	created, err := rbac.Create(ctx, cs, rbac.Spec{
		BaseName:      name,
		Namespace:     saNamespace,
		ClusterScoped: o.clusterScoped,
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
	if minted.Clamped {
		fmt.Fprintf(stderr, "tessera: warning: requested ttl %s clamped by cluster to %s\n",
			o.ttl, minted.ExpirationTimestamp.UTC().Format(time.RFC3339))
	}

	kcfg := kubeconfig.Build(kubeconfig.Params{
		RESTConfig: restCfg,
		Token:      minted.Token,
		Namespace:  saNamespace,
		SessionID:  sessionID,
	})
	path, err := kubeconfig.Write(kcfg, sessionID)
	if err != nil {
		_ = rbac.Rollback(ctx, cs, created)
		return fmt.Errorf("writing kubeconfig: %w", err)
	}

	fmt.Fprintln(stderr, audit(minted.ExpirationTimestamp.UTC()))
	if o.printKubeconfig {
		fmt.Fprintln(stdout, path)
	}
	return nil
}

// validate checks flag combinations that don't depend on the cluster.
func (o *mintOptions) validate() error {
	if len(o.resources) == 0 {
		return fmt.Errorf("--resource is required")
	}
	if o.printKubeconfig && o.exec {
		return fmt.Errorf("--print-kubeconfig and --exec are mutually exclusive")
	}
	return nil
}

// targetNamespace resolves the namespace for namespaced scope. Cluster-scoped
// sessions have no target namespace.
func (o *mintOptions) targetNamespace() string {
	if o.clusterScoped {
		return ""
	}
	if o.configFlags.Namespace != nil && *o.configFlags.Namespace != "" {
		return *o.configFlags.Namespace
	}
	if ns, _, err := o.configFlags.ToRawKubeConfigLoader().Namespace(); err == nil && ns != "" {
		return ns
	}
	return "default"
}

// resolveOwner asks the API server who the invoking user is via SelfSubjectReview,
// the authoritative source across token/cert/OIDC auth. Falls back to "unknown".
func resolveOwner(ctx context.Context, cs kubernetes.Interface) string {
	ssr, err := cs.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err == nil && ssr.Status.UserInfo.Username != "" {
		return ssr.Status.UserInfo.Username
	}
	return "unknown"
}

// buildAttributes expands resolved resources × verbs × optional names into SSAR
// attributes, clearing the namespace for cluster-scoped resources.
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

// auditLine renders the single stderr audit record. It MUST contain "session-id=<id>"
// — the acceptance protocol driver parses the session id from it (FR-014).
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

func roleKind(clusterScoped bool) string {
	if clusterScoped {
		return "cluster role"
	}
	return "role"
}
