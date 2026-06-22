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

// run orchestrates the full mint flow (FR-001): require a supported server (FR-016),
// resolve scope (FR-002), SSAR pre-flight on the grant (FR-003) and on the create
// permissions tessera needs (FR-016), then — unless previewing (--dry-run, FR-010) —
// create RBAC as the invoking user (FR-004) with rollback (FR-005), mint the token
// (FR-006), write a 0600 kubeconfig (FR-007), emit the audit line (FR-014) and deliver
// the session: a subshell (FR-009), the path on stdout (FR-008), or a JSON descriptor
// (FR-015). All diagnostics/audit go to stderr; stdout carries only the primary output.
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
	// Mode resolution (FR-009): --exec is the default — interactive unless the user
	// asked for --print-kubeconfig, --dry-run, or machine-readable output (-o json,
	// which has nowhere to stream into a subshell). validate() rejects the conflicting
	// combinations (--print-kubeconfig + --exec, -o json + --exec).
	interactive := !o.printKubeconfig && !o.dryRun && o.output != output.FormatJSON

	restCfg, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("resolving REST config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}
	// FR-016: fail clearly on a cluster that predates the TokenRequest API (k8s < 1.24)
	// before doing any work, rather than with an opaque 404 deep in the mint.
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
	// A cluster-wide binding (ClusterRole+ClusterRoleBinding) backs both --cluster-scoped
	// (cluster-scoped resource types) and -A/--all-namespaces (namespaced types, every
	// namespace). The two differ only in scope resolution and the SSAR namespace.
	clusterWide := o.clusterScoped || nsScope.all
	owner := sanitizeDNS1123(resolveOwner(ctx, cs))

	// Scope rules are namespace-independent; namespace consistency is enforced by the
	// flag resolution above and the cluster-scoped check inside Resolve.
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

	// FR-003 grant gate. Cluster-wide grants (--cluster-scoped or -A) are checked once with
	// an empty namespace; an explicit namespace list is checked in each namespace (FR-017).
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

	// The single ServiceAccount lives in the first requested namespace, or "default" for a
	// cluster-wide grant (no namespaced binding to anchor it to).
	saNamespace := "default"
	if !clusterWide {
		saNamespace = nsScope.namespaces[0]
	}
	bindNamespaces := nsScope.namespaces // nil for cluster-wide; ignored by rbac when ClusterScoped

	// FR-016: the scope pre-flight (above) proved the operator may exercise the grant;
	// this proves they may CREATE the RBAC objects that carry it. Without this, a missing
	// create permission surfaces as a raw API 403 mid-creation. Checked as the invoking
	// user (SSAR, never impersonation — NFR-002). For an explicit list, create is checked
	// in each namespace.
	createPF, err := preflight.Check(ctx, cs, buildCreateAttributes(clusterWide, saNamespace, bindNamespaces))
	if err != nil {
		return fmt.Errorf("pre-flight create authorization: %w", err)
	}
	if !createPF.AllAllowed() {
		preflight.RenderMissingCreate(stderr, createPF.Denied)
		return fmt.Errorf("mint refused: you lack permission to create the required RBAC objects")
	}

	// nsLabel is the audit/representation of the session's namespace breadth: a comma list,
	// "*" for all-namespaces, or "cluster" (applied inside auditLine for --cluster-scoped).
	nsLabel := strings.Join(nsScope.namespaces, ",")
	if nsScope.all {
		nsLabel = "*"
	}
	audit := func(effExpires time.Time) string {
		return auditLine(sessionID, owner, o.verbs, o.resources, o.resourceNames, nsLabel, o.ttl, effExpires, o.clusterScoped)
	}

	// FR-018: the all-namespaces grant is the widest scope tessera mints — surface it loudly
	// on the diagnostic stream for both the preview and the real mint.
	if nsScope.all {
		_, _ = fmt.Fprintln(stderr, "tessera: warning: this session grants the requested scope across ALL namespaces (cluster-wide), including namespaces created later")
	}

	// descNamespaces represents the session's namespace breadth in ls/json/dry-run output:
	// the explicit list, ["*"] for all-namespaces, or none for cluster-scoped.
	var descNamespaces []string
	switch {
	case o.clusterScoped:
		// cluster-scoped resource types have no namespace
	case nsScope.all:
		descNamespaces = []string{"*"}
	default:
		descNamespaces = nsScope.namespaces
	}

	if o.dryRun {
		// FR-010: preview the intended object set on the primary output; create nothing.
		// (SSRR discovery + the Incomplete notice is FR-013, deferred — see @manual.)
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

	// Bind the kubeconfig context to the namespace only for a single-namespace session;
	// multi-namespace, all-namespaces and cluster-scoped sessions leave it empty so the
	// operator selects a namespace per command.
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
		// FR-009: spawn ${SHELL:-/bin/bash} with KUBECONFIG pointing at the throwaway
		// file. On subshell exit (or SIGINT/SIGTERM) delete the RBAC object set and
		// remove the kubeconfig file. SIGKILL bypasses the trap; gc reclaims orphans.
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		// Surface the throwaway kubeconfig path on the diagnostic stream (stdout stays
		// the subshell's). NFR-008: stdout hygiene — only --print-kubeconfig writes there.
		_, _ = fmt.Fprintf(stderr, "tessera: kubeconfig=%s\n", path)

		exitCode, err := subshell.Run(ctx, subshell.Config{
			Shell:  shell,
			Env:    append(os.Environ(), "KUBECONFIG="+path),
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
			Cleanup: func(cleanupCtx context.Context) {
				// Best-effort teardown on a fresh context (it must survive the
				// SIGINT/SIGTERM that may have triggered it). Attempt both the RBAC
				// delete and the file removal regardless of either's outcome.
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
			os.Exit(exitCode) // cleanup already ran synchronously inside subshell.Run
		}
		return nil
	}

	// FR-015: machine-readable session descriptor on stdout (session-id, scope, effective
	// expiry, kubeconfig path, created objects). Like --print-kubeconfig, objects are left
	// for gc. Diagnostics/audit stay on stderr.
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

	// --print-kubeconfig: leave the objects for gc and emit only the path on stdout.
	_, _ = fmt.Fprintln(stdout, path)
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
	if err := output.Validate(o.output); err != nil {
		return err
	}
	// -o json is non-interactive output; it cannot coexist with an interactive subshell.
	if o.output == output.FormatJSON && o.exec {
		return fmt.Errorf("--exec and -o json are mutually exclusive")
	}
	return nil
}

// buildCreateAttributes lists the create permissions tessera needs to mint (FR-016): the
// ServiceAccount plus the (Cluster)Role and (Cluster)RoleBinding that carry the grant. A
// cluster-wide grant (--cluster-scoped or -A) needs create on the cluster kinds and the SA
// in saNamespace; an explicit namespace list needs create on roles/rolebindings in EACH
// namespace plus the SA in the first one (FR-017).
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
		attrs = append(attrs,
			preflight.Attribute{Verb: "create", Group: rbacGroup, Resource: "roles", Namespace: ns},
			preflight.Attribute{Verb: "create", Group: rbacGroup, Resource: "rolebindings", Namespace: ns},
		)
	}
	return attrs
}

// scopeSummary renders the requested grant the same way internal/session summarizes a
// Role's rules ("verbs:resources"), so a session reads identically in mint -o json,
// dry-run, and ls.
func scopeSummary(verbs, resources []string) string {
	return strings.Join(verbs, ",") + ":" + strings.Join(resources, ",")
}

// createdObjectNames lists the object set a mint creates, all sharing one base name.
func createdObjectNames(name string, clusterScoped bool) []string {
	if clusterScoped {
		return []string{"serviceaccount/" + name, "clusterrole/" + name, "clusterrolebinding/" + name}
	}
	return []string{"serviceaccount/" + name, "role/" + name, "rolebinding/" + name}
}

// namespaceScope is the resolved namespace mode for a mint: an explicit list of
// namespaces to bind in, or the all-namespaces wildcard. Both empty means cluster-scoped.
type namespaceScope struct {
	namespaces []string
	all        bool
}

// resolveNamespaceScope turns the namespace-related flags into a concrete mode and rejects
// contradictory combinations (FR-017/FR-018). -n accepts a comma-separated list; -A or the
// -n '*' sugar select the all-namespaces wildcard.
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

// parseNamespaceList splits a comma-separated --namespace value, de-duplicating (preserving
// first-seen order) and rejecting empty entries and a '*' mixed into a list — the wildcard
// must stand alone (or use -A).
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

// buildGrantAttributes expands the grant SSAR over the session's namespace breadth: a single
// pass with an empty namespace for a cluster-wide grant (--cluster-scoped or -A), or one pass
// per namespace for an explicit list (FR-017).
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
